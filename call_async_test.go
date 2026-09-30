package gorums_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestAsync(t *testing.T) {
	// a type alias short hand for the responses type
	type respType = *gorums.Responses[*pb.StringValue]
	tests := []struct {
		name      string
		call      func(respType) *gorums.Async[*pb.StringValue]
		numNodes  int
		wantValue string
		wantErr   bool
	}{
		{
			name:      "Majority",
			call:      respType.AsyncMajority,
			numNodes:  3,
			wantValue: "echo: test",
		},
		{
			name:      "First",
			call:      respType.AsyncFirst,
			numNodes:  3,
			wantValue: "echo: test",
		},
		{
			name:      "All",
			call:      respType.AsyncAll,
			numNodes:  3,
			wantValue: "echo: test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := gorumstest.Config(t, tt.numNodes, gorumstest.EchoServerFn)
			ctx := gorumstest.Context(t, 2*time.Second)
			responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
				config.Context(ctx),
				pb.String("test"),
				mock.TestMethod,
			)

			future := tt.call(responses.Responses)

			reply, err := future.Get()
			if !checkQuorumCall(t, err, nil) {
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
	config := gorumstest.Config(t, 3, gorumstest.EchoServerFn)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	responses := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
		config.Context(ctx),
		pb.String("test"),
		mock.TestMethod,
	)

	future := responses.AsyncMajority()
	_, err := future.Get()
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// TestAsyncDone verifies that Async.Done reports false while the call is still
// in flight and true once a result is available.
func TestAsyncDone(t *testing.T) {
	t.Run("PendingReportsNotDone", func(t *testing.T) {
		// A call over a never-dialed config never completes, so the future
		// stays pending and Done reports false.
		config := gorumstest.UnreachableConfig(t)
		ctx := gorumstest.Context(t, 2*time.Second)
		future := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
			config.Context(ctx),
			pb.String("test"),
			mock.TestMethod,
		).AsyncMajority()

		if future.Done() {
			t.Error("Done() = true for a call that has not completed, want false")
		}
	})

	t.Run("CompletedReportsDone", func(t *testing.T) {
		config := gorumstest.Config(t, 3, gorumstest.EchoServerFn)
		ctx := gorumstest.Context(t, 2*time.Second)
		future := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
			config.Context(ctx),
			pb.String("test"),
			mock.TestMethod,
		).AsyncMajority()

		if _, err := future.Get(); err != nil {
			t.Fatalf("Get() error: %v", err)
		}
		if !future.Done() {
			t.Error("Done() = false after Get() returned, want true")
		}
	})
}

func BenchmarkAsyncQuorumCall(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9} {
		config := gorumstest.Config(b, numNodes, gorumstest.EchoServerFn)
		cfgCtx := config.Context(b.Context())

		b.Run(fmt.Sprintf("AsyncMajority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				future := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
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
				_, err := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
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

// TestOnewayAsyncWaitAfterContextEnds verifies that Wait reports confirmed
// sends as successful even when the call's context has ended by the time Wait
// runs, which is the pattern Async exists for.
func TestOnewayAsyncWaitAfterContextEnds(t *testing.T) {
	config := gorumstest.Config(t, 3, gorumstest.DefaultServer)
	tests := []struct {
		name  string
		async func(ctx context.Context) *gorums.OnewayAsync
	}{
		{"Multicast", func(ctx context.Context) *gorums.OnewayAsync {
			return gorumsimpl.Multicast(config.Context(ctx), pb.String("x"), mock.TestMethod).Async()
		}},
		{"Unicast", func(ctx context.Context) *gorums.OnewayAsync {
			return gorumsimpl.Unicast(config.Nodes()[0].Context(ctx), pb.String("x"), mock.TestMethod).Async()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range 50 {
				ctx, cancel := context.WithCancel(t.Context())
				h := tt.async(ctx)
				time.Sleep(10 * time.Millisecond) // let the sends complete
				cancel()
				if err := h.Wait(); err != nil {
					t.Fatalf("iteration %d: Wait after cancel = %v, want nil for confirmed sends", i, err)
				}
			}
		})
	}
}
