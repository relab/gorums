package gorums_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mockcall"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// streamCall is the call handle returned by [mockcall.QuorumCallStream].
type streamCall = gorums.Call[*pb.StringValue, *pb.StringValue]

// prefixKey is a Matching key function that maps "echo: test-i" to
// "echo: test", so that all responses from [gorumstest.StreamServer] match.
func prefixKey(resp *pb.StringValue) string {
	prefix, _, _ := strings.Cut(resp.GetValue(), "-")
	return prefix
}

// agreementIDs returns the node IDs of the responses in agreement.
func agreementIDs(agreement gorums.Agreement[*pb.StringValue]) []gorums.ID {
	var ids []gorums.ID
	for _, resp := range agreement.Responses {
		ids = append(ids, resp.NodeID)
	}
	return ids
}

// addNodeID is a Reduce add function that records the node ID of each
// folded response.
func addNodeID(ids []gorums.ID, resp gorums.NodeResponse[*pb.StringValue]) ([]gorums.ID, error) {
	return append(ids, resp.NodeID), nil
}

// TestCallTerminalsStream runs terminal methods on a streaming quorum call.
// Each server sends three responses, so every node appears more than once
// in the response sequence, and the sequence ends only when the context ends.
func TestCallTerminalsStream(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		run      func(*streamCall) ([]gorums.ID, error)
		allNodes bool // want the IDs of all nodes; otherwise want no IDs
		wantErr  error
	}{
		{
			name:    "Reduce/CountsEachNodeOnce",
			timeout: 2 * time.Second,
			run: func(call *streamCall) ([]gorums.ID, error) {
				return call.Reduce(3, nil, addNodeID)
			},
			allNodes: true,
		},
		{
			name:    "Reduce/IncompleteWhenContextEnds",
			timeout: 100 * time.Millisecond,
			run: func(call *streamCall) ([]gorums.ID, error) {
				return call.Reduce(4, nil, addNodeID)
			},
			allNodes: true,
			wantErr:  gorums.ErrIncomplete,
		},
		{
			name:    "Matching/CountsEachNodeOnce",
			timeout: 2 * time.Second,
			run: func(call *streamCall) ([]gorums.ID, error) {
				agreement, err := call.Matching(3, prefixKey)
				return agreementIDs(agreement), err
			},
			allNodes: true,
		},
		{
			name:    "Matching/IncompleteWhenContextEnds",
			timeout: 100 * time.Millisecond,
			run: func(call *streamCall) ([]gorums.ID, error) {
				agreement, err := call.Matching(4, prefixKey)
				return agreementIDs(agreement), err
			},
			wantErr: gorums.ErrIncomplete,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := gorumstest.Config(t, 3, gorumstest.StreamServer(5*time.Millisecond))
			ctx := gorumstest.Context(t, tt.timeout)

			ids, err := tt.run(mockcall.QuorumCallStream(config.Context(ctx), "test"))
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			var wantIDs []gorums.ID
			if tt.allNodes {
				wantIDs = slices.Sorted(slices.Values(config.NodeIDs()))
			}
			if got := slices.Sorted(slices.Values(ids)); !slices.Equal(got, wantIDs) {
				t.Errorf("node IDs = %v, want %v", got, wantIDs)
			}
		})
	}
}

// countAtLeast returns an AggregateFunc that reports whether at least n nodes
// sent a successful response.
func countAtLeast(n int) gorums.AggregateFunc[*pb.StringValue, bool] {
	return func(seq gorums.ResponseSeq[*pb.StringValue]) (bool, error) {
		count := 0
		for range seq.IgnoreErrors() {
			count++
			if count >= n {
				return true, nil
			}
		}
		return false, nil
	}
}

// TestCallTerminalsAggregateFunc verifies that Aggregate accepts both an
// AggregateFunc value and a plain function, and infers the result type.
func TestCallTerminalsAggregateFunc(t *testing.T) {
	config := gorumstest.Config(t, 3, gorumstest.EchoServer)
	ctx := gorumstest.Context(t, 2*time.Second)

	ok, err := mockcall.QuorumCall(config.Context(ctx), "test").Aggregate(countAtLeast(2))
	if err != nil || !ok {
		t.Errorf("Aggregate(countAtLeast(2)) = %v, %v, want true, nil", ok, err)
	}

	value, err := mockcall.QuorumCall(config.Context(ctx), "test").Aggregate(
		func(seq gorums.ResponseSeq[*pb.StringValue]) (string, error) {
			for resp := range seq.IgnoreErrors() {
				return resp.Value.GetValue(), nil
			}
			return "", gorums.ErrIncomplete
		})
	if err != nil || value != "echo: test" {
		t.Errorf("Aggregate(func) = %q, %v, want %q, nil", value, err, "echo: test")
	}
}
