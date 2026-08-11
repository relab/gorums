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
// Register interceptors with [Call.Intercept] before consuming its responses.
// A Call may be consumed once.
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
//	resp, err := storage.ReadQC(ctx, req).Intercept(logging, audit).Majority()
//
// Intercept must be called before any consuming method (a terminal method,
// async terminal, correctable call, or ranging Results). Calling it after
// dispatch has started panics. Nil interceptors are ignored.
func (c *Call[Req, Resp]) Intercept(ics ...ClientInterceptor[Req, Resp]) *Call[Req, Resp] {
	c.ctx.intercept(ics...)
	// The interceptors may have wrapped the response sequence; re-sync the
	// embedded Responses so its terminal methods observe the wrapped sequence.
	c.Responses.seq = c.ctx.responseSeq
	return c
}

// OnewayCall represents a lazily dispatched multicast or unicast call.
// [OnewayCall.Send] and [OnewayCall.Async] each consume the call; invoking
// either after the call has been consumed panics.
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
// A server handler dispatching a back-channel call should release its hold on
// the server first, so that inbound processing is not blocked while the send
// completes. Use [OnewayCall.Async] to keep several sends in flight from a
// single goroutine.
//
// Send consumes the call; calling it again on the same handle panics.
func (c *OnewayCall[Req]) Send() error {
	c.dispatch()
	return c.collect()
}

// Async dispatches the request without waiting for the sends to complete, so a
// single goroutine can keep several one-way calls in flight:
//
//	h := Multicast(ctx, msg).Async()
//	// ... dispatch more calls ...
//	err := h.Wait()
//
// Async starts no goroutine; [OnewayAsync.Wait] collects the send confirmations
// on the caller's goroutine. Dropping the handle without calling Wait is safe.
//
// Async consumes the call; calling it again on the same handle panics.
func (c *OnewayCall[Req]) Async() *OnewayAsync {
	c.dispatch()
	return &OnewayAsync{collect: c.collect}
}

// dispatch installs the reply channel and sends the request exactly once.
// It panics if the handle was already consumed.
func (c *OnewayCall[Req]) dispatch() {
	if c.ctx.dispatched.Swap(true) {
		panic("gorums: OnewayCall.Send or OnewayCall.Async called more than once on the same handle")
	}
	c.ctx.replyChan = make(chan NodeResponse[*stream.Message], c.ctx.config.Size())
	c.ctx.sendOnce.Do(c.ctx.send)
}

// collect gathers one send confirmation per node and reports the failures,
// aggregating them for multicast and passing the single error through for
// unicast. Nodes skipped by a request transform are not failures.
func (c *OnewayCall[Req]) collect() error {
	if c.unicast {
		select {
		case r := <-c.ctx.replyChan:
			return r.Err
		case <-c.ctx.Done():
			return c.ctx.Err()
		}
	}
	var errs []conn.NodeError
	for range c.ctx.config.Size() {
		select {
		case r := <-c.ctx.replyChan:
			if r.Err != nil && !errors.Is(r.Err, ErrSkipNode) {
				errs = append(errs, conn.NewNodeError(r.NodeID, r.Err))
			}
		case <-c.ctx.Done():
			return c.ctx.Err()
		}
	}
	if len(errs) > 0 {
		return conn.NewQuorumCallError(ErrSendFailure, errs)
	}
	return nil
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
