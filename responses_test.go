package gorums

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// makeClientCtx is a helper to create a clientCtx with mock responses for unit tests.
// It creates a channel with the provided responses and returns a clientCtx.
func makeClientCtx[Req, Resp proto.Message](t *testing.T, numNodes int, responses []NodeResponse[proto.Message]) *clientCtx[Req, Resp] {
	t.Helper()

	resultChan := make(chan NodeResponse[proto.Message], len(responses))
	for _, r := range responses {
		resultChan <- r
	}
	close(resultChan)

	config := make(Configuration, numNodes)
	for i := range numNodes {
		config[i] = &Node{id: uint32(i + 1)}
	}

	c := &clientCtx[Req, Resp]{
		Context:         t.Context(),
		config:          config,
		replyChan:       resultChan,
		expectedReplies: numNodes,
	}
	// Mark sendOnce as done since test responses are already in the channel
	c.sendOnce.Do(func() {})
	c.responseSeq = c.defaultResponseSeq()
	return c
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

// TestTerminalMethods tests the terminal methods on Responses
func TestTerminalMethods(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, tt.numNodes, tt.responses)
			responses := NewResponses(clientCtx)

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

func TestTerminalMethodsThreshold(t *testing.T) {
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
			clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, tt.numNodes, tt.responses)
			responses := NewResponses(clientCtx)

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

// TestIteratorMethods tests the iterator helper methods
func TestIteratorMethods(t *testing.T) {
	t.Run("IgnoreErrors", func(t *testing.T) {
		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("response1"), Err: nil},
			{NodeID: 2, Value: nil, Err: errors.New("node error")},
			{NodeID: 3, Value: pb.String("response3"), Err: nil},
		}
		clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)
		r := NewResponses(clientCtx)

		var count int
		for range r.Seq().IgnoreErrors() {
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
		clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)
		r := NewResponses(clientCtx)

		// Filter to only node 2
		var count int
		for resp := range r.Seq().Filter(func(resp NodeResponse[*pb.StringValue]) bool {
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
		clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)
		r := NewResponses(clientCtx)

		collected := r.CollectN(2)
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
		clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)
		r := NewResponses(clientCtx)

		collected := r.CollectAll()
		if len(collected) != 3 {
			t.Errorf("Expected 3 collected responses, got %d", len(collected))
		}
	})
}
