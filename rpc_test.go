package gorums_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/tests/dummy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRPCCallSuccess(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		return initServer()
	})
	defer teardown()

	cfg, err := dummy.NewConfiguration(nil, gorums.WithNodeList(addrs), gorumsTestMgrOpts()...)
	if err != nil {
		t.Fatal(err)
	}

	node := cfg.Nodes()[0]
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: &dummy.Empty{},
		Method:  "dummy.Dummy.Test",
	})
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", nil, &dummy.Empty{})
	}
}

func TestRPCCallDownedNode(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		return initServer()
	})

	cfg, err := dummy.NewConfiguration(nil, gorums.WithNodeList(addrs), gorumsTestMgrOpts()...)
	if err != nil {
		t.Fatal(err)
	}

	teardown()                         // stop all servers on purpose
	time.Sleep(300 * time.Millisecond) // servers are not stopped immediately
	node := cfg.Nodes()[0]
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: &dummy.Empty{},
		Method:  "dummy.Dummy.Test",
	})
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("rpc error: code = Unavailable desc = stream is down"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRPCCallTimedOut(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		return initServer()
	})
	defer teardown()

	cfg, err := dummy.NewConfiguration(nil, gorums.WithNodeList(addrs), gorumsTestMgrOpts()...)
	if err != nil {
		t.Fatal(err)
	}

	node := cfg.Nodes()[0]
	ctx, cancel := context.WithTimeout(context.Background(), 0*time.Second)
	time.Sleep(50 * time.Millisecond)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: &dummy.Empty{},
		Method:  "dummy.Dummy.Test",
	})
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("context deadline exceeded"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func initServer() *gorums.Server {
	srv := gorums.NewServer()
	dummy.RegisterDummyServer(srv, &testSrv{})
	return srv
}

func gorumsTestMgrOpts() []gorums.ManagerOption {
	opts := []gorums.ManagerOption{gorums.WithGrpcDialOptions(
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)}
	return opts
}

type testSrv struct{}

func (t testSrv) Test(ctx gorums.ServerCtx, request *dummy.Empty) (response *dummy.Empty, err error) {
	return &dummy.Empty{}, nil
}
