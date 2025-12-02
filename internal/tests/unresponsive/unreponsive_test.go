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
	// defer goleak.VerifyNone(t)
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		gorumsSrv := gorums.NewServer()
		RegisterUnresponsiveServer(gorumsSrv, &testSrv{})
		return gorumsSrv
	})
	defer teardown()

	mgr := gorums.NewManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := cfg[0]

	for range 100 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_, err = TestUnresponsive(gorums.WithNodeContext(ctx, node), &Empty{})
		if err != nil && errors.Is(err, context.Canceled) {
			t.Error(err)
		}
		cancel()
	}
}
