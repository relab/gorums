package gorums

import (
	"github.com/relab/gorums/ordering"
)

// Async is a generic future type for asynchronous quorum calls.
// It encapsulates the state of an asynchronous call and provides methods
// for checking the status or waiting for completion.
//
// Type parameter Out is the output type returned by the quorum function,
// which may differ from the RPC response type.
type Async[Out any] struct {
	reply Out
	err   error
	c     chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *Async[Out]) Get() (Out, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *Async[Out]) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

// AsyncCall performs an asynchronous quorum call using an interceptor-based approach.
//
// Type parameters:
//   - Req: The request message type
//   - Resp: The response message type from individual nodes
//   - Out: The final output type returned by the interceptor chain
//
// The base parameter is the default terminal handler that processes responses
// (e.g., MajorityQuorum). This can be overridden via the WithQuorumFunc CallOption.
// The opts parameter accepts CallOption values such as WithQuorumFunc and Interceptors.
//
// This function should be used by generated code only.
func AsyncCall[Req, Resp msg, Out any](
	ctx *ConfigContext,
	req Req,
	method string,
	base QuorumFunc[Req, Resp, Out],
	opts ...CallOption,
) *Async[Out] {
	config := ctx.Configuration()

	// Apply options
	callOpts := getCallOptions(E_Async, opts...)

	// Use QuorumFunc from CallOptions if provided, otherwise use the base parameter
	qf := base
	if callOpts.quorumFunc != nil {
		qf = callOpts.quorumFunc.(QuorumFunc[Req, Resp, Out])
	}

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
	clientCtx.sendOnce = func() {}

	handler := Chain(qf, interceptorsFromCallOptions[Req, Resp, Out](callOpts)...)

	// Create the async future
	fut := &Async[Out]{c: make(chan struct{}, 1)}

	// Run the quorum call handler in a goroutine to collect responses
	go func() {
		defer close(fut.c)
		fut.reply, fut.err = handler(clientCtx)
	}()

	return fut
}
