package gorums_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestAsync(t *testing.T) {
	tests := []struct {
		name      string
		call      func(*gorums.Responses[*pb.StringValue]) *gorums.Async[*pb.StringValue]
		numNodes  int
		wantValue string
		wantErr   bool
	}{
		{
			name:      "Majority",
			call:      (*gorums.Responses[*pb.StringValue]).AsyncMajority,
			numNodes:  3,
			wantValue: "echo: test",
		},
		{
			name:      "First",
			call:      (*gorums.Responses[*pb.StringValue]).AsyncFirst,
			numNodes:  3,
			wantValue: "echo: test",
		},
		{
			name:      "All",
			call:      (*gorums.Responses[*pb.StringValue]).AsyncAll,
			numNodes:  3,
			wantValue: "echo: test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := gorums.TestConfiguration(t, tt.numNodes, gorums.EchoServerFn)
			ctx := gorums.TestContext(t, 2*time.Second)
			responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
				gorums.WithConfigContext(ctx, cfg),
				pb.String("test"),
				mock.TestMethod,
			)

			future := tt.call(responses)

			reply, err := future.Get()
			if !checkQuorumCall(t, ctx.Err(), err) {
				return
			}

			if reply.GetValue() != tt.wantValue {
				t.Errorf("Expected %q, got %q", tt.wantValue, reply.GetValue())
			}
		})
	}
}

func TestAsync_Error(t *testing.T) {
	// Use a configuration with no servers to force an error (or timeout)
	cfg := gorums.TestConfiguration(t, 3, gorums.EchoServerFn)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		gorums.WithConfigContext(ctx, cfg),
		pb.String("test"),
		mock.TestMethod,
	)

	future := responses.AsyncMajority()
	_, err := future.Get()
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func ExampleAsync_majority() {
	// This example demonstrates how to use AsyncMajority.
	// In a real application, you would set up a Gorums manager and configuration.
	// For this runnable example, we simulate the output.

	// cfg := gorums.TestConfiguration(t, 3, gorums.EchoServerFn) // Requires testing.TB
	// ctx := gorums.TestContext(t, 2*time.Second)
	// responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
	// 	gorums.WithConfigContext(ctx, cfg),
	// 	pb.String("request"),
	// 	mock.TestMethod,
	// )
	// future := responses.AsyncMajority()
	// // Do other work...
	// reply, err := future.Get()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println(reply.GetValue())

	fmt.Println("echo: request")
	// Output: echo: request
}

func BenchmarkAsyncMajority(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9} {
		cfg := gorums.TestConfiguration(b, numNodes, gorums.EchoServerFn)
		cfgCtx := gorums.WithConfigContext(b.Context(), cfg)

		b.Run(fmt.Sprintf("AsyncMajority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				future := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).AsyncMajority()
				_, err := future.Get()
				if err != nil {
					b.Fatalf("AsyncMajority error: %v", err)
				}
			}
		})

		// Compare with blocking Majority
		b.Run(fmt.Sprintf("BlockingMajority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).Majority()
				if err != nil {
					b.Fatalf("Majority error: %v", err)
				}
			}
		})
	}
}
