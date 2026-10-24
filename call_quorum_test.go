package gorums_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mockcall"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// checkQuorumCall validates a quorum call's outcome against expectations.
// If wantErr is nil, it expects success and verifies no error occurred.
// If wantErr is non-nil, it validates that the error matches and optionally checks
// the number of node errors in QuorumCallError (if expectedNodeErrors is provided).
func checkQuorumCall(t *testing.T, gotErr, wantErr error, expectedNodeErrors ...int) bool {
	t.Helper()

	// Handle expected error case
	if wantErr != nil {
		if gotErr == nil {
			t.Errorf("Expected error %v, got nil", wantErr)
			return false
		}
		if !errors.Is(gotErr, wantErr) {
			t.Errorf("Expected error %v, got %v", wantErr, gotErr)
			return false
		}
		// Validate QuorumCallError details if expectedNodeErrors provided
		if len(expectedNodeErrors) > 0 {
			var qcErr gorums.QuorumCallError
			if errors.As(gotErr, &qcErr) && qcErr.NumErrors() != expectedNodeErrors[0] {
				t.Errorf("Expected %d node errors, got %d", expectedNodeErrors[0], qcErr.NumErrors())
				return false
			}
		}
		return true
	}

	// Handle expected success case
	if gotErr != nil {
		t.Errorf("QuorumCall failed: %v", gotErr)
		return false
	}
	return true
}

func TestQuorumCall(t *testing.T) {
	// type alias short hand for the responses type
	type respType = *gorums.Call[*pb.StringValue, *pb.StringValue]
	tests := []struct {
		name      string
		call      func(respType) (*pb.StringValue, error)
		numNodes  int
		wantValue string
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
			config := gorumstest.Config(t, tt.numNodes, gorumstest.EchoServer)
			ctx := gorumstest.Context(t, 2*time.Second)
			responses := mockcall.QuorumCall(ctx, config, "test")

			result, err := tt.call(responses)
			if !checkQuorumCall(t, err, nil) {
				return
			}

			if result.GetValue() != tt.wantValue {
				t.Errorf("Expected %q, got %q", tt.wantValue, result.GetValue())
			}
		})
	}
}

func TestQuorumCallPartialFailures(t *testing.T) {
	// Function type for the call to test
	type callInfo struct {
		name     string
		callFunc func(context.Context, gorums.Config, *pb.StringValue) error
	}

	const numServers = 3

	type respType = *gorums.Call[*pb.StringValue, *pb.StringValue]

	// Helper to create QuorumCall variants
	quorumcall := func(name string, aggregateFunc func(respType) (*pb.StringValue, error)) callInfo {
		return callInfo{
			name: "QuorumCall/" + name,
			callFunc: func(ctx context.Context, config gorums.Config, req *pb.StringValue) error {
				_, err := aggregateFunc(mockcall.QuorumCall(ctx, config, req.GetValue()))
				return err
			},
		}
	}

	// Helper to create a Multicast variant; Wait returns the send error.
	multicast := func(name string) callInfo {
		return callInfo{
			name: "Multicast/" + name,
			callFunc: func(ctx context.Context, config gorums.Config, req *pb.StringValue) error {
				return mockcall.Multicast(ctx, config, req.GetValue()).Send()
			},
		}
	}

	tests := []struct {
		call    callInfo
		failing int // number of servers to stop (0 to 3)
		wantErr error
	}{
		// Multicast Wait: Fails if ANY node fails
		{multicast("Wait"), 0, nil},
		{multicast("Wait"), 1, gorums.ErrSendFailure},
		{multicast("Wait"), 2, gorums.ErrSendFailure},
		{multicast("Wait"), 3, gorums.ErrSendFailure},

		// QuorumCall Majority (2/3): Tolerates 1 failure
		{quorumcall("Majority", respType.Majority), 0, nil},
		{quorumcall("Majority", respType.Majority), 1, nil},
		{quorumcall("Majority", respType.Majority), 2, gorums.ErrIncomplete},
		{quorumcall("Majority", respType.Majority), 3, gorums.ErrIncomplete},

		// QuorumCall First (1/3): Tolerates 2 failures
		{quorumcall("First", respType.First), 0, nil},
		{quorumcall("First", respType.First), 1, nil},
		{quorumcall("First", respType.First), 2, nil},
		{quorumcall("First", respType.First), 3, gorums.ErrIncomplete},

		// QuorumCall All (3/3): Fails if any node fails
		{quorumcall("All", respType.All), 0, nil},
		{quorumcall("All", respType.All), 1, gorums.ErrIncomplete},
		{quorumcall("All", respType.All), 2, gorums.ErrIncomplete},
		{quorumcall("All", respType.All), 3, gorums.ErrIncomplete},
	}
	for _, tt := range tests {
		testName := fmt.Sprintf("%s/fail=%d", tt.call.name, tt.failing)
		t.Run(testName, func(t *testing.T) {
			var stopNodes func(...int)
			config := gorumstest.Config(t, numServers, nil, gorumstest.WithStopFunc(t, &stopNodes))
			ctx := t.Context()
			req := pb.String("test")

			// Warmup to ensure connections
			if err := tt.call.callFunc(ctx, config, req); err != nil {
				t.Fatalf("Warmup failed: %v", err)
			}

			if tt.failing > 0 {
				// Stop the first tt.failing servers
				indices := make([]int, tt.failing)
				for i := range indices {
					indices[i] = i
				}
				stopNodes(indices...)
				time.Sleep(50 * time.Millisecond)
			}

			// Execute test calls after stopping nodes, if any
			var err error
			for range 5 {
				err = tt.call.callFunc(ctx, config, req)
				if tt.wantErr != nil && err != nil {
					break // failed as expected
				}
				if tt.wantErr == nil && err != nil {
					break // failed unexpected
				}
				time.Sleep(10 * time.Millisecond)
			}

			// Validate error expectations using helper
			if !checkQuorumCall(t, err, tt.wantErr, tt.failing) {
				return
			}
		})
	}
}

// TestQuorumCallCustomAggregation tests custom response aggregation
func TestQuorumCallCustomAggregation(t *testing.T) {
	config := gorumstest.Config(t, 3, nil) // uses default server that returns (i+1)*10

	ctx := gorumstest.Context(t, 2*time.Second)
	responses := mockcall.Int32QuorumCall(ctx, config, 0)

	// Custom aggregation: sum all responses
	// Default server returns 10, 20, 30 for nodes 0, 1, 2
	var sum int32
	for r := range responses.Results().IgnoreErrors() {
		sum += r.Value.GetValue()
	}

	if sum != 60 { // 10 + 20 + 30 = 60
		t.Errorf("Expected sum 60, got %d", sum)
	}
}

func TestQuorumCallSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create configuration inside synctest bubble for controlled time
		config := gorumstest.Config(t, 3, gorumstest.EchoServer, gorumstest.SkipGoleak())
		ctx := gorumstest.Context(t, 2*time.Second)

		responses := mockcall.QuorumCall(ctx, config, "synctest-demo")

		resp, err := responses.Majority()
		if err != nil {
			t.Fatalf("QuorumCall failed: %v", err)
		}

		if resp.GetValue() != "echo: synctest-demo" {
			t.Errorf("Expected 'echo: synctest-demo', got '%s'", resp.GetValue())
		}

		t.Log("Gorums basic operations work with synctest!")
	})
}

func TestQuorumCallAsyncSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := gorumstest.Config(t, 5, gorumstest.EchoServer, gorumstest.SkipGoleak())
		ctx := gorumstest.Context(t, 3*time.Second)

		// Async sends both calls before either reply is collected.
		responses1 := mockcall.QuorumCall(ctx, config, "async1").Async()
		responses2 := mockcall.QuorumCall(ctx, config, "async2").Async()

		reply1, err1 := responses1.Majority()
		reply2, err2 := responses2.Majority()

		if err1 != nil {
			t.Fatalf("call 1 error: %v", err1)
		}
		if err2 != nil {
			t.Fatalf("call 2 error: %v", err2)
		}

		if reply1.GetValue() != "echo: async1" {
			t.Errorf("Expected 'echo: async1', got '%s'", reply1.GetValue())
		}
		if reply2.GetValue() != "echo: async2" {
			t.Errorf("Expected 'echo: async2', got '%s'", reply2.GetValue())
		}

		t.Log("Gorums async operations work with synctest!")
	})
}

// BenchmarkQuorumCallTerminalMethods benchmarks the built-in terminal methods with real servers.
func BenchmarkQuorumCallTerminalMethods(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9, 13, 17, 19} {
		config := gorumstest.Config(b, numNodes, gorumstest.EchoServer)
		ctx := b.Context()

		b.Run(fmt.Sprintf("Majority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := mockcall.QuorumCall(ctx, config, "test").Majority()
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
				resp, err := mockcall.QuorumCall(ctx, config, "test").Threshold(threshold)
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("First/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := mockcall.QuorumCall(ctx, config, "test").First()
				if err != nil {
					b.Fatalf("QuorumCall error: %v", err)
				}
				_ = resp.GetValue()
			}
		})

		b.Run(fmt.Sprintf("All/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := mockcall.QuorumCall(ctx, config, "test").All()
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
		config := gorumstest.Config(b, numNodes, gorumstest.EchoServer)
		ctx := b.Context()

		// Using manual iteration with early return
		b.Run(fmt.Sprintf("Iterator/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			quorum := numNodes/2 + 1
			for b.Loop() {
				responses := mockcall.QuorumCall(ctx, config, "test")
				var firstResp *pb.StringValue
				var count int
				for result := range responses.Results() {
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
