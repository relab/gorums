package stream

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// The drain protocol lets a server finish the Gorums requests it accepted
// before its streams close. It uses three control frames, which carry message
// ID 0 and a reserved method name. Real messages never use ID 0.
//
//  1. The draining server sends a draining frame on a stream.
//  2. The peer sends a final frame after the last request that it sends. Each
//     request ahead of it was sent before the peer learned of the drain, so the
//     server accepts it; a request behind it is rejected.
//  3. The server finishes its accepted handlers and sends a drained frame
//     after the replies that it queued. The peer then ends the stream.
//
// A server drains only streams whose dialer announced support with
// [drainCapableKey], since a peer without the protocol never sends a final
// frame.
const (
	controlPrefix    = "gorums.control."
	drainingMethod   = controlPrefix + "draining"
	drainFinalMethod = controlPrefix + "drain-final"
	drainedMethod    = controlPrefix + "drained"

	// drainCapableKey is the stream metadata key with which a dialer announces
	// that it implements the drain protocol.
	drainCapableKey = "gorums-drain"

	// drainPollInterval is how often a draining stream checks whether its
	// accepted handlers have finished.
	drainPollInterval = 5 * time.Millisecond
)

// ErrServerDraining is exported and documented as gorums.ErrServerDraining.
var ErrServerDraining = status.Error(codes.Unavailable, "server is draining")

// drainFrame returns the control frame for method.
func drainFrame(method string) *Message {
	return Message_builder{Method: method}.Build()
}

// drainState is a session's state for the drain protocol.
type drainState struct {
	// peerDraining is set when the peer sent a draining frame.
	peerDraining atomic.Bool
	// finalSent is set once the send loop has sent the final frame; own
	// requests behind it fail with [ErrServerDraining].
	finalSent atomic.Bool
	// peerDrained is set when the peer reported that it finished draining.
	peerDrained atomic.Bool

	// localDraining is set when this side started draining the stream.
	localDraining atomic.Bool
	// finalReceived is set when the peer's final frame arrived; requests behind
	// it are rejected.
	finalReceived atomic.Bool
	// drained is closed once the drained frame was sent.
	drained chan struct{}

	// work counts the handlers that this session accepted and that have not
	// returned.
	work atomic.Int64
}

// isControl reports whether msg is a drain control frame.
func isControl(msg *Message) bool {
	return msg.GetMessageSeqNo() == 0 && strings.HasPrefix(msg.GetMethod(), controlPrefix)
}

// control handles a control frame received on the session.
func (s *session) control(msg *Message) {
	switch msg.GetMethod() {
	case drainingMethod:
		// Queue the final frame behind the requests already queued. It waits
		// for queue space, so it runs apart from receiving.
		if s.peerDraining.CompareAndSwap(false, true) {
			go s.queue.push(Request{Ctx: s.ctx, Msg: drainFrame(drainFinalMethod)}, true)
		}
	case drainFinalMethod:
		if s.finalReceived.CompareAndSwap(false, true) {
			go s.finishDrain()
		}
	case drainedMethod:
		s.peerDrained.Store(true)
		s.end(nil)
	}
}

// controlSent records that the send loop sent a control frame.
func (s *session) controlSent(msg *Message) {
	switch msg.GetMethod() {
	case drainFinalMethod:
		s.finalSent.Store(true)
	case drainedMethod:
		close(s.drained)
	}
}

// rejectsDraining reports whether the send loop must fail req because the
// peer is draining and req is one of this side's own requests behind the final
// frame. Replies and control frames still pass.
func (s *session) rejectsDraining(req *Request) bool {
	seq := req.Msg.GetMessageSeqNo()
	return s.finalSent.Load() && seq != 0 && isServerSequenceNumber(seq) != s.serverRequests
}

// startLocalDrain begins draining the stream: it tells the peer to stop.
func (s *session) startLocalDrain() {
	if s.localDraining.CompareAndSwap(false, true) {
		s.queue.push(Request{Ctx: s.ctx, Msg: drainFrame(drainingMethod)}, true)
	}
}

// finishDrain waits for the accepted handlers to return, then queues the
// drained frame behind the replies. It returns early if the session ends.
func (s *session) finishDrain() {
	tick := time.NewTicker(drainPollInterval)
	defer tick.Stop()
	for s.work.Load() > 0 {
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
		}
	}
	s.queue.push(Request{Ctx: s.ctx, Msg: drainFrame(drainedMethod)}, true)
}

// drainCapable reports whether the dialer of the stream with context ctx
// announced support for the drain protocol.
func drainCapable(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	return ok && len(md.Get(drainCapableKey)) > 0
}

// newErrorReply returns an error reply to req with err.
func newErrorReply(req *Message, err error) *Message {
	st, _ := status.FromError(err)
	return Message_builder{
		MessageSeqNo: req.GetMessageSeqNo(),
		Method:       req.GetMethod(),
		Status:       st.Proto(),
	}.Build()
}
