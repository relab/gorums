package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Multicast is exported and documented as gorumsimpl.Multicast.
func Multicast[Req proto.Message](ctx context.Context, config Config, req Req, method string) *OnewayCall[Req] {
	callCtx := &CallContext[Req, *emptypb.Empty]{
		Context: ctx,
		config:  config,
		request: req,
		method:  method,
		oneway:  true,
	}
	callCtx.err = callCtx.configError()
	return &OnewayCall[Req]{ctx: callCtx}
}

// MulticastScatter is exported and documented as gorumsimpl.MulticastScatter.
func MulticastScatter[Req proto.Message](ctx context.Context, config Config, reqs []Req, method string) *OnewayCall[Req] {
	callCtx := &CallContext[Req, *emptypb.Empty]{
		Context:  ctx,
		config:   config,
		requests: reqs,
		scatter:  true,
		method:   method,
		oneway:   true,
	}
	callCtx.err = callCtx.configError()
	return &OnewayCall[Req]{ctx: callCtx}
}
