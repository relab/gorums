package gorums

import (
	"context"
	"errors"
	"slices"
	"strconv"
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

// loggingInterceptor creates a reusable logging interceptor for testing.
// It logs "before" and "after" entries to the provided tracker.
func loggingInterceptor[Req, Resp proto.Message](tracker *executionTracker) QuorumInterceptor[Req, Resp, Resp] {
	return func(next QuorumFunc[Req, Resp, Resp]) QuorumFunc[Req, Resp, Resp] {
		return func(ctx *ClientCtx[Req, Resp]) (Resp, error) {
			tracker.append("logging-before")
			result, err := next(ctx)
			tracker.append("logging-after")
			return result, err
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

	return &ClientCtx[Req, Resp]{
		Context:         t.Context(),
		config:          config,
		replyChan:       resultChan,
		expectedReplies: numNodes,
	}
}

// Iterator Utility Tests

// TestIteratorUtilities tests the iterator helper functions
func TestIteratorUtilities(t *testing.T) {
	tests := []struct {
		name            string
		responses       []NodeResponse[proto.Message]
		operation       string // "ignoreErrors", "collectN", "collectAll", "filter"
		collectN        int
		wantCount       int
		wantFilteredIDs []uint32
	}{
		{
			name: "IgnoreErrors",
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: nil, Err: errors.New("node error")},
				{NodeID: 3, Value: pb.String("response3"), Err: nil},
				{NodeID: 4, Value: nil, Err: errors.New("another error")},
				{NodeID: 5, Value: pb.String("response5"), Err: nil},
			},
			operation:       "ignoreErrors",
			wantCount:       3,
			wantFilteredIDs: []uint32{1, 3, 5},
		},
		{
			name: "CollectN",
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response"), Err: nil},
				{NodeID: 2, Value: nil, Err: errors.New("error")},
				{NodeID: 3, Value: pb.String("response"), Err: nil},
				{NodeID: 4, Value: pb.String("response"), Err: nil},
				{NodeID: 5, Value: pb.String("response"), Err: nil},
			},
			operation: "collectN",
			collectN:  3,
			wantCount: 3,
		},
		{
			name: "CollectAll",
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response"), Err: nil},
				{NodeID: 2, Value: nil, Err: errors.New("error")},
				{NodeID: 3, Value: pb.String("response"), Err: nil},
			},
			operation: "collectAll",
			wantCount: 3,
		},
		{
			name: "Filter",
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("keep"), Err: nil},
				{NodeID: 2, Value: pb.String("drop"), Err: nil},
				{NodeID: 3, Value: pb.String("keep"), Err: nil},
			},
			operation:       "filter",
			wantCount:       2,
			wantFilteredIDs: []uint32{1, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, len(tt.responses), tt.responses)

			switch tt.operation {
			case "ignoreErrors":
				count := 0
				for resp := range clientCtx.Responses().IgnoreErrors() {
					t.Logf("Node %d: %v", resp.NodeID, resp.Value.GetValue())
					count++
					if !slices.Contains(tt.wantFilteredIDs, resp.NodeID) {
						t.Errorf("Node %d should have been filtered out", resp.NodeID)
					}
				}
				if count != tt.wantCount {
					t.Errorf("Expected %d successful responses, got %d", tt.wantCount, count)
				}

			case "collectN":
				replies := clientCtx.Responses().CollectN(tt.collectN)
				if len(replies) != tt.wantCount {
					t.Errorf("Expected %d responses, got %d", tt.wantCount, len(replies))
				}

			case "collectAll":
				replies := clientCtx.Responses().CollectAll()
				if len(replies) != tt.wantCount {
					t.Errorf("Expected %d responses, got %d", tt.wantCount, len(replies))
				}

			case "filter":
				count := 0
				for resp := range clientCtx.Responses().Filter(func(r NodeResponse[*pb.StringValue]) bool {
					return r.Value.GetValue() == "keep"
				}) {
					count++
					if !slices.Contains(tt.wantFilteredIDs, resp.NodeID) {
						t.Errorf("Node %d should have been filtered out", resp.NodeID)
					}
				}
				if count != tt.wantCount {
					t.Errorf("Expected %d responses, got %d", tt.wantCount, count)
				}
			}
		})
	}
}

// Interceptor Unit Tests

// TestInterceptorChaining tests composing multiple interceptors
func TestInterceptorChaining(t *testing.T) {
	t.Run("ChainLoggingAndQuorum", func(t *testing.T) {
		// Track interceptor execution order
		tracker := &executionTracker{}

		// Create mock responses
		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("response1"), Err: nil},
			{NodeID: 2, Value: pb.String("response2"), Err: nil},
			{NodeID: 3, Value: pb.String("response3"), Err: nil},
		}
		clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)

		handler := Chain(
			MajorityQuorum[*pb.StringValue, *pb.StringValue],
			loggingInterceptor[*pb.StringValue, *pb.StringValue](tracker),
		)
		result, err := handler(clientCtx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result.GetValue() != "response1" {
			t.Errorf("Expected 'response1', got '%s'", result.GetValue())
		}

		// Check execution order
		tracker.check(t, []string{"logging-before", "logging-after"})
	})
}

// sumInterceptor returns the sum of the responses from all nodes.
// It demonstrates a custom aggregation interceptor; it is used in several tests.
var sumInterceptor = func(ctx *ClientCtx[*pb.Int32Value, *pb.Int32Value]) (*pb.Int32Value, error) {
	var sum int32
	for result := range ctx.Responses().IgnoreErrors() {
		sum += result.Value.GetValue()
	}
	return pb.Int32(sum), nil
}

// TestInterceptorCustomAggregation demonstrates custom interceptor for aggregation
func TestInterceptorCustomAggregation(t *testing.T) {
	t.Run("SumAggregation", func(t *testing.T) {
		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.Int32(10), Err: nil},
			{NodeID: 2, Value: pb.Int32(20), Err: nil},
			{NodeID: 3, Value: pb.Int32(30), Err: nil},
		}
		clientCtx := makeClientCtx[*pb.Int32Value, *pb.Int32Value](t, 3, responses)

		// Custom aggregation interceptors typically don't call next.
		result, err := sumInterceptor(clientCtx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if got, want := result.GetValue(), int32(60); got != want {
			t.Errorf("Sum = %d, want %d", got, want)
		}
	})
}

// TestInterceptorCustomReturnType demonstrates using interceptors to return a different type
func TestInterceptorCustomReturnType(t *testing.T) {
	type CustomResult struct {
		Total int
		Count int
	}

	t.Run("ConvertToCustomType", func(t *testing.T) {
		customReturnInterceptor := func(ctx *ClientCtx[*pb.Int32Value, *pb.Int32Value]) (*CustomResult, error) {
			var total int32
			var count int
			for result := range ctx.Responses().IgnoreErrors() {
				total += result.Value.GetValue()
				count++
			}
			return &CustomResult{Total: int(total), Count: count}, nil
		}

		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.Int32(10), Err: nil},
			{NodeID: 2, Value: pb.Int32(20), Err: nil},
			{NodeID: 3, Value: pb.Int32(30), Err: nil},
		}
		clientCtx := makeClientCtx[*pb.Int32Value, *pb.Int32Value](t, 3, responses)

		result, err := customReturnInterceptor(clientCtx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if got, want := result.Total, 60; got != want {
			t.Errorf("Total = %d, want %d", got, want)
		}

		if got, want := result.Count, 3; got != want {
			t.Errorf("Count = %d, want %d", got, want)
		}
	})
}

// AggregateResult is a custom return type that aggregates all responses.
type AggregateResult struct {
	Values []string
	Count  int
}

// TestInterceptorIntegration_CustomReturnType demonstrates using QuorumCallWithInterceptor
// with a custom return type through a user-defined quorum function.
func TestInterceptorIntegration_CustomReturnType(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, echoServerFn)
	t.Cleanup(closeServers)

	// Define a custom quorum function that returns an AggregateResult
	aggregateQF := func(ctx *ClientCtx[*pb.StringValue, *pb.StringValue]) (*AggregateResult, error) {
		result := &AggregateResult{
			Values: make([]string, 0),
		}
		for resp := range ctx.Responses().IgnoreErrors() {
			result.Values = append(result.Values, resp.Value.GetValue())
			result.Count++
		}
		if result.Count < 2 { // require at least 2 responses
			return nil, errors.New("not enough responses")
		}
		return result, nil
	}

	ctx := testContext(t, ctxTimeout)
	// QuorumCallWithInterceptor infers Out = *AggregateResult from aggregateQF
	result, err := QuorumCallWithInterceptor(
		ctx,
		NewConfig(t, addrs),
		pb.String("custom"),
		mock.TestMethod,
		aggregateQF,
	)
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}
	if result.Count < 2 {
		t.Errorf("Expected at least 2 responses, got %d", result.Count)
	}
	// Each value should be "echo: custom" since echoServerFn prepends "echo: "
	for _, v := range result.Values {
		if v != "echo: custom" {
			t.Errorf("Expected 'echo: custom', got %q", v)
		}
	}
}

// Interceptor Integration Tests with Real Servers

// TestInterceptorIntegration_MajorityQuorum tests the complete flow with real servers
func TestInterceptorIntegration_MajorityQuorum(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, echoServerFn)
	t.Cleanup(closeServers)

	ctx := testContext(t, ctxTimeout)
	result, err := QuorumCallWithInterceptor(
		ctx,
		NewConfig(t, addrs),
		pb.String("test"),
		mock.TestMethod,
		MajorityQuorum[*pb.StringValue, *pb.StringValue],
	)
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}
	if got, want := result.GetValue(), "echo: test"; got != want {
		t.Errorf("Response = %q, want %q", got, want)
	}
}

// TestInterceptorIntegration_CustomAggregation tests custom aggregation with real servers
func TestInterceptorIntegration_CustomAggregation(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, nil)
	t.Cleanup(closeServers)

	ctx := testContext(t, ctxTimeout)
	result, err := QuorumCallWithInterceptor(
		ctx,
		NewConfig(t, addrs),
		pb.Int32(0),
		mock.GetValueMethod,
		sumInterceptor,
	)
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}
	// Expected: 10 + 20 + 30 = 60
	if result.GetValue() != 60 {
		t.Errorf("Expected sum of 60, got %d", result.GetValue())
	}
}

// TestInterceptorIntegration_Chaining tests chained interceptors with real servers
func TestInterceptorIntegration_Chaining(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, echoServerFn)
	t.Cleanup(closeServers)

	// Track interceptor execution
	tracker := &executionTracker{}

	ctx := testContext(t, ctxTimeout)
	result, err := QuorumCallWithInterceptor(
		ctx,
		NewConfig(t, addrs),
		pb.String("test"),
		mock.TestMethod,
		MajorityQuorum[*pb.StringValue, *pb.StringValue], // Base
		WithQuorumInterceptors(loggingInterceptor[*pb.StringValue, *pb.StringValue](tracker)),
	)
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}
	if result.GetValue() != "echo: test" {
		t.Errorf("Expected 'echo: test', got '%s'", result.GetValue())
	}

	// Verify interceptor execution order
	tracker.check(t, []string{"logging-before", "logging-after"})
}

// TestInterceptorCollectAllResponses tests the CollectAllResponses interceptor
func TestInterceptorCollectAllResponses(t *testing.T) {
	responses := []NodeResponse[proto.Message]{
		{NodeID: 1, Value: pb.String("response1"), Err: nil},
		{NodeID: 2, Value: pb.String("response2"), Err: nil},
		{NodeID: 3, Value: nil, Err: errors.New("error3")},
	}
	clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)

	result, err := CollectAllResponses(clientCtx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(result) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(result))
	}

	if _, ok := result[1]; !ok {
		t.Error("Expected response from node 1")
	}
	if _, ok := result[2]; !ok {
		t.Error("Expected response from node 2")
	}
	if _, ok := result[3]; !ok {
		t.Error("Expected response from node 3 (even if error)")
	}
}

// TestInterceptorIntegration_CollectAll tests CollectAllResponses with real servers
func TestInterceptorIntegration_CollectAll(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, func(i int) ServerIface {
		srv := NewServer()
		srv.RegisterHandler(mock.TestMethod, func(_ ServerCtx, in *Message) (*Message, error) {
			req := AsProto[*pb.StringValue](in)
			resp := pb.String(req.GetValue() + "-node-" + strconv.Itoa(i))
			return NewResponseMessage(in.GetMetadata(), resp), nil
		})
		return srv
	})
	t.Cleanup(closeServers)

	config := NewConfig(t, addrs)
	ctx := testContext(t, ctxTimeout)
	result, err := QuorumCallWithInterceptor(
		ctx,
		config,
		pb.String("test"),
		mock.TestMethod,
		CollectAllResponses[*pb.StringValue, *pb.StringValue],
	)
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(result))
	}

	// Verify we got responses from all nodes in the configuration
	for _, node := range config.Nodes() {
		if _, ok := result[node.ID()]; !ok {
			t.Errorf("Missing response from node %d", node.ID())
		}
	}
}

// TestInterceptorIntegration_PerNodeTransform tests per-node transformation with real servers
func TestInterceptorIntegration_PerNodeTransform(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, echoServerFn)
	t.Cleanup(closeServers)

	// Create a transform that sends different values to each node
	transformInterceptor := PerNodeTransform[*pb.StringValue, *pb.StringValue, map[uint32]*pb.StringValue](
		func(req *pb.StringValue, node *RawNode) *pb.StringValue {
			return pb.String(req.GetValue() + "-node-" + strconv.Itoa(int(node.ID())))
		},
	)

	ctx := testContext(t, ctxTimeout)
	result, err := QuorumCallWithInterceptor(
		ctx,
		NewConfig(t, addrs),
		pb.String("test"),
		mock.TestMethod,
		CollectAllResponses[*pb.StringValue, *pb.StringValue], // Base
		WithQuorumInterceptors(transformInterceptor),
	)
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(result))
	}

	// Verify each node received the transformed request
	for nodeID, resp := range result {
		expected := "echo: test-node-" + strconv.Itoa(int(nodeID))
		if resp.GetValue() != expected {
			t.Errorf("Node %d: expected %q, got %q", nodeID, expected, resp.GetValue())
		}
	}
}

// TestInterceptorIntegration_PerNodeTransformSkip tests skipping nodes in per-node transformation
func TestInterceptorIntegration_PerNodeTransformSkip(t *testing.T) {
	addrs, closeServers := TestSetup(t, 3, echoServerFn)
	t.Cleanup(closeServers)

	config := NewConfig(t, addrs)
	nodes := config.Nodes()

	// Skip the second node (index 1)
	skipNodeID := nodes[1].ID()

	// Create a transform that skips one node by returning an invalid message
	transformInterceptor := PerNodeTransform[*pb.StringValue, *pb.StringValue, map[uint32]*pb.StringValue](
		func(req *pb.StringValue, node *RawNode) *pb.StringValue {
			if node.ID() == skipNodeID {
				return nil // Skip this node
			}
			return pb.String(req.GetValue() + "-node-" + strconv.Itoa(int(node.ID())))
		},
	)

	ctx := testContext(t, ctxTimeout)
	result, err := QuorumCallWithInterceptor(
		ctx,
		config,
		pb.String("test"),
		mock.TestMethod,
		CollectAllResponses[*pb.StringValue, *pb.StringValue], // Base
		WithQuorumInterceptors(transformInterceptor),
	)
	if !checkQuorumCall(t, ctx.Err(), err) {
		return
	}
	if len(result) != 2 {
		t.Errorf("Expected 2 responses (one node skipped), got %d", len(result))
	}

	// Verify we got responses from nodes 0 and 2, but not 1
	if _, ok := result[nodes[0].ID()]; !ok {
		t.Errorf("Expected response from node %d", nodes[0].ID())
	}
	if _, ok := result[skipNodeID]; ok {
		t.Errorf("Did not expect response from skipped node %d", skipNodeID)
	}
	if _, ok := result[nodes[2].ID()]; !ok {
		t.Errorf("Expected response from node %d", nodes[2].ID())
	}
}

// TestBaseQuorumFunctions tests all base quorum functions (FirstResponse, AllResponses, ThresholdQuorum, MajorityQuorum)
func TestBaseQuorumFunctions(t *testing.T) {
	tests := []struct {
		name        string
		quorumFunc  QuorumFunc[*pb.StringValue, *pb.StringValue, *pb.StringValue]
		numNodes    int
		responses   []NodeResponse[proto.Message]
		wantErr     bool
		wantErrType error
		wantValue   string
	}{
		// FirstResponse tests
		{
			name:       "FirstResponse_Success",
			quorumFunc: FirstResponse[*pb.StringValue, *pb.StringValue],
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("first"), Err: nil},
				{NodeID: 2, Value: pb.String("second"), Err: nil},
				{NodeID: 3, Value: pb.String("third"), Err: nil},
			},
			wantErr:   false,
			wantValue: "first",
		},
		{
			name:       "FirstResponse_AfterErrors",
			quorumFunc: FirstResponse[*pb.StringValue, *pb.StringValue],
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: nil, Err: errors.New("error1")},
				{NodeID: 2, Value: pb.String("second"), Err: nil},
				{NodeID: 3, Value: pb.String("third"), Err: nil},
			},
			wantErr:   false,
			wantValue: "second",
		},
		{
			name:       "FirstResponse_AllErrors",
			quorumFunc: FirstResponse[*pb.StringValue, *pb.StringValue],
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: nil, Err: errors.New("error1")},
				{NodeID: 2, Value: nil, Err: errors.New("error2")},
				{NodeID: 3, Value: nil, Err: errors.New("error3")},
			},
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
		{
			name:        "FirstResponse_NoResponses",
			quorumFunc:  FirstResponse[*pb.StringValue, *pb.StringValue],
			responses:   []NodeResponse[proto.Message]{},
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},

		// AllResponses tests
		{
			name:       "AllResponses_AllSuccess",
			quorumFunc: AllResponses[*pb.StringValue, *pb.StringValue],
			numNodes:   3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("first"), Err: nil},
				{NodeID: 2, Value: pb.String("second"), Err: nil},
				{NodeID: 3, Value: pb.String("third"), Err: nil},
			},
			wantErr:   false,
			wantValue: "first",
		},
		{
			name:       "AllResponses_OneError",
			quorumFunc: AllResponses[*pb.StringValue, *pb.StringValue],
			numNodes:   3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("first"), Err: nil},
				{NodeID: 2, Value: nil, Err: errors.New("error2")},
				{NodeID: 3, Value: pb.String("third"), Err: nil},
			},
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},

		// MajorityQuorum tests
		{
			name:       "MajorityQuorum_Success",
			quorumFunc: MajorityQuorum[*pb.StringValue, *pb.StringValue],
			numNodes:   5,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
				{NodeID: 3, Value: pb.String("response3"), Err: nil},
				{NodeID: 4, Value: nil, Err: errors.New("error4")},
				{NodeID: 5, Value: nil, Err: errors.New("error5")},
			},
			wantErr:   false,
			wantValue: "response1",
		},
		{
			name:       "MajorityQuorum_Insufficient",
			quorumFunc: MajorityQuorum[*pb.StringValue, *pb.StringValue],
			numNodes:   5,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("response1"), Err: nil},
				{NodeID: 2, Value: pb.String("response2"), Err: nil},
				{NodeID: 3, Value: nil, Err: errors.New("error3")},
				{NodeID: 4, Value: nil, Err: errors.New("error4")},
				{NodeID: 5, Value: nil, Err: errors.New("error5")},
			},
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
		{
			name:       "MajorityQuorum_Exact",
			quorumFunc: MajorityQuorum[*pb.StringValue, *pb.StringValue],
			numNodes:   3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("first"), Err: nil},
				{NodeID: 2, Value: pb.String("second"), Err: nil},
				{NodeID: 3, Value: nil, Err: errors.New("error")},
			},
			wantErr:   false,
			wantValue: "first",
		},
		{
			name:       "MajorityQuorum_Even_Success",
			quorumFunc: MajorityQuorum[*pb.StringValue, *pb.StringValue],
			numNodes:   4,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("first"), Err: nil},
				{NodeID: 2, Value: pb.String("second"), Err: nil},
				{NodeID: 3, Value: pb.String("third"), Err: nil},
				{NodeID: 4, Value: nil, Err: errors.New("error4")},
			},
			wantErr:   false,
			wantValue: "first",
		},
		{
			name:       "MajorityQuorum_Even_Insufficient",
			quorumFunc: MajorityQuorum[*pb.StringValue, *pb.StringValue],
			numNodes:   4,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("first"), Err: nil},
				{NodeID: 2, Value: pb.String("second"), Err: nil},
				{NodeID: 3, Value: nil, Err: errors.New("error3")},
				{NodeID: 4, Value: nil, Err: errors.New("error4")},
			},
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},

		// ThresholdQuorum tests
		{
			name:       "ThresholdQuorum_Met",
			quorumFunc: ThresholdQuorum[*pb.StringValue, *pb.StringValue](2),
			numNodes:   3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("first"), Err: nil},
				{NodeID: 2, Value: pb.String("second"), Err: nil},
				{NodeID: 3, Value: nil, Err: errors.New("error3")},
			},
			wantErr:   false,
			wantValue: "first",
		},
		{
			name:       "ThresholdQuorum_NotMet",
			quorumFunc: ThresholdQuorum[*pb.StringValue, *pb.StringValue](3),
			numNodes:   3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("first"), Err: nil},
				{NodeID: 2, Value: nil, Err: errors.New("error2")},
				{NodeID: 3, Value: nil, Err: errors.New("error3")},
			},
			wantErr:     true,
			wantErrType: ErrIncomplete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numNodes := tt.numNodes
			if numNodes == 0 {
				numNodes = len(tt.responses)
			}
			clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, numNodes, tt.responses)

			result, err := tt.quorumFunc(clientCtx)

			if !checkError(t, tt.wantErr, err, tt.wantErrType) {
				return
			}

			if !tt.wantErr && result.GetValue() != tt.wantValue {
				t.Errorf("Expected '%s', got '%s'", tt.wantValue, result.GetValue())
			}
		})
	}
}

// TestInterceptorQuorumSpecAdapter tests the QuorumSpecInterceptor adapter
func TestInterceptorQuorumSpecAdapter(t *testing.T) {
	t.Run("AdapterWithMajorityQuorum", func(t *testing.T) {
		// Define a legacy-style quorum function
		qf := func(_ *pb.StringValue, replies map[uint32]*pb.StringValue) (*pb.StringValue, bool) {
			quorumSize := 2 // Majority of 3
			if len(replies) >= quorumSize {
				// Return first reply
				for _, v := range replies {
					return v, true
				}
			}
			return nil, false
		}

		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("first"), Err: nil},
			{NodeID: 2, Value: pb.String("second"), Err: nil},
			{NodeID: 3, Value: nil, Err: errors.New("error3")},
		}
		clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)

		// Convert to interceptor
		interceptor := QuorumSpecInterceptor(qf)
		result, err := interceptor(MajorityQuorum[*pb.StringValue, *pb.StringValue])(clientCtx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Should get one of the successful responses
		if result.GetValue() != "first" && result.GetValue() != "second" {
			t.Errorf("Expected 'first' or 'second', got '%s'", result.GetValue())
		}
	})

	t.Run("AdapterQuorumNotReached", func(t *testing.T) {
		// Define a quorum function that needs all 3 responses
		qf := func(_ *pb.StringValue, replies map[uint32]*pb.StringValue) (*pb.StringValue, bool) {
			if len(replies) == 3 {
				return pb.String("success"), true
			}
			return nil, false
		}

		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("first"), Err: nil},
			{NodeID: 2, Value: nil, Err: errors.New("error2")},
			{NodeID: 3, Value: nil, Err: errors.New("error3")},
		}
		clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)

		interceptor := QuorumSpecInterceptor(qf)

		_, err := interceptor(MajorityQuorum[*pb.StringValue, *pb.StringValue])(clientCtx)
		if err == nil {
			t.Error("Expected error when quorum not reached")
		}
	})
}

// TestInterceptorUsage demonstrates correct usage of interceptor chaining.
func TestInterceptorUsage(t *testing.T) {
	t.Run("CorrectUsage", func(t *testing.T) {
		// Demonstrate correct usage: transform followed by aggregator
		transform := func(req *pb.StringValue, _ *RawNode) *pb.StringValue {
			return pb.String(req.GetValue() + "-transformed")
		}

		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("response1"), Err: nil},
			{NodeID: 2, Value: pb.String("response2"), Err: nil},
			{NodeID: 3, Value: pb.String("response3"), Err: nil},
		}
		clientCtx := makeClientCtx[*pb.StringValue, *pb.StringValue](t, 3, responses)

		// Correct chain: transform -> aggregator (MajorityQuorum completes the call)
		handler := Chain(
			MajorityQuorum[*pb.StringValue, *pb.StringValue],
			PerNodeTransform[*pb.StringValue, *pb.StringValue, *pb.StringValue](transform),
		)

		result, err := handler(clientCtx)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if result == nil || result.GetValue() != "response1" {
			t.Errorf("Expected 'response1', got %v", result)
		}
	})
}
