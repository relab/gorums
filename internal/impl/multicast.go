package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Multicast returns a lazy [OnewayCall] for every node in config.
// On an empty config, it sends nothing and reports [ErrEmptyConfig] when
// consumed.
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
