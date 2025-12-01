package gorums

import (
	"sync"

	"github.com/relab/gorums/ordering"
)

// Async is a generic future type for asynchronous quorum calls.
// It encapsulates the state of an asynchronous call and provides methods
// for checking the status or waiting for completion.
//
// Type parameter Resp is the response type from nodes.
type Async[Resp any] struct {
	reply Resp
	err   error
	c     chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *Async[Resp]) Get() (Resp, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *Async[Resp]) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// AsyncMajority returns an Async future that resolves when a majority quorum is reached.
// Messages are sent immediately (synchronously) to preserve ordering when multiple
// async calls are created in sequence.
func (r *Responses[Req, Resp]) AsyncMajority() *Async[Resp] {
	quorumSize := r.ctx.Size()/2 + 1
	return r.AsyncThreshold(quorumSize)
}

// AsyncFirst returns an Async future that resolves when the first response is received.
// Messages are sent immediately (synchronously) to preserve ordering.
func (r *Responses[Req, Resp]) AsyncFirst() *Async[Resp] {
	return r.AsyncThreshold(1)
}

// AsyncAll returns an Async future that resolves when all nodes have responded.
// Messages are sent immediately (synchronously) to preserve ordering.
func (r *Responses[Req, Resp]) AsyncAll() *Async[Resp] {
	return r.AsyncThreshold(r.ctx.Size())
}

// AsyncThreshold returns an Async future that resolves when the threshold is reached.
// Messages are sent immediately (synchronously) to preserve ordering when multiple
// async calls are created in sequence.
func (r *Responses[Req, Resp]) AsyncThreshold(threshold int) *Async[Resp] {
	// Force immediate sending for ordering
	r.ctx.sendOnce()

	fut := &Async[Resp]{c: make(chan struct{}, 1)}

	go func() {
		defer close(fut.c)
		fut.reply, fut.err = r.Threshold(threshold)
	}()

	return fut
}

// AsyncCall performs an asynchronous quorum call and returns an Async future.
// Messages are sent immediately (synchronously) to preserve ordering when multiple
// async calls are created in sequence.
//
// This function should be used by generated code only.
func AsyncCall[Req, Resp msg](
	ctx *ConfigContext,
	req Req,
	method string,
	opts ...CallOption,
) *Async[Resp] {
	config := ctx.Configuration()
	callOpts := getCallOptions(E_Async, opts...)

	// Create metadata with message ID and prepare reply channel.
	// Message ID is obtained now (synchronously) to maintain ordering
	// when multiple async calls are created in sequence.
	md := ordering.NewGorumsMetadata(ctx, config.getMsgID(), method)
	replyChan := make(chan NodeResponse[msg], len(config))

	// Create ClientCtx
	clientCtx := newClientCtx[Req, Resp](ctx, config, req, method, replyChan)

	// Send messages to all nodes synchronously (before spawning goroutine).
	// This ensures message ordering is preserved when multiple async calls
	// are created in sequence.
	var expected int
	for _, n := range config {
		// Apply registered request transformations (if any)
		msg := clientCtx.applyTransforms(req, n)
		if msg == nil {
			continue // Skip node if transformation function returns nil
		}
		expected++
		n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), responseChan: replyChan})
	}
	clientCtx.expectedReplies = expected

	// Mark messages as sent by setting sendOnce to a no-op
	clientCtx.sendOnce = sync.OnceFunc(func() {})

	// Create the Responses object and apply interceptors
	responses := &Responses[Req, Resp]{ctx: clientCtx}
	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(responses)
	}

	// Return async majority by default (matching old behavior)
	return responses.AsyncMajority()
}
