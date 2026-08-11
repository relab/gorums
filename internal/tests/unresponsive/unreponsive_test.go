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

func (testSrv) TestUnresponsive(ctx gorums.ServerCtx, _ *Empty) (resp *Empty, err error) {
	<-ctx.Done()
	return nil, nil
}

func serverFn(_ int) gorums.ServerIface {
	gorumsSrv := gorums.NewServer()
	RegisterUnresponsiveServer(gorumsSrv, &testSrv{})
	return gorumsSrv
}

// TestUnresponsiveServer checks that the client is not blocked when the server is not receiving messages
func TestUnresponsiveServer(t *testing.T) {
	node := gorumstest.Node(t, serverFn)

	for range 100 {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		_, err := TestUnresponsive(node.Context(ctx), &Empty{})
		if err != nil && errors.Is(err, context.Canceled) {
			t.Error(err)
		}
		cancel()
	}
}
