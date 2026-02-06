package gorums_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/testutils/mock"
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
			var qcErr gorums.QuorumCallError[uint32]
			if errors.As(gotErr, &qcErr) && qcErr.NodeErrors() != expectedNodeErrors[0] {
				t.Errorf("Expected %d node errors, got %d", expectedNodeErrors[0], qcErr.NodeErrors())
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
	type respType = *gorums.Responses[uint32, *pb.StringValue]
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
			config := gorums.TestConfiguration(t, tt.numNodes, gorums.EchoServerFn)
			ctx := gorums.TestContext(t, 2*time.Second)
			responses := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
				config.Context(ctx),
				pb.String("test"),
				mock.TestMethod,
			)

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
		callFunc func(*gorums.ConfigContext[uint32], *pb.StringValue) error
	}

	const numServers = 3

	type respType = *gorums.Responses[uint32, *pb.StringValue]

	// Helper to create QuorumCall variants
	quorumcall := func(name string, aggregateFunc func(respType) (*pb.StringValue, error)) callInfo {
		return callInfo{
			name: "QuorumCall/" + name,
			callFunc: func(ctx *gorums.ConfigContext[uint32], req *pb.StringValue) error {
				_, err := aggregateFunc(gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](ctx, req, mock.TestMethod))
				return err
			},
		}
	}

	// Helper to create Multicast variants
	multicast := func(name string, opts ...gorums.CallOption) callInfo {
		return callInfo{
			name: "Multicast/" + name,
			callFunc: func(ctx *gorums.ConfigContext[uint32], req *pb.StringValue) error {
				return gorums.Multicast(ctx, req, mock.TestMethod, opts...)
			},
		}
	}

	tests := []struct {
		call    callInfo
		failing int // number of servers to stop (0 to 3)
		wantErr error
	}{
		// Multicast: Fails if ANY node fails
		{multicast("Wait"), 0, nil},
		{multicast("Wait"), 1, gorums.ErrSendFailure},
		{multicast("Wait"), 2, gorums.ErrSendFailure},
		{multicast("Wait"), 3, gorums.ErrSendFailure},

		// Multicast with IgnoreErrors: Should not return error even if nodes fail
		{multicast("IgnoreErrors", gorums.IgnoreErrors()), 0, nil},
		{multicast("IgnoreErrors", gorums.IgnoreErrors()), 3, nil},

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
			config := gorums.TestConfiguration(t, numServers, gorums.DefaultTestServer, gorums.WithStopFunc(t, &stopNodes))
			ctx := config.Context(t.Context())
			req := pb.String("test")

			// Warmup to ensure connections
			if err := tt.call.callFunc(ctx, req); err != nil {
				t.Fatalf("Warmup failed: %v", err)
			}

			if tt.failing > 0 {
				// Collect indices to stop
				stopNodes(slices.Collect(gorums.Range(tt.failing))...)
				time.Sleep(50 * time.Millisecond)
			}

			// Execute test calls after stopping nodes, if any
			var err error
			for range 5 {
				err = tt.call.callFunc(ctx, req)
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
	config := gorums.TestConfiguration(t, 3, gorums.DefaultTestServer) // uses default server that returns (i+1)*10

	ctx := gorums.TestContext(t, 2*time.Second)
	responses := gorums.QuorumCall[uint32, *pb.Int32Value, *pb.Int32Value](
		config.Context(ctx),
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

// TestQuorumCallCollectAll tests collecting all responses
func TestQuorumCallCollectAll(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, gorums.EchoServerFn)

	ctx := gorums.TestContext(t, 2*time.Second)
	responses := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
		config.Context(ctx),
		pb.String("test"),
		mock.TestMethod,
	)

	collected := responses.CollectAll()
	if len(collected) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(collected))
	}
}

func TestQuorumCallSynctest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create configuration inside synctest bubble for controlled time
		config := gorums.TestConfiguration(t, 3, gorums.EchoServerFn, gorums.SkipGoleak())
		ctx := gorums.TestContext(t, 2*time.Second)
		cfgCtx := config.Context(ctx)

		responses := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
			cfgCtx,
			pb.String("synctest-demo"),
			mock.TestMethod,
		)

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
		config := gorums.TestConfiguration(t, 5, gorums.EchoServerFn, gorums.SkipGoleak())
		ctx := gorums.TestContext(t, 3*time.Second)
		cfgCtx := config.Context(ctx)

		// Test async operations
		future1 := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
			cfgCtx,
			pb.String("async1"),
			mock.TestMethod,
		).AsyncMajority()

		future2 := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
			cfgCtx,
			pb.String("async2"),
			mock.TestMethod,
		).AsyncMajority()

		// Wait for both to complete
		reply1, err1 := future1.Get()
		reply2, err2 := future2.Get()

		if err1 != nil {
			t.Fatalf("Future1 error: %v", err1)
		}
		if err2 != nil {
			t.Fatalf("Future2 error: %v", err2)
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
		config := gorums.TestConfiguration(b, numNodes, gorums.EchoServerFn)
		cfgCtx := config.Context(b.Context())

		b.Run(fmt.Sprintf("Majority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resp, err := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
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
				resp, err := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
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
				resp, err := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
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
				resp, err := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
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
		config := gorums.TestConfiguration(b, numNodes, gorums.EchoServerFn)
		cfgCtx := config.Context(b.Context())

		// Using CollectAll and then checking quorum
		b.Run(fmt.Sprintf("CollectAllThenCheck/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			quorum := numNodes/2 + 1
			for b.Loop() {
				responses := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
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
				responses := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
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
				responses := gorums.QuorumCall[uint32, *pb.StringValue, *pb.StringValue](
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
