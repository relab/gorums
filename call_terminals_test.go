package gorums_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mockcall"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// streamCall is the call handle returned by [mockcall.QuorumCallStream].
type streamCall = gorums.Call[*pb.StringValue, *pb.StringValue]

// addNodeID is a Reduce step function that records the node ID of each
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
