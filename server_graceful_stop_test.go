package gorums_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"go.uber.org/goleak"
)

// TestGracefulStopReleasesPeerConfig verifies that GracefulStop unblocks
// WaitForPeers and stops the peer configuration's connection goroutines.
// WithNodeList assigns the single unreachable peer ID 1, so myID is 2 and
// that peer is a real outbound channel rather than the in-process local node.
func TestGracefulStopReleasesPeerConfig(t *testing.T) {
	t.Cleanup(func() { goleak.VerifyNone(t) })

	const myID uint32 = 2
	srv := gorums.NewServer(gorums.WithPeers(
		myID,
		gorums.WithNodeList([]string{"127.0.0.1:1"}),
		gorumstest.InsecureDialOptions(t),
	))

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.WaitForPeers(context.Background(), func(gorums.Config) bool {
			return false
		})
	}()

	// Let WaitForPeers block before stopping.
	time.Sleep(20 * time.Millisecond)
	srv.GracefulStop()

	select {
	case err := <-errCh:
		if !errors.Is(err, gorums.ErrStopped) {
			t.Fatalf("WaitForPeers returned %v, want ErrStopped", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForPeers did not return after GracefulStop")
	}
}
