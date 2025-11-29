package gorums

import (
	"sync"

	"github.com/relab/gorums/ordering"
)

// QuorumCallWithInterceptor performs a quorum call using an interceptor-based approach.
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
// Interceptors are applied in the order they are provided via Interceptors:
//  1. First interceptor (outermost wrapper)
//  2. Second interceptor
//     ...
//  3. base (innermost handler, e.g. aggregation)
//
// Note: Messages are not sent to nodes before ctx.Responses() is called, applying any
// registered request transformations. This lazy sending is necessary to allow interceptors
// to register transformations prior to dispatch.
//
// This function should be used by generated code only.
func QuorumCallWithInterceptor[Req, Resp msg, Out any](
	ctx *ConfigContext,
	req Req,
	method string,
	base QuorumFunc[Req, Resp, Out],
	opts ...CallOption,
) (Out, error) {
	config := ctx.Configuration()

	// Apply options
	callOpts := getCallOptions(E_Quorumcall, opts...)

	// Use QuorumFunc from CallOptions if provided, otherwise use the base parameter
	qf := base
	if callOpts.quorumFunc != nil {
		qf = callOpts.quorumFunc.(QuorumFunc[Req, Resp, Out])
	}

	md := ordering.NewGorumsMetadata(ctx, config.getMsgID(), method)
	replyChan := make(chan NodeResponse[msg], len(config))

	// Create ClientCtx first so sendOnce can access it
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

	handler := Chain(qf, interceptorsFromCallOptions[Req, Resp, Out](callOpts)...)
	return handler(clientCtx)
}
