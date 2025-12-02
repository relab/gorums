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

// ServerIface is the interface that must be implemented by a server in order to support the TestSetup function.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}

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

// NewTestNode creates a node for the given server address and adds it to a new manager.
// The manager is automatically closed when the test finishes.
func NewTestNode(t testing.TB, srvAddr string, opts ...ManagerOption) *Node {
	t.Helper()
	mgrOpts := append([]ManagerOption{InsecureGrpcDialOptions(t)}, opts...)
	mgr := NewManager(mgrOpts...)
	t.Cleanup(mgr.Close)
	node, err := NewNode(srvAddr)
	if err != nil {
		t.Fatal(err)
	}
	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}
	return node
}

// TestSetup starts numServers gRPC servers using the given registration
// function, and returns the server addresses along with a stop function
// that should be called to shut down the test. The stop function will block
// until all servers have stopped. The provided srvFn is used to register
// the server handlers. The service, method, and message types must be registered
// in the global protobuf registry before calling this function. See TestMain
// for an example of how to register the required information. If srvFn is nil,
// a default mock server implementation is used.
//
// This function can be used by other packages for testing purposes, as long as
// the required types are registered in the global protobuf registry.
//
// IMPORTANT: To avoid goroutine leaks, ensure the manager is closed before
// calling the stop function. The recommended pattern is:
//
//	addrs, stopServers := gorums.TestSetup(t, 3, serverFn)
//	mgr := gorums.NewManager(...)
//	defer func() {
//		mgr.Close()
//		stopServers()
//	}()
//
// Or use [TestServers] which handles cleanup ordering via t.Cleanup.
func TestSetup(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func()) {
	t.Helper()
	return testSetupServers(t, numServers, srvFn)
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
// This function can be used by other packages for testing purposes, as long as
// the required types are registered in the global protobuf registry.
func TestServers(t testing.TB, numServers int, srvFn func(i int) ServerIface) []string {
	t.Helper()
	if _, ok := t.(*testing.B); !ok {
		// Skip goleak check for benchmarks
		// Register goleak check FIRST so it runs LAST (after all other cleanup)
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}
	addrs, stopFn := testSetupServers(t, numServers, srvFn)
	// Register server cleanup SECOND so it runs BEFORE goleak check
	t.Cleanup(stopFn)
	return addrs
}

// SetupConfiguration creates servers and a configuration for testing.
// Both server and manager cleanup are handled via t.Cleanup in the correct order:
// manager is closed first, then servers are stopped.
//
// The provided srvFn is used to create and register the server handlers.
// If srvFn is nil, a default mock server implementation is used.
//
// Optional ManagerOptions can be provided to customize the manager.
//
// This is the recommended way to set up tests that need both servers and a configuration.
// It ensures proper cleanup and detects goroutine leaks.
func SetupConfiguration(t testing.TB, numServers int, srvFn func(i int) ServerIface, opts ...ManagerOption) Configuration {
	t.Helper()

	// Register goleak check FIRST so it runs LAST (LIFO order)
	if _, ok := t.(*testing.B); !ok {
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}

	// Start servers and register cleanup
	addrs, stopFn := testSetupServers(t, numServers, srvFn)
	t.Cleanup(stopFn)

	// Check if preConnect hook is set and call it before connecting
	var tempOpts managerOptions
	for _, opt := range opts {
		opt(&tempOpts)
	}
	if tempOpts.preConnect != nil {
		tempOpts.preConnect(stopFn)
	}

	// Create manager and register its cleanup LAST so it runs FIRST (LIFO)
	mgrOpts := append([]ManagerOption{InsecureGrpcDialOptions(t)}, opts...)
	mgr := NewManager(mgrOpts...)
	t.Cleanup(mgr.Close)

	for _, addr := range addrs {
		node, err := NewNode(addr)
		if err != nil {
			t.Fatalf("Failed to create node: %v", err)
		}
		if err = mgr.AddNode(node); err != nil {
			t.Fatalf("Failed to add node: %v", err)
		}
	}

	cfg, err := NewConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// SetupNode creates a single server and returns the node for testing.
// Both server and manager cleanup are handled via t.Cleanup in the correct order.
//
// The provided srvFn is used to create and register the server handler.
// If srvFn is nil, a default mock server implementation is used.
//
// Optional ManagerOptions can be provided to customize the manager.
//
// This is the recommended way to set up tests that need only a single server node.
// It ensures proper cleanup and detects goroutine leaks.
func SetupNode(t testing.TB, srvFn func(i int) ServerIface, opts ...ManagerOption) *Node {
	t.Helper()
	return SetupConfiguration(t, 1, srvFn, opts...).Nodes()[0]
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
