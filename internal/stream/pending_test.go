package stream

import (
	"context"
	"testing"
)

func TestPendingCalls(t *testing.T) {
	newReq := func(streaming bool) Request {
		return Request{Ctx: context.Background(), Msg: &Message{}, Streaming: streaming, ResponseChan: make(chan response, 1)}
	}
	tests := []struct {
		name      string
		streaming bool
		takes     int // successful takes of msgID 1
	}{
		{name: "NonStreamingTakenOnce", streaming: false, takes: 1},
		{name: "StreamingStays", streaming: true, takes: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p pendingCalls
			if !p.add(1, newReq(tt.streaming)) {
				t.Fatal("add failed on an open table")
			}
			for i := range tt.takes {
				req, ok := p.take(1)
				if !ok {
					t.Fatalf("take %d failed", i)
				}
				if req.SendTime.IsZero() {
					t.Error("add did not stamp the send time")
				}
			}
			if _, ok := p.take(1); ok != tt.streaming {
				t.Errorf("take after %d takes = %t, want %t", tt.takes, ok, tt.streaming)
			}
			if _, ok := p.take(2); ok {
				t.Error("take of an unknown ID succeeded")
			}
		})
	}
}

func TestPendingCallsDrain(t *testing.T) {
	var p pendingCalls
	for i := range 3 {
		p.add(uint64(i), Request{Ctx: context.Background(), Msg: &Message{}})
	}
	if got := len(p.drain()); got != 3 {
		t.Errorf("drain returned %d calls, want 3", got)
	}
	if got := p.len(); got != 0 {
		t.Errorf("len after drain = %d, want 0", got)
	}
	if p.add(4, Request{Ctx: context.Background(), Msg: &Message{}}) {
		t.Error("add succeeded on a drained table")
	}
}

// TestPendingCallsSweepsExpiredCalls verifies that calls whose context ended
// are removed as the table grows, so it stays within about twice the number
// of live calls.
func TestPendingCallsSweepsExpiredCalls(t *testing.T) {
	var p pendingCalls
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	for i := range 10 * minSweepSize {
		p.add(uint64(i), Request{Ctx: expired, Msg: &Message{}, Streaming: true})
	}
	if got := p.len(); got > minSweepSize {
		t.Errorf("len = %d after adding only expired calls, want at most %d", got, minSweepSize)
	}
	const live = 3 * minSweepSize
	for i := range live {
		p.add(uint64(1_000_000+i), Request{Ctx: t.Context(), Msg: &Message{}})
	}
	if got := p.len(); got < live || got > 2*live {
		t.Errorf("len = %d, want between %d and %d", got, live, 2*live)
	}
}

// TestPendingCallsWatchExpiry verifies that a watching table removes a call as
// soon as its context ends, whether the call was added before or after the
// watch started, and reports each removal.
func TestPendingCallsWatchExpiry(t *testing.T) {
	var p pendingCalls
	before, cancelBefore := context.WithCancel(t.Context())
	after, cancelAfter := context.WithCancel(t.Context())
	defer cancelAfter()
	p.add(1, Request{Ctx: before, Msg: &Message{}, Streaming: true})
	p.add(2, Request{Ctx: t.Context(), Msg: &Message{}})

	removed := make(chan struct{}, 4)
	p.watchExpiry(func() { removed <- struct{}{} })
	p.add(3, Request{Ctx: after, Msg: &Message{}, Streaming: true})

	cancelBefore()
	<-removed
	cancelAfter()
	<-removed
	if got := p.len(); got != 1 {
		t.Errorf("len = %d, want 1 (the call whose context is live)", got)
	}
	if _, ok := p.take(2); !ok {
		t.Error("the live call was removed")
	}
}
