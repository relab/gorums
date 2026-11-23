package stream

import (
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LocalChannel is a [Channel] that passes requests to an in-process handler,
// bypassing the network. Handlers run one at a time, in request order, as on
// a stream.
type LocalChannel struct {
	id       ID
	handler  RequestHandler
	requests *dispatcher
	unmatchedErrors
}

// NewLocalChannel returns a channel that serves requests with handler. Up to
// recvBufferSize requests can wait for the running handler; it must be positive.
// If logger is non-nil, it logs each error reply to a one-way request, or to
// a two-way request whose context has ended; see [Channel.UnmatchedErrors].
func NewLocalChannel(id ID, handler RequestHandler, recvBufferSize uint, logger *log.Logger) *LocalChannel {
	c := &LocalChannel{id: id, handler: handler, requests: newDispatcher(nil, recvBufferSize)}
	c.unmatchedErrors.logger = logger
	return c
}

// Enqueue admits the request to the handler's request queue. If the channel
// cannot admit it, Enqueue returns an error at once: the context error if the
// request's context has ended, an error with the code [codes.Unimplemented] if
// no handler is registered, or [ErrSendQueueFull] if the request queue is full.
// An admitted request reports the handler's replies on the request's response
// channel; a one-way request instead reports its send confirmation.
// Enqueue panics if the request is both Oneway and Streaming.
func (c *LocalChannel) Enqueue(req NodeRequest) error {
	if req.Oneway && req.Streaming {
		panic("gorums: Oneway and Streaming are mutually exclusive")
	}
	if err := req.Ctx.Err(); err != nil {
		return err
	}
	if c.handler == nil {
		return status.Error(codes.Unimplemented, "no request handler registered")
	}
	ctx := req.Msg.appendToIncomingContext(req.Ctx)
	enqueued := time.Now()
	send := func(msg *Message) {
		err := msg.errorStatus()
		if err != nil && (!req.wantServerResponse() || req.Ctx.Err() != nil) {
			// No caller observes the error: the request is one-way, or its
			// caller is done.
			c.unmatchedErrors.record(c.id, msg, err)
		}
		if req.wantServerResponse() {
			req.deliver(response{NodeID: c.id, Value: msg, Err: err, Latency: time.Since(enqueued)})
		}
	}
	run := func(release func()) { c.handler.HandleRequest(ctx, c.id, req.Msg, release, send) }
	if !c.requests.tryPush(run) {
		return ErrSendQueueFull
	}

	if req.wantSendConfirmation() {
		req.deliver(response{NodeID: c.id})
	}
	return nil
}

// StreamUp reports true: a local channel can always carry requests.
func (*LocalChannel) StreamUp() bool { return true }

// LastErr returns nil: a local channel has no stream to fail.
func (*LocalChannel) LastErr() error { return nil }

// DroppedReplies returns 0: a local channel delivers replies directly.
func (*LocalChannel) DroppedReplies() int64 { return 0 }

// PendingCount returns 0: a local channel delivers replies directly.
func (*LocalChannel) PendingCount() int { return 0 }

// Wait waits until the handlers of the requests enqueued on the channel have
// returned.
func (c *LocalChannel) Wait() {
	c.requests.wait(nil)
}

// Close does nothing; queued requests still run.
func (*LocalChannel) Close() error { return nil }
