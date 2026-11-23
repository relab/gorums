package stream

import (
	"context"
	"errors"
	"io"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// newTestSession returns a session over a mock stream; it is not started.
func newTestSession(t *testing.T, sendBufferSize uint, handler RequestHandler, serverRequests, outbound bool) *session {
	t.Helper()
	e := newEndpoint(t.Context(), 1, sendBufferSize, testRecvBufferSize, handler, newLatency())
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	return newSession(ctx, cancel, &e, mock.NewBidiStream[*Message](), serverRequests, outbound)
}

// TestSessionHandleDeliversResponse verifies that a response is delivered to
// its pending call and updates the latency estimate, and that a second
// response for the same call is dropped.
func TestSessionHandleDeliversResponse(t *testing.T) {
	s := newTestSession(t, 1, nil, true, true)
	responseChan := make(chan response, 2)
	s.pending.add(42, NodeRequest{Ctx: t.Context(), Msg: &Message{}, ResponseChan: responseChan})

	msg := Message_builder{MessageSeqNo: 42}.Build()
	s.handle(msg, time.Now())
	s.handle(msg, time.Now())
	if got := len(responseChan); got != 1 {
		t.Fatalf("responses delivered = %d, want 1", got)
	}
	if got := (<-responseChan).NodeID; got != 1 {
		t.Errorf("NodeID = %d, want 1", got)
	}
	if s.latency.Load() < 0 {
		t.Error("latency estimate not updated")
	}
}

// TestSessionHandleDoesNotBlockOnCanceledRequest verifies that delivering to
// a cancelled call with a full response channel does not block.
func TestSessionHandleDoesNotBlockOnCanceledRequest(t *testing.T) {
	s := newTestSession(t, 1, nil, true, true)
	ctx, cancel := context.WithCancel(t.Context())
	responseChan := make(chan response, 1)
	responseChan <- response{NodeID: 99} // fill the channel
	s.pending.add(42, NodeRequest{Ctx: ctx, Msg: &Message{}, ResponseChan: responseChan})
	cancel()

	done := make(chan struct{})
	go func() {
		s.handle(Message_builder{MessageSeqNo: 42}.Build(), time.Now())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handle blocked on a canceled request with a full response channel")
	}
}

// TestSessionEndFailsPendingCalls verifies that each call pending when a
// session ends fails, and that none is queued to be sent again.
func TestSessionEndFailsPendingCalls(t *testing.T) {
	tests := []struct {
		name         string
		outbound     bool
		streaming    bool
		closeChannel bool
		wantErr      error
	}{
		{name: "TwoWay", outbound: true, wantErr: ErrStreamDown},
		{name: "Streaming", outbound: true, streaming: true, wantErr: ErrStreamDown},
		{name: "ClosedChannel", outbound: true, closeChannel: true, wantErr: ErrNodeClosed},
		{name: "ClosedChannelStreaming", outbound: true, streaming: true, closeChannel: true, wantErr: ErrNodeClosed},
		{name: "Inbound", outbound: false, wantErr: ErrStreamDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSession(t, 1, nil, true, tt.outbound)
			responseChan := make(chan response, 1)
			s.pending.add(1, NodeRequest{Ctx: t.Context(), Msg: &Message{}, Streaming: tt.streaming, ResponseChan: responseChan})
			if tt.closeChannel {
				s.endpoint.cancel()
			}
			s.end(nil)
			if !s.ended() {
				t.Error("session not ended")
			}
			if got := len(s.queue.ch); got != 0 {
				t.Errorf("requeued %d calls, want 0", got)
			}
			if err := responseErr(t, responseChan, "pending call"); !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// closeSendStream is a mock client stream that records CloseSend.
type closeSendStream struct {
	*mock.BidiStream[*Message]
	closed atomic.Bool
}

func (c *closeSendStream) CloseSend() error {
	c.closed.Store(true)
	return nil
}

// newDrainTestSession returns an outbound test session over a stream that
// records CloseSend.
func newDrainTestSession(t *testing.T) (*session, *closeSendStream) {
	t.Helper()
	e := newEndpoint(t.Context(), 1, 4, testRecvBufferSize, nil, newLatency())
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	st := &closeSendStream{BidiStream: mock.NewBidiStream[*Message]()}
	return newSession(ctx, cancel, &e, st, true, true), st
}

// TestSessionDrain verifies that a draining session stops taking requests,
// closes its side of the stream once its send loop has stopped, and still
// delivers the reply to a pending call.
func TestSessionDrain(t *testing.T) {
	s, st := newDrainTestSession(t)
	responseChan := make(chan response, 1)
	s.pending.add(1, NodeRequest{Ctx: t.Context(), Msg: &Message{}, ResponseChan: responseChan})
	queued := NodeRequest{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 2}.Build()}
	s.queue.tryPush(queued)

	s.startDrain()
	if st.closed.Load() {
		t.Fatal("session closed its side of the stream before its send loop stopped")
	}
	if carry := s.sendLoop(nil); carry != nil && carry.Msg.GetMessageSeqNo() != 2 {
		t.Fatalf("send loop returned request %d, want 2 or none", carry.Msg.GetMessageSeqNo())
	}
	if !st.closed.Load() {
		t.Fatal("session did not close its side of the stream after its send loop stopped")
	}
	if s.ended() {
		t.Fatal("session ended before the server ended the stream")
	}
	s.handle(Message_builder{MessageSeqNo: 1}.Build(), time.Now())
	select {
	case resp := <-responseChan:
		if resp.Err != nil {
			t.Errorf("reply after drain: %v", resp.Err)
		}
	default:
		t.Error("reply to the pending call was not delivered")
	}
}

// TestSessionFailRecordsOnlyLiveErrors verifies that a stream error is
// recorded in LastErr while the session is live, but not once the session has
// ended, when the error only reflects that ending.
func TestSessionFailRecordsOnlyLiveErrors(t *testing.T) {
	s := newTestSession(t, 1, nil, true, true)
	streamErr := errors.New("stream broken")
	s.fail(streamErr)
	if err := s.LastErr(); !errors.Is(err, streamErr) {
		t.Fatalf("LastErr = %v, want %v", err, streamErr)
	}
	s.recordHealth(nil) // a newer session moved data
	s.fail(context.Canceled)
	if err := s.LastErr(); err != nil {
		t.Errorf("LastErr = %v after the ended session failed again, want nil", err)
	}
}

// TestSessionDrainEndsWhenServerEndsStream verifies that a draining session
// ends when the server ends the stream, and that a call still pending fails
// with ErrStreamDown without being queued again or recorded as an error.
func TestSessionDrainEndsWhenServerEndsStream(t *testing.T) {
	s, _ := newDrainTestSession(t)
	responseChan := make(chan response, 1)
	s.pending.add(1, NodeRequest{Ctx: t.Context(), Msg: &Message{}, ResponseChan: responseChan})
	s.accept()
	s.startDrain()

	s.receiveFailed(io.EOF)
	if !s.ended() {
		t.Fatal("session did not end when the server ended the stream")
	}
	select {
	case resp := <-responseChan:
		if !errors.Is(resp.Err, ErrStreamDown) {
			t.Errorf("pending call error = %v, want %v", resp.Err, ErrStreamDown)
		}
	default:
		t.Error("pending call did not fail")
	}
	if n := len(s.queue.ch); n != 0 {
		t.Errorf("queued %d calls again, want 0", n)
	}
	if err := s.LastErr(); err != nil {
		t.Errorf("LastErr = %v, want nil", err)
	}
}

// TestLastErrReportsStalledSend verifies that LastErr reports ErrSendStalled
// while a send has been blocked for stallReportDelay or longer, and otherwise
// the outcome of the latest stream operation.
func TestLastErrReportsStalledSend(t *testing.T) {
	streamErr := errors.New("stream broken")
	tests := []struct {
		name    string
		blocked time.Duration // how long the send in progress has been blocked; 0 for none
		lastErr error
		want    error
	}{
		{name: "NoSend", want: nil},
		{name: "NoSendAfterError", lastErr: streamErr, want: streamErr},
		{name: "ShortSend", blocked: stallReportDelay / 2, lastErr: streamErr, want: streamErr},
		{name: "StalledSend", blocked: 2 * stallReportDelay, want: ErrSendStalled},
		{name: "StalledSendAfterError", blocked: 2 * stallReportDelay, lastErr: streamErr, want: ErrSendStalled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEndpoint(t.Context(), 1, 1, testRecvBufferSize, nil, nil)
			t.Cleanup(e.cancel)
			e.recordHealth(tt.lastErr)
			if tt.blocked > 0 {
				e.sendStart.Store(time.Now().Add(-tt.blocked).UnixNano())
			}
			if got := e.LastErr(); !errors.Is(got, tt.want) && got != tt.want {
				t.Errorf("LastErr = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSessionSendMarksSendInProgress verifies that a send records its start
// while it is blocked and clears it once it completes.
func TestSessionSendMarksSendInProgress(t *testing.T) {
	stream := mock.NewGatedBidiStream[*Message]()
	t.Cleanup(stream.Close)
	e := newEndpoint(t.Context(), 1, 4, testRecvBufferSize, nil, nil)
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	s := newSession(ctx, cancel, &e, stream, true, true)

	e.Enqueue(NodeRequest{Ctx: t.Context(), Oneway: true, Msg: Message_builder{MessageSeqNo: 1}.Build()})
	go s.sendLoop(nil)
	<-stream.Entered()
	if e.sendStart.Load() == 0 {
		t.Fatal("blocked send did not record its start")
	}
	stream.Release()
	waitID(t, stream.Sent(), 1, "send completion")
	deadline := time.Now().Add(defaultTestTimeout)
	for e.sendStart.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if e.sendStart.Load() != 0 {
		t.Error("completed send did not clear its start")
	}
}

// TestSessionDispatchNotWedgedByReentrantReply verifies that a back-channel
// handler can reply on a full send queue and still let the next request run:
// the reply is dropped instead of waiting for space, the handler returns, and
// the dispatcher starts the next request.
func TestSessionDispatchNotWedgedByReentrantReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The handler answers every request on the session it was dispatched
		// on: the reentrant back-channel reply.
		dispatched := make(chan uint64, 8)
		replied := make(chan uint64, 8)
		handler := requestHandlerFunc(func(_ context.Context, msg *Message, release func(), send func(*Message)) {
			defer release()
			dispatched <- msg.GetMessageSeqNo()
			send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo(), Method: mock.TestMethod}.Build())
			replied <- msg.GetMessageSeqNo()
		})

		// Capacity 0: once the send loop is occupied in Send, the queue has
		// no slack, so a reply would have to wait for space.
		stream := mock.NewGatedBidiStream[*Message]()
		e := newEndpoint(context.Background(), 1, 0, testRecvBufferSize, handler, nil)
		ctx, cancel := context.WithCancel(e.ctx)
		s := newSession(ctx, cancel, &e, stream, true, true)
		// As in an outbound channel, the receive loop runs while the session
		// sends; it decides the outcome of a send that the closed stream fails.
		go func() { _ = s.receive() }()
		go s.sendLoop(nil)
		defer func() {
			stream.Close()
			e.cancel()
			synctest.Wait()
		}()

		// Occupy the send loop: this one-way request is handed to it, and it
		// then blocks in Send on a transport that never drains.
		e.queue.push(NodeRequest{
			Ctx:    context.Background(),
			Oneway: true,
			Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()

		// Route a back-channel request as the receive loop does. The handler
		// replies on the same, now-full queue.
		first := ServerSequenceNumber(1)
		s.handle(Message_builder{MessageSeqNo: first, Method: mock.TestMethod}.Build(), time.Now())
		synctest.Wait()

		if got := drain(replied); !slices.Equal(got, []uint64{first}) {
			t.Fatalf("replied = %v, want [%d]: the reply blocked on a full send queue", got, first)
		}
		if got := e.DroppedReplies(); got != 1 {
			t.Fatalf("DroppedReplies() = %d, want 1: the reply did not reach the full send queue", got)
		}

		// The first handler has returned, so the dispatcher runs the next
		// request, whose reply is dropped the same way.
		second := ServerSequenceNumber(2)
		s.handle(Message_builder{MessageSeqNo: second, Method: mock.TestMethod}.Build(), time.Now())
		synctest.Wait()

		if got, want := drain(dispatched), []uint64{first, second}; !slices.Equal(got, want) {
			t.Fatalf("dispatched = %v, want %v", got, want)
		}
		if got := drain(replied); !slices.Equal(got, []uint64{second}) {
			t.Fatalf("replied = %v, want [%d]", got, second)
		}
		if got := e.DroppedReplies(); got != 2 {
			t.Errorf("DroppedReplies() = %d, want 2", got)
		}
	})
}

// TestSessionHandleKeepsReadingWhileHandlerUnreleased verifies that the
// receive loop can queue the next request and deliver a response while a
// handler has not released, and that the second request still waits for that
// release.
func TestSessionHandleKeepsReadingWhileHandlerUnreleased(t *testing.T) {
	releaseFirst := make(chan struct{})
	started := make(chan uint64, 2)
	handler := requestHandlerFunc(func(_ context.Context, msg *Message, release func(), _ func(*Message)) {
		started <- msg.GetMessageSeqNo()
		if msg.GetMessageSeqNo() == ServerSequenceNumber(1) {
			<-releaseFirst
		}
		release()
	})
	s := newTestSession(t, 0, handler, true, true)

	responseChan := make(chan response, 1)
	s.pending.add(42, NodeRequest{
		Ctx:          context.Background(),
		Msg:          &Message{},
		ResponseChan: responseChan,
	})

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		s.handle(Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build(), time.Now())
		s.handle(Message_builder{MessageSeqNo: ServerSequenceNumber(2), Method: mock.TestMethod}.Build(), time.Now())
		s.handle(Message_builder{MessageSeqNo: 42, Method: mock.TestMethod}.Build(), time.Now())
	}()

	select {
	case id := <-started:
		if id != ServerSequenceNumber(1) {
			t.Fatalf("first handler id = %d, want %d", id, ServerSequenceNumber(1))
		}
	case <-time.After(time.Second):
		t.Fatal("first handler did not start")
	}

	select {
	case <-responseChan:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("response was not delivered while a handler was unreleased")
	}

	select {
	case id := <-started:
		t.Fatalf("handler %d started before the first handler released", id)
	default:
	}

	select {
	case <-readerDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reader blocked dispatching the next request behind an unreleased handler")
	}

	close(releaseFirst)

	select {
	case id := <-started:
		if id != ServerSequenceNumber(2) {
			t.Fatalf("second handler id = %d, want %d", id, ServerSequenceNumber(2))
		}
	case <-time.After(time.Second):
		t.Fatal("second handler did not start after the first handler released")
	}
}

// TestSessionHandleBackChannel verifies how an outbound session routes a
// server-initiated request and an unknown response.
func TestSessionHandleBackChannel(t *testing.T) {
	t.Run("HandlerSeesMessageMetadata", func(t *testing.T) {
		const key, want = "request-id", "dedup-metadata"
		handlerMD := make(chan metadata.MD, 1)
		handler := requestHandlerFunc(func(ctx context.Context, _ *Message, release func(), _ func(*Message)) {
			defer release()
			md, _ := metadata.FromIncomingContext(ctx)
			handlerMD <- md
		})
		s := newTestSession(t, 0, handler, true, true)

		msgCtx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs(key, want))
		msg, err := NewRequest(msgCtx, ServerSequenceNumber(1), mock.TestMethod, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		s.handle(msg, time.Now())
		select {
		case md := <-handlerMD:
			if got := md.Get(key); len(got) != 1 || got[0] != want {
				t.Fatalf("incoming metadata %q = %v, want [%q]", key, got, want)
			}
		case <-time.After(defaultTestTimeout):
			t.Fatal("handler was not called")
		}
	})

	t.Run("NoHandlerOrUnknownIDIsDropped", func(t *testing.T) {
		s := newTestSession(t, 0, nil, true, true)
		s.handle(Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build(), time.Now())
		s.handle(Message_builder{MessageSeqNo: 999, Method: mock.TestMethod}.Build(), time.Now())
	})
}

// TestSessionHandleUnmatchedErrors verifies that an error reply that matches
// no pending call, as the server sends for a one-way call to an unknown
// method, is counted and logged with the peer, method, and message ID. So is
// an error reply to a pending call whose context has ended, even before a
// sweep removes the call. A reply without an error, or an error reply to a
// live pending call, is not.
func TestSessionHandleUnmatchedErrors(t *testing.T) {
	const msgID = 7
	errStatus := status.New(codes.Unimplemented, "no handler registered").Proto()
	tests := []struct {
		name      string
		pending   bool
		ended     bool // the pending call's context has ended
		msg       *Message
		wantCount int64
		wantLog   []string
	}{
		{
			name:      "ErrorReplyWithoutPendingCall",
			msg:       Message_builder{MessageSeqNo: msgID, Method: mock.TestMethod, Status: errStatus}.Build(),
			wantCount: 1,
			wantLog:   []string{"node 1", mock.TestMethod, "message 7", "Unimplemented"},
		},
		{
			name: "ReplyWithoutPendingCall",
			msg:  Message_builder{MessageSeqNo: msgID, Method: mock.TestMethod}.Build(),
		},
		{
			name:    "ErrorReplyToPendingCall",
			pending: true,
			msg:     Message_builder{MessageSeqNo: msgID, Method: mock.TestMethod, Status: errStatus}.Build(),
		},
		{
			name:      "ErrorReplyToEndedPendingCall",
			pending:   true,
			ended:     true,
			msg:       Message_builder{MessageSeqNo: msgID, Method: mock.TestMethod, Status: errStatus}.Build(),
			wantCount: 1,
			wantLog:   []string{"node 1", mock.TestMethod, "message 7", "Unimplemented"},
		},
		{
			name:    "ReplyToEndedPendingCall",
			pending: true,
			ended:   true,
			msg:     Message_builder{MessageSeqNo: msgID, Method: mock.TestMethod}.Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSession(t, 1, nil, true, true)
			var logBuf strings.Builder
			s.unmatchedErrors.logger = log.New(&logBuf, "", 0)
			responseChan := make(chan response, 1)
			if tt.pending {
				ctx, cancel := context.WithCancel(t.Context())
				if tt.ended {
					cancel()
				}
				defer cancel()
				s.pending.add(msgID, NodeRequest{Ctx: ctx, Msg: &Message{}, ResponseChan: responseChan})
			}

			s.handle(tt.msg, time.Now())
			if got := s.UnmatchedErrors(); got != tt.wantCount {
				t.Errorf("UnmatchedErrors() = %d, want %d", got, tt.wantCount)
			}
			logged := logBuf.String()
			if len(tt.wantLog) == 0 && logged != "" {
				t.Errorf("logged %q, want nothing", logged)
			}
			for _, want := range tt.wantLog {
				if !strings.Contains(logged, want) {
					t.Errorf("logged %q, want it to contain %q", logged, want)
				}
			}
			if tt.pending && tt.msg.GetStatus() != nil {
				if resp := <-responseChan; status.Code(resp.Err) != codes.Unimplemented {
					t.Errorf("pending call error = %v, want codes.Unimplemented", resp.Err)
				}
			}
		})
	}
}

// scriptedStream is a [BidiStream] whose Send returns sendErr and whose Recv
// returns the frames and errors queued on recv, in order.
type scriptedStream struct {
	sendErr error
	recv    chan scriptedRecv
}

// scriptedRecv is one result of [scriptedStream.Recv].
type scriptedRecv struct {
	msg *Message
	err error
}

func (s *scriptedStream) Send(*Message) error { return s.sendErr }

func (s *scriptedStream) Recv() (*Message, error) {
	r := <-s.recv
	return r.msg, r.err
}

// frame queues msg for Recv.
func (s *scriptedStream) frame(msg *Message) { s.recv <- scriptedRecv{msg: msg} }

// end queues err for Recv.
func (s *scriptedStream) end(err error) { s.recv <- scriptedRecv{err: err} }

// newScriptedSession returns an outbound session, or an inbound one if
// outbound is false, over a stream whose Send returns sendErr; it is not
// started.
func newScriptedSession(t *testing.T, sendErr error, outbound bool) (*session, *scriptedStream) {
	t.Helper()
	e := newEndpoint(t.Context(), 1, 4, testRecvBufferSize, nil, nil)
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	stream := &scriptedStream{sendErr: sendErr, recv: make(chan scriptedRecv, 4)}
	return newSession(ctx, cancel, &e, stream, true, outbound), stream
}

// responseErr returns the error of the response on ch, or fails the test if
// there is none.
func responseErr(t *testing.T, ch <-chan response, what string) error {
	t.Helper()
	select {
	case resp := <-ch:
		return resp.Err
	default:
		t.Fatalf("%s: no response", what)
		return nil
	}
}

// TestSessionRefusalFailsPendingCalls verifies that an outbound session whose
// stream ends with a refusal code before it delivered a frame, or with
// ResourceExhausted at any time, fails its pending calls with that status, and
// that every other end fails them with ErrStreamDown.
func TestSessionRefusalFailsPendingCalls(t *testing.T) {
	refusal := func(code codes.Code) error { return status.Error(code, "refused by test") }
	tests := []struct {
		name       string
		err        error
		afterFrame bool
		inbound    bool
		wantCode   codes.Code // the code both calls fail with; OK for ErrStreamDown
	}{
		{name: "Unauthenticated", err: refusal(codes.Unauthenticated), wantCode: codes.Unauthenticated},
		{name: "PermissionDenied", err: refusal(codes.PermissionDenied), wantCode: codes.PermissionDenied},
		{name: "InvalidArgument", err: refusal(codes.InvalidArgument), wantCode: codes.InvalidArgument},
		{name: "Unimplemented", err: refusal(codes.Unimplemented), wantCode: codes.Unimplemented},
		{name: "ResourceExhausted", err: refusal(codes.ResourceExhausted), wantCode: codes.ResourceExhausted},
		{name: "ResourceExhaustedAfterFrame", err: refusal(codes.ResourceExhausted), afterFrame: true, wantCode: codes.ResourceExhausted},
		{name: "Unavailable", err: refusal(codes.Unavailable)},
		{name: "Unknown", err: refusal(codes.Unknown)},
		{name: "Canceled", err: refusal(codes.Canceled)},
		{name: "EOF", err: io.EOF},
		{name: "RefusalAfterFrame", err: refusal(codes.Unauthenticated), afterFrame: true},
		{name: "Inbound", err: refusal(codes.Unauthenticated), inbound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, stream := newScriptedSession(t, nil, !tt.inbound)
			twoWay := make(chan response, 1)
			streaming := make(chan response, 1)
			s.pending.add(1, NodeRequest{Ctx: t.Context(), Msg: &Message{}, ResponseChan: twoWay})
			s.pending.add(2, NodeRequest{Ctx: t.Context(), Msg: &Message{}, Streaming: true, ResponseChan: streaming})
			if tt.afterFrame {
				stream.frame(Message_builder{MessageSeqNo: 999}.Build()) // an unknown response
			}
			stream.end(tt.err)
			if err := s.receive(); !errors.Is(err, tt.err) {
				t.Fatalf("receive() = %v, want %v", err, tt.err)
			}

			calls := map[string]chan response{"two-way call": twoWay, "streaming call": streaming}
			for what, ch := range calls {
				err := responseErr(t, ch, what)
				if tt.wantCode != codes.OK {
					if got := status.Code(err); got != tt.wantCode {
						t.Errorf("%s failed with code %v, want %v", what, got, tt.wantCode)
					}
				} else if !errors.Is(err, ErrStreamDown) {
					t.Errorf("%s failed with %v, want %v", what, err, ErrStreamDown)
				}
			}
			if got := len(s.queue.ch); got != 0 {
				t.Errorf("requeued %d calls, want 0", got)
			}
		})
	}
}

// TestSessionSendResourceExhaustedFailsPendingCalls verifies that a send that
// fails locally with ResourceExhausted fails the request and the other pending
// calls with that status, instead of sending them again on a new stream that
// would end the same way.
func TestSessionSendResourceExhaustedFailsPendingCalls(t *testing.T) {
	sizeErr := status.Error(codes.ResourceExhausted, "trying to send message larger than max")
	s, _ := newScriptedSession(t, sizeErr, true)
	oversized := make(chan response, 1)
	other := make(chan response, 1)
	s.pending.add(2, NodeRequest{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 2}.Build(), ResponseChan: other})
	req := NodeRequest{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 1}.Build(), ResponseChan: oversized}
	if left := s.sendLoop(&req); left != nil {
		t.Fatalf("sendLoop returned unsent request %d", left.Msg.GetMessageSeqNo())
	}
	for what, ch := range map[string]chan response{"oversized call": oversized, "other call": other} {
		if err := responseErr(t, ch, what); !errors.Is(err, sizeErr) {
			t.Errorf("%s failed with %v, want %v", what, err, sizeErr)
		}
	}
	if got := len(s.queue.ch); got != 0 {
		t.Errorf("requeued %d calls, want 0", got)
	}
}

// TestSessionFailedSendLeavesCallsToReceive verifies that a send that fails
// with io.EOF before the stream delivered a frame leaves the pending call to
// the receive loop, which learns the stream's status: a refusal fails the
// call with the refusal, a buffered response completes it, and any other end
// fails it with ErrStreamDown.
func TestSessionFailedSendLeavesCallsToReceive(t *testing.T) {
	tests := []struct {
		name     string
		recv     func(*scriptedStream)
		wantCode codes.Code // the code the call fails with; OK if it completes
		wantErr  error      // the error the call fails with, if wantCode is Unavailable
	}{
		{
			name:     "Refused",
			recv:     func(st *scriptedStream) { st.end(status.Error(codes.Unauthenticated, "refused")) },
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "Lost",
			recv:     func(st *scriptedStream) { st.end(status.Error(codes.Unavailable, "lost")) },
			wantCode: codes.Unavailable,
			wantErr:  ErrStreamDown,
		},
		{
			name: "BufferedResponse",
			recv: func(st *scriptedStream) {
				st.frame(Message_builder{MessageSeqNo: 1}.Build()) // the call's response
				st.end(status.Error(codes.Unauthenticated, "refused after a frame"))
			},
		},
		{
			name: "FrameAfterFailedSend",
			recv: func(st *scriptedStream) {
				st.frame(Message_builder{MessageSeqNo: 999}.Build()) // an unknown response
				st.end(status.Error(codes.Unauthenticated, "refused after a frame"))
			},
			wantCode: codes.Unavailable,
			wantErr:  ErrStreamDown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, stream := newScriptedSession(t, io.EOF, true)
			responseChan := make(chan response, 1)
			s.queue.tryPush(NodeRequest{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 1}.Build(), ResponseChan: responseChan})
			if req := s.sendLoop(nil); req != nil {
				t.Fatalf("send loop returned request %d, want none", req.Msg.GetMessageSeqNo())
			}
			if !s.ended() {
				t.Fatal("session did not end after its send failed")
			}
			if len(responseChan) != 0 || len(s.queue.ch) != 0 {
				t.Fatal("the failed send disposed of the pending call; want it left to the receive loop")
			}

			tt.recv(stream)
			_ = s.receive()
			if got := len(s.queue.ch); got != 0 {
				t.Errorf("requeued %d calls, want 0", got)
			}
			err := responseErr(t, responseChan, "call")
			switch {
			case tt.wantCode == codes.OK:
				if err != nil {
					t.Errorf("call failed with %v, want its buffered response", err)
				}
			case status.Code(err) != tt.wantCode:
				t.Errorf("call failed with code %v, want %v", status.Code(err), tt.wantCode)
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Errorf("call failed with %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestSessionFailedSendAfterFrameLeavesCallsToReceive verifies that a send
// that fails with io.EOF after the stream delivered a frame leaves the pending
// calls to the receive loop, which learns the stream's status. A stream that
// ends with ResourceExhausted fails the calls with that status; any other end
// fails them with ErrStreamDown and records the status in LastErr.
func TestSessionFailedSendAfterFrameLeavesCallsToReceive(t *testing.T) {
	resourceExhausted := status.Error(codes.ResourceExhausted, "grpc: received message larger than max")
	tests := []struct {
		name    string
		end     error
		wantErr error // the error both calls fail with
	}{
		{name: "ResourceExhausted", end: resourceExhausted, wantErr: resourceExhausted},
		{name: "Lost", end: status.Error(codes.Unavailable, "lost"), wantErr: ErrStreamDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, stream := newScriptedSession(t, io.EOF, true)
				received := make(chan error, 1)
				go func() { received <- s.receive() }()
				stream.frame(Message_builder{MessageSeqNo: 999}.Build()) // an unknown response
				synctest.Wait()                                          // the stream is accepted; receiving waits for more

				twoWay := make(chan response, 1)
				streaming := make(chan response, 1)
				s.pending.add(2, NodeRequest{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 2}.Build(), Streaming: true, ResponseChan: streaming})
				s.queue.tryPush(NodeRequest{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 1}.Build(), ResponseChan: twoWay})
				if req := s.sendLoop(nil); req != nil {
					t.Fatalf("send loop returned request %d, want none", req.Msg.GetMessageSeqNo())
				}
				if len(twoWay) != 0 || len(streaming) != 0 || len(s.queue.ch) != 0 {
					t.Fatal("the failed send disposed of the pending calls; want them left to the receive loop")
				}

				stream.end(tt.end)
				if err := <-received; !errors.Is(err, tt.end) {
					t.Fatalf("receive() = %v, want %v", err, tt.end)
				}
				if got := len(s.queue.ch); got != 0 {
					t.Errorf("requeued %d calls, want 0", got)
				}
				for what, ch := range map[string]chan response{"two-way call": twoWay, "streaming call": streaming} {
					if err := responseErr(t, ch, what); !errors.Is(err, tt.wantErr) {
						t.Errorf("%s failed with %v, want %v", what, err, tt.wantErr)
					}
				}
				if err := s.LastErr(); !errors.Is(err, tt.end) {
					t.Errorf("LastErr = %v, want %v", err, tt.end)
				}
			})
		})
	}
}

// TestSessionOnewaySendFailure verifies that a one-way call whose send fails
// with io.EOF before the stream delivered a frame fails with the server's
// refusal if the server refused the stream, and with the send's error if the
// stream was lost.
func TestSessionOnewaySendFailure(t *testing.T) {
	refusal := status.Error(codes.Unauthenticated, "refused")
	tests := []struct {
		name string
		end  error
		want error
	}{
		{name: "Refused", end: refusal, want: refusal},
		{name: "Lost", end: status.Error(codes.Unavailable, "lost"), want: io.EOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, stream := newScriptedSession(t, io.EOF, true)
				go func() { _ = s.receive() }()
				responseChan := make(chan response, 1)
				s.queue.tryPush(NodeRequest{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 1}.Build(), Oneway: true, ResponseChan: responseChan})
				go s.sendLoop(nil)
				synctest.Wait() // the send has failed; the stream has not ended yet
				stream.end(tt.end)
				synctest.Wait()
				if err := responseErr(t, responseChan, "one-way call"); !errors.Is(err, tt.want) {
					t.Errorf("one-way call failed with %v, want %v", err, tt.want)
				}
			})
		})
	}
}

// beforeEOFMatch pauses sendFailed after it loaded the handoff state.
type beforeEOFMatch func()

func (beforeEOFMatch) Error() string { return io.EOF.Error() }

func (f beforeEOFMatch) Is(target error) bool {
	f()
	return target == io.EOF
}

// TestSessionRefusalOwnsPendingAfterCancellation forces the receiver to cancel
// between the sender's handoff load and CAS, before the receiver drains calls.
func TestSessionRefusalOwnsPendingAfterCancellation(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "TwoWay"
		if streaming {
			name = "Streaming"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, _ := newScriptedSession(t, nil, true)
				responseChan := make(chan response, 1)
				s.pending.add(1, NodeRequest{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 1}.Build(), Streaming: streaming, ResponseChan: responseChan})
				refusal := status.Error(codes.Unauthenticated, "refused")
				canceled, resume := make(chan struct{}), make(chan struct{})
				cancel := s.cancel
				var cancels atomic.Int32
				s.cancel = func() {
					cancel()
					if cancels.Add(1) == 1 {
						close(canceled)
						<-resume
					}
				}
				s.sendFailed(beforeEOFMatch(func() {
					go s.receiveFailed(refusal)
					<-canceled
				}))
				close(resume)
				synctest.Wait()

				if got := len(s.queue.ch); got != 0 {
					t.Errorf("requeued %d calls, want 0", got)
				}
				if err := responseErr(t, responseChan, "refused call"); !errors.Is(err, refusal) {
					t.Errorf("call failed with %v, want %v", err, refusal)
				}
			})
		})
	}
}

// TestSessionVerdict verifies that a session's verdict is closed once the
// stream delivers its first frame or receiving ends, and that it reports a
// refusal only for a refused stream.
func TestSessionVerdict(t *testing.T) {
	tests := []struct {
		name        string
		recv        func(*scriptedStream)
		wantRefusal codes.Code // OK for none
	}{
		{name: "Frame", recv: func(st *scriptedStream) { st.frame(Message_builder{MessageSeqNo: 999}.Build()) }},
		{name: "Refused", recv: func(st *scriptedStream) { st.end(status.Error(codes.PermissionDenied, "refused")) }, wantRefusal: codes.PermissionDenied},
		{name: "Lost", recv: func(st *scriptedStream) { st.end(status.Error(codes.Unavailable, "lost")) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, stream := newScriptedSession(t, nil, true)
				go func() { _ = s.receive() }()
				defer func() {
					stream.end(context.Canceled) // ends a receive loop still running
					synctest.Wait()
				}()
				synctest.Wait()
				select {
				case <-s.verdict:
					t.Fatal("verdict closed before the stream delivered anything")
				default:
				}
				tt.recv(stream)
				synctest.Wait()
				select {
				case <-s.verdict:
				default:
					t.Fatal("verdict not closed")
				}
				if got := status.Code(s.refusal); got != tt.wantRefusal {
					t.Errorf("refusal = %v, want code %v", s.refusal, tt.wantRefusal)
				}
			})
		})
	}
}

// TestSessionHandleSetsResponseLatency verifies that every response of a
// call, also one of a streaming call and one with an error status, carries the
// time since the single send of the request.
func TestSessionHandleSetsResponseLatency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestSession(t, 1, nil, true, true)
		responseChan := make(chan response, 3)
		s.pending.add(42, NodeRequest{Ctx: t.Context(), Msg: &Message{}, Streaming: true, ResponseChan: responseChan})

		time.Sleep(10 * time.Millisecond)
		s.handle(Message_builder{MessageSeqNo: 42}.Build(), time.Now())
		time.Sleep(15 * time.Millisecond)
		s.handle(Message_builder{MessageSeqNo: 42}.Build(), time.Now())
		time.Sleep(5 * time.Millisecond)
		s.handle(Message_builder{MessageSeqNo: 42, Status: status.New(codes.Internal, "boom").Proto()}.Build(), time.Now())

		for i, want := range []time.Duration{10 * time.Millisecond, 25 * time.Millisecond, 30 * time.Millisecond} {
			if got := (<-responseChan).Latency; got != want {
				t.Errorf("response %d: Latency = %v, want %v", i+1, got, want)
			}
		}
	})
}

// doneRequestHandler marks one request done on wg for each request it handles.
type doneRequestHandler struct{ wg *sync.WaitGroup }

func (h *doneRequestHandler) HandleRequest(context.Context, ID, *Message, time.Time, func(), func(*Message)) {
	h.wg.Done()
}

// BenchmarkSessionHandleRequest measures the cost of routing one request to
// its handler as the receive loop does, including the receive time that the
// loop takes with time.Now for each frame. BenchmarkTimeNow isolates that
// clock read.
func BenchmarkSessionHandleRequest(b *testing.B) {
	var wg sync.WaitGroup
	handler := &doneRequestHandler{wg: &wg}
	e := newEndpoint(b.Context(), 1, 1, testRecvBufferSize, handler, newLatency())
	b.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	s := newSession(ctx, cancel, &e, mock.NewBidiStream[*Message](), false, false)
	msg := Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build()
	b.ReportAllocs()
	for b.Loop() {
		wg.Add(1)
		s.handle(msg, time.Now())
		wg.Wait()
	}
}

// BenchmarkTimeNow measures one clock read, the added cost of stamping each
// inbound request.
func BenchmarkTimeNow(b *testing.B) {
	for b.Loop() {
		time.Now()
	}
}
