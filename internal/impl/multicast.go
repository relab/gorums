package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Multicast is a one-way call to every node in config; no replies are
// returned to the client. It returns an
// [OnewayCall] handle that dispatches the request only when it is consumed:
// [OnewayCall.Send] blocks until the send completes for every node and reports
// any send failures, while [OnewayCall.Async] dispatches without waiting and
// defers those failures to [OnewayAsync.Wait]. If config is empty, the handle
// sends nothing and reports [ErrEmptyConfig].
//
// Register per-node request transforms with [OnewayCall.Intercept] and
// [MapRequest] before consuming the handle.
//
// This function should be used by generated code only.
func Multicast[Req proto.Message](ctx context.Context, config Config, req Req, method string) *OnewayCall[Req] {
	callCtx := &CallContext[Req, *emptypb.Empty]{
		Context: ctx,
		config:  config,
		request: req,
		method:  method,
		oneway:  true,
	}
	if len(config) == 0 {
		callCtx.err = ErrEmptyConfig
	}
	return &OnewayCall[Req]{ctx: callCtx}
}
