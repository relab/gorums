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

// MessageRouter handles response routing for pending calls on a bidi stream.
// It is owned by the Node and injected into each Channel, so the router
// survives channel replacement (e.g., inbound reconnects).
//
// The router maintains a map of pending calls keyed by message sequence number.
// When a response arrives, RouteResponse looks up the matching request and
// delivers the response on its response channel.
//
// The router also provides handler lookup via a shared handler map. All routers
// for the same role (server-side or client-side) share the same RequestHandler
// reference, so handlers registered once are visible to all routers.
type MessageRouter struct {
	mu      sync.Mutex
	pending map[uint64]Request
	latency time.Duration
	handler RequestHandler // shared by reference; may be nil
	// localMu serializes in-process handler dispatch, mirroring NodeStream's
	// lock+release pattern so local and remote nodes behave identically.
	localMu sync.Mutex
}

// NewMessageRouter creates a new MessageRouter with an optional RequestHandler.
// The handler, if provided, is used to dispatch incoming requests:
// in RouteMessage, it processes server-initiated back-channel calls (high-bit IDs);
// in RouteInboundMessage, it dispatches client-initiated requests (low-bit IDs).
// Passing nil (or omitting the argument) disables request dispatch on this router.
func NewMessageRouter(handler ...RequestHandler) *MessageRouter {
	handler = append(handler, nil) // ensure handler[0] is always valid
	return &MessageRouter{
		pending: make(map[uint64]Request),
		latency: -1 * time.Second,
		handler: handler[0],
	}
}

// NewMessageRouterWithLatency creates a new MessageRouter with an initial latency
// for testing. The latency may be updated by subsequent message routing operations.
// This function should only be used in tests.
//
// To change the latency after creation, use [MessageRouter.SetLatency].
func NewMessageRouterWithLatency(latency time.Duration) *MessageRouter {
	return &MessageRouter{
		pending: make(map[uint64]Request),
		latency: latency,
	}
}

// SetLatency directly sets the latency estimate. This function should only
// be used in tests to simulate latency changes without actual message routing.
func (r *MessageRouter) SetLatency(latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latency = latency
}

// DispatchLocalRequest handles the request in-process for the local node,
// bypassing the network. It delivers the request to the registered handler,
// serializing execution the same way remote nodes do: the next dispatch is
// blocked until the handler returns or calls [ServerCtx.Release].
//
// For one-way calls, send-completion is confirmed before the handler runs
// if WaitSendDone is true. For two-way calls, the response is delivered
// directly to the caller's response channel via the send closure.
func (r *MessageRouter) DispatchLocalRequest(nodeID uint32, req Request) {
	if req.Ctx.Err() != nil {
		req.replyError(nodeID, req.Ctx.Err())
		return
	}
	if r.handler == nil {
		req.replyError(nodeID, status.Error(codes.Unimplemented, "no request handler registered"))
		return
	}
	if req.WaitSendDone && req.ResponseChan != nil {
		if !req.deliver(response{NodeID: nodeID}) {
			return
		}
	}
	// For two-way calls, deliver the response via the send closure.
	// For one-way calls (WaitSendDone=true or ResponseChan==nil), send is a no-op:
	// the confirmation was already delivered above, and a second write would either
	// race with the caller consuming the channel or block on a full response channel.
	send := func(msg *Message) {
		if req.WaitSendDone || req.ResponseChan == nil {
			return
		}
		req.deliver(response{NodeID: nodeID, Value: msg, Err: msg.ErrorStatus()})
	}

	r.localMu.Lock()
	var once sync.Once
	release := func() { once.Do(r.localMu.Unlock) }

	go r.handler.HandleRequest(req.Msg.AppendToIncomingContext(req.Ctx), req.Msg, release, send)
}

// RouteMessage delivers a response to a pending call registered via [Register],
// or dispatches a server-initiated request to the registered handler.
// It is the primary entry point for messages received on the client-side stream.
//
// Responses to client-initiated calls are delivered to the matching pending call;
// responses to cancelled calls are silently dropped. Server-initiated requests
// (back-channel calls) are dispatched to the handler in a new goroutine.
func (r *MessageRouter) RouteMessage(ctx context.Context, nodeID uint32, msg *Message, enqueue func(Request)) {
	msgID := msg.GetMessageSeqNo()

	// A server-initiated ID identifies a back-channel request to this client,
	// not a response to any call the client registered.
	if isServerSequenceNumber(msgID) {
		if r.handler != nil {
			send := func(reply *Message) {
				enqueue(Request{Ctx: ctx, Msg: reply})
			}
			go r.handler.HandleRequest(ctx, msg, func() {}, send)
		}
		return
	}

	r.mu.Lock()
	req, ok := r.pending[msgID]
	if ok && !req.Streaming {
		delete(r.pending, msgID)
	}
	r.mu.Unlock()

	if ok {
		resp := response{NodeID: nodeID, Value: msg, Err: msg.ErrorStatus()}
		if resp.Err == nil {
			r.updateLatency(time.Since(req.SendTime))
		}
		req.deliver(resp)
	}
}

// Register registers a pending call awaiting a response.
// Called by Channel.sender() after all pre-send checks pass.
func (r *MessageRouter) Register(msgID uint64, req Request) {
	req.SendTime = time.Now()
	r.mu.Lock()
	r.pending[msgID] = req
	r.mu.Unlock()
}

// RouteInboundMessage delivers a response to a pending call registered via [Register],
// or dispatches a client-initiated request to the registered handler.
// It is the symmetric counterpart of [RouteMessage] for the server-side receive path.
//
// Responses to server-initiated calls are delivered to the matching pending call;
// responses to cancelled calls are silently absorbed. Client-initiated requests
// are dispatched to the handler in a new goroutine. The release function is always called.
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
	// Server-initiated response: look up pending call and deliver if found;
	// silently absorb if not found (stale response from a cancelled call).
	r.mu.Lock()
	req, ok := r.pending[msgID]
	if ok && !req.Streaming {
		delete(r.pending, msgID)
	}
	r.mu.Unlock()

	if ok {
		resp := response{NodeID: nodeID, Value: msg, Err: msg.ErrorStatus()}
		if resp.Err == nil {
			r.updateLatency(time.Since(req.SendTime))
		}
		req.deliver(resp)
	}
	release()
}

// RouteResponse delivers a response to a pending call registered via [Register].
// For non-streaming calls, the entry is removed after delivery.
// For streaming calls (correctable), the entry remains for subsequent responses.
//
// Unmatched server-initiated calls (back-channel responses) are absorbed and
// the method returns true. Returns false only for unmatched client-initiated
// calls (stale responses).
func (r *MessageRouter) RouteResponse(msgID uint64, resp response) bool {
	r.mu.Lock()
	req, ok := r.pending[msgID]
	if ok && !req.Streaming {
		delete(r.pending, msgID)
	}
	r.mu.Unlock()

	if ok {
		if resp.Err == nil {
			r.updateLatency(time.Since(req.SendTime))
		}
		req.deliver(resp)
		return true
	}
	return isServerSequenceNumber(msgID)
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
	for msgID, req := range r.pending {
		reqs = append(reqs, req)
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
	for msgID, req := range r.pending {
		delete(r.pending, msgID)
		if req.Streaming {
			cancel = append(cancel, req)
		} else {
			requeue = append(requeue, req)
		}
	}
	r.mu.Unlock()
	return requeue, cancel
}
