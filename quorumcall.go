package gorums

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
	callOpts := getCallOptions(E_Quorumcall, opts...)
	builder := newClientCtxBuilder[Req, Resp](ctx, req, method)
	if callOpts.streaming {
		builder = builder.WithStreaming()
	}
	clientCtx := builder.Build()

	for _, ic := range callOpts.interceptors {
		interceptor := ic.(QuorumInterceptor[Req, Resp])
		interceptor(clientCtx)
	}

	return NewResponses(clientCtx)
}
