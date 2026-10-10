package conn

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
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

func TestConfigNode(t *testing.T) {
	n1 := newTestNode(1, stream.NewChannelWithState(nil))
	n3 := newTestNode(3, stream.NewChannelWithState(nil))
	tests := []struct {
		name string
		cfg  Config
		id   ID
		want *Node
	}{
		{name: "First", cfg: Config{n1, n3}, id: 1, want: n1},
		{name: "Last", cfg: Config{n1, n3}, id: 3, want: n3},
		{name: "NotSortedByID", cfg: Config{n3, n1}, id: 1, want: n1},
		{name: "Absent", cfg: Config{n1, n3}, id: 2, want: nil},
		{name: "ReservedZero", cfg: Config{n1, n3}, id: 0, want: nil},
		{name: "Empty", cfg: Config{}, id: 1, want: nil},
		{name: "Nil", cfg: nil, id: 1, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Node(tt.id); got != tt.want {
				t.Errorf("Node(%d) = node %d, want node %d", tt.id, got.ID(), tt.want.ID())
			}
		})
	}
}

func TestConfigSort(t *testing.T) {
	const unmeasured = -1 * time.Second
	// makeNode returns a node with the given latency and last error.
	// An unmeasured node keeps the latency estimate of a fresh transport.
	makeNode := func(id uint32, latency time.Duration, err error) *Node {
		n := newTestNode(id, stream.NewChannelWithState(err))
		if latency != unmeasured {
			NodeTransport(n).Latency().Store(latency)
		}
		return n
	}
	someErr := errors.New("some error")
	thenID := func(first func(a, b *Node) int) func(a, b *Node) int {
		return func(a, b *Node) int {
			if r := first(a, b); r != 0 {
				return r
			}
			return ByID(a, b)
		}
	}
	errNodes := func() Config {
		return Config{
			makeNode(100, unmeasured, nil),
			makeNode(101, unmeasured, someErr),
			makeNode(42, unmeasured, nil),
			makeNode(99, unmeasured, someErr),
		}
	}

	tests := []struct {
		name    string
		cfg     Config
		cmp     func(a, b *Node) int
		wantIDs []uint32
	}{
		{name: "ByID", cfg: errNodes(), cmp: ByID, wantIDs: []uint32{42, 99, 100, 101}},
		// Stable sort: nodes with equal error status keep their relative order.
		{name: "ByLastError", cfg: errNodes(), cmp: ByLastError, wantIDs: []uint32{100, 42, 101, 99}},
		{name: "ByLastErrorThenID", cfg: errNodes(), cmp: thenID(ByLastError), wantIDs: []uint32{42, 100, 99, 101}},
		{
			// Unmeasured nodes sort after measured nodes.
			name: "ByLatency",
			cfg: Config{
				makeNode(1, 30*time.Millisecond, nil),
				makeNode(2, 10*time.Millisecond, nil),
				makeNode(3, unmeasured, nil),
				makeNode(4, 20*time.Millisecond, nil),
			},
			cmp:     ByLatency,
			wantIDs: []uint32{2, 4, 1, 3},
		},
		{
			// All latencies compare equal, so the stable sort keeps the order.
			name: "ByLatency/AllUnmeasured",
			cfg: Config{
				makeNode(3, unmeasured, nil),
				makeNode(1, unmeasured, nil),
				makeNode(2, unmeasured, nil),
			},
			cmp:     ByLatency,
			wantIDs: []uint32{3, 1, 2},
		},
		{
			name: "ByLatencyThenID",
			cfg: Config{
				makeNode(10, 20*time.Millisecond, nil),
				makeNode(5, 10*time.Millisecond, nil),
				makeNode(7, 20*time.Millisecond, nil),
			},
			cmp:     thenID(ByLatency),
			wantIDs: []uint32{5, 7, 10},
		},
		{
			name: "ByLastErrorThenLatency",
			cfg: Config{
				makeNode(1, 10*time.Millisecond, someErr),
				makeNode(2, 30*time.Millisecond, nil),
				makeNode(3, unmeasured, nil),
				makeNode(4, 20*time.Millisecond, nil),
			},
			cmp: func(a, b *Node) int {
				if r := ByLastError(a, b); r != 0 {
					return r
				}
				return ByLatency(a, b)
			},
			wantIDs: []uint32{4, 2, 3, 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origIDs := tt.cfg.NodeIDs()
			sorted := tt.cfg.Sort(tt.cmp)
			if got := sorted.NodeIDs(); !slices.Equal(got, tt.wantIDs) {
				t.Errorf("Sort: IDs = %v, want %v", got, tt.wantIDs)
			}
			if got := tt.cfg.NodeIDs(); !slices.Equal(got, origIDs) {
				t.Errorf("Sort modified the original config: IDs = %v, want %v", got, origIDs)
			}
			if &sorted[0] == &tt.cfg[0] {
				t.Error("Sort returned the original backing array")
			}
		})
	}

	t.Run("Empty/ReturnsNil", func(t *testing.T) {
		for _, empty := range []Config{nil, {}} {
			if got := empty.Sort(ByID); got != nil {
				t.Errorf("Sort(ByID) on %#v = %v, want nil", empty, got)
			}
			if got := empty.Sort(ByLatency); got != nil {
				t.Errorf("Sort(ByLatency) on %#v = %v, want nil", empty, got)
			}
		}
	})
}
