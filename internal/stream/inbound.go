package stream

import (
	"context"
	"sync"

	"google.golang.org/grpc/metadata"
)

// InboundOptions configures an [InboundChannel].
type InboundOptions struct {
	// SendBufferSize is the send queue capacity.
	SendBufferSize uint
	// RecvBufferSize is the number of received requests that can wait for the
	// running handler before receiving waits; it must be positive.
	RecvBufferSize uint
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

// InboundStream is the server side of a stream that a dialer opened to this
// node. grpc.BidiStreamingServer[Message, Message] satisfies it.
type InboundStream interface {
	BidiStream
	SendHeader(metadata.MD) error
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
//
// Create a channel only for a stream that the server accepts. Before it sends
// any message, the send loop tells the dialer that the server accepted the
// stream.
func NewInboundChannel(ctx context.Context, id ID, stream InboundStream, opts InboundOptions) *InboundChannel {
	c := &InboundChannel{
		endpoint: newEndpoint(ctx, id, opts.SendBufferSize, opts.RecvBufferSize, opts.Handler, opts.Latency),
	}
	c.waitingReplies = opts.WaitingReplies
	sessionCtx, cancel := context.WithCancel(c.ctx)
	c.session = newSession(sessionCtx, cancel, &c.endpoint, stream, false, false)
	c.closeOnce = sync.OnceValue(func() error {
		c.cancel()
		c.session.end(nil)
		c.queue.close()
		return nil
	})
	go func() {
		c.accept(stream)
		if req := c.session.sendLoop(nil); req != nil {
			c.queue.fail(*req, ErrStreamDown)
		}
		c.cancel() // ends waits for queue space before the queue closes
		c.queue.close()
	}()
	return c
}

// accept tells the dialer that the server accepted stream. It sends the
// gorums-accepted response header. A stream can send its response headers only
// once, so the header fails if a gRPC stream interceptor on the server, such as
// one set with grpc.StreamInterceptor, has already sent them. accept then sends
// an empty acceptance frame instead. The dialer treats either one as the
// acceptance.
func (c *InboundChannel) accept(stream InboundStream) {
	if err := stream.SendHeader(metadata.Pairs(acceptedKey, "1")); err != nil && c.ctx.Err() == nil {
		_ = stream.Send(acceptFrame())
	}
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
