package gorums_test

import (
	"context"
	"fmt"
	"sync"
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
	node := gorums.TestNode(t, gorums.DefaultTestServer)

	ctx := gorums.TestContext(t, 5*time.Second)
	nodeCtx := node.Context(ctx)
	response, err := gorums.RPCCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String(""), mock.TestMethod)
	if err != nil {
		t.Fatalf("Unexpected error, got: %v, want: %v", err, nil)
	}
	if response == nil {
		t.Fatalf("Unexpected response, got: %v, want: non-nil", nil)
	}
}

func TestRPCCallDownedNode(t *testing.T) {
	node := gorums.TestNode(t, gorums.DefaultTestServer, gorums.WithPreConnect(t, func(stopServers func()) {
		stopServers()
		time.Sleep(300 * time.Millisecond) // wait for servers to fully stop
	}))

	ctx := gorums.TestContext(t, 5*time.Second)
	nodeCtx := node.Context(ctx)
	response, err := gorums.RPCCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String(""), mock.TestMethod)
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("rpc error: code = Unavailable desc = stream is down"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRPCCallTimedOut(t *testing.T) {
	node := gorums.TestNode(t, gorums.DefaultTestServer)

	ctx, cancel := context.WithTimeout(t.Context(), 0*time.Second)
	time.Sleep(50 * time.Millisecond)
	defer cancel()
	nodeCtx := node.Context(ctx)
	response, err := gorums.RPCCall[*pb.StringValue, *pb.StringValue](nodeCtx, pb.String(""), mock.TestMethod)
	if err == nil {
		t.Fatalf("Expected error, got: %v, want: %v", err, fmt.Errorf("context deadline exceeded"))
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRPCCallTypeMismatch(t *testing.T) {
	node := gorums.TestNode(t, gorums.DefaultTestServer)

	ctx := gorums.TestContext(t, 5*time.Second)
	nodeCtx := node.Context(ctx)
	response, err := gorums.RPCCall[*pb.StringValue, *pb.Int32Value](nodeCtx, pb.String(""), mock.TestMethod)
	if err != gorums.ErrTypeMismatch {
		t.Fatalf("Expected error, got: %v, want: %v", err, gorums.ErrTypeMismatch)
	}
	if response != nil {
		t.Fatalf("Unexpected response, got: %v, want: %v", response, nil)
	}
}

func TestRPCCallConcurrentAccess(t *testing.T) {
	node := gorums.TestNode(t, gorums.DefaultTestServer)

	concurrency := 10
	errCh := make(chan error, concurrency)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() {
			_, err := gorums.RPCCall[*pb.StringValue, *pb.StringValue](node.Context(t.Context()), pb.String(""), mock.TestMethod)
			if err != nil {
				errCh <- err
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}
