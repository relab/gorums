package gorums_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/dynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/proto"
)

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

func TestRPCCallSuccess(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		dynamic.Register(t)
		return initServer()
	})
	defer teardown()

	mgr := gorumsTestMgr()
	defer mgr.Close()

	node, err := gorums.NewRawNode(addrs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := mgr.Nodes()[0].RPCCall(ctx, gorums.CallData{
		Message: dynamic.NewRequest(""),
		Method:  "mock.Server.Test",
	})
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: non-nil", nil)
	}
}

func TestRPCCallDownedNode(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, func(_ int) gorums.ServerIface {
		dynamic.Register(t)
		return initServer()
	})
	mgr := gorumsTestMgr()
	defer mgr.Close()

	node, err := gorums.NewRawNode(addrs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}

	teardown()                         // stop all servers on purpose
	time.Sleep(300 * time.Millisecond) // servers are not stopped immediately
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := mgr.Nodes()[0].RPCCall(ctx, gorums.CallData{
		Message: dynamic.NewRequest(""),
		Method:  "mock.Server.Test",
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
		dynamic.Register(t)
		return initServer()
	})
	defer teardown()

	mgr := gorumsTestMgr()
	defer mgr.Close()

	node, err := gorums.NewRawNode(addrs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err = mgr.AddNode(node); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 0*time.Second)
	time.Sleep(50 * time.Millisecond)
	defer cancel()
	response, err := mgr.Nodes()[0].RPCCall(ctx, gorums.CallData{
		Message: dynamic.NewRequest(""),
		Method:  "mock.Server.Test",
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
	srv.RegisterHandler("mock.Server.Test", func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[proto.Message](in)
		resp, err := (&testSrv{}).Test(ctx, req)
		return gorums.NewResponseMessage(in.GetMetadata(), resp), err
	})
	return srv
}

func gorumsTestMgr() *gorums.RawManager {
	mgr := gorums.NewRawManager(
		gorums.WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	return mgr
}

type testSrv struct{}

func (t testSrv) Test(ctx gorums.ServerCtx, request proto.Message) (response proto.Message, err error) {
	return dynamic.NewResponse(""), nil
}
