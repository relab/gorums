package stream

import (
	"context"
	"sync"
	"time"
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
// back to the communicating peer. For two-way call types, send must be
// called exactly once and synchronously (before HandleRequest returns).
// For one-way call types, the client has no pending router entry to
// receive a response; any message delivered via send will not be
// routable on the client side and silently dropped.
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
}

// NewMessageRouter creates a new MessageRouter with an optional RequestHandler.
// The handler, if provided, is used to dispatch server-initiated requests that
// arrive on the bidirectional back-channel: when the client receives a message
// that does not match a pending outbound call, the Channel passes it to the
// handler via RequestHandler(). Passing nil (or omitting the argument) disables
// server-initiated dispatch on this router.
func NewMessageRouter(handler ...RequestHandler) *MessageRouter {
	handler = append(handler, nil) // ensure handler[0] is always valid
	return &MessageRouter{
		pending: make(map[uint64]Request),
		latency: -1 * time.Second,
		handler: handler[0],
	}
}

// RequestHandler returns the RequestHandler if set.
// Returns the handler and true if found, or nil and false otherwise.
func (r *MessageRouter) RequestHandler() (RequestHandler, bool) {
	if r.handler == nil {
		return nil, false
	}
	return r.handler, true
}

// Register registers a pending call awaiting a response.
// Called by Channel.sender() after all pre-send checks pass.
func (r *MessageRouter) Register(msgID uint64, req Request) {
	req.SendTime = time.Now()
	r.mu.Lock()
	r.pending[msgID] = req
	r.mu.Unlock()
}

// RouteResponse routes a response to a pending call by message sequence number.
// If a matching request is found, the response is delivered on the request's
// ResponseChan, and the method returns true. For successful (non-error) responses,
// the router also updates its latency estimate from the request's SendTime.
// For non-streaming calls, the entry is removed from the map. For streaming
// calls (correctable), the entry remains for subsequent responses.
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
		req.ResponseChan <- resp
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
