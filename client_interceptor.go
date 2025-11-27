package gorums

import (
	"context"
	"iter"
	"sync"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// msg is a type alias for proto.Message intended to be used as a type parameter.
type msg = proto.Message

// QuorumInterceptor intercepts and processes quorum calls, allowing modification of
// requests, responses, and aggregation logic. Interceptors can be chained together,
// with each interceptor wrapping a QuorumFunc.
//
// Type parameters:
//   - Req: The request message type sent to nodes
//   - Resp: The response message type from individual nodes
//   - Out: The final output type returned by the interceptor chain
//
// The Out type parameter allows interceptors to transform responses to a type different
// from the original response type specified in the service method's definition.
type QuorumInterceptor[Req, Resp msg, Out any] func(QuorumFunc[Req, Resp, Out]) QuorumFunc[Req, Resp, Out]

// QuorumFunc processes a quorum call and returns the aggregated result.
// This is the function type that interceptors call to continue the chain.
//
// In a chain of interceptors, the final QuorumFunc is the base quorum function
// (e.g. MajorityQuorum) that actually collects and aggregates responses.
type QuorumFunc[Req, Resp msg, Out any] func(*ClientCtx[Req, Resp]) (Out, error)

// Results is an iterator that yields Result[T] values from a quorum call.
type Results[T msg] iter.Seq[Result[T]]

// ClientCtx provides context and access to the quorum call state for interceptors.
// It exposes the request, configuration, and an iterator over node responses.
type ClientCtx[Req, Resp msg] struct {
	context.Context
	config        RawConfiguration
	request       Req
	method        string
	replyChan     <-chan Result[msg]
	reqTransforms []func(Req, *RawNode) Req

	// sendOnce is called lazily on the first call to Responses().
	sendOnce func()
}

// newClientCtx creates a new ClientCtx for a quorum call.
func newClientCtx[Req, Resp msg](
	ctx context.Context,
	config RawConfiguration,
	req Req,
	method string,
	replyChan <-chan Result[msg],
) *ClientCtx[Req, Resp] {
	return &ClientCtx[Req, Resp]{
		Context:       ctx,
		config:        config,
		request:       req,
		method:        method,
		replyChan:     replyChan,
		reqTransforms: nil,
	}
}

// -------------------------------------------------------------------------
// ClientCtx Methods
// -------------------------------------------------------------------------

// Request returns the original request message for this quorum call.
func (c ClientCtx[Req, Resp]) Request() Req {
	return c.request
}

// Config returns the configuration (set of nodes) for this quorum call.
func (c ClientCtx[Req, Resp]) Config() RawConfiguration {
	return c.config
}

// Method returns the name of the RPC method being called.
func (c ClientCtx[Req, Resp]) Method() string {
	return c.method
}

// Nodes returns the slice of nodes in this configuration.
func (c ClientCtx[Req, Resp]) Nodes() []*RawNode {
	return c.config.Nodes()
}

// Size returns the number of nodes in this configuration.
func (c ClientCtx[Req, Resp]) Size() int {
	return c.config.Size()
}

// RegisterTransformFunc registers a transformation function to modify the request message
// for a specific node. Transformation functions are applied in the order they are registered.
//
// This method is intended to be used by interceptors to modify requests before they are sent.
// It must be called before the first call to Responses(), which triggers the actual sending.
func (c *ClientCtx[Req, Resp]) RegisterTransformFunc(fn func(Req, *RawNode) Req) {
	c.reqTransforms = append(c.reqTransforms, fn)
}

// applyTransforms returns the transformed request as a proto.Message, or nil if the result is
// invalid or the node should be skipped. It applies the registered transformation functions to
// the given request for the specified node. Transformation functions are applied in the order
// they were registered.
func (c *ClientCtx[Req, Resp]) applyTransforms(req Req, node *RawNode) proto.Message {
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
func (c *ClientCtx[Req, Resp]) Responses() Results[Resp] {
	// Trigger lazy sending
	if c.sendOnce != nil {
		c.sendOnce()
	}
	return func(yield func(Result[Resp]) bool) {
		// Wait for at most c.Size() responses
		for range c.Size() {
			select {
			case r := <-c.replyChan:
				// We get a Result[proto.Message] from the channel layer's
				// response router; however, we convert it to Result[Resp]
				// here to match the calltype's expected response type.
				res := Result[Resp]{
					NodeID: r.NodeID,
					Err:    r.Err,
				}
				if r.Err == nil {
					if val, ok := r.Value.(Resp); ok {
						res.Value = val
					} else {
						res.Err = ErrTypeMismatch
					}
				}
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
func (seq Results[Resp]) IgnoreErrors() Results[Resp] {
	return func(yield func(Result[Resp]) bool) {
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
func (seq Results[Resp]) Filter(keep func(Result[Resp]) bool) Results[Resp] {
	return func(yield func(Result[Resp]) bool) {
		for result := range seq {
			if keep(result) {
				if !yield(result) {
					return
				}
			}
		}
	}
}

// CollectN collects up to n responses from the iterator into a map by node ID.
// It includes both successful and error responses.
// It returns early if n responses are collected or the iterator is exhausted.
func (seq Results[Resp]) CollectN(n int) map[uint32]Resp {
	replies := make(map[uint32]Resp, n)
	for result := range seq {
		replies[result.NodeID] = result.Value
		if len(replies) >= n {
			break
		}
	}
	return replies
}

// CollectAll collects all responses from the iterator into a map by node ID.
// It includes both successful and error responses.
func (seq Results[Resp]) CollectAll() map[uint32]Resp {
	replies := make(map[uint32]Resp)
	for result := range seq {
		replies[result.NodeID] = result.Value
	}
	return replies
}

// -------------------------------------------------------------------------
// Interceptors (Middleware)
// -------------------------------------------------------------------------

// PerNodeTransform returns an interceptor that applies per-node request transformations.
//
// The transform function receives the original request and a node, and returns the transformed
// request to send to that node. If the function returns an invalid message or nil, the request to
// that node is skipped.
//
// Multiple PerNodeTransform interceptors can be chained together, with transforms applied in order.
//
// Example:
//
//	interceptor := PerNodeTransform(func(req *Request, node *gorums.RawNode) *Request {
//	    // Send different shard to each node
//	    return &Request{Shard: int(node.ID())}
//	})
func PerNodeTransform[Req, Resp msg, Out any](transform func(Req, *RawNode) Req) QuorumInterceptor[Req, Resp, Out] {
	return func(next QuorumFunc[Req, Resp, Out]) QuorumFunc[Req, Resp, Out] {
		return func(ctx *ClientCtx[Req, Resp]) (Out, error) {
			ctx.RegisterTransformFunc(transform)
			return next(ctx)
		}
	}
}

// QuorumSpecInterceptor returns an interceptor that wraps a legacy QuorumSpec-style quorum function.
// This adapter allows gradual migration from the legacy QuorumCall approach to interceptors.
//
// The quorum function receives the original request and a map of replies, and returns
// the aggregated result and a boolean indicating whether quorum was reached.
//
// Note: This is a terminal handler that collects all responses itself. Any base quorum function
// passed when using this interceptor will be ignored.
//
// Example:
//
//	// Legacy QuorumSpec function
//	qf := func(req *Request, replies map[uint32]*Response) (*Result, bool) {
//	    if len(replies) > len(config)/2 {
//	        return replies[0], true
//	    }
//	    return nil, false
//	}
//
//	// Convert to interceptor
//	interceptor := QuorumSpecInterceptor(qf)
func QuorumSpecInterceptor[Req, Resp msg, Out any](
	qf func(Req, map[uint32]Resp) (Out, bool),
) QuorumInterceptor[Req, Resp, Out] {
	return func(_ QuorumFunc[Req, Resp, Out]) QuorumFunc[Req, Resp, Out] {
		return func(ctx *ClientCtx[Req, Resp]) (Out, error) {
			replies := ctx.Responses().IgnoreErrors().CollectAll()
			resp, ok := qf(ctx.Request(), replies)
			if !ok {
				var zero Out
				return zero, QuorumCallError{cause: ErrIncomplete, replies: len(replies)}
			}
			return resp, nil
		}
	}
}

// -------------------------------------------------------------------------
// Base Quorum Functions (Aggregators)
// -------------------------------------------------------------------------

// ThresholdQuorum returns a QuorumFunc that waits for a threshold number of
// successful responses. It returns the first response once the threshold is reached.
//
// This is a base quorum function that terminates the interceptor chain.
//
// Example:
//
//	// Create a quorum that needs 2 out of 3 responses
//	qf := ThresholdQuorum[*Request, *Response](2)
func ThresholdQuorum[Req, Resp msg](threshold int) QuorumFunc[Req, Resp, Resp] {
	return func(ctx *ClientCtx[Req, Resp]) (Resp, error) {
		var (
			firstResp Resp
			found     bool
			count     int
			errs      []nodeError
		)

		for result := range ctx.Responses() {
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
}

// MajorityQuorum returns the first response once a simple majority (⌈(n+1)/2⌉)
// of successful responses are received.
//
// This is a base quorum function that terminates the interceptor chain.
func MajorityQuorum[Req, Resp msg](ctx *ClientCtx[Req, Resp]) (Resp, error) {
	quorumSize := ctx.Size()/2 + 1
	return ThresholdQuorum[Req, Resp](quorumSize)(ctx)
}

// FirstResponse returns the first successful response received from any node.
// This is useful for read-any patterns where any single response is sufficient.
//
// This is a base quorum function that terminates the interceptor chain.
func FirstResponse[Req, Resp msg](ctx *ClientCtx[Req, Resp]) (Resp, error) {
	return ThresholdQuorum[Req, Resp](1)(ctx)
}

// AllResponses returns the first response once all nodes have responded successfully.
// If any node fails, it returns an error. This is useful for write-all patterns.
//
// This is a base quorum function that terminates the interceptor chain.
func AllResponses[Req, Resp msg](ctx *ClientCtx[Req, Resp]) (Resp, error) {
	return ThresholdQuorum[Req, Resp](ctx.Size())(ctx)
}

// CollectAllResponses returns a map of all successful responses indexed by node ID.
//
// This is a base quorum function that terminates the interceptor chain.
func CollectAllResponses[Req, Resp msg](ctx *ClientCtx[Req, Resp]) (map[uint32]Resp, error) {
	return ctx.Responses().CollectAll(), nil
}

// -------------------------------------------------------------------------
// Chain
// -------------------------------------------------------------------------

// Chain returns a QuorumFunc that composes the provided interceptors around the base function.
// The interceptors are executed in the order provided, wrapping the base QuorumFunc.
//
// The base QuorumFunc is the terminal handler that actually processes the responses.
// Interceptors can wrap this handler to add behavior before or after the base handler.
//
// Execution order:
//  1. interceptors[0] (outermost wrapper)
//  2. interceptors[1]
//     ...
//  3. base (innermost handler, e.g. aggregation)
func Chain[Req, Resp msg, Out any](
	base QuorumFunc[Req, Resp, Out],
	interceptors ...QuorumInterceptor[Req, Resp, Out],
) QuorumFunc[Req, Resp, Out] {
	handler := base
	for i := len(interceptors) - 1; i >= 0; i-- {
		handler = interceptors[i](handler)
	}
	return handler
}

// -------------------------------------------------------------------------
// QuorumCallWithInterceptor
// -------------------------------------------------------------------------

// QuorumCallWithInterceptor performs a quorum call using an interceptor-based approach.
//
// Type parameters:
//   - Req: The request message type
//   - Resp: The response message type from individual nodes
//   - Out: The final output type returned by the interceptor chain
//
// The base parameter is the terminal handler that processes responses (e.g., MajorityQuorum).
// The interceptors parameter accepts one or more interceptors that wrap the base handler.
//
// Execution order:
//  1. interceptors[0] (outermost wrapper)
//  2. interceptors[1]
//     ...
//  3. base (innermost handler, e.g. aggregation)
//
// Note: Messages are not sent to nodes before ctx.Responses() is called, applying any
// registered request transformations. This lazy sending is necessary to allow interceptors
// to register transformations prior to dispatch.
//
// This function should be used by generated code only.
func QuorumCallWithInterceptor[Req, Resp msg, Out any](
	ctx context.Context,
	config RawConfiguration,
	req Req,
	method string,
	base QuorumFunc[Req, Resp, Out],
	interceptors ...QuorumInterceptor[Req, Resp, Out],
) (Out, error) {
	md := ordering.NewGorumsMetadata(ctx, config.getMsgID(), method)
	replyChan := make(chan Result[msg], len(config))

	// Create ClientCtx first so sendOnce can access it
	clientCtx := newClientCtx[Req, Resp](ctx, config, req, method, replyChan)

	// Create sendOnce function that will be called lazily on first Responses() call
	sendOnce := func() {
		for _, n := range config {
			// Apply registered request transformations (if any)
			msg := clientCtx.applyTransforms(req, n)
			if msg == nil {
				continue // Skip node if transformation function returns nil
			}
			n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), responseChan: replyChan})
		}
	}

	// Wrap sendOnce with sync.OnceFunc to ensure it's only called once
	clientCtx.sendOnce = sync.OnceFunc(sendOnce)

	handler := Chain(base, interceptors...)
	return handler(clientCtx)
}
