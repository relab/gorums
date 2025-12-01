package gorums

import "github.com/relab/gorums/ordering"

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
	callOpts := getCallOptions(E_Quorumcall, opts...)
	md := ordering.NewGorumsMetadata(ctx, config.getMsgID(), method)
	replyChan := make(chan NodeResponse[msg], len(config))

	clientCtx := newClientCtx[Req, Resp](ctx, config, req, method, md, replyChan)

	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(clientCtx)
	}

	return NewResponses(clientCtx)
}
