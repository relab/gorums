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
