package stream

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newTestSession returns a session over a mock stream; it is not started.
func newTestSession(t *testing.T, sendBufferSize uint, handler RequestHandler, serverRequests, requeue bool) *session {
	t.Helper()
	e := newEndpoint(t.Context(), 1, sendBufferSize, 0, handler, NewLatency())
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	return newSession(ctx, cancel, &e, newMockBidiStream(), serverRequests, requeue)
}

// TestSessionHandleDeliversResponse verifies that a response is delivered to
// its pending call and updates the latency estimate, and that a second
// response for the same call is dropped.
func TestSessionHandleDeliversResponse(t *testing.T) {
	s := newTestSession(t, 1, nil, true, true)
	reply := make(chan response, 2)
	s.pending.add(42, Request{Ctx: t.Context(), Msg: &Message{}, ResponseChan: reply})

	msg := Message_builder{MessageSeqNo: 42}.Build()
	s.handle(msg)
	s.handle(msg)
	if got := len(reply); got != 1 {
		t.Fatalf("responses delivered = %d, want 1", got)
	}
	if got := (<-reply).NodeID; got != 1 {
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
	reply := make(chan response, 1)
	reply <- response{NodeID: 99} // fill the channel
	s.pending.add(42, Request{Ctx: ctx, Msg: &Message{}, ResponseChan: reply})
	cancel()

	done := make(chan struct{})
	go func() {
		s.handle(Message_builder{MessageSeqNo: 42}.Build())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handle blocked on a canceled request with a full reply channel")
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
			reply := make(chan response, 1)
			s.pending.add(1, Request{Ctx: t.Context(), Msg: &Message{}, Streaming: tt.streaming, ResponseChan: reply})
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
				case got := <-reply:
					if !errors.Is(got.Err, tt.wantErr) {
						t.Errorf("error = %v, want %v", got.Err, tt.wantErr)
					}
				default:
					t.Errorf("no reply, want %v", tt.wantErr)
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
// while a send has been blocked for StallReportDelay or longer, and otherwise
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
		{name: "ShortSend", blocked: StallReportDelay / 2, lastErr: streamErr, want: streamErr},
		{name: "StalledSend", blocked: 2 * StallReportDelay, want: ErrSendStalled},
		{name: "StalledSendAfterError", blocked: 2 * StallReportDelay, lastErr: streamErr, want: ErrSendStalled},
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
	stream := newBlockingSendStream()
	t.Cleanup(stream.close)
	e := newEndpoint(t.Context(), 1, 4, 0, nil, nil)
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	s := newSession(ctx, cancel, &e, stream, true, true)

	e.Enqueue(Request{Ctx: t.Context(), Oneway: true, Msg: Message_builder{MessageSeqNo: 1}.Build()})
	go s.sendLoop(nil)
	<-stream.entered
	if e.sendStart.Load() == 0 {
		t.Fatal("blocked send did not record its start")
	}
	stream.release()
	waitID(t, stream.sends, 1, "send completion")
	deadline := time.Now().Add(defaultTestTimeout)
	for e.sendStart.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if e.sendStart.Load() != 0 {
		t.Error("completed send did not clear its start")
	}
}
