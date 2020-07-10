package reconnect

import (
	context "context"
	"testing"
	"time"

	"github.com/relab/gorums"
	grpc "google.golang.org/grpc"
)

type testSrv struct{}

func (t testSrv) Test(_ context.Context, in *Echo, out func(*Echo, error)) {
	out(in, nil)
}

func TestReconnect(t *testing.T) {
	want := "Hello"

	addrs, teardown := gorums.TestSetup(t, 1, func() interface{} {
		srv := NewGorumsServer()
		srv.RegisterReconnectServer(&testSrv{})
		return srv
	})
	defer teardown()

	mgr, err := NewManager(WithNodeList(addrs), WithDialTimeout(100*time.Millisecond), WithGrpcDialOptions(
		grpc.WithBlock(),
		grpc.WithInsecure(),
	))

	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	node := mgr.Nodes()[0]

	testRPC := func() {
		resp, err := node.Test(context.Background(), &Echo{Value: want})
		if err != nil {
			t.Fatalf("RPC error: %v", err)
		}

		if resp.GetValue() != want {
			t.Fatalf("Test() == %s, want %s", resp.GetValue(), want)
		}
	}

	testRPC()

	// cause the stream to close
	_ = node.gorumsStream.CloseSend()

	// wait for stream to reconnect
	time.Sleep(100 * time.Millisecond)

	// Try again
	testRPC()
}
