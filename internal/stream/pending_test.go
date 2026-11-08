package stream

import (
	"context"
	"testing"
)

func TestPendingCalls(t *testing.T) {
	newReq := func(streaming bool) NodeRequest {
		return NodeRequest{Ctx: context.Background(), Msg: &Message{}, Streaming: streaming, ResponseChan: make(chan response, 1)}
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
				if req.sendTime.IsZero() {
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
		p.add(uint64(i), NodeRequest{Ctx: context.Background(), Msg: &Message{}})
	}
	if got := len(p.drain()); got != 3 {
		t.Errorf("drain returned %d calls, want 3", got)
	}
	if got := p.len(); got != 0 {
		t.Errorf("len after drain = %d, want 0", got)
	}
	if p.add(4, NodeRequest{Ctx: context.Background(), Msg: &Message{}}) {
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
		p.add(uint64(i), NodeRequest{Ctx: expired, Msg: &Message{}, Streaming: true})
	}
	if got := p.len(); got > minSweepSize {
		t.Errorf("len = %d after adding only expired calls, want at most %d", got, minSweepSize)
	}
	const live = 3 * minSweepSize
	for i := range live {
		p.add(uint64(1_000_000+i), NodeRequest{Ctx: t.Context(), Msg: &Message{}})
	}
	if got := p.len(); got < live || got > 2*live {
		t.Errorf("len = %d, want between %d and %d", got, live, 2*live)
	}
}
