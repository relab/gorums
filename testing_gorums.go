package gorums

import (
	"net"
	"sync"
	"testing"

	"github.com/relab/gorums/internal/testutils/mock"
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

// NewConfig creates a configuration for the given node addresses and adds it to a new manager.
// The manager is automatically closed when the test finishes.
func NewConfig(t testing.TB, addrs []string, opts ...ManagerOption) RawConfiguration {
	t.Helper()
	mgrOpts := []ManagerOption{
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	}
	mgrOpts = append(mgrOpts, opts...)
	mgr := NewRawManager(mgrOpts...)
	t.Cleanup(mgr.Close)

	for _, addr := range addrs {
		node, err := NewRawNode(addr)
		if err != nil {
			t.Fatalf("Failed to create node: %v", err)
		}
		if err = mgr.AddNode(node); err != nil {
			t.Fatalf("Failed to add node: %v", err)
		}
	}
	cfg, err := NewRawConfiguration(mgr, WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
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
func TestSetup(t testing.TB, numServers int, srvFn func(i int) ServerIface) ([]string, func()) {
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
