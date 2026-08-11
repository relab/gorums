package gorums

import (
	"context"
	"slices"
	"sync"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ClientInterceptor intercepts and processes quorum calls, allowing modification of
// requests, responses, and aggregation logic. Interceptors can be chained together.
//
// Type parameters:
//   - Req: The request message type sent to nodes
//   - Resp: The response message type from individual nodes
//
// The interceptor receives the CallContext for metadata access, the current response
// iterator (next), and returns a new response iterator. This pattern allows
// interceptors to wrap the response stream with custom logic.
//
// Custom interceptors can be created like this:
//
//	func LoggingInterceptor[Req, Resp proto.Message](
//	    ctx *gorums.CallContext[Req, Resp],
//	    next gorums.ResponseSeq[Resp],
//	) gorums.ResponseSeq[Resp] {
//	    return func(yield func(gorums.NodeResponse[Resp]) bool) {
//	        for resp := range next {
//	            log.Printf("Response from node %d", resp.NodeID)
//	            if !yield(resp) { return }
//	        }
//	    }
//	}
type ClientInterceptor[Req, Resp msg] func(ctx *CallContext[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp]

// CallContext provides context and access to the quorum call state for interceptors.
// It exposes the request, configuration, metadata about the call, and the response iterator.
type CallContext[Req, Resp msg] struct {
	context.Context
	config    Config
	request   Req
	method    string
	msgID     uint64
	replyChan chan NodeResponse[*stream.Message]

	// reqTransforms holds request transformation functions registered by interceptors.
	reqTransforms []func(Req, *Node) Req

	// responseSeq is the iterator that yields node responses.
	// Interceptors can wrap this iterator to modify responses.
	responseSeq ResponseSeq[Resp]

	// streaming indicates whether this is a streaming call (for correctable streams).
	streaming bool

	// oneway indicates whether this is a one-way call (for multicast).
	oneway bool

	// sendOnce ensures messages are sent exactly once, on the first
	// call to Responses(). This deferred sending allows interceptors
	// to register request transformations before dispatch.
	sendOnce sync.Once
}

// sendNow triggers request dispatch exactly once.
func (c *CallContext[Req, Resp]) sendNow() {
	c.sendOnce.Do(c.send)
}

// newQuorumCallContext constructs a CallContext for quorum calls (two-way, always returns responses).
// A reply channel is always created; streaming controls both its buffer size and the response iterator type.
func newQuorumCallContext[Req, Resp msg](
	ctx *ConfigContext,
	req Req,
	method string,
	streaming bool,
	interceptors []any,
) *CallContext[Req, Resp] {
	config := ctx.Config()
	n := config.Size()
	if streaming {
		n *= 10
	}
	clientCtx := &CallContext[Req, Resp]{
		Context:   ctx,
		config:    config,
		request:   req,
		method:    method,
		msgID:     config.nextMsgID(),
		streaming: streaming,
		replyChan: make(chan NodeResponse[*stream.Message], n),
	}
	if streaming {
		clientCtx.responseSeq = clientCtx.streamingResponseSeq()
	} else {
		clientCtx.responseSeq = clientCtx.defaultResponseSeq()
	}
	clientCtx.applyInterceptors(interceptors)
	return clientCtx
}

// newMulticastCallContext constructs a CallContext for multicast (one-way, no responses).
// A reply channel is created only when waitForSend=true (blocking send); fire-and-forget
// calls receive a nil channel, meaning no router entry is registered.
func newMulticastCallContext[Req msg](
	ctx *ConfigContext,
	req Req,
	method string,
	waitForSend bool,
	interceptors []any,
) *CallContext[Req, *emptypb.Empty] {
	config := ctx.Config()
	var replyChan chan NodeResponse[*stream.Message]
	if waitForSend {
		replyChan = make(chan NodeResponse[*stream.Message], config.Size())
	}
	clientCtx := &CallContext[Req, *emptypb.Empty]{
		Context:   ctx,
		config:    config,
		request:   req,
		method:    method,
		msgID:     config.nextMsgID(),
		oneway:    true,
		replyChan: replyChan,
	}
	clientCtx.responseSeq = clientCtx.defaultResponseSeq()
	clientCtx.applyInterceptors(interceptors)
	return clientCtx
}

// -------------------------------------------------------------------------
// CallContext Methods
// -------------------------------------------------------------------------

// Request returns the original request message for this quorum call.
func (c *CallContext[Req, Resp]) Request() Req {
	return c.request
}

// Config returns the configuration (set of nodes) for this quorum call.
func (c *CallContext[Req, Resp]) Config() Config {
	return c.config
}

// Method returns the name of the RPC method being called.
func (c *CallContext[Req, Resp]) Method() string {
	return c.method
}

// Nodes returns the slice of nodes in this configuration.
func (c *CallContext[Req, Resp]) Nodes() []*Node {
	return c.config.Nodes()
}

// Node returns the node with the given ID.
func (c *CallContext[Req, Resp]) Node(id uint32) *Node {
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
func (c *CallContext[Req, Resp]) Size() int {
	return c.config.Size()
}

// reportNodeError sends an error response for the given node to replyChan.
// It is a no-op for fire-and-forget calls where replyChan is nil.
func (c *CallContext[Req, Resp]) reportNodeError(nodeID uint32, err error) {
	if c.replyChan != nil {
		c.replyChan <- NodeResponse[*stream.Message]{NodeID: nodeID, Err: err}
	}
}

// enqueue sends a stream.Request to the given node, populating the shared
// fields from CallContext so call sites only need to supply the message.
func (c *CallContext[Req, Resp]) enqueue(n *Node, msg *stream.Message) {
	n.Enqueue(stream.Request{
		Ctx:          c.Context,
		Msg:          msg,
		Streaming:    c.streaming,
		Oneway:       c.oneway,
		ResponseChan: c.replyChan,
	})
}

// applyInterceptors chains the given interceptors, wrapping the response sequence.
// Each interceptor receives the current response sequence and returns a new one.
// Interceptors are applied in order, with each wrapping the previous result.
func (c *CallContext[Req, Resp]) applyInterceptors(interceptors []any) {
	responseSeq := c.responseSeq
	for _, ic := range interceptors {
		interceptor := ic.(ClientInterceptor[Req, Resp])
		responseSeq = interceptor(c, responseSeq)
	}
	c.responseSeq = responseSeq
}

// send dispatches requests to all nodes. It delegates to sendWithPerNodeTransformation
// if any per-node request transformations are registered. Otherwise, it uses sendShared
// to marshal the request once and send the same message to all nodes.
func (c *CallContext[Req, Resp]) send() {
	if len(c.reqTransforms) == 0 {
		c.sendShared()
	} else {
		c.sendWithPerNodeTransformation()
	}
}

// sendShared marshals the request once and enqueues the shared message to all nodes.
// On marshal error, it reports the error to every node and returns early.
func (c *CallContext[Req, Resp]) sendShared() {
	sharedMsg, err := stream.NewMessage(c.Context, c.msgID, c.method, c.request)
	if err != nil {
		// Marshaling fails identically for all nodes; report and return.
		for _, n := range c.config {
			c.reportNodeError(n.ID(), err)
		}
		return
	}
	for _, n := range c.config {
		c.enqueue(n, sharedMsg)
	}
}

// sendWithPerNodeTransformation applies per-node request transformations before
// marshaling and enqueues each individually transformed message to its node.
func (c *CallContext[Req, Resp]) sendWithPerNodeTransformation() {
	for _, n := range c.config {
		streamMsg := c.transformAndMarshal(n)
		if streamMsg == nil {
			continue // Skip node: transformAndMarshal already sent ErrSkipNode
		}
		c.enqueue(n, streamMsg)
	}
}

// transformAndMarshal applies transformations to the request for the given node,
// then marshals it into a stream.Message. Returns nil if transformation fails
// or marshaling fails (in which case the error is reported via reportNodeError).
func (c *CallContext[Req, Resp]) transformAndMarshal(n *Node) *stream.Message {
	transformedRequest := c.request
	for _, transform := range c.reqTransforms {
		transformedRequest = transform(transformedRequest, n)
	}
	// Check if the result is valid
	if protoReq, ok := any(transformedRequest).(proto.Message); !ok || protoReq == nil || !protoReq.ProtoReflect().IsValid() {
		c.reportNodeError(n.ID(), ErrSkipNode)
		return nil
	}
	streamMsg, err := stream.NewMessage(c.Context, c.msgID, c.method, transformedRequest)
	if err != nil {
		c.reportNodeError(n.ID(), err)
		return nil
	}
	return streamMsg
}

// defaultResponseSeq returns an iterator that yields at most c.expectedReplies responses
// from nodes until the context is canceled or all expected responses are received.
func (c *CallContext[Req, Resp]) defaultResponseSeq() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		// Trigger sending on first iteration
		c.sendNow()
		for range c.Size() {
			select {
			case r := <-c.replyChan:
				res := mapToCallResponse[Resp](r)
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
func (c *CallContext[Req, Resp]) streamingResponseSeq() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		// Trigger sending on first iteration
		c.sendNow()
		for {
			select {
			case r := <-c.replyChan:
				res := mapToCallResponse[Resp](r)
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
// an ErrSkipNode error is sent for that node, indicating it was skipped.
func MapRequest[Req, Resp msg](fn func(Req, *Node) Req) ClientInterceptor[Req, Resp] {
	return func(ctx *CallContext[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp] {
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
func MapResponse[Req, Resp msg](fn func(Resp, *Node) Resp) ClientInterceptor[Req, Resp] {
	return func(ctx *CallContext[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp] {
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
