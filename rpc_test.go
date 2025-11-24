package gorums_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/encoding"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

func TestRPCCallSuccess(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, nil)
	defer teardown()

	node := gorums.NewNode(t, addrs[0])

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: pb.String(""),
		Method:  mock.TestMethod,
	})
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: non-nil", nil)
	}
}

func TestRPCCallDownedNode(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, nil)
	node := gorums.NewNode(t, addrs[0])

	teardown()                         // stop all servers on purpose
	time.Sleep(300 * time.Millisecond) // servers are not stopped immediately
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: pb.String(""),
		Method:  mock.TestMethod,
	})
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("rpc error: code = Unavailable desc = stream is down"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRPCCallTimedOut(t *testing.T) {
	addrs, teardown := gorums.TestSetup(t, 1, nil)
	defer teardown()

	node := gorums.NewNode(t, addrs[0])

	ctx, cancel := context.WithTimeout(context.Background(), 0*time.Second)
	time.Sleep(50 * time.Millisecond)
	defer cancel()
	response, err := node.RPCCall(ctx, gorums.CallData{
		Message: pb.String(""),
		Method:  mock.TestMethod,
	})
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("context deadline exceeded"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}
