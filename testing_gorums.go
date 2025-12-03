package gorums

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestContext creates a context with timeout for testing.
// It uses t.Context() as the parent and automatically cancels on cleanup.
func TestContext(t testing.TB, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// InsecureGrpcDialOptions returns the default insecure gRPC dial options for testing.
func InsecureGrpcDialOptions(_ testing.TB) ManagerOption {
	return WithGrpcDialOptions(
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// TestConfiguration creates servers and a configuration for testing.
// Both server and manager cleanup are handled via t.Cleanup in the correct order:
// manager is closed first, then servers are stopped.
//
// The provided srvFn is used to create and register the server handlers.
// If srvFn is nil, a default mock server implementation is used.
//
// Optional TestOptions can be provided to customize the manager, server, or configuration.
//
// By default, nodes are assigned sequential IDs (0, 1, 2, ...) matching the server
// creation order. This can be overridden by providing a NodeListOption.
//
// This is the recommended way to set up tests that need both servers and a configuration.
// It ensures proper cleanup and detects goroutine leaks.
func TestConfiguration(t testing.TB, numServers int, srvFn func(i int) ServerIface, opts ...TestOption) Configuration {
	t.Helper()

	testOpts := extractTestOptions(opts)

	// Register goleak check FIRST so it runs LAST (LIFO order)
	// Only register if not reusing an existing manager (to avoid duplicate checks)
	if _, ok := t.(*testing.B); !ok && !testOpts.hasManager() {
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}

	// Start servers and register cleanup
	addrs, stopFn := testSetupServers(t, numServers, testOpts.serverFunc(srvFn))
	t.Cleanup(stopFn)

	// Capture stop function if requested
	if testOpts.stopFuncPtr != nil {
		*testOpts.stopFuncPtr = stopFn
	}

	// Call preConnect hook if set (before connecting to servers)
	if testOpts.preConnectHook != nil {
		testOpts.preConnectHook(stopFn)
	}

	mgr := testOpts.getOrCreateManager(t)
	cfg, err := NewConfiguration(mgr, testOpts.nodeListOption(addrs))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestNode creates a single server and returns the node for testing.
// Both server and manager cleanup are handled via t.Cleanup in the correct order.
//
// The provided srvFn is used to create and register the server handler.
// If srvFn is nil, a default mock server implementation is used.
//
// Optional TestOptions can be provided to customize the manager, server, or configuration.
//
// This is the recommended way to set up tests that need only a single server node.
// It ensures proper cleanup and detects goroutine leaks.
func TestNode(t testing.TB, srvFn func(i int) ServerIface, opts ...TestOption) *Node {
	t.Helper()
	return TestConfiguration(t, 1, srvFn, opts...).Nodes()[0]
}

// TestServers starts numServers gRPC servers using the given registration
// function. Servers are automatically stopped when the test finishes via t.Cleanup.
// The cleanup is registered first, so it runs after any subsequently registered
// cleanups (e.g., manager.Close()), ensuring proper shutdown ordering.
//
// Goroutine leak detection via goleak is automatically enabled and runs after
// all other cleanup functions complete.
//
// The provided srvFn is used to create and register the server handlers.
// If srvFn is nil, a default mock server implementation is used.
//
// Example usage:
//
//	addrs := gorums.TestServers(t, 3, serverFn)
//	mgr := gorums.NewManager(gorums.InsecureGrpcDialOptions(t))
//	t.Cleanup(mgr.Close)
//	...
//
// This function can be used by other packages for testing purposes, as long as
// the required service, method, and message types are registered in the global
// protobuf registry before calling this function.
func TestServers(t testing.TB, numServers int, srvFn func(i int) ServerIface) []string {
	t.Helper()
	// Skip goleak check for benchmarks
	if _, ok := t.(*testing.B); !ok {
		// Register goleak check FIRST so it runs LAST (after all other cleanup)
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}
	addrs, stopFn := testSetupServers(t, numServers, srvFn)
	// Register server cleanup SECOND so it runs BEFORE goleak check
	t.Cleanup(stopFn)
	return addrs
}

// ServerIface is the interface that must be implemented by a server in order to support the TestSetup function.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}

// testSetupServers is the internal implementation of server setup.
// It starts servers and returns addresses and a stop function.
func testSetupServers(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func()) {
	t.Helper()
	servers := make([]ServerIface, numServers)
	listeners := make([]net.Listener, numServers)
	addrs := make([]string, numServers)
	srvStopped := make(chan struct{}, numServers)
	for i := range numServers {
		var srv ServerIface
		if srvFn != nil {
			srv = srvFn(i)
		} else {
			srv = initServer(i)
		}
		// listen on any available port
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to listen on port: %v", err)
		}
		listeners[i] = lis
		addrs[i] = lis.Addr().String()
		servers[i] = srv
		go func() {
			_ = srv.Serve(lis) // blocks until the server is stopped
			srvStopped <- struct{}{}
		}()
	}
	stopFn := sync.OnceFunc(func() {
		for i := range numServers {
			err := listeners[i].Close()
			if err != nil {
				t.Errorf("failed to close listener: %v", err)
			}
			servers[i].Stop()
		}
		// wait for all servers to stop
		for range numServers {
			<-srvStopped
		}
	})
	return addrs, stopFn
}

func initServer(i int) *Server {
	srv := NewServer()
	ts := testSrv{val: int32((i + 1) * 10)}
	srv.RegisterHandler(mock.TestMethod, func(ctx ServerCtx, in *Message) (*Message, error) {
		resp, err := ts.Test(ctx, in.GetProtoMessage())
		return NewResponseMessage(in.GetMetadata(), resp), err
	})
	srv.RegisterHandler(mock.GetValueMethod, func(ctx ServerCtx, in *Message) (*Message, error) {
		resp, err := ts.GetValue(ctx, in.GetProtoMessage())
		return NewResponseMessage(in.GetMetadata(), resp), err
	})
	return srv
}

type testSrv struct {
	val int32
}

func (testSrv) Test(_ ServerCtx, _ proto.Message) (proto.Message, error) {
	return pb.String(""), nil
}

func (ts testSrv) GetValue(_ ServerCtx, _ proto.Message) (proto.Message, error) {
	return pb.Int32(ts.val), nil
}

func echoServerFn(_ int) ServerIface {
	srv := NewServer()
	srv.RegisterHandler(mock.TestMethod, func(ctx ServerCtx, in *Message) (*Message, error) {
		resp, err := echoSrv{}.Test(ctx, in.GetProtoMessage())
		return NewResponseMessage(in.GetMetadata(), resp), err
	})

	return srv
}

// echoSrv implements a simple echo server handler for testing
type echoSrv struct{}

func (echoSrv) Test(_ ServerCtx, req proto.Message) (proto.Message, error) {
	return pb.String("echo: " + mock.GetVal(req)), nil
}
