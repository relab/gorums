package stream

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The drain protocol lets a server finish the Gorums requests it accepted
// before its streams close. It uses three control frames, which carry message
// ID 0 and a reserved method name. Real messages never use ID 0.
//
//  1. The draining server sends a draining frame on a stream.
//  2. When the draining frame arrives, the peer stops sending its own requests
//     on that stream and fails the unsent ones with [ErrServerDraining]. It
//     then sends a final frame on the same stream. The server accepts each
//     request ahead of the final frame and rejects a request behind it.
//  3. The server finishes its accepted handlers and sends a drained frame
//     after the replies that it queued. The peer then ends the stream.
//
// Replies still pass in both directions while the stream drains.
const (
	controlPrefix    = "gorums.control."
	drainingMethod   = controlPrefix + "draining"
	drainFinalMethod = controlPrefix + "drain-final"
	drainedMethod    = controlPrefix + "drained"

	// drainPollInterval is how often a draining server checks whether its
	// accepted handlers have finished.
	drainPollInterval = 5 * time.Millisecond
)

// ErrServerDraining is exported and documented as gorums.ErrServerDraining.
var ErrServerDraining = status.Error(codes.Unavailable, "server is draining")

// isServerDraining reports whether err is [ErrServerDraining], also when it
// arrived as the status of a stream that a draining server refused.
func isServerDraining(err error) bool {
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.Unavailable && st.Message() == status.Convert(ErrServerDraining).Message()
}

// drainFrame returns the control frame for method.
func drainFrame(method string) *Message {
	msg := &Message{}
	msg.SetMethod(method)
	return msg
}

// isControl reports whether msg is a drain control frame.
func isControl(msg *Message) bool {
	return msg.GetMessageSeqNo() == 0 && strings.HasPrefix(msg.GetMethod(), controlPrefix)
}

// drainState is a session's state for the drain protocol.
type drainState struct {
	// peerDraining is set when the peer's draining frame arrived.
	peerDraining atomic.Bool
	// notice is closed when peerDraining is set; it wakes the send loop.
	notice chan struct{}
	// finalOnce sends the final frame once.
	finalOnce sync.Once
	// peerDrained is set when the peer's drained frame arrived.
	peerDrained atomic.Bool

	// localDraining is set when this side started draining the stream.
	localDraining atomic.Bool
	// finalReceived is set when the peer's final frame arrived; requests behind
	// it are rejected.
	finalReceived atomic.Bool
	// drained is closed once the drained frame was sent, or the session ended.
	drained     chan struct{}
	drainedOnce sync.Once
}

// newDrainState returns the drain state of a new session.
func newDrainState() drainState {
	return drainState{notice: make(chan struct{}), drained: make(chan struct{})}
}

// control handles a control frame received on the session.
func (s *session) control(msg *Message) {
	switch msg.GetMethod() {
	case drainingMethod:
		if s.peerDraining.CompareAndSwap(false, true) {
			close(s.notice)
			s.sendFinalIfStopped()
		}
	case drainFinalMethod:
		if s.finalReceived.CompareAndSwap(false, true) {
			go s.finishDrain()
		}
	case drainedMethod:
		s.peerDrained.Store(true)
		// While this side still drains, ending the session would cancel its
		// accepted handlers; its own drained frame ends the session instead.
		if !s.localDraining.Load() {
			s.end(nil)
		}
	}
}

// controlSent records that the send loop sent a control frame.
func (s *session) controlSent(msg *Message) {
	if msg.GetMethod() == drainedMethod {
		s.closeDrained()
		if s.peerDrained.Load() {
			s.end(nil)
		}
	}
}

// rejectsDraining reports whether the send loop must fail req because the
// peer is draining and req is one of this side's own requests. Replies and
// control frames still pass.
func (s *session) rejectsDraining(req *NodeRequest) bool {
	seq := req.Msg.GetMessageSeqNo()
	return s.peerDraining.Load() && seq != 0 && isServerSequenceNumber(seq) != s.serverRequests
}

// sendFinal sends the final frame on this session's stream, once. Only the
// session's send loop, or a caller after the send loop has stopped, may call
// it, since a stream allows one sender at a time.
func (s *session) sendFinal() error {
	var err error
	s.finalOnce.Do(func() {
		err = s.stream.Send(drainFrame(drainFinalMethod))
	})
	return err
}

// sendFinalIfStopped sends the final frame if the peer is draining and the
// send loop has stopped, as it has after a GOAWAY. Both conditions are
// checked by the two goroutines that set them, so one of them sends the frame.
// A failed send is ignored: the stream is broken, and receiving ends the
// session.
func (s *session) sendFinalIfStopped() {
	if s.peerDraining.Load() && s.sendStopped.Load() {
		go func() { _ = s.sendFinal() }()
	}
}

// startLocalDrain begins draining the stream: it tells the peer to stop, and
// it keeps the queued handlers running if the stream ends before they do.
func (s *session) startLocalDrain() {
	if s.localDraining.CompareAndSwap(false, true) {
		s.requests.keep.Store(true)
		// The push waits for queue space, so it must not block the caller,
		// whose own deadline then still applies.
		go s.queue.push(NodeRequest{Ctx: s.ctx, Msg: drainFrame(drainingMethod)}, true)
	}
}

// closeDrained closes the drained channel; it is idempotent.
func (s *session) closeDrained() {
	s.drainedOnce.Do(func() { close(s.drained) })
}

// isDrained reports whether the drained frame was sent or the session ended.
func (s *session) isDrained() bool {
	select {
	case <-s.drained:
		return true
	default:
		return false
	}
}

// finishDrain waits for the accepted handlers to return, then queues the
// drained frame behind their replies. It returns early if the session ends.
func (s *session) finishDrain() {
	tick := time.NewTicker(drainPollInterval)
	defer tick.Stop()
	for s.requests.active.Load() > 0 {
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
		}
	}
	s.queue.push(NodeRequest{Ctx: s.ctx, Msg: drainFrame(drainedMethod)}, true)
}
