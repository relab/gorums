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

	mgr := gorumsTestMgr()

	_, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]
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
	mgr := gorumsTestMgr()

	_, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	teardown()                         // stop all servers on purpose
	time.Sleep(300 * time.Millisecond) // servers are not stopped immediately
	node := mgr.Nodes()[0]
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

	mgr := gorumsTestMgr()

	_, err := mgr.NewConfiguration(gorums.WithNodeList(addrs))
	if err != nil {
		t.Fatal(err)
	}

	node := mgr.Nodes()[0]
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

func gorumsTestMgr() *dummy.Manager {
	mgr := dummy.NewManager(
		gorums.WithDialTimeout(time.Second),
		gorums.WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	return mgr
}

type testSrv struct{}

func (t testSrv) Test(ctx gorums.ServerCtx, request *dummy.Empty) (response *dummy.Empty, err error) {
	return &dummy.Empty{}, nil
}
