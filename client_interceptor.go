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
//   - Req: The request message type sent to nodes
//   - Resp: The response message type from individual nodes
//
// The interceptor receives the ClientCtx for metadata access, the current response
// iterator (next), and returns a new response iterator. This pattern allows
// interceptors to wrap the response stream with custom logic.
//
// Custom interceptors can be created like this:
//
//	func LoggingInterceptor[Req, Resp proto.Message](
//	    ctx *gorums.ClientCtx[Req, Resp],
//	    next gorums.ResponseSeq[Resp],
//	) gorums.ResponseSeq[Resp] {
//	    return func(yield func(gorums.NodeResponse[Resp]) bool) {
//	        for resp := range next {
//	            log.Printf("Response from node %d", resp.NodeID)
//	            if !yield(resp) { return }
//	        }
//	    }
//	}
type QuorumInterceptor[Req, Resp msg] func(ctx *ClientCtx[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp]

// ClientCtx provides context and access to the quorum call state for interceptors.
// It exposes the request, configuration, metadata about the call, and the response iterator.
type ClientCtx[Req, Resp msg] struct {
	context.Context
	config    Configuration
	request   Req
	method    string
	md        *ordering.Metadata
	replyChan chan NodeResponse[msg]

	// reqTransforms holds request transformation functions registered by interceptors.
	reqTransforms []func(Req, *Node) Req

	// responseSeq is the iterator that yields node responses.
	// Interceptors can wrap this iterator to modify responses.
	responseSeq ResponseSeq[Resp]

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
type clientCtxBuilder[Req, Resp msg] struct {
	c *ClientCtx[Req, Resp]
	// chanMultiplier is the buffer multiplier for the reply channel.
	// Default is 1; streaming calls use a larger multiplier.
	chanMultiplier int
}

// newClientCtxBuilder creates a new builder for constructing a ClientCtx.
// The required parameters are provided upfront; optional settings use builder methods.
// The metadata and reply channel are created at Build() time.
func newClientCtxBuilder[Req, Resp msg](
	ctx *ConfigContext,
	req Req,
	method string,
) *clientCtxBuilder[Req, Resp] {
	return &clientCtxBuilder[Req, Resp]{
		c: &ClientCtx[Req, Resp]{
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
func (b *clientCtxBuilder[Req, Resp]) WithStreaming() *clientCtxBuilder[Req, Resp] {
	b.c.streaming = true
	b.chanMultiplier = 10
	return b
}

// WithWaitSendDone configures the clientCtx to wait for send completion.
// Used by multicast calls to ensure messages are sent before returning.
func (b *clientCtxBuilder[Req, Resp]) WithWaitSendDone(waitSendDone bool) *clientCtxBuilder[Req, Resp] {
	b.c.waitSendDone = waitSendDone
	return b
}

// Build finalizes the ClientCtx configuration and returns the constructed instance.
// It creates the metadata and reply channel, and sets up the appropriate response iterator.
func (b *clientCtxBuilder[Req, Resp]) Build() *ClientCtx[Req, Resp] {
	// Create metadata and reply channel at build time
	b.c.md = ordering.NewGorumsMetadata(b.c.Context, b.c.config.nextMsgID(), b.c.method)
	b.c.replyChan = make(chan NodeResponse[msg], b.c.config.Size()*b.chanMultiplier)

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
func (c *ClientCtx[Req, Resp]) Request() Req {
	return c.request
}

// Config returns the configuration (set of nodes) for this quorum call.
func (c *ClientCtx[Req, Resp]) Config() Configuration {
	return c.config
}

// Method returns the name of the RPC method being called.
func (c *ClientCtx[Req, Resp]) Method() string {
	return c.method
}

// Nodes returns the slice of nodes in this configuration.
func (c *ClientCtx[Req, Resp]) Nodes() []*Node {
	return c.config.Nodes()
}

// Node returns the node with the given ID.
func (c *ClientCtx[Req, Resp]) Node(id uint32) *Node {
	nodes := c.config.Nodes()
	index := slices.IndexFunc(nodes, func(n *Node) bool {
		return n.ID() == id
	})
	if index != -1 {
		return nodes[index]
	}
	return nil
}

// Size returns the number of nodes in this configuration.
func (c *ClientCtx[Req, Resp]) Size() int {
	return c.config.Size()
}

// applyTransforms returns the transformed request as a proto.Message, or nil if the result is
// invalid or the node should be skipped. It applies the registered transformation functions to
// the given request for the specified node. Transformation functions are applied in the order
// they were registered.
func (c *ClientCtx[Req, Resp]) applyTransforms(req Req, node *Node) proto.Message {
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
func (c *ClientCtx[Req, Resp]) applyInterceptors(interceptors []any) {
	responseSeq := c.responseSeq
	for _, ic := range interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		responseSeq = interceptor(c, responseSeq)
	}
	c.responseSeq = responseSeq
}

// send dispatches requests to all nodes, applying any registered transformations.
// It updates expectedReplies based on how many nodes actually receive requests
// (nodes may be skipped if a transformation returns nil).
func (c *ClientCtx[Req, Resp]) send() {
	var expected int
	for _, n := range c.config {
		msg := c.applyTransforms(c.request, n)
		if msg == nil {
			continue // Skip node if transformation returns nil
		}
		expected++
		// Clone metadata for each request to avoid race conditions during
		// concurrent marshaling when gorumsMarshal calls SetMessageData.
		md := proto.CloneOf(c.md)
		n.channel.enqueue(request{
			ctx:          c.Context,
			msg:          newRequestMessage(md, msg),
			streaming:    c.streaming,
			waitSendDone: c.waitSendDone,
			responseChan: c.replyChan,
		})
	}
	c.expectedReplies = expected
}

// defaultResponseSeq returns an iterator that yields at most c.expectedReplies responses
// from nodes until the context is canceled or all expected responses are received.
func (c *ClientCtx[Req, Resp]) defaultResponseSeq() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		// Trigger sending on first iteration
		c.sendOnce.Do(c.send)
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

// streamingResponseSeq returns an iterator that yields responses as they arrive
// from nodes until the context is canceled or breaking from the range loop.
func (c *ClientCtx[Req, Resp]) streamingResponseSeq() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		// Trigger sending on first iteration
		c.sendOnce.Do(c.send)
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
// Interceptors (Middleware)
// -------------------------------------------------------------------------

// MapRequest returns an interceptor that applies per-node request transformations.
// Multiple interceptors can be chained together, with transforms applied in order.
//
// The fn receives the original request and a node, and returns the transformed
// request to send to that node. If the function returns an invalid message or nil,
// the request to that node is skipped.
func MapRequest[Req, Resp msg](fn func(Req, *Node) Req) QuorumInterceptor[Req, Resp] {
	return func(ctx *ClientCtx[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp] {
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
func MapResponse[Req, Resp msg](fn func(Resp, *Node) Resp) QuorumInterceptor[Req, Resp] {
	return func(ctx *ClientCtx[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp] {
		if fn == nil {
			return next
		}
		// Wrap the response iterator with the transformation logic.
		return func(yield func(NodeResponse[Resp]) bool) {
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
