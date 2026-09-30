package gorums_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/runtime/gorumsimpl"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestBackChannelNestedCallBeforeRelease verifies that a client handler can
// call the server that sent the request before calling Release, and that a
// second in-flight request does not stop the reply from being read.
func TestBackChannelNestedCallBeforeRelease(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.EchoMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		return gorums.NewResponseMessage(in, pb.String("echo")), nil
	})
	addrs := gorumstest.Servers(t, 1, func(int) gorums.ServerIface { return srv })

	var (
		clientCfg      gorums.Config
		handlers       atomic.Int32
		firstStarted   = make(chan struct{})
		secondInFlight = make(chan struct{})
	)
	clientSrv := gorums.NewServer()
	clientSrv.RegisterHandler(mock.TestMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		if handlers.Add(1) == 1 {
			close(firstStarted)
			<-secondInFlight
		}
		nctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](clientCfg.Nodes()[0].Context(nctx), pb.String("nested"), mock.EchoMethod)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs), gorumstest.DialOptions(t), gorums.WithBackChannel(clientSrv))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorumstest.Closer(t, cfg))
	clientCfg = cfg

	if err := srv.WaitForClients(gorumstest.Context(t, 5*time.Second), func(c gorums.Config) bool { return c.Size() == 1 }); err != nil {
		t.Fatal(err)
	}
	clientNode := srv.ConnectedClients().Nodes()[0]

	errCh := make(chan error, 2)
	call := func() {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_, callErr := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](clientNode.Context(ctx), pb.String("bc"), mock.TestMethod)
		errCh <- callErr
	}
	go call()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first back-channel handler did not start")
	}
	go call()
	// Let the second request reach the client before the first handler calls
	// back, so the reply is not already waiting when that call is made.
	time.Sleep(50 * time.Millisecond)
	close(secondInFlight)

	for range 2 {
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("back-channel call: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("back-channel call did not return")
		}
	}
}

// TestServerHandlerNestedCallBeforeRelease verifies that a server handler can
// call the client that sent the request before calling Release. The reply
// arrives on the same stream the handler's request was read from.
func TestServerHandlerNestedCallBeforeRelease(t *testing.T) {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		clients := ctx.ConnectedClients()
		if clients.Size() == 0 {
			t.Error("ConnectedClients is empty")
			return nil, context.DeadlineExceeded
		}
		nctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		resp, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](clients.Nodes()[0].Context(nctx), pb.String("nested"), mock.EchoMethod)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	addrs := gorumstest.Servers(t, 1, func(int) gorums.ServerIface { return srv })

	clientSrv := gorums.NewServer()
	clientSrv.RegisterHandler(mock.EchoMethod, func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		return gorums.NewResponseMessage(in, pb.String("echo")), nil
	})
	cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs), gorumstest.DialOptions(t), gorums.WithBackChannel(clientSrv))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gorumstest.Closer(t, cfg))
	if err := srv.WaitForClients(gorumstest.Context(t, 5*time.Second), func(c gorums.Config) bool { return c.Size() == 1 }); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := gorumsimpl.RemoteCall[*pb.StringValue, *pb.StringValue](cfg.Nodes()[0].Context(ctx), pb.String("trigger"), mock.TestMethod); err != nil {
		t.Fatalf("server handler nested call: %v", err)
	}
}
