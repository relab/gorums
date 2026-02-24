package stream

import (
	"sync"
	"time"
)

// MessageRouter handles response routing for pending calls on a bidi stream.
// It is owned by the Node and injected into each Channel, so the router
// survives channel replacement (e.g., inbound reconnects).
//
// The router maintains a map of pending calls keyed by message sequence number.
// When a response arrives, RouteResponse looks up the matching request and
// delivers the response on its response channel.
type MessageRouter struct {
	mu      sync.Mutex
	pending map[uint64]Request
	latency time.Duration
}

// NewMessageRouter creates a new MessageRouter.
func NewMessageRouter() *MessageRouter {
	return &MessageRouter{
		pending: make(map[uint64]Request),
		latency: -1 * time.Second,
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
