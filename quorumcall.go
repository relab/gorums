package gorums

import (
	"sync"

	"github.com/relab/gorums/ordering"
)

// QuorumCallWithInterceptor performs a quorum call and returns a Responses object
// that provides access to node responses via terminal methods and fluent iteration.
//
// Type parameters:
//   - Req: The request message type
//   - Resp: The response message type from individual nodes
//
// The opts parameter accepts CallOption values such as Interceptors.
// Interceptors are applied in the order they are provided via Interceptors,
// modifying the clientCtx before the user calls a terminal method.
//
// Note: Messages are not sent to nodes until a terminal method (like Majority, First)
// or iterator method (like Seq) is called, applying any registered request transformations.
// This lazy sending is necessary to allow interceptors to register transformations prior to dispatch.
//
// This function should be used by generated code only.
func QuorumCallWithInterceptor[Req, Resp msg](
	ctx *ConfigContext,
	req Req,
	method string,
	opts ...CallOption,
) *Responses[Resp] {
	config := ctx.Configuration()

	// Apply options
	callOpts := getCallOptions(E_Quorumcall, opts...)

	md := ordering.NewGorumsMetadata(ctx, config.getMsgID(), method)
	replyChan := make(chan NodeResponse[msg], len(config))

	// Create clientCtx first so sendOnce can access it
	clientCtx := newClientCtx[Req, Resp](ctx, config, req, method, replyChan)

	// Create sendOnce function that will be called lazily on first Responses() call
	sendOnce := func() {
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
	}

	// Wrap sendOnce with sync.OnceFunc to ensure it's only called once
	clientCtx.sendOnce = sync.OnceFunc(sendOnce)

	// Apply interceptors
	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(clientCtx)
	}

	return NewResponses(clientCtx)
}
