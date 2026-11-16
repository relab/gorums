package impl

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
)

// CallContext is exported and documented as gorums.CallContext.
type CallContext[Req, Resp proto.Message] struct {
	context.Context
	config       Config
	request      Req
	method       string
	responseChan chan NodeResponse[*stream.Message]

	// reqTransforms holds request transformation functions registered by interceptors.
	reqTransforms []func(Req, *Node) Req

	// responseSeq is the iterator that yields node responses.
	// Interceptors can wrap this iterator to modify responses.
	responseSeq ResponseSeq[Resp]

	// streaming indicates whether this is a streaming call (for correctable streams).
	streaming bool

	// oneway indicates whether this is a one-way call (for multicast).
	oneway bool

	// err, if set, is the error that the call reports instead of sending.
	err error

	// sendOnce ensures messages are sent exactly once, on the first
	// call to sendNow. This deferred sending allows interceptors
	// to register request transformations before dispatch.
	sendOnce sync.Once

	// dispatched is set once dispatch has been initiated (by sendNow or by
	// marking a correctable call); after that, Intercept panics. It is atomic
	// because the goroutine a correctable call spawns calls sendNow while the
	// caller may check or set the flag.
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
// since interceptors cannot influence an in-flight call.
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

// newQuorumCallContext constructs a CallContext for quorum calls (two-way,
// always returns responses). A response channel is always created; streaming
// controls both its buffer size and the response iterator type.
func newQuorumCallContext[Req, Resp proto.Message](
	ctx context.Context,
	config Config,
	req Req,
	method string,
	streaming bool,
) *CallContext[Req, Resp] {
	n := config.Size()
	if streaming {
		n *= 10
	}
	callCtx := &CallContext[Req, Resp]{
		Context:      ctx,
		config:       config,
		request:      req,
		method:       method,
		streaming:    streaming,
		responseChan: make(chan NodeResponse[*stream.Message], n),
	}
	switch {
	case len(config) == 0:
		callCtx.err = ErrEmptyConfig
		callCtx.responseSeq = callCtx.errorResponseSeq()
	case streaming:
		callCtx.responseSeq = callCtx.streamingResponseSeq()
	default:
		callCtx.responseSeq = callCtx.defaultResponseSeq()
	}
	return callCtx
}

// -------------------------------------------------------------------------
// CallContext Methods
// -------------------------------------------------------------------------

// Request returns the original request message for this call.
func (c *CallContext[Req, Resp]) Request() Req {
	return c.request
}

// Config returns the configuration (set of nodes) for this call.
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

// Node returns the node with the given ID, or nil if the call's configuration
// has no such node.
func (c *CallContext[Req, Resp]) Node(id ID) *Node {
	return c.config.Node(id)
}

// Size returns the number of nodes in this configuration.
func (c *CallContext[Req, Resp]) Size() int {
	return c.config.Size()
}

// send dispatches requests and feeds admission failures to the response
// sequence. Accepted requests report their later outcomes through the transport.
func (c *CallContext[Req, Resp]) send() {
	for r := range c.admit() {
		if r.Err != nil {
			c.responseChan <- r
		}
	}
}

// admit attempts every destination and yields its admission result. With no
// transforms, the payload and ordinary-node message are shared. Shared-stream
// nodes need individual IDs from their server-initiated ID space.
func (c *CallContext[Req, Resp]) admit() ResponseSeq[*stream.Message] {
	return func(yield func(NodeResponse[*stream.Message]) bool) {
		var payload []byte
		var marshalErr error
		var sharedMsg *stream.Message
		start := 0
		if len(c.reqTransforms) == 0 {
			payload, marshalErr = proto.Marshal(c.request)
			// Rotate fan-out so no destination is always last.
			start = rand.IntN(max(len(c.config), 1))
		}
		for i := range c.config {
			n := c.config[(start+i)%len(c.config)]
			msg, err := sharedMsg, marshalErr
			if len(c.reqTransforms) != 0 {
				msg, err = c.transformAndMarshal(n)
			} else if err == nil && (msg == nil || n.IsShared()) {
				msg = stream.NewRequestFromPayload(c.Context, conn.NodeTransport(n).NextMsgID(), c.method, payload)
				if !n.IsShared() {
					sharedMsg = msg
				}
			}
			if err == nil {
				err = conn.NodeTransport(n).Enqueue(stream.NodeRequest{
					Ctx:          c.Context,
					Msg:          msg,
					Streaming:    c.streaming,
					Oneway:       c.oneway,
					ResponseChan: c.responseChan,
				})
			}
			if !yield(NodeResponse[*stream.Message]{NodeID: n.ID(), Err: err}) {
				return
			}
		}
	}
}

// transformAndMarshal applies per-node transformations and returns the message
// or the admission error. An invalid transformed message skips the node.
func (c *CallContext[Req, Resp]) transformAndMarshal(n *Node) (*stream.Message, error) {
	transformedRequest := c.request
	for _, transform := range c.reqTransforms {
		transformedRequest = transform(transformedRequest, n)
	}
	if protoReq, ok := any(transformedRequest).(proto.Message); !ok || protoReq == nil || !protoReq.ProtoReflect().IsValid() {
		return nil, ErrSkipNode
	}
	return stream.NewRequest(c.Context, conn.NodeTransport(n).NextMsgID(), c.method, transformedRequest)
}

// defaultResponseSeq returns an iterator that yields at most c.Size() responses
// from nodes until the context is canceled or all expected responses are received.
func (c *CallContext[Req, Resp]) defaultResponseSeq() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		// Trigger sending on first iteration
		c.sendNow()
		for range c.Size() {
			select {
			case r := <-c.responseChan:
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

// errorResponseSeq returns an iterator that sends nothing and yields one
// response that holds c.err.
func (c *CallContext[Req, Resp]) errorResponseSeq() ResponseSeq[Resp] {
	return func(yield func(NodeResponse[Resp]) bool) {
		c.markDispatched()
		yield(NodeResponse[Resp]{Err: c.err})
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
			case r := <-c.responseChan:
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
