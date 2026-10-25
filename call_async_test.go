package gorums_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/mockcall"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCallAsyncCanceledContext verifies that a terminal method reports an
// error for a call dispatched with Call.Async whose context has ended.
func TestCallAsyncCanceledContext(t *testing.T) {
	config := gorumstest.Config(t, 3, gorumstest.EchoServer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	responses := mockcall.QuorumCall(config.Context(ctx), "test").Async()

	if _, err := responses.Majority(); err == nil {
		t.Error("Majority() error = nil, want an error for a canceled context")
	}
}

func BenchmarkAsyncQuorumCall(b *testing.B) {
	for _, numNodes := range []int{3, 5, 7, 9} {
		config := gorumstest.Config(b, numNodes, gorumstest.EchoServer)
		cfgCtx := config.Context(b.Context())

		b.Run(fmt.Sprintf("AsyncThenMajority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				responses := mockcall.QuorumCall(cfgCtx, "test").Async()
				if _, err := responses.Majority(); err != nil {
					b.Fatalf("Async().Majority() error: %v", err)
				}
			}
		})

		// Compare with blocking Majority
		b.Run(fmt.Sprintf("BlockingMajority/%d", numNodes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := mockcall.QuorumCall(cfgCtx, "test").Majority()
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
//
// Each node's channel sends in order and confirms a one-way send before it
// sends the next request, so a completed two-way call issued after Async
// proves that the one-way sends were confirmed.
func TestOnewayAsyncWaitAfterContextEnds(t *testing.T) {
	config := gorumstest.Config(t, 3, nil)
	node := config.Nodes()[0]
	tests := []struct {
		name    string
		async   func(ctx context.Context) *gorums.OnewayAsync
		barrier func() error
	}{
		{
			name: "Multicast",
			async: func(ctx context.Context) *gorums.OnewayAsync {
				return mockcall.Multicast(config.Context(ctx), "x").Async()
			},
			barrier: func() error {
				_, err := mockcall.QuorumCall(config.Context(t.Context()), "x").All()
				return err
			},
		},
		{
			name: "Unicast",
			async: func(ctx context.Context) *gorums.OnewayAsync {
				return mockcall.Unicast(node.Context(ctx), "x").Async()
			},
			barrier: func() error {
				_, err := mockcall.RemoteCall(node.Context(t.Context()), "x")
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range 50 {
				ctx, cancel := context.WithCancel(t.Context())
				h := tt.async(ctx)
				if err := tt.barrier(); err != nil {
					t.Fatalf("iteration %d: barrier call: %v", i, err)
				}
				cancel()
				if err := h.Wait(); err != nil {
					t.Fatalf("iteration %d: Wait after cancel = %v, want nil for confirmed sends", i, err)
				}
			}
		})
	}
}

// TestCallAsyncTerminals verifies that every terminal method collects the
// replies of a call dispatched with Call.Async.
func TestCallAsyncTerminals(t *testing.T) {
	type respType = *gorums.Responses[*pb.StringValue]
	key := func(r *pb.StringValue) string { return r.GetValue() }
	tests := []struct {
		name    string
		collect func(respType) (string, error)
	}{
		{name: "First", collect: func(r respType) (string, error) {
			resp, err := r.First()
			return resp.GetValue(), err
		}},
		{name: "Majority", collect: func(r respType) (string, error) {
			resp, err := r.Majority()
			return resp.GetValue(), err
		}},
		{name: "All", collect: func(r respType) (string, error) {
			resp, err := r.All()
			return resp.GetValue(), err
		}},
		{name: "Threshold", collect: func(r respType) (string, error) {
			resp, err := r.Threshold(2)
			return resp.GetValue(), err
		}},
		{name: "Reduce", collect: func(r respType) (string, error) {
			return r.Reduce(3, "", func(_ string, resp gorums.NodeResponse[*pb.StringValue]) (string, error) {
				return resp.Value.GetValue(), nil
			})
		}},
		{name: "Matching", collect: func(r respType) (string, error) {
			resp, err := r.Matching(3, key)
			return resp.GetValue(), err
		}},
		{name: "MatchingAgreement", collect: func(r respType) (string, error) {
			agreement, err := r.MatchingAgreement(3, key)
			if err == nil && len(agreement.Responses) != 3 {
				return "", fmt.Errorf("agreement has %d responses, want 3", len(agreement.Responses))
			}
			return agreement.Value.GetValue(), err
		}},
		{name: "Aggregate", collect: func(r respType) (string, error) {
			return r.Aggregate(func(seq gorums.ResponseSeq[*pb.StringValue]) (string, error) {
				for resp := range seq.IgnoreErrors() {
					return resp.Value.GetValue(), nil
				}
				return "", gorums.ErrIncomplete
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := gorumstest.Config(t, 3, gorumstest.EchoServer)
			ctx := gorumstest.Context(t, 2*time.Second)
			responses := mockcall.QuorumCall(config.Context(ctx), "test").Async()

			got, err := tt.collect(responses)
			if !checkQuorumCall(t, err, nil) {
				return
			}
			if got != "echo: test" {
				t.Errorf("got %q, want %q", got, "echo: test")
			}
		})
	}
}

// TestCallAsyncSendsBeforeReturn verifies that Call.Async sends the requests
// before it returns: every server receives the request although the caller
// never reads the replies.
func TestCallAsyncSendsBeforeReturn(t *testing.T) {
	const numNodes = 3
	received := make(chan struct{}, numNodes)
	config := gorumstest.Config(t, numNodes, func(int) gorumstest.ServerIface {
		srv := gorums.NewServer()
		echo := gorumstest.EchoHandler("echo")
		srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
			received <- struct{}{}
			return echo(ctx, in)
		})
		return srv
	})
	ctx := gorumstest.Context(t, 2*time.Second)

	_ = mockcall.QuorumCall(config.Context(ctx), "test").Async()

	if got := gorumstest.Collect(t, 2*time.Second, numNodes, received); len(got) != numNodes {
		t.Errorf("servers received %d requests before any reply was read, want %d", len(got), numNodes)
	}
}

// TestCallAsyncAfterDispatchPanics verifies that Call.Async consumes the call:
// a second Async, an Async after the call was dispatched, and an Intercept
// after Async all panic.
func TestCallAsyncAfterDispatchPanics(t *testing.T) {
	identity := gorums.MapResponse[*pb.StringValue](func(r *pb.StringValue, _ *gorums.Node) *pb.StringValue { return r })
	type callType = *gorums.Call[*pb.StringValue, *pb.StringValue]
	tests := []struct {
		name       string
		dispatch   func(callType)
		then       func(callType)
		wantPrefix string
	}{
		{
			name:       "AsyncTwice",
			dispatch:   func(c callType) { c.Async() },
			then:       func(c callType) { c.Async() },
			wantPrefix: "gorums: Call.Async called after the call was dispatched",
		},
		{
			name:       "AsyncAfterResults",
			dispatch:   func(c callType) { c.Results() },
			then:       func(c callType) { c.Async() },
			wantPrefix: "gorums: Call.Async called after the call was dispatched",
		},
		{
			name: "AsyncAfterTerminal",
			dispatch: func(c callType) {
				if _, err := c.Majority(); err != nil {
					t.Fatalf("Majority: %v", err)
				}
			},
			then:       func(c callType) { c.Async() },
			wantPrefix: "gorums: Call.Async called after the call was dispatched",
		},
		{
			name:       "InterceptAfterAsync",
			dispatch:   func(c callType) { c.Async() },
			then:       func(c callType) { c.Intercept(identity) },
			wantPrefix: "gorums: Intercept called after the call was dispatched",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := gorumstest.Config(t, 3, gorumstest.EchoServer)
			ctx := gorumstest.Context(t, 2*time.Second)
			call := mockcall.QuorumCall(config.Context(ctx), "test")
			tt.dispatch(call)
			assertPanic(t, tt.wantPrefix, func() { tt.then(call) })
		})
	}
}
