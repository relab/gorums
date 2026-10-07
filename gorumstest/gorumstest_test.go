package gorumstest_test

import (
	"fmt"
	"testing"

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
