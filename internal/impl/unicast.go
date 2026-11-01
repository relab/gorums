package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Unicast returns a lazy [OnewayCall] for node.
// If node is nil, it sends nothing and reports [ErrNoNode] when consumed.
func Unicast[Req proto.Message](ctx context.Context, node *Node, req Req, method string) *OnewayCall[Req] {
	callCtx := &CallContext[Req, *emptypb.Empty]{
		Context: ctx,
		request: req,
		method:  method,
		oneway:  true,
	}
	if node == nil {
		callCtx.err = ErrNoNode
	} else {
		callCtx.config = Config{node}
	}
	return &OnewayCall[Req]{ctx: callCtx, unicast: true}
}
