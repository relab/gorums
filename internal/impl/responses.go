package impl

import (
	"errors"
	"iter"

	"google.golang.org/protobuf/proto"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
)

// NodeResponse contains a node's response value or error.
type NodeResponse[T any] = stream.NodeResponse[T]

// mapToCallResponse converts a NodeResponse[*stream.Message] to a NodeResponse[Resp].
// This is necessary because the channel layer's response router returns a
// NodeResponse[*stream.Message] while the calltype expects a NodeResponse[Resp].
func mapToCallResponse[Resp proto.Message](channelResp NodeResponse[*stream.Message]) NodeResponse[Resp] {
	callResp := NodeResponse[Resp]{
		NodeID: channelResp.NodeID,
		Err:    channelResp.Err,
	}
	if channelResp.Err == nil {
		respMsg, err := unmarshalResponse(channelResp.Value)
		if err != nil {
			callResp.Err = err
		} else if val, ok := respMsg.(Resp); ok {
			callResp.Value = val
		} else {
			callResp.Err = stream.ErrTypeMismatch
		}
	}
	return callResp
}

// -------------------------------------------------------------------------
// Iterator Helpers
// -------------------------------------------------------------------------

// ResponseSeq yields the responses from a quorum call.
type ResponseSeq[T proto.Message] iter.Seq[NodeResponse[T]]

// IgnoreErrors returns a sequence containing only successful responses.
func (seq ResponseSeq[Resp]) IgnoreErrors() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		for result := range seq {
			if result.Err == nil {
				if !yield(result) {
					return
				}
			}
		}
	}
}

// Filter returns a sequence containing the responses for which keep returns true.
func (seq ResponseSeq[Resp]) Filter(keep func(NodeResponse[Resp]) bool) ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		for result := range seq {
			if keep(result) {
				if !yield(result) {
					return
				}
			}
		}
	}
}

// CollectN collects up to n values from the iterator into a map by node ID.
// It returns early if n entries are collected or the iterator is exhausted.
// When a node response carries an error, the zero value of Resp is stored for
// that node ID; use [ResponseSeq.IgnoreErrors] to skip errored nodes entirely.
func (seq ResponseSeq[Resp]) CollectN(n int) map[uint32]Resp {
	replies := make(map[uint32]Resp, n)
	for result := range seq {
		replies[result.NodeID] = result.Value
		if len(replies) >= n {
			break
		}
	}
	return replies
}

// CollectAll collects all values from the iterator into a map by node ID.
// When a node response carries an error, the zero value of Resp is stored for
// that node ID; use [ResponseSeq.IgnoreErrors] to skip errored nodes entirely.
func (seq ResponseSeq[Resp]) CollectAll() map[uint32]Resp {
	replies := make(map[uint32]Resp)
	for result := range seq {
		replies[result.NodeID] = result.Value
	}
	return replies
}

// -------------------------------------------------------------------------
// Response Methods
// -------------------------------------------------------------------------

// Responses provides response iteration and aggregation for a quorum call.
type Responses[Resp proto.Message] struct {
	seq   ResponseSeq[Resp]
	size  int
	start starter
}

type starter interface {
	sendNow()
	markDispatched()
}

// markDispatched marks the underlying call as dispatched without sending, so a
// later Intercept panics. Async and correctable calls use this before starting
// their goroutine.
func (r *Responses[Resp]) markDispatched() {
	r.start.markDispatched()
}

// newResponses builds the [Responses] handle returned by a quorum call from
// its [CallContext].
func newResponses[Req, Resp proto.Message](ctx *CallContext[Req, Resp]) *Responses[Resp] {
	return &Responses[Resp]{
		seq:   ctx.responseSeq,
		size:  ctx.Size(),
		start: ctx,
	}
}

// Size returns the number of nodes in the configuration.
func (r *Responses[Resp]) Size() int {
	return r.size
}

// Results returns a single-use sequence that yields responses as they arrive.
// Iteration dispatches the call and ends after every node responds or the
// context is canceled. Calling [Call.Intercept] after Results panics.
func (r *Responses[Resp]) Results() ResponseSeq[Resp] {
	r.markDispatched()
	return r.seq
}

// sendNow triggers immediate sending of requests.
func (r *Responses[Resp]) sendNow() {
	r.start.sendNow()
}

// -------------------------------------------------------------------------
// Terminal Methods (Aggregators)
// -------------------------------------------------------------------------

// First returns the first successful response.
func (r *Responses[Resp]) First() (Resp, error) {
	return r.Threshold(1)
}

// Majority returns the first response once a simple majority (⌈(n+1)/2⌉)
// of successful responses are received.
func (r *Responses[Resp]) Majority() (Resp, error) {
	quorumSize := r.size/2 + 1
	return r.Threshold(quorumSize)
}

// All returns the first response once all nodes have responded successfully.
// If any node fails, it returns an error.
func (r *Responses[Resp]) All() (Resp, error) {
	return r.Threshold(r.size)
}

// Threshold waits for a threshold number of successful responses.
// It returns the first response once the threshold is reached.
// A response skipped by a request transform ([ErrSkipNode]) counts toward
// neither the successful-response count nor the reported node errors.
func (r *Responses[Resp]) Threshold(threshold int) (resp Resp, err error) {
	var (
		count int
		errs  []conn.NodeError
	)
	for result := range r.seq {
		if errors.Is(result.Err, ErrSkipNode) {
			continue
		}
		if result.Err != nil {
			errs = append(errs, conn.NewNodeError(result.NodeID, result.Err))
			continue
		}
		if count == 0 {
			resp = result.Value
		}
		count++

		// Check if we have reached the threshold
		if count >= threshold {
			return resp, nil
		}
	}
	return resp, conn.NewQuorumCallError(ErrIncomplete, errs)
}
