package impl

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Call is exported and documented as gorums.Call.
type Call[Req, Resp proto.Message] struct {
	*Responses[Resp]
	ctx *CallContext[Req, Resp]
}

// ClientInterceptor is exported and documented as gorums.ClientInterceptor.
type ClientInterceptor[Req, Resp proto.Message] func(ctx *CallContext[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp]

// MapRequest is exported and documented as gorums.MapRequest.
func MapRequest[Req, Resp proto.Message](fn func(Req, *Node) Req) ClientInterceptor[Req, Resp] {
	return func(ctx *CallContext[Req, Resp], next ResponseSeq[Resp]) ResponseSeq[Resp] {
		if fn != nil {
			ctx.reqTransforms = append(ctx.reqTransforms, fn)
		}
		return next
	}
}

// MapResponse is exported and documented as gorums.MapResponse.
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

// OnewayCall is exported and documented as gorums.OnewayCall.
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

// Send attempts to queue the request for every destination and returns the
// admission errors, without waiting for queue space or stream writes.
// A full queue rejects that destination with [stream.ErrSendQueueFull]; multicast
// continues to the remaining nodes and returns per-node failures in a
// [conn.QuorumCallError] with cause [ErrSendFailure]. Unicast returns its single error.
// Intentionally skipped multicast destinations do not count as failures.
//
// Success means local queue acceptance. Later transport failures are available
// through [Node.LastErr]. To await stream-write outcomes, use [OnewayCall.Async]
// followed by [OnewayAsync.Wait]. Neither operation confirms remote processing. A local
// self-call completes at admission to its handler queue.
//
// The context bounds admission only. If the context has already ended, Send
// returns its error and sends nothing; a call without a target returns
// [ErrNoNode] or [ErrEmptyConfig] instead. An accepted request stays queued when
// the context ends later, so the caller may cancel the context as soon as
// Send returns. To let the context withdraw requests that still wait in the
// queue, use [OnewayCall.Async].
//
// Send dispatches the call; calling Send or Async again panics.
func (c *OnewayCall[Req]) Send() error {
	if err := c.dispatch(); err != nil {
		return err
	}
	return c.result(c.ctx.admit(context.WithoutCancel(c.ctx.Context)))
}

// Async attempts nonblocking admission to every destination and returns a
// handle for admission failures and stream-write outcomes, so a
// single goroutine can keep several one-way calls in flight:
//
//	h := client.WriteMulticast(ctx, msg).Async()
//	// ... dispatch more calls ...
//	err := h.Wait()
//
// Async starts no goroutine; [OnewayAsync.Wait] collects the send confirmations
// on the caller's goroutine. Dropping the handle without calling Wait is safe,
// but the send errors are then lost.
//
// The context bounds the call until its stream writes complete. If the
// context has already ended, the call sends nothing and Wait returns the
// context error; a call without a target returns [ErrNoNode] or
// [ErrEmptyConfig] instead. A request that still waits in the queue when the
// context ends is discarded.
//
// Async dispatches the call; calling Send or Async on the same handle after
// dispatch panics.
func (c *OnewayCall[Req]) Async() *OnewayAsync {
	if err := c.dispatch(); err != nil {
		return &OnewayAsync{collect: func() error { return err }}
	}
	c.ctx.responseChan = make(chan NodeResponse[*stream.Message], c.ctx.config.Size())
	failed := c.ctx.admit(c.ctx.Context)
	return &OnewayAsync{collect: func() error { return c.collect(failed) }}
}

// dispatch marks the call as dispatched and returns the error that the call
// reports instead of sending, if any: the error that the handle holds, or the
// error of a context that has already ended. It panics if the call was already
// dispatched.
func (c *OnewayCall[Req]) dispatch() error {
	if c.ctx.dispatched.Swap(true) {
		panic("gorums: OnewayCall.Send or OnewayCall.Async called more than once on the same handle")
	}
	if c.ctx.err != nil {
		return c.ctx.err
	}
	return c.ctx.Err()
}

// collect waits for the outcome of every accepted request and returns the
// result of the call. The failed responses hold the admission failures. If the
// context ends first, each node whose request has no outcome fails with the
// context error.
func (c *OnewayCall[Req]) collect(failed []NodeResponse[*stream.Message]) error {
	// pending holds the nodes whose accepted request has no outcome yet.
	pending := make([]ID, 0, c.ctx.config.Size()-len(failed))
	for _, n := range c.ctx.config {
		if !slices.ContainsFunc(failed, func(r NodeResponse[*stream.Message]) bool { return r.NodeID == n.ID() }) {
			pending = append(pending, n.ID())
		}
	}
	for range len(pending) {
		r, err := c.receive()
		if err != nil {
			for _, id := range pending {
				failed = append(failed, NodeResponse[*stream.Message]{NodeID: id, Err: err})
			}
			break
		}
		pending = slices.DeleteFunc(pending, func(id ID) bool { return id == r.NodeID })
		if r.Err != nil {
			failed = append(failed, r)
		}
	}
	return c.result(failed)
}

// result returns the error of a one-way call whose failed responses hold one
// error per failed node. Unicast returns its single error. Multicast returns
// the errors in a [conn.QuorumCallError] with cause [ErrSendFailure]. A node
// skipped by a request transform ([ErrSkipNode]) is left out of it, since the
// sends to the other nodes still went out; a skipped unicast sent nothing at
// all, so its [ErrSkipNode] is returned.
func (c *OnewayCall[Req]) result(failed []NodeResponse[*stream.Message]) error {
	if c.unicast {
		if len(failed) == 0 {
			return nil
		}
		return failed[0].Err
	}
	var errs []conn.NodeError
	for _, r := range failed {
		if !errors.Is(r.Err, ErrSkipNode) {
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

// OnewayAsync is exported and documented as gorums.OnewayAsync.
type OnewayAsync struct {
	collect func() error
	once    sync.Once
	err     error
}

// Wait collects admission failures and stream-write outcomes until all are
// known or the call context ends. It may be called more than once and returns
// the same result each time. It does not confirm remote processing; local
// self-calls complete at handler-queue admission. Do not wait while holding a
// lock needed for protocol progress. In a server handler, dispatch before
// ServerContext.Release and wait afterward to preserve relay order.
//
// If the context ends first, a unicast returns the context error. A multicast
// returns a [conn.QuorumCallError] that holds the failures known so far and
// the context error for each node whose stream write has no outcome; see
// [conn.QuorumCallError.NodeErrors].
func (a *OnewayAsync) Wait() error {
	a.once.Do(func() { a.err = a.collect() })
	return a.err
}
