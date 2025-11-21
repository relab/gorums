package gorums_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/dynamic"
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

	node := gorums.NewNode(t, addrs[0])

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: dynamic.NewRequest(""),
		Method:  dynamic.MockServerMethodName,
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
	node := gorums.NewNode(t, addrs[0])

	teardown()                         // stop all servers on purpose
	time.Sleep(300 * time.Millisecond) // servers are not stopped immediately
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: dynamic.NewRequest(""),
		Method:  dynamic.MockServerMethodName,
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

	node := gorums.NewNode(t, addrs[0])

	ctx, cancel := context.WithTimeout(context.Background(), 0*time.Second)
	time.Sleep(50 * time.Millisecond)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: dynamic.NewRequest(""),
		Method:  dynamic.MockServerMethodName,
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
	srv.RegisterHandler(dynamic.MockServerMethodName, func(ctx gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[proto.Message](in)
		resp, err := (&testSrv{}).Test(ctx, req)
		return gorums.NewResponseMessage(in.GetMetadata(), resp), err
	})
	return srv
}

type testSrv struct{}

func (t testSrv) Test(ctx gorums.ServerCtx, request proto.Message) (response proto.Message, err error) {
	return dynamic.NewResponse(""), nil
}
