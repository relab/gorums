package unresponsive

import (
	context "context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums"
	"google.golang.org/grpc"
)

type testSrv struct{}

func (srv testSrv) TestUnresponsive(ctx context.Context, _ *Empty, _ func(*Empty, error)) {
	<-ctx.Done()
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
		gorums.WithDialTimeout(100*time.Millisecond),
		gorums.WithGrpcDialOptions(grpc.WithInsecure(), grpc.WithBlock()),
	)
	_, err := mgr.NewConfiguration(nil, gorums.WithNodeList(addrs))
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
