package unresponsive

import (
	context "context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testSrv struct{}

func (srv testSrv) TestUnresponsive(ctx gorums.ServerCtx, _ *Empty) (resp *Empty, err error) {
	<-ctx.Done()
	return nil, nil
}

// TestUnresponsiveServer checks that the client is not blocked when the server is not receiving messages
func TestUnresponsiveServer(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		gorumsSrv := gorums.NewServer()
		srv := &testSrv{}
		RegisterUnresponsiveServer(gorumsSrv, srv)
		return gorumsSrv
	})
	defer teardown()

	mgr := NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	_, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]

	for range 1000 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_, err = TestUnresponsive(gorums.WithNodeContext(ctx, node.RawNode), &Empty{})
		if err != nil && errors.Is(err, context.Canceled) {
			t.Error(err)
		}
		cancel()
	}
}
