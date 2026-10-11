package stream

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// newTestSession returns a session over a mock stream; it is not started.
func newTestSession(t *testing.T, sendBufferSize uint, handler RequestHandler, serverRequests, requeue bool) *session {
	t.Helper()
	e := newEndpoint(t.Context(), 1, sendBufferSize, 0, handler, newLatency())
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	return newSession(ctx, cancel, &e, mock.NewBidiStream[*Message](), serverRequests, requeue)
}

// TestSessionHandleDeliversResponse verifies that a response is delivered to
// its pending call and updates the latency estimate, and that a second
// response for the same call is dropped.
func TestSessionHandleDeliversResponse(t *testing.T) {
	s := newTestSession(t, 1, nil, true, true)
	responseChan := make(chan response, 2)
	s.pending.add(42, Request{Ctx: t.Context(), Msg: &Message{}, ResponseChan: responseChan})

	msg := Message_builder{MessageSeqNo: 42}.Build()
	s.handle(msg)
	s.handle(msg)
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
	s.pending.add(42, Request{Ctx: ctx, Msg: &Message{}, ResponseChan: responseChan})
	cancel()

	done := make(chan struct{})
	go func() {
		s.handle(Message_builder{MessageSeqNo: 42}.Build())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handle blocked on a canceled request with a full response channel")
	}
}

// TestSessionEndRetriesPendingCalls verifies what happens to each pending call
// when a session ends.
func TestSessionEndRetriesPendingCalls(t *testing.T) {
	tests := []struct {
		name         string
		requeue      bool
		streaming    bool
		closeChannel bool
		wantRequeued bool
		wantErr      error
	}{
		{name: "Requeued", requeue: true, wantRequeued: true},
		{name: "StreamingFails", requeue: true, streaming: true, wantErr: ErrStreamDown},
		{name: "ClosedChannelFails", requeue: true, streaming: true, closeChannel: true, wantErr: ErrNodeClosed},
		{name: "NoRequeueFails", requeue: false, wantErr: ErrStreamDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSession(t, 1, nil, true, tt.requeue)
			responseChan := make(chan response, 1)
			s.pending.add(1, Request{Ctx: t.Context(), Msg: &Message{}, Streaming: tt.streaming, ResponseChan: responseChan})
			if tt.closeChannel {
				s.endpoint.cancel()
			}
			s.end()
			if !s.ended() {
				t.Error("session not ended")
			}
			if got := len(s.queue.ch) == 1; got != tt.wantRequeued {
				t.Errorf("requeued = %t, want %t", got, tt.wantRequeued)
			}
			if tt.wantErr != nil {
				select {
				case got := <-responseChan:
					if !errors.Is(got.Err, tt.wantErr) {
						t.Errorf("error = %v, want %v", got.Err, tt.wantErr)
					}
				default:
					t.Errorf("no response, want %v", tt.wantErr)
				}
			}
		})
	}
}

// TestSessionDrain verifies that a draining session stops taking requests and
// ends once it has stopped sending and its pending calls have completed.
func TestSessionDrain(t *testing.T) {
	s := newTestSession(t, 4, nil, true, true)
	s.pending.add(1, Request{Ctx: t.Context(), Msg: &Message{}, ResponseChan: make(chan response, 1)})
	queued := Request{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 2}.Build()}
	s.queue.push(queued, false)

	s.startDrain()
	if carry := s.sendLoop(nil); carry != nil && carry.Msg.GetMessageSeqNo() != 2 {
		t.Fatalf("send loop returned request %d, want 2 or none", carry.Msg.GetMessageSeqNo())
	}
	if s.ended() {
		t.Fatal("session ended with a call still pending")
	}
	s.handle(Message_builder{MessageSeqNo: 1}.Build())
	s.endIfDrained()
	if !s.ended() {
		t.Fatal("drained session did not end after its last pending call completed")
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

// TestSessionDrainEndsWhenCallerIsDone verifies that a draining session whose
// remaining pending call is streaming ends once that call's context ends.
func TestSessionDrainEndsWhenCallerIsDone(t *testing.T) {
	s := newTestSession(t, 1, nil, true, true)
	ctx, cancel := context.WithCancel(t.Context())
	s.pending.add(1, Request{Ctx: ctx, Msg: &Message{}, Streaming: true, ResponseChan: make(chan response, 1)})
	s.startDrain()
	s.stopSending()
	if s.ended() {
		t.Fatal("session ended while a streaming call was live")
	}
	cancel()
	select {
	case <-s.done:
	case <-time.After(defaultTestTimeout):
		t.Fatal("draining session did not end after its streaming call's context ended")
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
			e := newEndpoint(t.Context(), 1, 1, 0, nil, nil)
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
	e := newEndpoint(t.Context(), 1, 4, 0, nil, nil)
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	s := newSession(ctx, cancel, &e, stream, true, true)

	e.Enqueue(Request{Ctx: t.Context(), Oneway: true, Msg: Message_builder{MessageSeqNo: 1}.Build()})
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
		e := newEndpoint(context.Background(), 1, 0, 0, handler, nil)
		ctx, cancel := context.WithCancel(e.ctx)
		s := newSession(ctx, cancel, &e, stream, true, true)
		go s.sendLoop(nil)
		defer func() {
			stream.Close()
			e.cancel()
			synctest.Wait()
		}()

		// Occupy the send loop: this one-way request is handed to it, and it
		// then blocks in Send on a transport that never drains.
		e.Enqueue(Request{
			Ctx:    context.Background(),
			Oneway: true,
			Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()

		// Route a back-channel request as the receive loop does. The handler
		// replies on the same, now-full queue.
		first := ServerSequenceNumber(1)
		s.handle(Message_builder{MessageSeqNo: first, Method: mock.TestMethod}.Build())
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
		s.handle(Message_builder{MessageSeqNo: second, Method: mock.TestMethod}.Build())
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
	s.pending.add(42, Request{
		Ctx:          context.Background(),
		Msg:          &Message{},
		ResponseChan: responseChan,
	})

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		s.handle(Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build())
		s.handle(Message_builder{MessageSeqNo: ServerSequenceNumber(2), Method: mock.TestMethod}.Build())
		s.handle(Message_builder{MessageSeqNo: 42, Method: mock.TestMethod}.Build())
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
		msg, err := NewMessage(msgCtx, ServerSequenceNumber(1), mock.TestMethod, nil)
		if err != nil {
			t.Fatalf("NewMessage: %v", err)
		}
		s.handle(msg)
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
		s.handle(Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build())
		s.handle(Message_builder{MessageSeqNo: 999, Method: mock.TestMethod}.Build())
	})
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
// requeue is false, over a stream whose Send returns sendErr; it is not
// started.
func newScriptedSession(t *testing.T, sendErr error, requeue bool) (*session, *scriptedStream) {
	t.Helper()
	e := newEndpoint(t.Context(), 1, 4, 0, nil, nil)
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	stream := &scriptedStream{sendErr: sendErr, recv: make(chan scriptedRecv, 4)}
	return newSession(ctx, cancel, &e, stream, true, requeue), stream
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
// stream ends with a refusal code before it delivered a frame fails its
// pending calls with the refusal, and that every other end retries them.
func TestSessionRefusalFailsPendingCalls(t *testing.T) {
	refusal := func(code codes.Code) error { return status.Error(code, "refused by test") }
	tests := []struct {
		name       string
		err        error
		afterFrame bool
		inbound    bool
		wantCode   codes.Code // the code both calls fail with; OK if retried
	}{
		{name: "Unauthenticated", err: refusal(codes.Unauthenticated), wantCode: codes.Unauthenticated},
		{name: "PermissionDenied", err: refusal(codes.PermissionDenied), wantCode: codes.PermissionDenied},
		{name: "InvalidArgument", err: refusal(codes.InvalidArgument), wantCode: codes.InvalidArgument},
		{name: "Unimplemented", err: refusal(codes.Unimplemented), wantCode: codes.Unimplemented},
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
			s.pending.add(1, Request{Ctx: t.Context(), Msg: &Message{}, ResponseChan: twoWay})
			s.pending.add(2, Request{Ctx: t.Context(), Msg: &Message{}, Streaming: true, ResponseChan: streaming})
			if tt.afterFrame {
				stream.frame(Message_builder{MessageSeqNo: 999}.Build()) // an unknown response
			}
			stream.end(tt.err)
			if err := s.receive(); !errors.Is(err, tt.err) {
				t.Fatalf("receive() = %v, want %v", err, tt.err)
			}

			calls := map[string]chan response{"two-way call": twoWay, "streaming call": streaming}
			switch {
			case tt.wantCode != codes.OK:
				for what, ch := range calls {
					if got := status.Code(responseErr(t, ch, what)); got != tt.wantCode {
						t.Errorf("%s failed with code %v, want %v", what, got, tt.wantCode)
					}
				}
				if got := len(s.queue.ch); got != 0 {
					t.Errorf("requeued %d calls, want 0", got)
				}
			case tt.inbound:
				for what, ch := range calls {
					if err := responseErr(t, ch, what); !errors.Is(err, ErrStreamDown) {
						t.Errorf("%s failed with %v, want %v", what, err, ErrStreamDown)
					}
				}
			default:
				if got := len(s.queue.ch); got != 1 {
					t.Errorf("requeued %d calls, want the two-way call", got)
				}
				if err := responseErr(t, streaming, "streaming call"); !errors.Is(err, ErrStreamDown) {
					t.Errorf("streaming call failed with %v, want %v", err, ErrStreamDown)
				}
			}
		})
	}
}

// TestSessionFailedSendLeavesCallsToReceive verifies that a send that fails
// with io.EOF before the stream delivered a frame leaves the pending calls to
// the receive loop, which learns the stream's status: a refusal fails them,
// and any other end, or a frame that shows that the server accepted the
// stream, retries them.
func TestSessionFailedSendLeavesCallsToReceive(t *testing.T) {
	tests := []struct {
		name         string
		recv         func(*scriptedStream)
		wantCode     codes.Code // OK if retried
		wantRequeued bool
	}{
		{
			name:     "Refused",
			recv:     func(st *scriptedStream) { st.end(status.Error(codes.Unauthenticated, "refused")) },
			wantCode: codes.Unauthenticated,
		},
		{
			name:         "Lost",
			recv:         func(st *scriptedStream) { st.end(status.Error(codes.Unavailable, "lost")) },
			wantRequeued: true,
		},
		{
			name: "FrameAfterFailedSend",
			recv: func(st *scriptedStream) {
				st.frame(Message_builder{MessageSeqNo: 1}.Build()) // the call's response
				st.end(status.Error(codes.Unauthenticated, "refused after a frame"))
			},
			wantRequeued: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, stream := newScriptedSession(t, io.EOF, true)
			responseChan := make(chan response, 1)
			s.queue.push(Request{Ctx: t.Context(), Msg: Message_builder{MessageSeqNo: 1}.Build(), ResponseChan: responseChan}, false)
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
			if got := len(s.queue.ch) == 1; got != tt.wantRequeued {
				t.Errorf("requeued = %t, want %t", got, tt.wantRequeued)
			}
			if tt.wantCode != codes.OK {
				if got := status.Code(responseErr(t, responseChan, "call")); got != tt.wantCode {
					t.Errorf("call failed with code %v, want %v", got, tt.wantCode)
				}
			} else if got := len(responseChan); got != 0 {
				t.Errorf("call got %d responses, want it retried", got)
			}
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
