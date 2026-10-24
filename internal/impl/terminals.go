package impl

import (
	"errors"
	"fmt"
	"slices"

	"github.com/relab/gorums/internal/conn"
	"google.golang.org/protobuf/proto"
)

// checkThreshold panics if the threshold n passed to method is not positive.
func checkThreshold(method string, n int) {
	if n <= 0 {
		panic(fmt.Sprintf("gorums: %s called with non-positive threshold %d", method, n))
	}
}

// nodeTally counts the nodes that contributed a successful response and
// retains the node errors for a possible ErrIncomplete result.
type nodeTally struct {
	counted []ID
	errs    []conn.NodeError
}

// add reports whether a response with the given node ID and error is the
// first successful response from that node. It records errors other than
// ErrSkipNode and the IDs of counted nodes.
func (t *nodeTally) add(id ID, err error) bool {
	if errors.Is(err, ErrSkipNode) {
		return false
	}
	if err != nil {
		t.errs = append(t.errs, conn.NewNodeError(id, err))
		return false
	}
	if slices.Contains(t.counted, id) {
		return false
	}
	t.counted = append(t.counted, id)
	return true
}

// incomplete returns a QuorumCallError caused by ErrIncomplete that holds the
// recorded node errors.
func (t *nodeTally) incomplete() error {
	return conn.NewQuorumCallError(ErrIncomplete, t.errs)
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
	acc := initial
	tally := nodeTally{counted: make([]ID, 0, min(n, r.size))}
	for result := range r.seq {
		if !tally.add(result.NodeID, result.Err) {
			continue
		}
		var err error
		if acc, err = step(acc, result); err != nil {
			return acc, err
		}
		if len(tally.counted) >= n {
			return acc, nil
		}
	}
	return acc, tally.incomplete()
}

// Certificate contains a representative value and the successful node
// responses that established it. Responses are retained in arrival order.
type Certificate[Resp proto.Message] struct {
	Value     Resp
	Responses []NodeResponse[Resp]
}

// Matching waits for successful responses from n distinct nodes that produce
// the same comparable key. It returns the first response belonging to the
// first key bucket that reaches n responses.
//
// Matching ignores repeated responses from an already-counted node. It returns
// a QuorumCallError caused by ErrIncomplete if no key reaches n responses.
//
// Keys compare by value: a pointer key, such as the response itself, never
// matches another response, and an interface key holding a slice panics.
//
// Matching panics if n is not positive or key is nil.
func (r *Responses[Resp]) Matching[Key comparable](
	n int,
	key func(Resp) Key,
) (Resp, error) {
	checkThreshold("Matching", n)
	if key == nil {
		panic("gorums: Matching called with nil key function")
	}
	cert, err := r.MatchingCertificate(n, key)
	return cert.Value, err
}

// MatchingCertificate waits for successful responses from n distinct nodes
// that produce the same comparable key. It returns the representative value
// and the responses that established the first matching certificate.
//
// The certificate contains only successful responses, ordered by arrival.
// MatchingCertificate ignores repeated responses from an already-counted
// node. It returns a QuorumCallError caused by ErrIncomplete if no key reaches
// n responses.
//
// Keys compare by value: a pointer key, such as the response itself, never
// matches another response, and an interface key holding a slice panics.
//
// MatchingCertificate panics if n is not positive or key is nil.
func (r *Responses[Resp]) MatchingCertificate[Key comparable](
	n int,
	key func(Resp) Key,
) (Certificate[Resp], error) {
	checkThreshold("MatchingCertificate", n)
	if key == nil {
		panic("gorums: MatchingCertificate called with nil key function")
	}
	var (
		tally   = nodeTally{counted: make([]ID, 0, r.size)}
		buckets map[Key][]NodeResponse[Resp]
	)
	for result := range r.seq {
		if !tally.add(result.NodeID, result.Err) {
			continue
		}
		if buckets == nil {
			buckets = make(map[Key][]NodeResponse[Resp])
		}
		k := key(result.Value)
		buckets[k] = append(buckets[k], result)
		if bucket := buckets[k]; len(bucket) >= n {
			return Certificate[Resp]{Value: bucket[0].Value, Responses: bucket}, nil
		}
	}
	return Certificate[Resp]{}, tally.incomplete()
}
