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

// TestUnresponsive checks that the client is not blocked when the server is not receiving messages
func TestUnresponsive(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		gorumsSrv := gorums.NewServer()
		srv := &testSrv{}
		RegisterUnresponsiveServer(gorumsSrv, srv)
		return gorumsSrv
	})
	defer teardown()

	mgr := NewManager(
		gorums.WithGrpcDialOptions[uint32](
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	_, err := mgr.NewConfiguration(gorums.WithNodeList[uint32](addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]

	for i := 0; i < 1000; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_, err = node.TestUnresponsive(ctx, &Empty{})
		if err != nil && errors.Is(err, context.Canceled) {
			t.Error(err)
		}
		cancel()
	}
}
