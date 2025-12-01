package gorums

import (
	"context"
	"iter"
	"slices"

	"google.golang.org/protobuf/proto"
)

// msg is a type alias for proto.Message intended to be used as a type parameter.
type msg = proto.Message

// QuorumInterceptor intercepts and processes quorum calls, allowing modification of
// requests, responses, and aggregation logic. Interceptors can be chained together.
//
// Type parameters:
//   - Req: The request message type sent to nodes
//   - Resp: The response message type from individual nodes
//
// The interceptor receives the internal clientCtx for request transformations
// and returns a Responses object for the user.
type QuorumInterceptor[Req, Resp msg] func(*clientCtx[Req, Resp])

// ResponseSeq is an iterator that yields NodeResponse[T] values from a quorum call.
type ResponseSeq[T msg] iter.Seq[NodeResponse[T]]

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
	responseSeq ResponseSeq[Resp]
	size        int
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
	return r.responseSeq
}

// IgnoreErrors returns a Responses that yields only successful responses,
// discarding any responses with errors.
func (r *Responses[Resp]) IgnoreErrors() *Responses[Resp] {
	r.responseSeq = r.responseSeq.IgnoreErrors()
	return r
}

// Filter returns a Responses that yields only the responses for which the
// provided keep function returns true.
func (r *Responses[Resp]) Filter(keep func(NodeResponse[Resp]) bool) *Responses[Resp] {
	r.responseSeq = r.responseSeq.Filter(keep)
	return r
}

// CollectN collects up to n responses, including errors, into a map by node ID.
// It returns early if n responses are collected or the iterator is exhausted.
func (r *Responses[Resp]) CollectN(n int) map[uint32]Resp {
	return r.responseSeq.CollectN(n)
}

// CollectAll collects all responses, including errors, into a map by node ID.
func (r *Responses[Resp]) CollectAll() map[uint32]Resp {
	return r.responseSeq.CollectAll()
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

	for result := range r.responseSeq {
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

		for result := range r.responseSeq {
			if result.Err != nil {
				errs = append(errs, nodeError{nodeID: result.NodeID, cause: result.Err})
				continue
			}

			count++
			lastResp = result.Value
			found = true

			// Check if we have reached the threshold
			done := count >= threshold
			corr.Update(lastResp, count, done, nil)
			if done {
				return
			}
		}

		// If we didn't reach the threshold, mark as done with error
		if !found {
			var zero Resp
			corr.Update(zero, count, true, QuorumCallError{cause: ErrIncomplete, errors: errs, replies: count})
		} else {
			corr.Update(lastResp, count, true, QuorumCallError{cause: ErrIncomplete, errors: errs, replies: count})
		}
	}()

	return corr
}

// clientCtx provides context and access to the quorum call state for interceptors.
// It exposes the request, configuration, and an iterator over node responses.
type clientCtx[Req, Resp msg] struct {
	context.Context
	config        RawConfiguration
	request       Req
	method        string
	replyChan     <-chan NodeResponse[msg]
	reqTransforms []func(Req, *RawNode) Req

	// expectedReplies is the number of responses we expect to receive.
	// It defaults to the configuration size but may be lower if nodes are skipped.
	expectedReplies int

	// sendOnce is called lazily on the first call to Responses().
	sendOnce func()

	// responseSeq is the iterator that yields node responses.
	// Interceptors can wrap this iterator to modify responses.
	responseSeq ResponseSeq[Resp]
}

// newClientCtx creates a new clientCtx for a quorum call.
func newClientCtx[Req, Resp msg](
	ctx context.Context,
	config RawConfiguration,
	req Req,
	method string,
	replyChan <-chan NodeResponse[msg],
) *clientCtx[Req, Resp] {
	c := &clientCtx[Req, Resp]{
		Context:         ctx,
		config:          config,
		request:         req,
		method:          method,
		replyChan:       replyChan,
		reqTransforms:   nil,
		expectedReplies: config.Size(),
	}
	c.responseSeq = c.defaultResponseSeq()
	return c
}

// -------------------------------------------------------------------------
// clientCtx Methods
// -------------------------------------------------------------------------

// Request returns the original request message for this quorum call.
func (c clientCtx[Req, Resp]) Request() Req {
	return c.request
}

// Config returns the configuration (set of nodes) for this quorum call.
func (c clientCtx[Req, Resp]) Config() RawConfiguration {
	return c.config
}

// Method returns the name of the RPC method being called.
func (c clientCtx[Req, Resp]) Method() string {
	return c.method
}

// Nodes returns the slice of nodes in this configuration.
func (c clientCtx[Req, Resp]) Nodes() []*RawNode {
	return c.config.Nodes()
}

// Node returns the node with the given ID.
func (c clientCtx[Req, Resp]) Node(id uint32) *RawNode {
	nodes := c.config.Nodes()
	index := slices.IndexFunc(nodes, func(n *RawNode) bool {
		return n.ID() == id
	})
	if index != -1 {
		return nodes[index]
	}
	return nil
}

// Size returns the number of nodes in this configuration.
func (c clientCtx[Req, Resp]) Size() int {
	return c.config.Size()
}

// RegisterTransformFunc registers a transformation function to modify the request message
// for a specific node. Transformation functions are applied in the order they are registered.
//
// This method is intended to be used by interceptors to modify requests before they are sent.
// It must be called before the first call to Responses(), which triggers the actual sending.
func (c *clientCtx[Req, Resp]) RegisterTransformFunc(fn func(Req, *RawNode) Req) {
	c.reqTransforms = append(c.reqTransforms, fn)
}

// applyTransforms returns the transformed request as a proto.Message, or nil if the result is
// invalid or the node should be skipped. It applies the registered transformation functions to
// the given request for the specified node. Transformation functions are applied in the order
// they were registered.
func (c *clientCtx[Req, Resp]) applyTransforms(req Req, node *RawNode) proto.Message {
	result := req
	for _, transform := range c.reqTransforms {
		result = transform(result, node)
	}
	if protoMsg, ok := any(result).(proto.Message); ok {
		if protoMsg.ProtoReflect().IsValid() {
			return protoMsg
		}
	}
	return nil
}

// Responses returns an iterator that yields node responses as they arrive.
// It returns a single-use iterator.
//
// Messages are not sent to nodes before ctx.Responses() is called, applying any
// registered request transformations. This lazy sending is necessary to allow
// interceptors to register transformations prior to dispatch.
//
// The iterator will:
//   - Yield responses as they arrive from nodes
//   - Continue until the context is canceled or all expected responses are received
//   - Allow early termination by breaking from the range loop
//
// Example usage:
//
//	for result := range ctx.Responses() {
//	    if result.Err != nil {
//	        // Handle node error
//	        continue
//	    }
//	    // Process result.Value
//	}
func (c *clientCtx[Req, Resp]) Responses() ResponseSeq[Resp] {
	return c.responseSeq
}

// defaultResponseSeq returns an iterator that yields at most c.expectedReplies responses.
func (c *clientCtx[Req, Resp]) defaultResponseSeq() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		// Trigger lazy sending
		if c.sendOnce != nil {
			c.sendOnce()
		}
		// Wait for at most c.expectedReplies
		for range c.expectedReplies {
			select {
			case r := <-c.replyChan:
				res := newNodeResponse[Resp](r)
				if !yield(res) {
					return // Consumer stopped iteration
				}
			case <-c.Done():
				return // Context canceled
			}
		}
	}
}

// streamingResponseSeq returns an iterator that yields responses as they arrive from nodes
// until the context is canceled or breaking from the range loop.
func (c *clientCtx[Req, Resp]) streamingResponseSeq() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		// Trigger lazy sending
		if c.sendOnce != nil {
			c.sendOnce()
		}
		for {
			select {
			case r := <-c.replyChan:
				res := newNodeResponse[Resp](r)
				if !yield(res) {
					return // Consumer stopped iteration
				}
			case <-c.Done():
				return // Context canceled
			}
		}
	}
}

// -------------------------------------------------------------------------
// Iterator Helpers
// -------------------------------------------------------------------------

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

// -------------------------------------------------------------------------
// Interceptors (Middleware)
// -------------------------------------------------------------------------

// MapRequest returns an interceptor that applies per-node request transformations.
//
// The fn receives the original request and a node, and returns the transformed
// request to send to that node. If the function returns an invalid message or nil,
// the request to that node is skipped.
func MapRequest[Req, Resp msg](fn func(Req, *RawNode) Req) QuorumInterceptor[Req, Resp] {
	return func(ctx *clientCtx[Req, Resp]) {
		if fn != nil {
			ctx.RegisterTransformFunc(fn)
		}
	}
}

// MapResponse returns an interceptor that applies per-node response transformations.
//
// The fn receives the response from a node and the node itself, and returns the
// transformed response.
func MapResponse[Req, Resp msg](fn func(Resp, *RawNode) Resp) QuorumInterceptor[Req, Resp] {
	return func(ctx *clientCtx[Req, Resp]) {
		if fn != nil {
			// Wrap the existing response iterator with the transformation logic.
			// We capture the current iterator (oldResponses) and replace it with a new one
			// that applies fn to each successful response.
			oldResponses := ctx.responseSeq
			ctx.responseSeq = func(yield func(NodeResponse[Resp]) bool) {
				for resp := range oldResponses {
					// We only apply the transformation if there is no error.
					// Errors are passed through as-is.
					if resp.Err == nil {
						if node := ctx.Node(resp.NodeID); node != nil {
							resp.Value = fn(resp.Value, node)
						}
					}
					if !yield(resp) {
						return
					}
				}
			}
		}
	}
}

// Map returns an interceptor that applies per-node request and response transformations.
//
// The reqFunc receives the original request and a node, and returns the transformed
// request to send to that node. If the function returns an invalid message or nil,
// the request to that node is skipped.
//
// The respFunc receives the response from a node and the node itself, and returns the
// transformed response.
//
// Multiple Map interceptors can be chained together, with transforms applied in order.
//
// Example:
//
//	interceptor := Map(
//	    func(req *Request, node *gorums.RawNode) *Request {
//	        // Send different shard to each node
//	        return &Request{Shard: int(node.ID())}
//	    },
//	    func(resp *Response, node *gorums.RawNode) *Response {
//	        // Add node ID to response
//	        resp.NodeID = node.ID()
//	        return resp
//	    },
//	)
func Map[Req, Resp msg](
	reqFunc func(Req, *RawNode) Req,
	respFunc func(Resp, *RawNode) Resp,
) QuorumInterceptor[Req, Resp] {
	return func(ctx *clientCtx[Req, Resp]) {
		MapRequest[Req, Resp](reqFunc)(ctx)
		MapResponse[Req, Resp](respFunc)(ctx)
	}
}
