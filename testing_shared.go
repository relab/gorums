package gorums

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net"
	"slices"
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

// InsecureDialOptions returns the default insecure gRPC dial options for testing.
func InsecureDialOptions(_ testing.TB) ManagerOption {
	return WithDialOptions(
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// TestQuorumCallError creates a QuorumCallError for testing.
// The nodeErrors map contains node IDs and their corresponding errors.
func TestQuorumCallError(_ testing.TB, nodeErrors map[uint32]error) QuorumCallError {
	errs := make([]nodeError, 0, len(nodeErrors))
	for nodeID, err := range nodeErrors {
		errs = append(errs, nodeError{cause: err, nodeID: nodeID})
	}
	return QuorumCallError{cause: ErrIncomplete, errors: errs}
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
	// and if goleak checks are not explicitly skipped
	if _, ok := t.(*testing.B); !ok && !testOpts.shouldSkipGoleak() {
		t.Cleanup(func() { goleak.VerifyNone(t) })
	}

	// Start servers and register cleanup - implementation varies by build tag
	addrs, stopFn := testSetupServers(t, numServers, testOpts.serverFunc(srvFn))
	stopAllFn := func() { stopFn() } // wrap to call without arguments to stop all servers
	t.Cleanup(stopAllFn)

	// Capture the provided stop function to stop individual servers later
	if testOpts.stopFuncPtr != nil {
		*testOpts.stopFuncPtr = stopFn
	}

	// Call preConnect hook if set (before connecting to servers)
	if testOpts.preConnectHook != nil {
		testOpts.preConnectHook(stopAllFn)
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
//	mgr := gorums.NewManager(gorums.InsecureDialOptions(t))
//	t.Cleanup(gorums.Closer(t, mgr))
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
	t.Cleanup(func() { stopFn() }) // wrap to call without arguments to stop all servers
	return addrs
}

// ServerIface is the interface that must be implemented by a server in order to support the TestSetup function.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}

// Closer returns a cleanup function that closes the given io.Closer.
func Closer(t testing.TB, c io.Closer) func() {
	t.Helper()
	return func() {
		if err := c.Close(); err != nil {
			t.Errorf("c.Close() = %q, expected no error", err.Error())
		}
	}
}

type serverState struct {
	srv     ServerIface
	lis     net.Listener
	stopped chan struct{}
}

func (s *serverState) start(_ testing.TB) {
	_ = s.srv.Serve(s.lis)
	s.stopped <- struct{}{}
}

func (s *serverState) stop(t testing.TB) {
	t.Helper()
	if err := s.lis.Close(); err != nil {
		t.Errorf("Failed to close listener: %v", err)
	}
	s.srv.Stop()
	<-s.stopped
}

// setupServers is the internal implementation of server setup.
// It starts servers and returns addresses and a variadic stop function.
// The stop function should be called with the indices of servers to stop,
// or with no arguments to stop all servers.
func setupServers(t testing.TB, numServers int, srvFn func(i int) ServerIface, listenFn func(i int) net.Listener) ([]string, func(...int)) {
	t.Helper()

	addrs := make([]string, numServers)
	muActive := &sync.Mutex{}
	active := make(map[int]*serverState)

	for i := range numServers {
		var srv ServerIface
		if srvFn != nil {
			srv = srvFn(i)
		} else {
			srv = initServer(i)
		}
		lis := listenFn(i)

		addrs[i] = lis.Addr().String()
		state := &serverState{srv: srv, lis: lis, stopped: make(chan struct{})}
		muActive.Lock()
		active[i] = state
		muActive.Unlock()

		go state.start(t)
	}

	stopNodesFn := func(indices ...int) {
		if len(indices) == 0 {
			// Stop all active servers
			indices = slices.Collect(Range(numServers))
		}
		// Stop specific servers
		toStop := make([]*serverState, 0, len(indices))
		muActive.Lock()
		for _, idx := range indices {
			if state, ok := active[idx]; ok {
				delete(active, idx)
				toStop = append(toStop, state)
			}
		}
		muActive.Unlock()

		// Stop and wait for each server
		for _, state := range toStop {
			state.stop(t)
		}
	}
	return addrs, stopNodesFn
}

func Range(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
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

func EchoServerFn(_ int) ServerIface {
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

func StreamServerFn(_ int) ServerIface {
	srv := NewServer()
	srv.RegisterHandler(mock.Stream, func(ctx ServerCtx, in *Message) (*Message, error) {
		req := in.GetProtoMessage()
		val := mock.GetVal(req)

		// Send 3 responses
		for i := 1; i <= 3; i++ {
			resp := pb.String(fmt.Sprintf("echo: %s-%d", val, i))
			msg := NewResponseMessage(in.GetMetadata(), resp)
			if err := ctx.SendMessage(msg); err != nil {
				return nil, err
			}
			time.Sleep(10 * time.Millisecond)
		}
		return nil, nil
	})
	return srv
}

func StreamBenchmarkServerFn(_ int) ServerIface {
	srv := NewServer()
	srv.RegisterHandler(mock.Stream, func(ctx ServerCtx, in *Message) (*Message, error) {
		req := in.GetProtoMessage()
		val := mock.GetVal(req)

		// Send 3 responses
		for i := 1; i <= 3; i++ {
			resp := pb.String(fmt.Sprintf("echo: %s-%d", val, i))
			msg := NewResponseMessage(in.GetMetadata(), resp)
			if err := ctx.SendMessage(msg); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	return srv
}
