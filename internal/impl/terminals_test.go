package impl

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/conn"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// checkPanic calls fn and reports an error unless fn panics with want.
func checkPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Errorf("did not panic, want panic %q", want)
			return
		}
		if got, ok := r.(string); !ok || got != want {
			t.Errorf("panic = %v, want %q", r, want)
		}
	}()
	fn()
}

func TestTerminalsAggregate(t *testing.T) {
	responses := []NodeResponse[proto.Message]{
		{NodeID: 1, Value: pb.String("a")},
		{NodeID: 2, Err: errors.New("node error")},
		{NodeID: 3, Value: pb.String("c")},
	}
	newTestResponses := func(t *testing.T) *Responses[*pb.StringValue] {
		t.Helper()
		return newResponses(makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, responses))
	}

	t.Run("IntResult", func(t *testing.T) {
		got, err := newTestResponses(t).Aggregate(func(seq ResponseSeq[*pb.StringValue]) (int, error) {
			count := 0
			for range seq.IgnoreErrors() {
				count++
			}
			return count, nil
		})
		if err != nil {
			t.Fatalf("Aggregate() error = %v, want nil", err)
		}
		if got != 2 {
			t.Errorf("Aggregate() = %d, want 2", got)
		}
	})

	t.Run("StringResult", func(t *testing.T) {
		got, err := newTestResponses(t).Aggregate(func(seq ResponseSeq[*pb.StringValue]) (string, error) {
			var b strings.Builder
			for resp := range seq.IgnoreErrors() {
				b.WriteString(resp.Value.GetValue())
			}
			return b.String(), nil
		})
		if err != nil {
			t.Fatalf("Aggregate() error = %v, want nil", err)
		}
		if got != "ac" {
			t.Errorf("Aggregate() = %q, want %q", got, "ac")
		}
	})

	t.Run("ErrorUnchanged", func(t *testing.T) {
		errAggregate := errors.New("aggregate error")
		_, err := newTestResponses(t).Aggregate(func(ResponseSeq[*pb.StringValue]) (int, error) {
			return 0, errAggregate
		})
		if err != errAggregate {
			t.Errorf("Aggregate() error = %v, want %v", err, errAggregate)
		}
	})

	t.Run("NilFunctionPanics", func(t *testing.T) {
		r := newTestResponses(t)
		var fn func(ResponseSeq[*pb.StringValue]) (int, error)
		checkPanic(t, "gorums: Aggregate called with nil function", func() {
			_, _ = r.Aggregate(fn)
		})
	})

	t.Run("InterceptAfterAggregatePanics", func(t *testing.T) {
		callCtx := makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, responses)
		call := &Call[*pb.StringValue, *pb.StringValue]{Responses: newResponses(callCtx), ctx: callCtx}
		_, _ = call.Aggregate(func(ResponseSeq[*pb.StringValue]) (int, error) {
			return 0, nil
		})
		checkPanic(t, "gorums: Intercept called after the call was dispatched", func() {
			call.Intercept()
		})
	})
}

// reduceAcc records the node IDs and values passed to a Reduce step function.
type reduceAcc struct {
	ids    []ID
	values []string
}

func TestTerminalsReduce(t *testing.T) {
	errNode := errors.New("node error")
	errStep := errors.New("step error")
	tests := []struct {
		name      string
		numNodes  int
		responses []NodeResponse[proto.Message]
		n         int
		failAt    int // step returns errStep when it folds this many responses; 0 never fails
		wantIDs   []ID
		wantVals  []string
		wantErr   error
		wantNodes int // number of node errors in the QuorumCallError
		wantNotIs error
	}{
		{
			name:     "StopsAfterN",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("a")},
				{NodeID: 2, Value: pb.String("b")},
				{NodeID: 3, Value: pb.String("c")},
			},
			n:        2,
			wantIDs:  []ID{1, 2},
			wantVals: []string{"a", "b"},
		},
		{
			name:     "SkipsErrorsBeforeN",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Err: errNode},
				{NodeID: 2, Value: pb.String("b")},
				{NodeID: 3, Value: pb.String("c")},
			},
			n:        2,
			wantIDs:  []ID{2, 3},
			wantVals: []string{"b", "c"},
		},
		{
			name:     "StepError",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("a")},
				{NodeID: 2, Value: pb.String("b")},
				{NodeID: 3, Value: pb.String("c")},
			},
			n:        3,
			failAt:   2,
			wantIDs:  []ID{1, 2},
			wantVals: []string{"a", "b"},
			wantErr:  errStep,
		},
		{
			name:     "StepErrorOnNthFoldWins",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("a")},
				{NodeID: 2, Value: pb.String("b")},
				{NodeID: 3, Value: pb.String("c")},
			},
			n:        2,
			failAt:   2,
			wantIDs:  []ID{1, 2},
			wantVals: []string{"a", "b"},
			wantErr:  errStep,
		},
		{
			name:     "ErrorThenSuccessFromSameNodeCounted",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Err: errNode},
				{NodeID: 1, Value: pb.String("a")},
				{NodeID: 2, Value: pb.String("b")},
			},
			n:        2,
			wantIDs:  []ID{1, 2},
			wantVals: []string{"a", "b"},
		},
		{
			name:     "DuplicateNodeIDFoldedOnce",
			numNodes: 4,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("a")},
				{NodeID: 1, Value: pb.String("a2")},
				{NodeID: 2, Value: pb.String("b")},
			},
			n:        2,
			wantIDs:  []ID{1, 2},
			wantVals: []string{"a", "b"},
		},
		{
			name:     "SkipNodeNotCountedOrReported",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Err: ErrSkipNode},
				{NodeID: 2, Value: pb.String("b")},
				{NodeID: 3, Err: errNode},
			},
			n:         2,
			wantIDs:   []ID{2},
			wantVals:  []string{"b"},
			wantErr:   ErrIncomplete,
			wantNodes: 1,
			wantNotIs: ErrSkipNode,
		},
		{
			name:     "Incomplete",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("a")},
				{NodeID: 2, Err: errNode},
				{NodeID: 3, Err: errNode},
			},
			n:         2,
			wantIDs:   []ID{1},
			wantVals:  []string{"a"},
			wantErr:   ErrIncomplete,
			wantNodes: 2,
		},
		{
			name:     "NLargerThanSize",
			numNodes: 3,
			responses: []NodeResponse[proto.Message]{
				{NodeID: 1, Value: pb.String("a")},
				{NodeID: 2, Value: pb.String("b")},
				{NodeID: 3, Value: pb.String("c")},
			},
			n:        5,
			wantIDs:  []ID{1, 2, 3},
			wantVals: []string{"a", "b", "c"},
			wantErr:  ErrIncomplete,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newResponses(makeCallContext[*pb.StringValue, *pb.StringValue](t, tt.numNodes, tt.responses))
			got, err := r.Reduce(tt.n, reduceAcc{}, func(acc reduceAcc, resp NodeResponse[*pb.StringValue]) (reduceAcc, error) {
				if resp.Err != nil {
					t.Errorf("step got response with error %v", resp.Err)
				}
				acc.ids = append(acc.ids, resp.NodeID)
				acc.values = append(acc.values, resp.Value.GetValue())
				if len(acc.ids) == tt.failAt {
					return acc, errStep
				}
				return acc, nil
			})
			if !slices.Equal(got.ids, tt.wantIDs) {
				t.Errorf("Reduce() ids = %v, want %v", got.ids, tt.wantIDs)
			}
			if !slices.Equal(got.values, tt.wantVals) {
				t.Errorf("Reduce() values = %v, want %v", got.values, tt.wantVals)
			}
			switch tt.wantErr {
			case nil:
				if err != nil {
					t.Fatalf("Reduce() error = %v, want nil", err)
				}
				return
			case errStep:
				if err != errStep {
					t.Fatalf("Reduce() error = %v, want %v", err, errStep)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Reduce() error = %v, want %v", err, tt.wantErr)
			}
			var qcErr conn.QuorumCallError
			if !errors.As(err, &qcErr) {
				t.Fatalf("Reduce() error = %v, want a QuorumCallError", err)
			}
			if got := qcErr.NumErrors(); got != tt.wantNodes {
				t.Errorf("NumErrors() = %d, want %d", got, tt.wantNodes)
			}
			if tt.wantNodes > 0 && !errors.Is(err, errNode) {
				t.Errorf("Reduce() error = %v, want it to wrap %v", err, errNode)
			}
			if tt.wantNotIs != nil && errors.Is(err, tt.wantNotIs) {
				t.Errorf("Reduce() error = %v, want it not to wrap %v", err, tt.wantNotIs)
			}
		})
	}

	t.Run("IntCounter", func(t *testing.T) {
		responses := []NodeResponse[proto.Message]{
			{NodeID: 1, Value: pb.String("a")},
			{NodeID: 2, Err: errNode},
			{NodeID: 3, Value: pb.String("c")},
		}
		r := newResponses(makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, responses))
		got, err := r.Reduce(2, 10, func(count int, _ NodeResponse[*pb.StringValue]) (int, error) {
			return count + 1, nil
		})
		if err != nil {
			t.Fatalf("Reduce() error = %v, want nil", err)
		}
		if got != 12 {
			t.Errorf("Reduce() = %d, want 12", got)
		}
	})

	panicTests := []struct {
		name string
		n    int
		step func(int, NodeResponse[*pb.StringValue]) (int, error)
		want string
	}{
		{name: "ZeroThreshold", n: 0, step: func(c int, _ NodeResponse[*pb.StringValue]) (int, error) { return c, nil }, want: "gorums: Reduce called with non-positive threshold 0"},
		{name: "NegativeThreshold", n: -1, step: func(c int, _ NodeResponse[*pb.StringValue]) (int, error) { return c, nil }, want: "gorums: Reduce called with non-positive threshold -1"},
		{name: "NilStep", n: 1, step: nil, want: "gorums: Reduce called with nil step function"},
	}
	for _, tt := range panicTests {
		t.Run(tt.name+"Panics", func(t *testing.T) {
			r := newResponses(makeCallContext[*pb.StringValue, *pb.StringValue](t, 3, nil))
			checkPanic(t, tt.want, func() {
				_, _ = r.Reduce(tt.n, 0, tt.step)
			})
		})
	}
}
