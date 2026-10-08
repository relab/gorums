package gorumstest_test

import (
	"fmt"
	"net"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/relab/gorums/gorumstest"
)

// fatalRecorder is a [testing.TB] that records Fatalf instead of stopping
// the test, so a test can check that a helper fails the test.
type fatalRecorder struct {
	testing.TB
	msg string
}

func (*fatalRecorder) Helper() {}

func (f *fatalRecorder) Fatalf(format string, args ...any) {
	f.msg = fmt.Sprintf(format, args...)
}

// TestGorumstestPeerNode verifies that PeerNode returns the node with the
// given ID, and fails the test when no such node exists.
func TestGorumstestPeerNode(t *testing.T) {
	cfg := gorumstest.Config(t, 3, nil)
	for _, id := range cfg.NodeIDs() {
		if got := gorumstest.PeerNode(t, cfg, id); got.ID() != id {
			t.Errorf("PeerNode(cfg, %d).ID() = %d, want %d", id, got.ID(), id)
		}
	}

	rec := &fatalRecorder{TB: t}
	if got := gorumstest.PeerNode(rec, cfg, 4); got != nil {
		t.Errorf("PeerNode(cfg, 4) = node %d, want nil", got.ID())
	}
	if rec.msg == "" {
		t.Error("PeerNode(cfg, 4) did not fail the test")
	}
}

func TestGorumstestCollect(t *testing.T) {
	tests := []struct {
		name  string
		send  []int
		want  int
		close bool
		got   []int
	}{
		{
			name:  "ClosedShort",
			send:  []int{1, 2},
			want:  3,
			close: true,
			got:   []int{1, 2},
		},
		{
			name:  "ClosedExact",
			send:  []int{1, 2},
			want:  2,
			close: true,
			got:   []int{1, 2},
		},
		{
			name:  "OpenExact",
			send:  []int{1, 2},
			want:  2,
			close: false,
			got:   []int{1, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan int, len(tt.send))
			for _, v := range tt.send {
				ch <- v
			}
			if tt.close {
				close(ch)
			}
			got := gorumstest.Collect(t, time.Second, tt.want, ch)
			if !slices.Equal(got, tt.got) {
				t.Errorf("Collect() = %v, want %v", got, tt.got)
			}
		})
	}
}

// linuxEphemeralStart is the lower bound of the default ephemeral port range
// on Linux. The macOS and IANA ranges start higher, at 49152, so a port below
// this value is outside every one of those ranges.
const linuxEphemeralStart = 32768

func TestGorumstestUnreachableConfigSentinelOutsideEphemeralRange(t *testing.T) {
	cfg := gorumstest.UnreachableConfig(t)
	nodes := cfg.Nodes()
	if len(nodes) != 1 {
		t.Fatalf("Nodes() = %d nodes, want 1", len(nodes))
	}
	host, portStr, err := net.SplitHostPort(nodes[0].Address())
	if err != nil {
		t.Fatalf("Address() = %q: %v", nodes[0].Address(), err)
	}
	if host != "127.0.0.1" {
		t.Errorf("sentinel host = %q, want 127.0.0.1", host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("sentinel port %q: %v", portStr, err)
	}
	if port == 0 || port >= linuxEphemeralStart {
		t.Errorf("sentinel port %d is inside an ephemeral range", port)
	}
}
