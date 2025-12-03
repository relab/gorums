package unresponsive

import (
	context "context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums"
)

type testSrv struct{}

func (srv testSrv) TestUnresponsive(ctx gorums.ServerCtx, _ *Empty) (resp *Empty, err error) {
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
	node := gorums.TestNode(t, serverFn)

	for range 100 {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		_, err := TestUnresponsive(gorums.WithNodeContext(ctx, node), &Empty{})
		if err != nil && errors.Is(err, context.Canceled) {
			t.Error(err)
		}
		cancel()
	}
}
