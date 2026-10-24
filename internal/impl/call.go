package impl

import (
	"errors"
	"sync"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Call represents a lazily dispatched quorum call.
// Register interceptors with [Call.Intercept] before the call is dispatched.
// A Call may be dispatched once.
type Call[Req, Resp proto.Message] struct {
	*Responses[Resp]
	ctx *CallContext[Req, Resp]
}

// ClientInterceptor transforms a call's request or response sequence.
type ClientInterceptor[Req, Resp proto.Message] func(ctx *CallContext[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp]

// MapRequest returns an interceptor that transforms the request for each node.
// Returning nil or an invalid message skips the node with [ErrSkipNode].
func MapRequest[Req, Resp proto.Message](fn func(Req, *Node) Req) ClientInterceptor[Req, Resp] {
	return func(ctx *CallContext[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp] {
		if fn != nil {
			ctx.reqTransforms = append(ctx.reqTransforms, fn)
		}
		return next
	}
}

// MapResponse returns an interceptor that transforms each successful response.
func MapResponse[Req, Resp proto.Message](fn func(Resp, *Node) Resp) ClientInterceptor[Req, Resp] {
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

// Intercept registers interceptors for this call, applied in call-site order.
// It returns the same handle for fluent chaining:
//
//	resp, err := client.ReadQC(ctx, req).Intercept(logging, audit).Majority()
//
// Intercept must be called before any method that dispatches the call: a
// terminal method, [Call.Async], [Responses.Correctable], or
// [Responses.Results]. Calling it after the call was dispatched panics. Nil
// interceptors are ignored.
func (c *Call[Req, Resp]) Intercept(ics ...ClientInterceptor[Req, Resp]) *Call[Req, Resp] {
	c.ctx.intercept(ics...)
	// The interceptors may have wrapped the response sequence; re-sync the
	// embedded Responses so its terminal methods observe the wrapped sequence.
	c.Responses.seq = c.ctx.responseSeq
	return c
}

// Async sends the requests now and returns the dispatched responses, so a
// single goroutine can keep several quorum calls in flight, in send order:
//
//	a := client.WriteQC(ctx, reqA).Async()
//	b := client.WriteQC(ctx, reqB).Async()
//	_, errA := a.Majority()
//	_, errB := b.Majority()
//
// Any terminal method collects the replies later, on the caller's goroutine;
// Async starts no goroutine. A call buffers one reply per node, and a
// streaming call buffers ten. A streaming call whose nodes send more replies
// holds up the node's receive path until the caller reads them or the context
// ends, so read the replies of such a call soon, or use
// [Responses.Correctable].
//
// Async dispatches the call. Calling Async or [Call.Intercept] after the call
// was dispatched panics.
func (c *Call[Req, Resp]) Async() *Responses[Resp] {
	if c.ctx.dispatched.Swap(true) {
		panic("gorums: Call.Async called after the call was dispatched")
	}
	c.ctx.sendOnce.Do(c.ctx.send)
	return c.Responses
}

// OnewayCall represents a lazily dispatched multicast or unicast call.
// [OnewayCall.Send] and [OnewayCall.Async] each dispatch the call; invoking
// either after the call was dispatched panics.
type OnewayCall[Req proto.Message] struct {
	ctx     *CallContext[Req, *emptypb.Empty]
	unicast bool
}

// Intercept registers interceptors for this one-way call, applied in call-site
// order, and returns the same handle for fluent chaining. It must be called
// before [OnewayCall.Send] or [OnewayCall.Async]; calling it after dispatch
// panics. Nil interceptors are ignored. Only request transforms (see
// [MapRequest]) take effect for one-way calls, since no responses are collected.
func (c *OnewayCall[Req]) Intercept(ics ...ClientInterceptor[Req, *emptypb.Empty]) *OnewayCall[Req] {
	c.ctx.intercept(ics...)
	return c
}

// Send dispatches the request and blocks until every message has reached its
// node's stream. A one-way call carries no reply, so there is nothing further
// to await.
//
// For multicast, Send returns nil only if the send completes for every target
// node; send failures are returned as a [QuorumCallError] with cause
// [ErrSendFailure] and per-node errors. For unicast, Send returns the single
// send error, or the context error if the context is cancelled first.
//
// A server handler that relays a request in FIFO order dispatches the relay
// with [OnewayCall.Async] before it calls ServerContext.Release, and waits for
// it with [OnewayAsync.Wait] after Release, so that the next request does not
// wait for the send. Use [OnewayCall.Async] also to keep several sends in
// flight from a single goroutine.
//
// Send dispatches the call; calling it again on the same handle panics.
func (c *OnewayCall[Req]) Send() error {
	c.dispatch()
	return c.collect()
}

// Async dispatches the request without waiting for the sends to complete, so a
// single goroutine can keep several one-way calls in flight:
//
//	h := client.WriteMulticast(ctx, msg).Async()
//	// ... dispatch more calls ...
//	err := h.Wait()
//
// Async starts no goroutine; [OnewayAsync.Wait] collects the send confirmations
// on the caller's goroutine. Dropping the handle without calling Wait is safe.
//
// Async dispatches the call; calling it again on the same handle panics.
func (c *OnewayCall[Req]) Async() *OnewayAsync {
	c.dispatch()
	return &OnewayAsync{collect: c.collect}
}

// dispatch installs the response channel and sends the request exactly once.
// It panics if the handle was already dispatched.
func (c *OnewayCall[Req]) dispatch() {
	if c.ctx.dispatched.Swap(true) {
		panic("gorums: OnewayCall.Send or OnewayCall.Async called more than once on the same handle")
	}
	c.ctx.responseChan = make(chan NodeResponse[*stream.Message], c.ctx.config.Size())
	c.ctx.sendOnce.Do(c.ctx.send)
}

// collect gathers one send confirmation per node and reports the failures,
// aggregating them for multicast and passing the single error through for
// unicast. A node skipped by a request transform ([ErrSkipNode]) is dropped
// from the multicast aggregate, since the sends to the other nodes still went
// out. A skipped unicast sent nothing at all, so its [ErrSkipNode] is returned.
func (c *OnewayCall[Req]) collect() error {
	if c.unicast {
		r, err := c.receive()
		if err != nil {
			return err
		}
		return r.Err
	}
	var errs []conn.NodeError
	for range c.ctx.config.Size() {
		r, err := c.receive()
		if err != nil {
			return err
		}
		if r.Err != nil && !errors.Is(r.Err, ErrSkipNode) {
			errs = append(errs, conn.NewNodeError(r.NodeID, r.Err))
		}
	}
	if len(errs) > 0 {
		return conn.NewQuorumCallError(ErrSendFailure, errs)
	}
	return nil
}

// receive returns the next send confirmation. A confirmation already buffered
// is returned even if the context has ended, so a call whose sends completed
// reports them after its context is done. The context error is returned only
// when no confirmation is available.
func (c *OnewayCall[Req]) receive() (NodeResponse[*stream.Message], error) {
	select {
	case r := <-c.ctx.responseChan:
		return r, nil
	default:
	}
	select {
	case r := <-c.ctx.responseChan:
		return r, nil
	case <-c.ctx.Done():
		return NodeResponse[*stream.Message]{}, c.ctx.Err()
	}
}

// OnewayAsync is the send-completion handle of a one-way call dispatched with
// [OnewayCall.Async].
type OnewayAsync struct {
	collect func() error
	once    sync.Once
	err     error
}

// Wait blocks until send completion is known and returns the same error
// [OnewayCall.Send] would have returned. It may be called more than once and
// returns the same result each time.
func (a *OnewayAsync) Wait() error {
	a.once.Do(func() { a.err = a.collect() })
	return a.err
}
