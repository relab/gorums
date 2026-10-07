package impl

import (
	"context"
	"errors"
	"testing"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// makeCallContext is a helper to create a CallContext with mock responses for unit tests.
// It creates a channel with the provided responses and returns a CallContext.
func makeCallContext[Req, Resp proto.Message](t *testing.T, numNodes int, responses []NodeResponse[proto.Message]) *CallContext[Req, Resp] {
	t.Helper()
	c, err := newReplayCallContext[Req, Resp](t.Context(), numNodes, responses)
	if err != nil {
		t.Fatalf("failed to marshal mock response: %v", err)
	}
	return c
}

// newReplayCallContext returns a CallContext for numNodes nodes whose call
// is already dispatched and whose responses are the given responses.
func newReplayCallContext[Req, Resp proto.Message](ctx context.Context, numNodes int, responses []NodeResponse[proto.Message]) (*CallContext[Req, Resp], error) {
	responseChan := make(chan NodeResponse[*stream.Message], len(responses))
	for _, r := range responses {
		var sm *stream.Message
		if r.Value != nil {
			var err error
			sm, err = stream.NewMessage(ctx, 1, mock.TestMethod, r.Value)
			if err != nil {
				return nil, err
			}
		}
		responseChan <- NodeResponse[*stream.Message]{
			NodeID: r.NodeID,
			Value:  sm,
			Err:    r.Err,
		}
	}
	close(responseChan)

	config := make(Config, numNodes)
	for i := range numNodes {
		config[i] = conn.NewNodeForTest(uint32(i+1), nil)
	}

	c := &CallContext[Req, Resp]{
		Context:      ctx,
		config:       config,
		responseChan: responseChan,
	}
	// Mark sendOnce as done since test responses are already in the channel
	c.sendOnce.Do(func() {})
	c.responseSeq = c.defaultResponseSeq()
	return c, nil
}

// checkError returns true if the error matches the expected error.
func checkError(t *testing.T, wantErr bool, err, wantErrType error) bool {
	t.Helper()
	if wantErr {
		if err == nil {
			t.Error("Expected error, got nil")
			return false
		}
		if wantErrType != nil && !errors.Is(err, wantErrType) {
			t.Errorf("Expected error type %v, got %v", wantErrType, err)
			return false
		}
		return true
	}
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
		return false
	}
	return true
}

// -------------------------------------------------------------------------
// Terminal Method Tests
// -------------------------------------------------------------------------

// TestResponsesTerminalMethods tests the terminal methods on Responses
func TestResponsesTerminalMethods(t *testing.T) {
	type respType = *Responses[*pb.StringValue]
	tests := []struct {
		name        string
		numNodes    int
		responses   []NodeResponse[proto.Message]
		call        func(resp respType) (*pb.StringValue, error)
		wantValue   string
		wantErr     bool
		wantErrType error
	}{
		// First tests
		{
			name:     "First_Success",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
			},
			call:      respType.First,
			wantValue: "response1",
		},
		{
			name:     "First_Error",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: nil, Err: errors.New("node error")},
				{NodeID: 2, Value: nil, Err: errors.New("node error")},
				{NodeID: 3, Value: nil, Err: errors.New("node error")},
			},
			call:        respType.First,
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
		// Majority tests
		{
			name:     "Majority_Success_3Nodes",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
			},
			call:      respType.Majority,
			wantValue: "response1",
		},
		{
			name:     "Majority_Insufficient",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: nil, Err: errors.New("node error")},
				{NodeID: 3, Value: nil, Err: errors.New("node error")},
			},
			call:        respType.Majority,
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
		{
			name:     "Majority_Even_Success",
			numNodes: 4,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
				{NodeID: 3, Value: pb.String("response3"), Err: nil},
			},
			call:      respType.Majority,
			wantValue: "response1",
		},
		// All tests
		{
			name:     "All_Success",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
				{NodeID: 3, Value: pb.String("response3"), Err: nil},
			},
			call:      respType.All,
			wantValue: "response1",
		},
		{
			name:     "All_PartialFailure",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
				{NodeID: 3, Value: nil, Err: errors.New("node error")},
			},
			call:        respType.All,
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
		// ErrSkipNode must count toward neither success nor failure.
		{
			name:     "Majority_AllSkipped_Incomplete",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: nil, Err: ErrSkipNode},
				{NodeID: 2, Value: nil, Err: ErrSkipNode},
				{NodeID: 3, Value: nil, Err: ErrSkipNode},
			},
			call:        respType.Majority,
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
		{
			name:     "All_OneSkipped_Incomplete",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
				{NodeID: 3, Value: nil, Err: ErrSkipNode},
			},
			call:        respType.All,
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, tt.numNodes, tt.responses)
			responses := newResponses(callCtx)

			result, err := tt.call(responses)

			if !checkError(t, tt.wantErr, err, tt.wantErrType) {
				return
			}
			if !tt.wantErr && result.GetValue() != tt.wantValue {
				t.Errorf("Expected value %q, got %q", tt.wantValue, result.GetValue())
			}
		})
	}
}

func TestResponsesThreshold(t *testing.T) {
	type respType = *Responses[*pb.StringValue]
	tests := []struct {
		name        string
		numNodes    int
		responses   []NodeResponse[proto.Message]
		call        func(resp respType, threshold int) (*pb.StringValue, error)
		threshold   int
		wantValue   string
		wantErr     bool
		wantErrType error
	}{
		{
			name:     "Threshold_Success",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
			},
			call:      respType.Threshold,
			threshold: 2,
			wantValue: "response1",
		},
		{
			name:     "Threshold_Insufficient",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: nil, Err: errors.New("node error")},
				{NodeID: 3, Value: nil, Err: errors.New("node error")},
			},
			call:        respType.Threshold,
			threshold:   2,
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, tt.numNodes, tt.responses)
			responses := newResponses(callCtx)

			result, err := tt.call(responses, tt.threshold)

			if !checkError(t, tt.wantErr, err, tt.wantErrType) {
				return
			}
			if !tt.wantErr && result.GetValue() != tt.wantValue {
				t.Errorf("Expected value %q, got %q", tt.wantValue, result.GetValue())
			}
		})
	}
}

// -------------------------------------------------------------------------
// Iterator Method Tests
// -------------------------------------------------------------------------

// TestResponseSeqMethods tests the iterator helper methods
func TestResponseSeqMethods(t *testing.T) {
	t.Run("IgnoreErrors", func(t *testing.T) {
		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("response1"), Err: nil},
			{NodeID: 2, Value: nil, Err: errors.New("node error")},
			{NodeID: 3, Value: pb.String("response3"), Err: nil},
		}
		callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, responses)
		r := newResponses(callCtx)

		var count int
		for range r.Results().IgnoreErrors() {
			count++
		}
		if count != 2 {
			t.Errorf("Expected 2 successful responses, got %d", count)
		}
	})

	t.Run("Filter", func(t *testing.T) {
		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("response1"), Err: nil},
			{NodeID: 2, Value: pb.String("response2"), Err: nil},
			{NodeID: 3, Value: pb.String("response3"), Err: nil},
		}
		callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, responses)
		r := newResponses(callCtx)

		// Filter to only node 2
		var count int
		for resp := range r.Results().Filter(func(resp NodeResponse[*pb.StringValue]) bool {
			return resp.NodeID == 2
		}) {
			count++
			if resp.Value.GetValue() != "response2" {
				t.Errorf("Expected 'response2', got '%s'", resp.Value.GetValue())
			}
		}
		if count != 1 {
			t.Errorf("Expected 1 filtered response, got %d", count)
		}
	})

	t.Run("CollectN", func(t *testing.T) {
		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("response1"), Err: nil},
			{NodeID: 2, Value: pb.String("response2"), Err: nil},
			{NodeID: 3, Value: pb.String("response3"), Err: nil},
		}
		callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, responses)
		r := newResponses(callCtx)

		collected := r.Results().CollectN(2)
		if len(collected) != 2 {
			t.Errorf("Expected 2 collected responses, got %d", len(collected))
		}
	})

	t.Run("CollectAll", func(t *testing.T) {
		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("response1"), Err: nil},
			{NodeID: 2, Value: pb.String("response2"), Err: nil},
			{NodeID: 3, Value: pb.String("response3"), Err: nil},
		}
		callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, responses)
		r := newResponses(callCtx)

		collected := r.Results().CollectAll()
		if len(collected) != 3 {
			t.Errorf("Expected 3 collected responses, got %d", len(collected))
		}
	})
}
