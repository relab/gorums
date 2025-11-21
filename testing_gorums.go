package gorums

import (
	"net"
	"sync"
	"testing"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

// ServerIface is the interface that must be implemented by a server in order to support the TestSetup function.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}

// NewNode creates a node for the given server address and adds it to a new manager.
// The manager is automatically closed when the test finishes.
func NewNode(t testing.TB, srvAddr string, opts ...ManagerOption) *RawNode {
	t.Helper()
	mgrOpts := []ManagerOption{
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	}
	mgrOpts = append(mgrOpts, opts...)
	mgr := NewRawManager(mgrOpts...)
	t.Cleanup(mgr.Close)
	node, err := NewRawNode(srvAddr)
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
// until all servers have stopped.
// This function can be used by other packages for testing purposes.
func TestSetup(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func()) {
	t.Helper()
	servers := make([]ServerIface, numServers)
	listeners := make([]net.Listener, numServers)
	addrs := make([]string, numServers)
	srvStopped := make(chan struct{}, numServers)
	// Register mock types in the global protobuf registry for default server implementation.
	// This is called even when a custom srvFn is provided to ensure mock types are available for tests.
	// Note: this uses global state and may have implications for concurrent test execution.
	mock.Register(t)
	for i := range numServers {
		var srv ServerIface
		if srvFn != nil {
			srv = srvFn(i)
		} else {
			srv = initServer()
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

func initServer() *Server {
	srv := NewServer()
	srv.RegisterHandler(mock.ServerMethodName, func(ctx ServerCtx, in *Message) (*Message, error) {
		resp, err := (&testSrv{}).Test(ctx, in.GetProtoMessage())
		return NewResponseMessage(in.GetMetadata(), resp), err
	})
	return srv
}

type testSrv struct{}

func (testSrv) Test(_ ServerCtx, _ proto.Message) (proto.Message, error) {
	return mock.NewResponse(""), nil
}
