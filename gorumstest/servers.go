package gorumstest

import (
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/servers"
	"go.uber.org/goleak"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// ServerIface is the interface a server must implement to be started by
// [Config], [Node], or [Servers]. [gorums.Server] implements it, as does a
// gRPC server.
type ServerIface = servers.ServerIface

// Servers starts numServers gRPC servers using the given registration
// function. Servers are automatically stopped when the test finishes via t.Cleanup.
// The cleanup is registered first, so it runs after any subsequently registered
// cleanups (e.g., closing a configuration), ensuring proper shutdown ordering.
//
// Goroutine leak detection via goleak is automatically enabled and runs after
// all other cleanup functions complete.
//
// The provided srvFn is used to create and register the server handlers.
// If srvFn is nil, a default mock server implementation is used.
//
// Example usage:
//
//	addrs := gorumstest.Servers(t, 3, serverFn)
//	cfg, err := gorums.NewConfig(gorums.WithNodeList(addrs), gorumstest.DialOptions(t))
//	t.Cleanup(gorumstest.Closer(t, cfg))
//	...
//
// This function can be used by other packages for testing purposes, as long as
// the required service, method, and message types are registered in the global
// protobuf registry before calling this function.
func Servers(t testing.TB, numServers int, srvFn func(i int) ServerIface) []string {
	t.Helper()
	// Skip goleak check for benchmarks
	if _, ok := t.(*testing.B); !ok {
		// Register goleak check FIRST so it runs LAST (after all other cleanup)
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}
	if srvFn == nil {
		srvFn = func(i int) ServerIface { return newDefaultServer(i) }
	}
	addrs, stopFn := servers.Start(t, numServers, srvFn)
	// Register server cleanup SECOND so it runs BEFORE goleak check
	t.Cleanup(func() { stopFn() }) // wrap to call without arguments to stop all servers
	return addrs
}

// LocalServers returns n started Gorums servers forming a symmetric peer
// group (see [gorums.NewLocalServers]), connected in memory in the default
// build and over localhost TCP under the integration build tag. Each
// server auto-creates a peer [gorums.Config] over the group, accessible
// via [gorums.Server.PeerConfig]. The servers are automatically stopped
// when the test finishes via t.Cleanup. Any [gorums.ServerOption]s (for
// example [gorums.WithStreamDedup]) are applied to every server.
func LocalServers(t testing.TB, n int, opts ...gorums.ServerOption) []*gorums.Server {
	t.Helper()

	// Skip goleak check for benchmarks
	if _, ok := t.(*testing.B); !ok {
		// Register goleak check FIRST so it runs LAST (after all other cleanup)
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}

	srvs, stop, err := gorums.NewLocalServers(n,
		gorums.WithLocalServerOptions(opts...),
		gorums.WithLocalDialOptions(DialOptions(t)),
		gorums.WithLocalListeners(servers.Listen(t)),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Register server cleanup SECOND so it runs BEFORE goleak check
	t.Cleanup(stop)

	for _, srv := range srvs {
		go srv.ListenAndServe()
	}

	return srvs
}

// WaitForPeers blocks until every server in srvs is connected to all the
// peers in its [gorums.Server.PeerConfig], or fails the test after 10
// seconds. Use it after [LocalServers] when a test needs every peer
// connection before it issues calls. Under [gorums.WithStreamDedup], it waits
// for the same condition as [gorums.Server.WaitForAll].
func WaitForPeers(t testing.TB, srvs []*gorums.Server) {
	t.Helper()
	ctx := Context(t, 10*time.Second)
	for i, srv := range srvs {
		want := srv.PeerConfig().Size()
		if err := srv.WaitForPeers(ctx, func(cfg gorums.Config) bool {
			return cfg.Size() == want
		}); err != nil {
			t.Fatalf("server %d: WaitForPeers: %v (connected %v, want %v)",
				i+1, err, srv.ConnectedPeers().NodeIDs(), srv.PeerConfig().NodeIDs())
		}
	}
}

// newDefaultServer creates the mock server that [Config], [Node], and
// [Servers] use when their srvFn argument is nil, with the given server
// options.
func newDefaultServer(i int, opts ...gorums.ServerOption) ServerIface {
	srv := gorums.NewServer(opts...)
	ts := testSrv{val: int32((i + 1) * 10)}
	srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		resp, err := ts.Test(ctx, req)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	srv.RegisterHandler(mock.GetValueMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.Int32Value](in)
		resp, err := ts.GetValue(ctx, req)
		if err != nil {
			return nil, err
		}
		return gorums.NewResponseMessage(in, resp), nil
	})
	return srv
}

type testSrv struct {
	val int32
}

func (testSrv) Test(_ gorums.ServerContext, _ *pb.StringValue) (*pb.StringValue, error) {
	return pb.String(""), nil
}

func (ts testSrv) GetValue(_ gorums.ServerContext, _ *pb.Int32Value) (*pb.Int32Value, error) {
	return pb.Int32(ts.val), nil
}

// EchoHandler returns a handler that replies to a string-valued request with
// prefix+": "+value.
func EchoHandler(prefix string) gorums.Handler {
	return func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*pb.StringValue](in)
		return gorums.NewResponseMessage(in, pb.String(prefix+": "+req.GetValue())), nil
	}
}

// EchoServer returns a server that echoes back its request, prefixed with
// "echo: ", suitable for use as the srvFn argument to [Config],
// [Node], or [Servers].
func EchoServer(_ int) ServerIface {
	srv := gorums.NewServer()
	srv.RegisterHandler(mock.TestMethod, EchoHandler("echo"))
	return srv
}

// StreamServer returns a srvFn for [Config], [Node], or [Servers] whose
// servers respond to a request with three echoed responses, each followed by
// delay. A zero delay sends the responses back-to-back.
func StreamServer(delay time.Duration) func(int) ServerIface {
	return func(int) ServerIface {
		srv := gorums.NewServer()
		srv.RegisterHandler(mock.StreamMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*pb.StringValue](in)
			val := req.GetValue()

			// Send 3 responses
			for i := 1; i <= 3; i++ {
				resp := pb.String(fmt.Sprintf("echo: %s-%d", val, i))
				out := gorums.NewResponseMessage(in, resp)
				ctx.SendMessage(out)
				if delay > 0 {
					time.Sleep(delay)
				}
			}
			return nil, nil
		})
		return srv
	}
}
