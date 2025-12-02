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
	node := gorums.SetupNode(t, nil)

	ctx := gorums.TestContext(t, 5*time.Second)
	nodeCtx := gorums.WithNodeContext(ctx, node)
	response, err := gorums.RPCCall(nodeCtx, pb.String(""), mock.TestMethod)
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: non-nil", nil)
	}
}

func TestRPCCallDownedNode(t *testing.T) {
	// This test needs TestSetup since it deliberately stops servers early
	addrs, teardown := gorums.TestSetup(t, 1, nil)
	node := gorums.NewTestNode(t, addrs[0])

	teardown()                         // stop all servers on purpose
	time.Sleep(300 * time.Millisecond) // servers are not stopped immediately

	ctx := gorums.TestContext(t, 5*time.Second)
	nodeCtx := gorums.WithNodeContext(ctx, node)
	response, err := gorums.RPCCall(nodeCtx, pb.String(""), mock.TestMethod)
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("rpc error: code = Unavailable desc = stream is down"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRPCCallTimedOut(t *testing.T) {
	node := gorums.SetupNode(t, nil)

	ctx, cancel := context.WithTimeout(t.Context(), 0*time.Second)
	time.Sleep(50 * time.Millisecond)
	defer cancel()
	nodeCtx := gorums.WithNodeContext(ctx, node)
	response, err := gorums.RPCCall(nodeCtx, pb.String(""), mock.TestMethod)
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("context deadline exceeded"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}
