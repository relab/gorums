package gorums

import (
	"context"
	"slices"
	"sync"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// QuorumInterceptor intercepts and processes quorum calls, allowing modification of
// requests, responses, and aggregation logic. Interceptors can be chained together.
//
// Type parameters:
//   - T: The node ID type
//   - Req: The request message type sent to nodes
//   - Resp: The response message type from individual nodes
//
// The interceptor receives the ClientCtx for metadata access, the current response
// iterator (next), and returns a new response iterator. This pattern allows
// interceptors to wrap the response stream with custom logic.
//
// Custom interceptors can be created like this:
//
//	func LoggingInterceptor[T NodeID, Req, Resp proto.Message](
//	    ctx *gorums.ClientCtx[T, Req, Resp],
//	    next gorums.ResponseSeq[T, Resp],
//	) gorums.ResponseSeq[T, Resp] {
//	    return func(yield func(gorums.NodeResponse[T, Resp]) bool) {
//	        for resp := range next {
//	            log.Printf("Response from node %v", resp.NodeID)
//	            if !yield(resp) { return }
//	        }
//	    }
//	}
type QuorumInterceptor[T NodeID, Req, Resp msg] func(ctx *ClientCtx[T, Req, Resp], next ResponseSeq[T, Resp]) ResponseSeq[T, Resp]

// ClientCtx provides context and access to the quorum call state for interceptors.
// It exposes the request, configuration, metadata about the call, and the response iterator.
type ClientCtx[T NodeID, Req, Resp msg] struct {
	context.Context
	config    Configuration[T]
	request   Req
	method    string
	md        *ordering.Metadata
	replyChan chan NodeResponse[T, msg]

	// reqTransforms holds request transformation functions registered by interceptors.
	reqTransforms []func(Req, *Node[T]) Req

	// responseSeq is the iterator that yields node responses.
	// Interceptors can wrap this iterator to modify responses.
	responseSeq ResponseSeq[T, Resp]

	// expectedReplies is the number of responses expected from nodes.
	// It is set when messages are sent and may be lower than config size
	// if some nodes are skipped by request transformations.
	expectedReplies int

	// streaming indicates whether this is a streaming call (for correctable streams).
	streaming bool

	// waitSendDone indicates whether the caller waits for send completion (for multicast).
	waitSendDone bool

	// sendOnce ensures messages are sent exactly once, on the first
	// call to Responses(). This deferred sending allows interceptors
	// to register request transformations before dispatch.
	sendOnce sync.Once
}

// clientCtxBuilder provides an interface for constructing ClientCtx instances.
type clientCtxBuilder[T NodeID, Req, Resp msg] struct {
	c *ClientCtx[T, Req, Resp]
	// chanMultiplier is the buffer multiplier for the reply channel.
	// Default is 1; streaming calls use a larger multiplier.
	chanMultiplier int
}

// newClientCtxBuilder creates a new builder for constructing a ClientCtx.
// The required parameters are provided upfront; optional settings use builder methods.
// The metadata and reply channel are created at Build() time.
func newClientCtxBuilder[T NodeID, Req, Resp msg](
	ctx *ConfigContext[T],
	req Req,
	method string,
) *clientCtxBuilder[T, Req, Resp] {
	return &clientCtxBuilder[T, Req, Resp]{
		c: &ClientCtx[T, Req, Resp]{
			Context:         ctx,
			config:          ctx.Configuration(),
			request:         req,
			method:          method,
			expectedReplies: ctx.Configuration().Size(),
		},
		chanMultiplier: 1,
	}
}

// WithStreaming configures the clientCtx for streaming responses.
// When enabled, the response iterator continues until context cancellation
// rather than stopping after expectedReplies responses.
// It also increases the reply channel buffer size (10x) to handle streaming volume.
func (b *clientCtxBuilder[T, Req, Resp]) WithStreaming() *clientCtxBuilder[T, Req, Resp] {
	b.c.streaming = true
	b.chanMultiplier = 10
	return b
}

// WithWaitSendDone configures the clientCtx to wait for send completion.
// Used by multicast calls to ensure messages are sent before returning.
func (b *clientCtxBuilder[T, Req, Resp]) WithWaitSendDone(waitSendDone bool) *clientCtxBuilder[T, Req, Resp] {
	b.c.waitSendDone = waitSendDone
	return b
}

// Build finalizes the ClientCtx configuration and returns the constructed instance.
// It creates the metadata and reply channel, and sets up the appropriate response iterator.
func (b *clientCtxBuilder[T, Req, Resp]) Build() *ClientCtx[T, Req, Resp] {
	// Create metadata and reply channel at build time
	b.c.md = ordering.NewGorumsMetadata(b.c.Context, b.c.config.nextMsgID(), b.c.method)
	b.c.replyChan = make(chan NodeResponse[T, msg], b.c.config.Size()*b.chanMultiplier)

	if b.c.streaming {
		b.c.responseSeq = b.c.streamingResponseSeq()
	} else {
		b.c.responseSeq = b.c.defaultResponseSeq()
	}
	return b.c
}

// -------------------------------------------------------------------------
// ClientCtx Methods
// -------------------------------------------------------------------------

// Request returns the original request message for this quorum call.
func (c *ClientCtx[T, Req, Resp]) Request() Req {
	return c.request
}

// Config returns the configuration (set of nodes) for this quorum call.
func (c *ClientCtx[T, Req, Resp]) Config() Configuration[T] {
	return c.config
}

// Method returns the name of the RPC method being called.
func (c *ClientCtx[T, Req, Resp]) Method() string {
	return c.method
}

// Nodes returns the slice of nodes in this configuration.
func (c *ClientCtx[T, Req, Resp]) Nodes() []*Node[T] {
	return c.config.Nodes()
}

// Node returns the node with the given ID.
func (c *ClientCtx[T, Req, Resp]) Node(id T) *Node[T] {
	nodes := c.config.Nodes()
	index := slices.IndexFunc(nodes, func(n *Node[T]) bool {
		return n.ID() == id
	})
	if index != -1 {
		return nodes[index]
	}
	return nil
}

// Size returns the number of nodes in this configuration.
func (c *ClientCtx[T, Req, Resp]) Size() int {
	return c.config.Size()
}

// applyTransforms returns the transformed request as a proto.Message, or nil if the result is
// invalid or the node should be skipped. It applies the registered transformation functions to
// the given request for the specified node. Transformation functions are applied in the order
// they were registered.
func (c *ClientCtx[T, Req, Resp]) applyTransforms(req Req, node *Node[T]) proto.Message {
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

// applyInterceptors chains the given interceptors, wrapping the response sequence.
// Each interceptor receives the current response sequence and returns a new one.
// Interceptors are applied in order, with each wrapping the previous result.
func (c *ClientCtx[T, Req, Resp]) applyInterceptors(interceptors []any) {
	responseSeq := c.responseSeq
	for _, ic := range interceptors {
		interceptor := ic.(QuorumInterceptor[T, Req, Resp])
		responseSeq = interceptor(c, responseSeq)
	}
	c.responseSeq = responseSeq
}

// send dispatches requests to all nodes, applying any registered transformations.
// It updates expectedReplies based on how many nodes actually receive requests
// (nodes may be skipped if a transformation returns nil).
func (c *ClientCtx[T, Req, Resp]) send() {
	var expected int
	for _, n := range c.config {
		msg := c.applyTransforms(c.request, n)
		if msg == nil {
			continue // Skip node if transformation returns nil
		}
		expected++
		n.channel.enqueue(request[T]{
			ctx:          c.Context,
			msg:          NewRequestMessage(c.md, msg),
			streaming:    c.streaming,
			waitSendDone: c.waitSendDone,
			responseChan: c.replyChan,
		})
	}
	c.expectedReplies = expected
}

// defaultResponseSeq returns an iterator that yields at most c.expectedReplies responses
// from nodes until the context is canceled or all expected responses are received.
func (c *ClientCtx[T, Req, Resp]) defaultResponseSeq() ResponseSeq[T, Resp] {
	return func(yield func(NodeResponse[T, Resp]) bool) {
		// Trigger sending on first iteration
		c.sendOnce.Do(c.send)
		for range c.expectedReplies {
			select {
			case r := <-c.replyChan:
				res := newNodeResponse[T, Resp](r)
				if !yield(res) {
					return // Consumer stopped iteration
				}
			case <-c.Done():
				return // Context canceled
			}
		}
	}
}

// streamingResponseSeq returns an iterator that yields responses as they arrive
// from nodes until the context is canceled or breaking from the range loop.
func (c *ClientCtx[T, Req, Resp]) streamingResponseSeq() ResponseSeq[T, Resp] {
	return func(yield func(NodeResponse[T, Resp]) bool) {
		// Trigger sending on first iteration
		c.sendOnce.Do(c.send)
		for {
			select {
			case r := <-c.replyChan:
				res := newNodeResponse[T, Resp](r)
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
// Interceptors (Middleware)
// -------------------------------------------------------------------------

// MapRequest returns an interceptor that applies per-node request transformations.
// Multiple interceptors can be chained together, with transforms applied in order.
//
// The fn receives the original request and a node, and returns the transformed
// request to send to that node. If the function returns an invalid message or nil,
// the request to that node is skipped.
func MapRequest[T NodeID, Req, Resp msg](fn func(Req, *Node[T]) Req) QuorumInterceptor[T, Req, Resp] {
	return func(ctx *ClientCtx[T, Req, Resp], next ResponseSeq[T, Resp]) ResponseSeq[T, Resp] {
		if fn != nil {
			ctx.reqTransforms = append(ctx.reqTransforms, fn)
		}
		return next
	}
}

// MapResponse returns an interceptor that applies per-node response transformations.
//
// The fn receives the response from a node and the node itself, and returns the
// transformed response.
func MapResponse[T NodeID, Req, Resp msg](fn func(Resp, *Node[T]) Resp) QuorumInterceptor[T, Req, Resp] {
	return func(ctx *ClientCtx[T, Req, Resp], next ResponseSeq[T, Resp]) ResponseSeq[T, Resp] {
		if fn == nil {
			return next
		}
		// Wrap the response iterator with the transformation logic.
		return func(yield func(NodeResponse[T, Resp]) bool) {
			for resp := range next {
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
