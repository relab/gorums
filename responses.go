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
//	for resp := range ctx.Responses().IgnoreErrors() {
//	    // resp is guaranteed to be a successful response
//	    process(resp)
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
func (seq ResponseSeq[Resp]) CollectAll() map[uint32]Resp {
	replies := make(map[uint32]Resp)
	for result := range seq {
		replies[result.NodeID] = result.Value
	}
	return replies
}

// Responses provides access to quorum call responses and terminal methods.
// It is returned by quorum call functions and allows fluent-style API usage:
//
//	resp, err := ReadQC(ctx, req).Majority()
//	// or
//	resp, err := ReadQC(ctx, req).First()
//	// or
//	replies := ReadQC(ctx, req).IgnoreErrors().CollectAll()
//
// Type parameter:
//   - Resp: The response message type from individual nodes
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
func (r *Responses[Resp]) Threshold(threshold int) (Resp, error) {
	var (
		firstResp Resp
		found     bool
		count     int
		errs      []nodeError
	)

	for result := range r.ResponseSeq {
		if result.Err != nil {
			errs = append(errs, nodeError{nodeID: result.NodeID, cause: result.Err})
			continue
		}

		count++
		if !found {
			firstResp = result.Value
			found = true
		}

		// Check if we have reached the threshold
		if count >= threshold {
			return firstResp, nil
		}
	}

	var zero Resp
	return zero, QuorumCallError{cause: ErrIncomplete, errors: errs, replies: count}
}

// WaitForLevel returns a Correctable that provides progressive updates
// as responses arrive. The level increases with each successful response.
// Use this for correctable quorum patterns where you want to observe
// intermediate states.
//
// Example:
//
//	corr := ReadQC(ctx, req).WaitForLevel(2)
//	// Wait for level 2 to be reached
//	<-corr.Watch(2)
//	resp, level, err := corr.Get()
func (r *Responses[Resp]) WaitForLevel(threshold int) *Correctable[Resp] {
	corr := &Correctable[Resp]{
		level:  LevelNotSet,
		donech: make(chan struct{}, 1),
	}

	go func() {
		var (
			lastResp Resp
			found    bool
			count    int
			errs     []nodeError
		)

		for result := range r.ResponseSeq {
			if result.Err != nil {
				errs = append(errs, nodeError{nodeID: result.NodeID, cause: result.Err})
				continue
			}

			count++
			lastResp = result.Value
			found = true

			// Check if we have reached the threshold
			done := count >= threshold
			corr.update(lastResp, count, done, nil)
			if done {
				return
			}
		}

		// If we didn't reach the threshold, mark as done with error
		if !found {
			var zero Resp
			corr.update(zero, count, true, QuorumCallError{cause: ErrIncomplete, errors: errs, replies: count})
		} else {
			corr.update(lastResp, count, true, QuorumCallError{cause: ErrIncomplete, errors: errs, replies: count})
		}
	}()

	return corr
}
