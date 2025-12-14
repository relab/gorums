package gorums

import (
	"iter"

	"google.golang.org/protobuf/proto"
)

// msg is a type alias for proto.Message intended to be used as a type parameter.
type msg = proto.Message

// -------------------------------------------------------------------------
// Iterator Helpers
// -------------------------------------------------------------------------

// ResponseSeq is an iterator that yields NodeResponse[T] values from a quorum call.
type ResponseSeq[T msg] iter.Seq[NodeResponse[T]]

// IgnoreErrors returns an iterator that yields only successful responses,
// discarding any responses with errors. This is useful when you want to process
// only valid responses from nodes.
//
// Example:
//
//	responses := QuorumCall(ctx, Request_builder{Num: uint64(42)}.Build())
//	var sum int32
//	for resp := range responses.IgnoreErrors() {
//	    // resp is guaranteed to be a successful response
//		sum += resp.Value.GetValue()
//	}
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

// Filter returns an iterator that yields only the responses for which the
// provided keep function returns true. This is useful for verifying or filtering
// responses from servers before further processing.
//
// Example:
//
//	responses := QuorumCall(ctx, req)
//	// Filter to only responses from a specific node
//	for resp := range responses.Filter(func(r NodeResponse[Resp]) bool {
//		return r.NodeID == 1
//	}) {
//		// process resp
//	}
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

// CollectN collects up to n responses, including errors, from the iterator
// into a map by node ID. It returns early if n responses are collected or
// the iterator is exhausted.
//
// Example:
//
//	responses := QuorumCall(ctx, req)
//	// Collect the first 2 responses (including errors)
//	replies := responses.CollectN(2)
//	// or collect 2 successful responses
//	replies = responses.IgnoreErrors().CollectN(2)
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

// CollectAll collects all responses, including errors, from the iterator
// into a map by node ID.
//
// Example:
//
//	responses := QuorumCall(ctx, req)
//	// Collect all responses (including errors)
//	replies := responses.CollectAll()
//	// or collect all successful responses
//	replies = responses.IgnoreErrors().CollectAll()
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

// Responses provides access to quorum call responses and terminal methods.
// It is returned by quorum call functions and allows fluent-style API usage:
//
//	resp, err := ReadQuorumCall(ctx, req).Majority()
//	// or
//	resp, err := ReadQuorumCall(ctx, req).First()
//	// or
//	replies := ReadQuorumCall(ctx, req).IgnoreErrors().CollectAll()
//
// Type parameter:
//   - Resp: The response message type
type Responses[Resp msg] struct {
	ResponseSeq[Resp]
	size    int
	sendNow func() // sendNow triggers immediate sending of requests
}

func NewResponses[Req, Resp msg](ctx *clientCtx[Req, Resp]) *Responses[Resp] {
	return &Responses[Resp]{
		ResponseSeq: ctx.responseSeq,
		size:        ctx.Size(),
		sendNow:     func() { ctx.sendOnce.Do(ctx.send) },
	}
}

// Size returns the number of nodes in the configuration.
func (r *Responses[Resp]) Size() int {
	return r.size
}

// Seq returns the underlying response iterator for advanced use cases.
// Users can use this to implement custom aggregation logic.
//
// Note: This method triggers lazy sending of requests.
func (r *Responses[Resp]) Seq() ResponseSeq[Resp] {
	return r.ResponseSeq
}

// -------------------------------------------------------------------------
// Terminal Methods (Aggregators)
// -------------------------------------------------------------------------

// First returns the first successful response received from any node.
// This is useful for read-any patterns where any single response is sufficient.
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
func (r *Responses[Resp]) Threshold(threshold int) (resp Resp, err error) {
	var (
		count int
		errs  []nodeError
	)
	for result := range r.ResponseSeq {
		if result.Err != nil {
			errs = append(errs, nodeError{nodeID: result.NodeID, cause: result.Err})
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
	return resp, QuorumCallError{cause: ErrIncomplete, errors: errs}
}
