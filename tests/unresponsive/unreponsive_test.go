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

func (srv testSrv) TestUnresponsiveFF(ctx context.Context, _ *Empty) {
	<-ctx.Done()
}

// TestUnresponsive checks that the client is not blocked when the server is not receiving messages
func TestUnresponsive(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func() interface{} {
		gorumsSrv := gorums.NewServer()
		srv := &testSrv{}
		RegisterUnresponsiveServer(gorumsSrv, srv)
		return gorumsSrv
	})
	defer teardown()

	mgr, err := NewManager(
		gorums.WithNodeList(addrs),
		gorums.WithDialTimeout(100*time.Millisecond),
		gorums.WithGrpcDialOptions(grpc.WithInsecure(), grpc.WithBlock()),
		gorums.WithSendTimeout(1*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]

	for i := 0; i < 1000; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		_, err = node.TestUnresponsive(ctx, &Empty{})
		if err != nil && errors.Is(err, context.Canceled) {
			t.Error(err)
		}
		cancel()
	}

	for i := 0; i < 1000; i++ {
		node.TestUnresponsiveFF(&Empty{})
	}
}