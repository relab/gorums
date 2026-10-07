package stream

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LocalChannel is a [Channel] that passes requests to an in-process handler,
// bypassing the network. Handlers run one at a time, in request order, as on
// a stream.
type LocalChannel struct {
	id       uint32
	handler  RequestHandler
	requests *dispatcher
}

// NewLocalChannel returns a channel that serves requests with handler.
func NewLocalChannel(id uint32, handler RequestHandler) *LocalChannel {
	return &LocalChannel{id: id, handler: handler, requests: newDispatcher(nil, 0)}
}

// Enqueue queues req for the handler. A one-way request is confirmed once
// queued. When the queue is full, a two-way request fails with
// [ErrSendQueueFull] and a one-way request waits for space until its context
// ends. Without a handler, req fails with [codes.Unimplemented].
func (c *LocalChannel) Enqueue(req Request) {
	if req.Oneway && req.Streaming {
		panic("gorums: Oneway and Streaming are mutually exclusive")
	}
	if err := req.Ctx.Err(); err != nil {
		req.sendErrorResponse(c.id, err)
		return
	}
	if c.handler == nil {
		req.sendErrorResponse(c.id, status.Error(codes.Unimplemented, "no request handler registered"))
		return
	}
	ctx := req.Msg.appendToIncomingContext(req.Ctx)
	send := func(msg *Message) {
		if req.wantServerResponse() {
			req.deliver(response{NodeID: c.id, Value: msg, Err: msg.errorStatus()})
		}
	}
	run := func(release func()) { c.handler.HandleRequest(ctx, req.Msg, release, send) }
	if req.wantServerResponse() {
		if !c.requests.tryPush(run) {
			req.sendErrorResponse(c.id, ErrSendQueueFull)
		}
		return
	}
	if !c.requests.push(req.Ctx, run) {
		req.sendErrorResponse(c.id, req.Ctx.Err())
		return
	}
	if req.wantSendConfirmation() {
		req.deliver(response{NodeID: c.id})
	}
}

// StreamUp reports true: a local channel can always carry requests.
func (*LocalChannel) StreamUp() bool { return true }

// LastErr returns nil: a local channel has no stream to fail.
func (*LocalChannel) LastErr() error { return nil }

// DroppedReplies returns 0: a local channel delivers replies directly.
func (*LocalChannel) DroppedReplies() int64 { return 0 }

// PendingCount returns 0: a local channel delivers replies directly.
func (*LocalChannel) PendingCount() int { return 0 }

// Close does nothing; queued requests still run.
func (*LocalChannel) Close() error { return nil }
