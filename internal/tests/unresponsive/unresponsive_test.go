package unresponsive

import (
	context "context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
)

type testSrv struct{}

func (testSrv) TestUnresponsive(ctx gorums.ServerContext, _ *Empty) (resp *Empty, err error) {
	<-ctx.Done()
	return nil, nil
}

func serverFn(_ int) gorumstest.ServerIface {
	gorumsSrv := gorums.NewServer()
	RegisterUnresponsiveServer(gorumsSrv, &testSrv{})
	return gorumsSrv
}

// TestUnresponsiveServer verifies that a call to a server that never replies
// returns when the call's context ends, so the client is not blocked.
func TestUnresponsiveServer(t *testing.T) {
	node := gorumstest.Node(t, serverFn)

	for range 100 {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		start := time.Now()
		_, err := TestUnresponsive(node.Context(ctx), &Empty{})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("TestUnresponsive() error = %v, want %v", err, context.DeadlineExceeded)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("TestUnresponsive() returned after %v, want shortly after the 10ms deadline", elapsed)
		}
	}
}
