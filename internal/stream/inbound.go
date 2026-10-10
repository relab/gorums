package stream

import (
	"context"
	"sync"
)

// InboundOptions configures an [InboundChannel].
type InboundOptions struct {
	// SendBufferSize is the send queue capacity.
	SendBufferSize uint
	// DispatchSize is the number of received requests that can wait for the
	// running handler before receiving waits; 0 selects the default.
	DispatchSize uint
	// Handler serves requests the client sends on the stream; may be nil.
	Handler RequestHandler
	// Latency receives round-trip samples; may be nil.
	Latency *Latency
	// WaitingReplies makes handler replies wait for send queue space instead
	// of being dropped when the queue is full. Set it only for a peer that
	// this node sends no requests to: such a peer's receiving never waits on
	// its own handlers, so a waiting reply cannot deadlock the two sides.
	WaitingReplies bool
}

// InboundChannel is a [Channel] over one stream accepted by this node's
// server. It cannot re-establish its stream: once the stream ends, pending
// calls fail with [ErrStreamDown] and later requests with [ErrNodeClosed].
type InboundChannel struct {
	endpoint
	session   *session
	closeOnce func() error
}

// NewInboundChannel returns a channel over stream and starts its send loop.
// ctx must be the stream's context; handlers receive contexts derived from it.
// Call [InboundChannel.Serve] to receive from the stream.
func NewInboundChannel(ctx context.Context, id ID, stream BidiStream, opts InboundOptions) *InboundChannel {
	c := &InboundChannel{
		endpoint: newEndpoint(ctx, id, opts.SendBufferSize, opts.DispatchSize, opts.Handler, opts.Latency),
	}
	c.waitingReplies = opts.WaitingReplies
	sessionCtx, cancel := context.WithCancel(c.ctx)
	c.session = newSession(sessionCtx, cancel, &c.endpoint, stream, false, false)
	c.closeOnce = sync.OnceValue(func() error {
		c.cancel()
		c.session.end()
		c.queue.close()
		return nil
	})
	go func() {
		if req := c.session.sendLoop(nil); req != nil {
			c.queue.fail(*req, ErrStreamDown)
		}
		c.cancel() // ends waits for queue space before the queue closes
		c.queue.close()
	}()
	return c
}

// Serve receives from the stream and routes what it receives until receiving
// fails, and returns the receive error.
func (c *InboundChannel) Serve() error {
	return c.session.receive()
}

// StreamUp reports true; an inbound channel is detached from its node when its
// stream ends.
func (*InboundChannel) StreamUp() bool {
	return true
}

// PendingCount returns the number of two-way calls awaiting responses.
func (c *InboundChannel) PendingCount() int {
	return c.session.pending.len()
}

// Close ends the stream's session, fails pending calls with [ErrStreamDown]
// and queued requests with [ErrNodeClosed], and returns without waiting for
// the send loop. A send blocked on flow control ends with an error once the
// stream's RPC returns, and the send loop then exits. It is idempotent.
func (c *InboundChannel) Close() error {
	return c.closeOnce()
}
