package impl

import (
	"errors"
	"fmt"
	"slices"

	"github.com/relab/gorums/internal/conn"
)

// checkThreshold panics if the threshold n passed to method is not positive.
func checkThreshold(method string, n int) {
	if n <= 0 {
		panic(fmt.Sprintf("gorums: %s called with non-positive threshold %d", method, n))
	}
}

// Aggregate applies fn to the single-use response sequence and returns its
// result. It supports reusable aggregation functions whose result type differs
// from the response type.
//
// Consuming the sequence passed to fn triggers dispatch. The aggregation
// function is responsible for interpreting individual response errors and
// deciding when to stop consuming responses.
//
// Aggregate panics if fn is nil.
func (r *Responses[Resp]) Aggregate[Out any](
	fn func(ResponseSeq[Resp]) (Out, error),
) (Out, error) {
	if fn == nil {
		panic("gorums: Aggregate called with nil function")
	}
	return fn(r.Results())
}

// Reduce folds successful responses from up to n distinct nodes into an
// accumulator and returns the resulting accumulator.
//
// Acc is caller-defined aggregation state and is independent of Resp. It may,
// for example, be a response selected so far, a scalar counter, a protocol
// state struct, a map, or a certificate builder. The initial value is passed
// to the first invocation of step, and the accumulator returned by each
// invocation is passed to the next.
//
// The NodeResponse passed to step is always successful, but retains its NodeID
// so the accumulator can record evidence. Responses containing errors and
// repeated responses from an already-counted node are not passed to step.
// Non-skip node errors are retained for a possible ErrIncomplete result.
//
// Reduce stops after folding n successful responses. If step returns an error,
// Reduce returns the accumulator returned by that invocation and the same
// error. If fewer than n successful responses are available, Reduce returns
// the latest accumulator and a QuorumCallError caused by ErrIncomplete.
//
// Reduce panics if n is not positive or step is nil.
func (r *Responses[Resp]) Reduce[Acc any](
	n int,
	initial Acc,
	step func(Acc, NodeResponse[Resp]) (Acc, error),
) (Acc, error) {
	checkThreshold("Reduce", n)
	if step == nil {
		panic("gorums: Reduce called with nil step function")
	}
	var (
		acc     = initial
		counted = make([]ID, 0, min(n, r.size))
		errs    []conn.NodeError
	)
	for result := range r.seq {
		if errors.Is(result.Err, ErrSkipNode) {
			continue
		}
		if result.Err != nil {
			errs = append(errs, conn.NewNodeError(result.NodeID, result.Err))
			continue
		}
		if slices.Contains(counted, result.NodeID) {
			continue
		}
		counted = append(counted, result.NodeID)

		var err error
		if acc, err = step(acc, result); err != nil {
			return acc, err
		}
		if len(counted) >= n {
			return acc, nil
		}
	}
	return acc, conn.NewQuorumCallError(ErrIncomplete, errs)
}
