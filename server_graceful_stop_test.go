package gorums_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	"go.uber.org/goleak"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
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

// TestGracefulStopWithOpenClientStream verifies that GracefulStop returns
// while a client stream is open, including when the client keeps calling
// during the drain.
func TestGracefulStopWithOpenClientStream(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		return gorums.NewResponseMessage(in, pb.String("echo")), nil
	})
	cfg := gorumstest.Config(t, 1, func(int) gorums.ServerIface { return srv })
	node := cfg.Nodes()[0]

	call := func() error {
		ctx := gorumstest.Context(t, 2*time.Second)
		_, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](node.Context(ctx), pb.String("x"), mock.TestMethod)
		return err
	}
	if err := call(); err != nil {
		t.Fatalf("call before GracefulStop: %v", err)
	}

	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	// The client keeps the stream open; calls during the drain may fail, but
	// must not keep GracefulStop from returning.
	_ = call()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("GracefulStop did not return with a client stream open")
	}
}
