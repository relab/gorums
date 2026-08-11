package impl

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Unicast is a one-way call to the single node in ctx; no reply is returned to
// the client. It returns an [OnewayCall] handle that dispatches the request only
// when it is consumed: [OnewayCall.Send] blocks until the send completes and
// reports any send error, while [OnewayCall.Async] dispatches without waiting
// and defers that error to [OnewayAsync.Wait].
//
// Register request transforms with [OnewayCall.Intercept] and [MapRequest]
// before consuming the handle.
//
// This function should be used by generated code only.
func Unicast[Req proto.Message](ctx *NodeContext, req Req, method string) *OnewayCall[Req] {
	callCtx := &CallContext[Req, *emptypb.Empty]{
		Context: ctx,
		config:  Config{ctx.Node()},
		request: req,
		method:  method,
		oneway:  true,
	}
	return &OnewayCall[Req]{ctx: callCtx, unicast: true}
}
