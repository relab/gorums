package gorums

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CallContext provides context and access to the quorum call state for interceptors.
// It exposes the request, configuration, metadata about the call, and the response iterator.
type CallContext[Req, Resp proto.Message] struct {
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

	// dispatched is set once dispatch has been initiated (by sendNow or by
	// marking an async/correctable call). Once set, Intercept panics because
	// interceptors can no longer affect the in-flight call. It is an
	// atomic.Bool rather than a plain bool because sendNow can be called again,
	// redundantly, from the goroutine an async or correctable call spawns
	// (ranging over responseSeq calls sendNow), concurrently with a caller
	// checking or setting the flag on another goroutine.
	dispatched atomic.Bool
}

// sendNow triggers request dispatch exactly once.
func (c *CallContext[Req, Resp]) sendNow() {
	c.markDispatched()
	c.sendOnce.Do(c.send)
}

// markDispatched records that dispatch has been initiated, so a later Intercept
// panics. It is idempotent and does not itself send anything.
func (c *CallContext[Req, Resp]) markDispatched() {
	c.dispatched.Store(true)
}

// intercept applies the given interceptors in order, before dispatch. Nil
// interceptors are ignored. It panics if the call has already been dispatched,
// since interceptors can no longer influence an in-flight call.
func (c *CallContext[Req, Resp]) intercept(ics ...ClientInterceptor[Req, Resp]) {
	if c.dispatched.Load() {
		panic("gorums: Intercept called after the call was dispatched")
	}
	for _, ic := range ics {
		if ic == nil {
			continue
		}
		c.responseSeq = ic(c, c.responseSeq)
	}
}

// newQuorumCallContext constructs a CallContext for quorum calls (two-way, always returns responses).
// A reply channel is always created; streaming controls both its buffer size and the response iterator type.
func newQuorumCallContext[Req, Resp proto.Message](
	ctx *ConfigContext,
	req Req,
	method string,
	streaming bool,
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
	return clientCtx
}

// newOnewayCallContext constructs a CallContext for a one-way call over config.
// The reply channel is installed by [OnewayCall.dispatch], since only a consumed
// handle collects send confirmations.
func newOnewayCallContext[Req proto.Message](
	ctx context.Context,
	config Config,
	req Req,
	method string,
) *CallContext[Req, *emptypb.Empty] {
	callCtx := &CallContext[Req, *emptypb.Empty]{
		Context: ctx,
		config:  config,
		request: req,
		method:  method,
		msgID:   config.nextMsgID(),
		oneway:  true,
	}
	callCtx.responseSeq = callCtx.defaultResponseSeq()
	return callCtx
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
