package gorums

// QuorumCall performs a quorum call and returns a Responses object
// that provides access to node responses via terminal methods and fluent iteration.
//
// Type parameters:
//   - T: The node ID type (e.g., uint32)
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
func QuorumCall[T NodeID, Req, Resp msg](
	ctx *ConfigContext[T],
	req Req,
	method string,
	opts ...CallOption,
) *Responses[T, Resp] {
	return invokeQuorumCall[T, Req, Resp](ctx, req, method, false, opts...)
}

// QuorumCallStream performs a streaming quorum call and returns a Responses object.
// This is used for correctable stream methods where the server sends multiple responses.
//
// In streaming mode, the response iterator continues indefinitely until the context
// is canceled, allowing the server to send multiple responses over time.
//
// This function should be used by generated code only.
func QuorumCallStream[T NodeID, Req, Resp msg](
	ctx *ConfigContext[T],
	req Req,
	method string,
	opts ...CallOption,
) *Responses[T, Resp] {
	return invokeQuorumCall[T, Req, Resp](ctx, req, method, true, opts...)
}

// invokeQuorumCall is the internal implementation shared by QuorumCall and QuorumCallStream.
func invokeQuorumCall[T NodeID, Req, Resp msg](
	ctx *ConfigContext[T],
	req Req,
	method string,
	streaming bool,
	opts ...CallOption,
) *Responses[T, Resp] {
	callOpts := getCallOptions(E_Quorumcall, opts...)
	builder := newClientCtxBuilder[T, Req, Resp](ctx, req, method)
	if streaming {
		builder = builder.WithStreaming()
	}
	clientCtx := builder.Build()
	clientCtx.applyInterceptors(callOpts.interceptors)

	return NewResponses(clientCtx)
}
