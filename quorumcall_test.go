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
	// type alias short hand for the responses type
	type respType = *gorums.Responses[*pb.StringValue]
	tests := []struct {
		name      string
		call      func(respType) (*pb.StringValue, error)
		numNodes  int
		wantValue string
		wantErr   bool
	}{
		{
			name:      "Majority",
			call:      respType.Majority,
			numNodes:  3,
			wantValue: "echo: test",
		},
		{
			name:      "First",
			call:      respType.First,
			numNodes:  3,
			wantValue: "echo: test",
		},
		{
			name:      "All",
			call:      respType.All,
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

// BenchmarkQuorumCallTerminalMethods benchmarks the built-in terminal methods with real servers.
func BenchmarkQuorumCallTerminalMethods(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		cfg := gorums.TestConfiguration(b, numNodes, gorums.EchoServerFn)
		cfgCtx := gorums.WithConfigContext(b.Context(), cfg)

		b.Run(fmt.Sprintf("Majority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).Majority()
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("Threshold/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			threshold := numNodes/2 + 1
			for b.Loop() {
				resp, err := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).Threshold(threshold)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("First/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).First()
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("All/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				).All()
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})
	}
}

// BenchmarkQuorumCall benchmarks custom aggregation using different iterator patterns.
func BenchmarkQuorumCall(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		cfg := gorums.TestConfiguration(b, numNodes, gorums.EchoServerFn)
		cfgCtx := gorums.WithConfigContext(b.Context(), cfg)

		// Using CollectAll and then checking quorum
		b.Run(fmt.Sprintf("CollectAllThenCheck/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			quorum := numNodes/2 + 1
			for b.Loop() {
				responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				)
				replies := responses.CollectAll()
				if len(replies) < quorum {
					b.Fatalf("not enough replies: %d < %d", len(replies), quorum)
				}
				// Return first reply
				for _, r := range replies {
					_ = r.GetValue()
					break
				}
			}
		})

		// Using CollectN for early termination
		b.Run(fmt.Sprintf("CollectN/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			quorum := numNodes/2 + 1
			for b.Loop() {
				responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				)
				replies := responses.CollectN(quorum)
				if len(replies) < quorum {
					b.Fatalf("not enough replies: %d < %d", len(replies), quorum)
				}
				// Return first reply
				for _, r := range replies {
					_ = r.GetValue()
					break
				}
			}
		})

		// Using manual iteration with early return
		b.Run(fmt.Sprintf("Iterator/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			quorum := numNodes/2 + 1
			for b.Loop() {
				responses := gorums.QuorumCall[*pb.StringValue, *pb.StringValue](
					cfgCtx,
					pb.String("test"),
					mock.TestMethod,
				)
				var firstResp *pb.StringValue
				var count int
				for result := range responses.Seq() {
					if result.Err == nil {
						count++
						if firstResp == nil {
							firstResp = result.Value
						}
						if count >= quorum {
							break
						}
					}
				}
				if count < quorum {
					b.Fatalf("not enough responses: %d < %d", count, quorum)
				}
				_ = firstResp.GetValue()
			}
		})
	}
}
