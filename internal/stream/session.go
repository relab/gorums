package stream

import (
	"cmp"
	"context"
	"sync/atomic"
	"time"
)

// session is one stream of a channel. It sends queued requests on the stream,
// routes the frames received on it, and holds the two-way calls sent on it.
type session struct {
	*endpoint
	stream BidiStream
	done   <-chan struct{} // closed when the session ends
	cancel context.CancelFunc

	// serverRequests reports whether the peer's requests carry
	// server-initiated IDs, as they do on a stream this side dialed.
	serverRequests bool
	// requeue reports whether calls left pending when the session ends are
	// sent again on the channel's next session.
	requeue bool

	pending pendingCalls

	drain       chan struct{} // closed when the session starts draining
	draining    atomic.Bool
	sendStopped atomic.Bool
	received    atomic.Bool // a frame has been received
}

// newSession returns a session over stream that ends when ctx ends or cancel
// is called.
func newSession(e *endpoint, stream BidiStream, ctx context.Context, cancel context.CancelFunc, serverRequests, requeue bool) *session {
	return &session{
		endpoint:       e,
		stream:         stream,
		done:           ctx.Done(),
		cancel:         cancel,
		serverRequests: serverRequests,
		requeue:        requeue,
		drain:          make(chan struct{}),
	}
}

// sendLoop sends req, if non-nil, and then requests from the send queue,
// until the session ends or starts draining. It returns the request it took
// from the queue but did not send, or nil.
func (s *session) sendLoop(req *Request) *Request {
	defer s.stopSending()
	for {
		if req == nil {
			select {
			case <-s.done:
				return nil
			case <-s.drain:
				return nil
			case r := <-s.queue.ch:
				req = &r
			}
		}
		if s.ended() || s.draining.Load() {
			return req
		}
		if err := req.Ctx.Err(); err != nil {
			req.ReplyError(s.id, err)
			req = nil
			continue
		}
		if req.wantServerResponse() && !s.pending.add(req.Msg.GetMessageSeqNo(), *req) {
			return req // the session ended concurrently
		}
		if !s.send(*req) {
			return nil
		}
		req = nil
	}
}

// send sends req on the stream and reports whether the stream is still usable.
// A two-way request must already be pending; a one-way request is confirmed
// once it is sent.
func (s *session) send(req Request) bool {
	err := s.stream.Send(req.Msg)
	if err != nil {
		s.fail(err)
		if !req.wantServerResponse() {
			req.ReplyError(s.id, cmp.Or(req.Ctx.Err(), err))
		}
		return false
	}
	s.recordHealth(nil)
	if req.wantSendConfirmation() {
		req.deliver(response{NodeID: s.id})
	}
	return true
}

// receive routes the frames received on the stream until receiving fails, then
// ends the session and returns the receive error.
func (s *session) receive() error {
	for {
		msg, err := s.stream.Recv()
		if err != nil {
			s.fail(err)
			return err
		}
		s.recordHealth(nil)
		s.received.Store(true)
		s.handle(msg)
		s.endIfDrained()
	}
}

// handle routes one received frame. A frame in the peer's ID space is a new
// request, queued for the handler; it is dropped if there is no handler. A
// frame in this side's ID space is a response to a pending call; it is dropped
// if the call is no longer pending.
func (s *session) handle(msg *Message) {
	msgID := msg.GetMessageSeqNo()
	if isServerSequenceNumber(msgID) == s.serverRequests {
		if s.handler != nil {
			ctx := msg.AppendToIncomingContext(s.ctx)
			s.requests.push(s.ctx, func(release func()) {
				s.handler.HandleRequest(ctx, msg, release, s.reply)
			})
		}
		return
	}
	req, ok := s.pending.take(msgID)
	if !ok {
		return
	}
	resp := response{NodeID: s.id, Value: msg, Err: msg.ErrorStatus()}
	if resp.Err == nil {
		s.latency.observe(time.Since(req.SendTime))
	}
	req.deliver(resp)
}

// fail records err as the channel's health and ends the session, unless the
// session had already ended, in which case err only reflects that ending.
func (s *session) fail(err error) {
	if !s.ended() {
		s.recordHealth(err)
	}
	s.end()
}

// end ends the session and retries or fails its pending calls; see
// [session.retry]. It is idempotent.
func (s *session) end() {
	s.cancel()
	for _, req := range s.pending.drain() {
		s.retry(req)
	}
}

// ended reports whether the session has ended.
func (s *session) ended() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// retry queues req for the channel's next session if the session requeues
// calls and req is not streaming; a streaming call may already have received
// responses, so it cannot be sent again. Otherwise it fails req with
// [ErrNodeClosed] if the session requeues calls and the channel is closed, or
// with [ErrStreamDown].
func (s *session) retry(req Request) {
	switch {
	case s.requeue && !req.Streaming:
		s.queue.push(req, false)
	case s.requeue && s.ctx.Err() != nil:
		req.ReplyError(s.id, ErrNodeClosed)
	default:
		req.ReplyError(s.id, ErrStreamDown)
	}
}

// startDrain stops the session from taking requests from the send queue. The
// session ends once each pending call has completed or its context has ended.
func (s *session) startDrain() {
	if s.draining.CompareAndSwap(false, true) {
		s.pending.watchExpiry(s.endIfDrained)
		close(s.drain)
	}
	s.endIfDrained()
}

// stopSending records that the send loop has returned.
func (s *session) stopSending() {
	s.sendStopped.Store(true)
	s.endIfDrained()
}

// endIfDrained ends a draining session that has stopped sending and has no
// pending calls left.
func (s *session) endIfDrained() {
	if s.draining.Load() && s.sendStopped.Load() && s.pending.len() == 0 {
		s.end()
	}
}
