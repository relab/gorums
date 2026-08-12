package gorums

import "google.golang.org/protobuf/proto"

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
	return &OnewayCall[Req]{
		ctx:     newOnewayCallContext(ctx, Config{ctx.Node()}, req, method),
		unicast: true,
	}
}
