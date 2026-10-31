package stream

import (
	"cmp"
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"
)

// BidiStream abstracts both client-side and server-side bidirectional streams.
// Both grpc.BidiStreamingClient[Message, Message] and
// grpc.BidiStreamingServer[Message, Message] satisfy this interface.
type BidiStream interface {
	Send(*Message) error
	Recv() (*Message, error)
}

// session is a channel's state for one gRPC stream. A session sends queued
// requests on the stream, routes the frames received from the stream, and
// holds the two-way calls awaiting responses on the stream. An inbound channel
// has one session. An outbound channel starts a new session for each stream
// that the channel opens.
type session struct {
	*endpoint
	stream BidiStream
	done   <-chan struct{} // closed when the session ends
	cancel context.CancelFunc

	// serverRequests reports whether the peer's requests carry
	// server-initiated IDs, as they do on a stream this side dialed.
	serverRequests bool
	// requeue is set on an outbound session. Calls still pending when the
	// session ends are then queued again for the channel's next session.
	requeue bool

	pending pendingCalls

	drain       chan struct{} // closed when the session starts draining
	draining    atomic.Bool
	sendStopped atomic.Bool
	received    atomic.Bool // a frame has been received

	// verdict is closed when the stream delivers its first frame or receiving
	// ends. refusal is set before that if the server refused the stream.
	verdict chan struct{}
	refusal error
	// handoff decides which goroutine disposes of the pending calls when a
	// stream ends before it delivered a frame.
	handoff atomic.Int32

	// suspect is set on a session opened after a refusal, before any stream
	// proved viable. opened is when the session was created.
	suspect bool
	opened  time.Time
}

// Values of [session.handoff].
const (
	streamFresh      int32 = iota // no frame yet, and neither a send nor receiving failed
	streamSendFailed              // a send failed first; receiving disposes of the pending calls
	streamAccepted                // a frame arrived
	streamClosed                  // receiving ended; receiving disposes of the pending calls
)

// newSession returns a session of e over stream that ends when ctx ends or cancel
// is called.
func newSession(ctx context.Context, cancel context.CancelFunc, e *endpoint, stream BidiStream, serverRequests, requeue bool) *session {
	return &session{
		endpoint:       e,
		stream:         stream,
		done:           ctx.Done(),
		cancel:         cancel,
		serverRequests: serverRequests,
		requeue:        requeue,
		drain:          make(chan struct{}),
		verdict:        make(chan struct{}),
		opened:         time.Now(),
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
			req.sendErrorResponse(s.id, err)
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
	s.sendStart.Store(time.Now().UnixNano())
	err := s.stream.Send(req.Msg)
	s.sendStart.Store(0)
	if err != nil {
		receiveDecides := s.sendFailed(err)
		if !req.wantServerResponse() {
			if receiveDecides {
				// The stream is canceled, so receiving ends promptly and
				// tells whether the server refused the stream.
				<-s.verdict
				err = cmp.Or(s.refusal, err)
			}
			req.sendErrorResponse(s.id, cmp.Or(req.Ctx.Err(), err))
		}
		return false
	}
	if s.healthy() {
		s.recordHealth(nil)
	}
	if req.wantSendConfirmation() {
		req.deliver(response{NodeID: s.id})
	}
	return true
}

// proven reports whether the stream delivered a frame or lived
// [viableStreamAge].
func (s *session) proven() bool {
	return s.received.Load() || time.Since(s.opened) >= viableStreamAge
}

// healthy reports whether a completed send shows that the stream works. A send
// on a stream that the server will refuse completes locally, so a suspect
// session counts only once its stream is proven.
func (s *session) healthy() bool {
	return !s.suspect || s.proven()
}

// sendFailed ends the session after a send failed with err. If the session
// is outbound and receiving has ended or err is io.EOF, the receive loop
// disposes of pending calls, and sendFailed returns true. Otherwise, pending
// calls fail or are retried as in [session.fail], and sendFailed returns
// false.
func (s *session) sendFailed(err error) bool {
	if s.requeue {
		for {
			state := s.handoff.Load()
			if state == streamClosed {
				s.cancel()
				return true
			}
			if !errors.Is(err, io.EOF) {
				break
			}
			if state == streamAccepted {
				// Send returns io.EOF once the stream has ended, so receiving
				// ends promptly with the stream's status.
				return true
			}
			if s.handoff.CompareAndSwap(streamFresh, streamSendFailed) {
				s.cancel()
				return true
			}
		}
	}
	s.fail(err)
	return false
}

// receive routes the frames received on the stream until receiving fails, then
// ends the session and returns the receive error.
func (s *session) receive() error {
	for {
		msg, err := s.stream.Recv()
		if err != nil {
			s.receiveFailed(err)
			return err
		}
		s.recordHealth(nil)
		s.received.Store(true)
		if s.handoff.Load() != streamAccepted {
			s.accept()
		}
		s.handle(msg)
		s.endIfDrained()
	}
}

// accept records that the stream delivered its first frame.
func (s *session) accept() {
	s.handoff.Store(streamAccepted)
	close(s.verdict)
}

// receiveFailed ends the session after a receive failed with err. If the
// session is outbound and err is due to a refused stream or resource
// exhaustion, pending calls fail with err. Otherwise, they are retried.
func (s *session) receiveFailed(err error) {
	prev := s.handoff.Swap(streamClosed)
	if prev != streamAccepted {
		if s.requeue && isRefusal(err) {
			s.refusal = err
		}
		close(s.verdict)
	}
	switch {
	case s.refusal != nil || s.requeue && isResourceExhausted(err):
		s.recordHealth(err)
		s.cancel()
		for _, req := range s.pending.drain() {
			req.sendErrorResponse(s.id, err)
		}
	case prev == streamSendFailed:
		// The failed send ended the session without recording its error;
		// err is the stream's status.
		s.recordHealth(err)
		s.end()
	default:
		s.fail(err)
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
			ctx := msg.appendToIncomingContext(s.ctx)
			s.requests.push(s.ctx, func(release func()) {
				s.handler.HandleRequest(ctx, s.id, msg, release, s.reply)
			})
		}
		return
	}
	req, ok := s.pending.take(msgID)
	if !ok {
		return
	}
	resp := response{NodeID: s.id, Value: msg, Err: msg.errorStatus()}
	if resp.Err == nil {
		s.latency.observe(time.Since(req.sendTime))
	}
	req.deliver(resp)
}

// fail ends the session. It records err as the channel's health unless the
// session has already ended. If the session is outbound and err is due to
// resource exhaustion, pending calls fail with err. Otherwise, they are
// retried.
func (s *session) fail(err error) {
	if !s.ended() {
		s.recordHealth(err)
	}
	if s.requeue && isResourceExhausted(err) {
		s.cancel()
		for _, req := range s.pending.drain() {
			req.sendErrorResponse(s.id, err)
		}
		return
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

// outcome reports whether the stream proved viable, and the refusal that ended
// it, if any. A refusal is nil for a live stream and for one that ended
// without a refusal. A refused stream can also be viable, if it lived long
// enough.
func (s *session) outcome() (viable bool, refusal error) {
	if s.ended() {
		// The session canceled the stream, so receiving ends promptly. It may
		// still deliver a buffered frame, which proves the stream viable.
		<-s.verdict
		refusal = s.refusal
	}
	return s.proven(), refusal
}

// retry queues a call that was pending when the session ended, to be sent on
// the channel's next session. It fails a streaming call with [ErrStreamDown],
// and any call with [ErrSendQueueFull] if the send queue is full or
// [ErrNodeClosed] if the channel is closed. An inbound session has no next
// session, so retry fails every call with [ErrStreamDown].
func (s *session) retry(req Request) {
	switch {
	case s.requeue && !req.Streaming:
		s.queue.push(req, false)
	case s.requeue && s.ctx.Err() != nil:
		req.sendErrorResponse(s.id, ErrNodeClosed)
	default:
		req.sendErrorResponse(s.id, ErrStreamDown)
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
