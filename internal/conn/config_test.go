package conn

import (
	"context"
	"testing"
	"time"
)

func TestConfigWatch(t *testing.T) {
	makeNodeWithLatency := newTestNodeWithLatency

	// allNodes has five nodes; top-3 by ascending latency are 2(10ms), 3(20ms), 1(30ms).
	allNodes := Config{
		makeNodeWithLatency(1, 30*time.Millisecond),
		makeNodeWithLatency(2, 10*time.Millisecond),
		makeNodeWithLatency(3, 20*time.Millisecond),
		makeNodeWithLatency(4, 40*time.Millisecond),
		makeNodeWithLatency(5, 50*time.Millisecond),
	}
	const quorumSize = 3
	fastTop3 := func(c Config) Config { return c.Sort(ByLatency)[:quorumSize] }

	t.Run("EmitsInitialSnapshot", func(t *testing.T) {
		// Use a very long interval so only the initial emission fires.
		updates := allNodes.Watch(t.Context(), time.Hour, fastTop3)
		snap := <-updates
		if len(snap) != quorumSize {
			t.Fatalf("initial snapshot size = %d, want %d", len(snap), quorumSize)
		}
		wantIDs := []uint32{2, 3, 1}
		for i, n := range snap {
			if n.ID() != wantIDs[i] {
				t.Errorf("position %d: got id %d, want %d", i, n.ID(), wantIDs[i])
			}
		}
	})

	t.Run("NoEmissionWhenUnchanged", func(t *testing.T) {
		updates := allNodes.Watch(t.Context(), 10*time.Millisecond, fastTop3)
		<-updates // drain initial emission

		// Latencies are fixed, so no further emission should arrive.
		select {
		case cfg, ok := <-updates:
			if ok {
				t.Errorf("unexpected second emission: got ids %v", cfg.NodeIDs())
			}
		case <-time.After(100 * time.Millisecond):
			// expected: no second emission
		}
	})

	t.Run("EmitsOnOrderChange", func(t *testing.T) {
		n1 := makeNodeWithLatency(1, 10*time.Millisecond)
		n2 := makeNodeWithLatency(2, 30*time.Millisecond)
		n3 := makeNodeWithLatency(3, 20*time.Millisecond)
		cfg := Config{n1, n2, n3}
		top2 := func(c Config) Config { return c.Sort(ByLatency)[:2] }

		const interval = 20 * time.Millisecond
		updates := cfg.Watch(t.Context(), interval, top2)
		first := <-updates
		// Initial top-2: [1(10ms), 3(20ms)]
		wantFirst := []uint32{1, 3}
		for i, n := range first {
			if n.ID() != wantFirst[i] {
				t.Errorf("initial: position %d got id %d, want %d", i, n.ID(), wantFirst[i])
			}
		}

		// Swap latencies: node 2 becomes fastest.
		NodeTransport(n1).Latency().Store(40 * time.Millisecond)
		NodeTransport(n2).Latency().Store(5 * time.Millisecond)

		select {
		case second := <-updates:
			// New top-2: [2(5ms), 3(20ms)]
			wantSecond := []uint32{2, 3}
			for i, n := range second {
				if n.ID() != wantSecond[i] {
					t.Errorf("after swap: position %d got id %d, want %d", i, n.ID(), wantSecond[i])
				}
			}
		case <-time.After(5 * interval):
			t.Error("expected a second emission after latency swap, but none arrived")
		}
	})

	t.Run("ChannelClosedOnCtxCancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		updates := allNodes.Watch(ctx, time.Hour, fastTop3)
		<-updates // drain initial
		cancel()
		select {
		case _, ok := <-updates:
			if ok {
				t.Error("channel should be closed after ctx cancel")
			}
		case <-time.After(time.Second):
			t.Error("channel should be closed promptly after ctx cancel")
		}
	})
}
