package gorums

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// Test helper types and functions

// ctxTimeout is the timeout for test contexts. If this is exceeded,
// the test will fail, indicating a bug in the test or the code under test.
const ctxTimeout = 2 * time.Second

// testContext creates a context with timeout for testing.
// It uses t.Context() as the parent and automatically cancels on cleanup.
func testContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return ctx
}

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

// executionTracker tracks interceptor execution order for testing.
type executionTracker struct {
	mu  sync.Mutex
	log []string
}

func (et *executionTracker) append(entry string) {
	et.mu.Lock()
	defer et.mu.Unlock()
	et.log = append(et.log, entry)
}

func (et *executionTracker) get() []string {
	et.mu.Lock()
	defer et.mu.Unlock()
	return append([]string(nil), et.log...)
}

func (et *executionTracker) check(t *testing.T, want []string) {
	t.Helper()
	got := et.get()
	if len(got) != len(want) {
		t.Errorf("Expected %d log entries, got %d: %v", len(want), len(got), got)
		return
	}
	for i, wantEntry := range want {
		if i >= len(got) || got[i] != wantEntry {
			t.Errorf("log[%d] = %v, want %s", i, got, wantEntry)
		}
	}
}

// makeClientCtx is a helper to create a ClientCtx with mock responses for unit tests.
// It creates a channel with the provided responses and returns a ClientCtx.
func makeClientCtx[Req, Resp proto.Message](t *testing.T, numNodes int, responses []NodeResponse[proto.Message]) *ClientCtx[Req, Resp] {
	t.Helper()

	resultChan := make(chan NodeResponse[proto.Message], len(responses))
	for _, r := range responses {
		resultChan <- r
	}
	close(resultChan)

	config := make(RawConfiguration, numNodes)
	for i := range numNodes {
		config[i] = &RawNode{id: uint32(i + 1)}
	}

	c := &ClientCtx[Req, Resp]{
		Context:         t.Context(),
		config:          config,
		replyChan:       resultChan,
		expectedReplies: numNodes,
	}
	c.responseSeq = c.defaultResponseSeq()
	return c
}

// makeResponses creates a Responses object from a ClientCtx for testing terminal methods.
func makeResponses[Req, Resp proto.Message](ctx *ClientCtx[Req, Resp]) *Responses[Req, Resp] {
	return &Responses[Req, Resp]{ctx: ctx}
}

// -------------------------------------------------------------------------
// Terminal Method Tests
// -------------------------------------------------------------------------

// TestTerminalMethods tests the terminal methods on Responses
func TestTerminalMethods(t *testing.T) {
	tests := []struct {
		name        string
		numNodes    int
		responses   []NodeResponse[proto.Message]
		method      string // "First", "Majority", "All", "Threshold"
		threshold   int    // for Threshold method
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
			method:    "First",
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
			method:      "First",
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
			method:    "Majority",
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
			method:      "Majority",
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
			method:    "Majority",
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
			method:    "All",
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
			method:      "All",
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
		// Threshold tests
		{
			name:     "Threshold_Success",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
			},
			method:    "Threshold",
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
			method:      "Threshold",
			threshold:   2,
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, tt.numNodes, tt.responses)
			responses := makeResponses(clientCtx)

			var result *pb.StringValue
			var err error

			switch tt.method {
			case "First":
				result, err = responses.First()
			case "Majority":
				result, err = responses.Majority()
			case "All":
				result, err = responses.All()
			case "Threshold":
				result, err = responses.Threshold(tt.threshold)
			}

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

		var count int
		for range clientCtx.Responses().IgnoreErrors() {
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

		// Filter to only node 2
		var count int
		for r := range clientCtx.Responses().Filter(func(r NodeResponse[*pb.StringValue]) bool {
			return r.NodeID == 2
		}) {
			count++
			if r.Value.GetValue() != "response2" {
				t.Errorf("Expected 'response2', got '%s'", r.Value.GetValue())
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

		collected := clientCtx.Responses().CollectN(2)
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

		collected := clientCtx.Responses().CollectAll()
		if len(collected) != 3 {
			t.Errorf("Expected 3 collected responses, got %d", len(collected))
		}
	})
}

// -------------------------------------------------------------------------
// Integration Tests
// -------------------------------------------------------------------------

// TestInterceptorIntegration_First tests the complete flow with real servers
func TestInterceptorIntegration_First(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, echoServerFn)
	t.Cleanup(closeServers)

	ctx := testContext(t, ctxTimeout)
	responses := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
		NewTestConfigContext(t, ctx, addrs),
		pb.String("test"),
		mock.TestMethod,
	)

	result, err := responses.First()
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}

	if result.GetValue() != "echo: test" {
		t.Errorf("Expected 'echo: test', got '%s'", result.GetValue())
	}
}

// TestInterceptorIntegration_Majority tests majority quorum with real servers
func TestInterceptorIntegration_Majority(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, echoServerFn)
	t.Cleanup(closeServers)

	ctx := testContext(t, ctxTimeout)
	responses := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
		NewTestConfigContext(t, ctx, addrs),
		pb.String("test"),
		mock.TestMethod,
	)

	result, err := responses.Majority()
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}

	if result.GetValue() != "echo: test" {
		t.Errorf("Expected 'echo: test', got '%s'", result.GetValue())
	}
}

// TestInterceptorIntegration_CustomAggregation tests custom response aggregation
func TestInterceptorIntegration_CustomAggregation(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, nil) // uses default server that returns (i+1)*10
	t.Cleanup(closeServers)

	ctx := testContext(t, ctxTimeout)
	responses := QuorumCallWithInterceptor[*pb.Int32Value, *pb.Int32Value](
		NewTestConfigContext(t, ctx, addrs),
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

// TestInterceptorIntegration_CollectAll tests collecting all responses
func TestInterceptorIntegration_CollectAll(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, echoServerFn)
	t.Cleanup(closeServers)

	ctx := testContext(t, ctxTimeout)
	responses := QuorumCallWithInterceptor[*pb.StringValue, *pb.StringValue](
		NewTestConfigContext(t, ctx, addrs),
		pb.String("test"),
		mock.TestMethod,
	)

	collected := responses.CollectAll()
	if len(collected) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(collected))
	}
}
