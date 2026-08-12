package impl

import "google.golang.org/protobuf/proto"

// QuorumCall performs a quorum call and returns a [Call] handle that provides
// access to node responses via terminal methods and fluent iteration.
//
// Type parameters:
//   - Req: The request message type
//   - Resp: The response message type from individual nodes
//
// Register interceptors with [Call.Intercept] before invoking a terminal
// method. Messages are not sent to nodes until a terminal method (like Majority
// or First) or iterator method (like Results) is called, applying any registered
// request transformations. This lazy sending is what lets interceptors register
// transformations prior to dispatch.
//
// This function should only be used by generated code.
func QuorumCall[Req, Resp proto.Message](ctx *ConfigContext, req Req, method string) *Call[Req, Resp] {
	return invokeQuorumCall[Req, Resp](ctx, req, method, false)
}

// QuorumCallStream performs a streaming quorum call and returns a [Call] handle.
// This is used for correctable stream methods where the server sends multiple responses.
//
// In streaming mode, the response iterator continues indefinitely until the context
// is canceled, allowing the server to send multiple responses over time.
//
// This function should only be used by generated code.
func QuorumCallStream[Req, Resp proto.Message](ctx *ConfigContext, req Req, method string) *Call[Req, Resp] {
	return invokeQuorumCall[Req, Resp](ctx, req, method, true)
}

// invokeQuorumCall is the internal implementation shared by QuorumCall and QuorumCallStream.
func invokeQuorumCall[Req, Resp proto.Message](ctx *ConfigContext, req Req, method string, streaming bool) *Call[Req, Resp] {
	callCtx := newQuorumCallContext[Req, Resp](ctx, req, method, streaming)
	return &Call[Req, Resp]{Responses: newResponses(callCtx), ctx: callCtx}
}
