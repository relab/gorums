package stream

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RequestHandler is the interface that wraps the HandleRequest method.
//
// HandleRequest handles an incoming request message from the stream,
// dispatching it to the appropriate method handler, as encoded in the
// message's method field. It should be called in a new goroutine for
// every request.
//
// The release function must be idempotent. It must be called in the
// handler to allow processing the next request from the stream.
//
// The send function is used to deliver the provided response message
// back to the communicating peer. For two-way call types, send may be
// called zero or more times (e.g., for streaming correctable calls).
// However, callers must not invoke send after HandleRequest returns,
// as the underlying routing state may no longer be valid. For one-way
// call types, the client has no pending router entry to receive a
// response; any message delivered via send will not be routable on
// the client side and will be silently dropped.
type RequestHandler interface {
	HandleRequest(ctx context.Context, msg *Message, release func(), send func(*Message))
}

// pendingOwner is an opaque identity token recording which channel registered
// a pending call. Each channel allocates one token and tags its registrations
// with it, so that closing or requeueing a retired channel affects only that
// channel's calls and never those of its replacement on the same router.
// The struct must not be zero-sized: Go gives distinct zero-size allocations
// the same address, which would make separate tokens compare equal; the
// padding byte guarantees each token a unique address.
type pendingOwner struct {
	_ byte
}

// pendingRequest is a router map entry: a pending call plus the owner token
// of the channel that sent it (nil when registered via the exported Register).
type pendingRequest struct {
	request Request
	owner   *pendingOwner
}

// MessageRouter handles response routing for pending calls on a bidi stream.
// It is owned by the Node and injected into each Channel, so the router
// survives channel replacement (e.g., inbound reconnects).
//
// The router maintains a map of pending calls keyed by message sequence number.
// When a response arrives, deliverPending looks up the matching request and
// delivers the response on its response channel.
//
// The router also provides handler lookup via a shared handler map. All routers
// for the same role (server-side or client-side) share the same RequestHandler
// reference, so handlers registered once are visible to all routers.
type MessageRouter struct {
	mu      sync.Mutex
	pending map[uint64]pendingRequest
	latency time.Duration
	handler RequestHandler // shared by reference; may be nil
	// dispatchMu serializes handler dispatch when no stream-owned ordering lock
	// exists, covering local and client-side back-channel requests.
	dispatchMu sync.Mutex
}

// NewMessageRouter creates a new MessageRouter with an optional RequestHandler.
// The handler, if provided, is used to dispatch incoming requests: on the client
// side it handles server-initiated back-channel calls; on the server side it
// dispatches client-initiated requests. Passing nil (or omitting the argument)
// disables request dispatch on this router.
func NewMessageRouter(handler ...RequestHandler) *MessageRouter {
	handler = append(handler, nil) // ensure handler[0] is always valid
	return &MessageRouter{
		pending: make(map[uint64]pendingRequest),
		latency: -1 * time.Second,
		handler: handler[0],
	}
}

// SetLatency directly sets the latency estimate. This function should only
// be used in tests to simulate latency changes without actual message routing.
func (r *MessageRouter) SetLatency(latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latency = latency
}

// PendingCount returns the number of pending calls currently registered in the router.
func (r *MessageRouter) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pending)
}

// DispatchLocalRequest handles the request in-process for the local node,
// bypassing the network. It delivers the request to the registered handler,
// serializing execution the same way remote nodes do: the next dispatch is
// blocked until the handler returns or invokes the release callback it was
// dispatched with.
//
// For one-way calls, send-completion is confirmed before the handler runs.
// For two-way calls, the response is delivered directly to the caller's
// response channel via the send closure.
func (r *MessageRouter) DispatchLocalRequest(nodeID uint32, req Request) {
	if req.Ctx.Err() != nil {
		req.ReplyError(nodeID, req.Ctx.Err())
		return
	}
	if r.handler == nil {
		req.ReplyError(nodeID, status.Error(codes.Unimplemented, "no request handler registered"))
		return
	}
	// One-way calls: confirm "send" completion before running the handler,
	// since the caller blocks until confirmation arrives on ResponseChan.
	if req.wantSendConfirmation() {
		if !req.deliver(response{NodeID: nodeID}) {
			return // request cancelled while waiting for send confirmation; do not run the handler.
		}
	}
	send := func(msg *Message) {
		// One-way fire-and-forget calls have no ResponseChan, so send is a no-op.
		if !req.wantServerResponse() {
			return
		}
		// Two-way calls: deliver the handler's response on ResponseChan.
		req.deliver(response{NodeID: nodeID, Value: msg, Err: msg.ErrorStatus()})
	}

	r.dispatchSerialized(req.Msg.AppendToIncomingContext(req.Ctx), req.Msg, send)
}

// dispatchSerialized starts a handler while holding the router's dispatch lock.
// The next dispatch blocks until the handler invokes the idempotent release
// callback, matching the ordering contract enforced by NodeStream.
func (r *MessageRouter) dispatchSerialized(ctx context.Context, msg *Message, send func(*Message)) {
	r.dispatchMu.Lock()
	var once sync.Once
	release := func() { once.Do(r.dispatchMu.Unlock) }
	go r.handler.HandleRequest(ctx, msg, release, send)
}

// RouteMessage demultiplexes a message received on the client-side (outbound) stream.
// Server-initiated requests (back-channel calls, high-bit IDs) are dispatched to the
// handler in a new goroutine. Responses to client-initiated calls (low-bit IDs) are
// delivered to the matching pending call; responses to cancelled or unknown calls are
// silently dropped.
func (r *MessageRouter) RouteMessage(ctx context.Context, nodeID uint32, msg *Message, enqueue func(Request)) {
	msgID := msg.GetMessageSeqNo()

	// A server-initiated ID identifies a back-channel request to this client,
	// not a response to any call the client registered.
	if isServerSequenceNumber(msgID) {
		if r.handler != nil {
			send := func(reply *Message) {
				enqueue(Request{Ctx: ctx, Msg: reply})
			}
			r.dispatchSerialized(msg.AppendToIncomingContext(ctx), msg, send)
		}
		return
	}

	r.deliverPending(msgID, response{NodeID: nodeID, Value: msg, Err: msg.ErrorStatus()})
}

// Register registers an unowned pending call awaiting a response.
// Full-router cancellation and requeue operations include unowned calls,
// while channel-scoped operations do not.
func (r *MessageRouter) Register(msgID uint64, req Request) {
	r.register(nil, msgID, req)
}

// register associates a pending call with the channel that sent it.
func (r *MessageRouter) register(owner *pendingOwner, msgID uint64, req Request) {
	req.SendTime = time.Now()
	r.mu.Lock()
	r.pending[msgID] = pendingRequest{request: req, owner: owner}
	r.mu.Unlock()
}

// RouteInboundMessage demultiplexes a message received on the server-side (inbound) stream.
// It is the symmetric counterpart of [RouteMessage] for the server-side receive path.
// Client-initiated requests (low-bit IDs) are dispatched to the handler in a new goroutine,
// or release is called immediately when no handler is registered. Responses to server-initiated
// calls (high-bit IDs) are delivered to the matching pending call; stale responses from
// cancelled calls are silently absorbed. The release function is always called.
func (r *MessageRouter) RouteInboundMessage(ctx context.Context, nodeID uint32, msg *Message, release func(), send func(*Message)) {
	msgID := msg.GetMessageSeqNo()
	if !isServerSequenceNumber(msgID) {
		// Client-initiated request: dispatch to handler or unblock the ordering lock.
		if r.handler != nil {
			go r.handler.HandleRequest(msg.AppendToIncomingContext(ctx), msg, release, send)
		} else {
			release()
		}
		return
	}
	// Server-initiated response: deliver to the matching pending call (if any) and
	// release the ordering lock. Stale responses from cancelled calls are silently absorbed.
	r.deliverPending(msgID, response{NodeID: nodeID, Value: msg, Err: msg.ErrorStatus()})
	release()
}

// deliverPending looks up the pending call for msgID and delivers resp to it.
// For non-streaming calls, the entry is removed after delivery.
// For streaming calls (correctable), the entry remains for subsequent responses.
// Returns true if a matching pending entry was found (delivery is attempted but
// may be a no-op if the caller's context is already canceled), false otherwise.
func (r *MessageRouter) deliverPending(msgID uint64, resp response) bool {
	r.mu.Lock()
	pending, ok := r.pending[msgID]
	if ok && !pending.request.Streaming {
		delete(r.pending, msgID)
	}
	r.mu.Unlock()

	if ok {
		req := pending.request
		if resp.Err == nil {
			r.updateLatency(time.Since(req.SendTime))
		}
		req.deliver(resp)
	}
	return ok
}

// Latency returns the estimated round-trip latency based on recent responses.
// Returns -1s if no successful responses have been routed yet.
func (r *MessageRouter) Latency() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latency
}

// updateLatency updates the latency estimate using a simple moving average.
func (r *MessageRouter) updateLatency(rtt time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.latency < 0 {
		r.latency = rtt
	} else {
		r.latency = time.Duration(0.8*float64(r.latency) + 0.2*float64(rtt))
	}
}

// CancelPending removes and returns all pending requests.
// The caller should send an error response to each returned request.
func (r *MessageRouter) CancelPending() []Request {
	r.mu.Lock()
	reqs := make([]Request, 0, len(r.pending))
	for msgID, pending := range r.pending {
		reqs = append(reqs, pending.request)
		delete(r.pending, msgID)
	}
	r.mu.Unlock()
	return reqs
}

// cancelPending removes pending requests owned by owner.
func (r *MessageRouter) cancelPending(owner *pendingOwner) []Request {
	r.mu.Lock()
	reqs := make([]Request, 0)
	for msgID, pending := range r.pending {
		if pending.owner != owner {
			continue
		}
		reqs = append(reqs, pending.request)
		delete(r.pending, msgID)
	}
	r.mu.Unlock()
	return reqs
}

// RequeuePending removes all pending requests and splits them into two groups:
// non-streaming requests (safe to retry) and streaming requests (must be cancelled).
//
// Streaming requests cannot be safely retried because the caller may have already
// received partial responses; a silent re-send would deliver a second, independent
// result sequence on the same channel, violating the correctable call contract.
func (r *MessageRouter) RequeuePending() (requeue, cancel []Request) {
	r.mu.Lock()
	requeue = make([]Request, 0, len(r.pending))
	cancel = make([]Request, 0)
	for msgID, pending := range r.pending {
		delete(r.pending, msgID)
		req := pending.request
		if req.Streaming {
			cancel = append(cancel, req)
		} else {
			requeue = append(requeue, req)
		}
	}
	r.mu.Unlock()
	return requeue, cancel
}

// requeuePending removes and classifies pending requests owned by owner.
func (r *MessageRouter) requeuePending(owner *pendingOwner) (requeue, cancel []Request) {
	r.mu.Lock()
	for msgID, pending := range r.pending {
		if pending.owner != owner {
			continue
		}
		delete(r.pending, msgID)
		if pending.request.Streaming {
			cancel = append(cancel, pending.request)
		} else {
			requeue = append(requeue, pending.request)
		}
	}
	r.mu.Unlock()
	return requeue, cancel
}
