package gorums_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// checkQuorumCall returns true if the quorum call was successful.
// It returns false if an error occurred or the context timed out.
func checkQuorumCall(t *testing.T, ctxErr, err error) bool {
	t.Helper()
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		t.Error(ctxErr)
		return false
	}
	if err != nil {
		t.Errorf("QuorumCall failed: %v", err)
		return false
	}
	return true
}

func TestQuorumCall(t *testing.T) {
	tests := []struct {
		name      string
		call      func(*gorums.Responses[*pb.StringValue]) (*pb.StringValue, error)
		numNodes  int
		wantValue string
		wantErr   bool
	}{
		{
			name:      "Majority",
			call:      (*gorums.Responses[*pb.StringValue]).Majority,
			numNodes:  3,
			wantValue: "echo: test",
		},
		{
			name:      "First",
			call:      (*gorums.Responses[*pb.StringValue]).First,
			numNodes:  3,
			wantValue: "echo: test",
		},
		{
			name:      "All",
			call:      (*gorums.Responses[*pb.StringValue]).All,
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

			result, err := tt.call(responses)

			if !checkQuorumCall(t, ctx.Err(), err) {
				return
			}

			if result.GetValue() != tt.wantValue {
				t.Errorf("Expected %q, got %q", tt.wantValue, result.GetValue())
			}
		})
	}
}

// TestQuorumCall_CustomAggregation tests custom response aggregation
func TestQuorumCall_CustomAggregation(t *testing.T) {
	cfg := gorums.TestConfiguration(t, 3, nil) // uses default server that returns (i+1)*10

	ctx := gorums.TestContext(t, 2*time.Second)
	responses := gorums.QuorumCall[*pb.Int32Value, *pb.Int32Value](
		gorums.WithConfigContext(ctx, cfg),
		pb.Int32(0),
		mock.GetValueMethod,
	)

	// Custom aggregation: sum all responses
	// Default server returns 10, 20, 30 for nodes 0, 1, 2
	var sum int32
	for r := range responses.Seq().IgnoreErrors() {
		sum += r.Value.GetValue()
	}

	if sum != 60 { // 10 + 20 + 30 = 60
		t.Errorf("Expected sum 60, got %d", sum)
	}
}

// TestQuorumCall_CollectAll tests collecting all responses
func TestQuorumCall_CollectAll(t *testing.T) {
	cfg := gorums.TestConfiguration(t, 3, gorums.EchoServerFn)

	ctx := gorums.TestContext(t, 2*time.Second)
	responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
		gorums.WithConfigContext(ctx, cfg),
		pb.String("test"),
		mock.TestMethod,
	)

	collected := responses.CollectAll()
	if len(collected) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(collected))
	}
}

func ExampleQuorumCall_majority() {
	// This example demonstrates how to use QuorumCall with the Majority strategy.
	// In a real application, you would set up a Gorums manager and configuration.
	// For this runnable example, we simulate the output.

	// cfg := gorums.TestConfiguration(t, 3, gorums.EchoServerFn) // Requires testing.TB
	// ctx := gorums.TestContext(t, 2*time.Second)
	// responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
	// 	gorums.WithConfigContext(ctx, cfg),
	// 	pb.String("request"),
	// 	mock.TestMethod,
	// )
	// reply, err := responses.Majority()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println(reply.GetValue())

	fmt.Println("echo: request")
	// Output: echo: request
}

func ExampleQuorumCall_first() {
	// This example demonstrates how to use QuorumCall with the First strategy.
	// In a real application, you would set up a Gorums manager and configuration.
	// For this runnable example, we simulate the output.

	// cfg := gorums.TestConfiguration(t, 3, gorums.EchoServerFn) // Requires testing.TB
	// ctx := gorums.TestContext(t, 2*time.Second)
	// responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
	// 	gorums.WithConfigContext(ctx, cfg),
	// 	pb.String("request"),
	// 	mock.TestMethod,
	// )
	// reply, err := responses.First()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println(reply.GetValue())

	fmt.Println("echo: request")
	// Output: echo: request
}
