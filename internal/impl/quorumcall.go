package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// QuorumCall is exported and documented as gorumsimpl.QuorumCall.
func QuorumCall[Req, Resp proto.Message](ctx context.Context, config Config, req Req, method string) *Call[Req, Resp] {
	return invokeQuorumCall[Req, Resp](ctx, config, req, method, false)
}

// QuorumCallStream is exported and documented as gorumsimpl.QuorumCallStream.
func QuorumCallStream[Req, Resp proto.Message](ctx context.Context, config Config, req Req, method string) *Call[Req, Resp] {
	return invokeQuorumCall[Req, Resp](ctx, config, req, method, true)
}

// invokeQuorumCall is the internal implementation shared by QuorumCall and QuorumCallStream.
func invokeQuorumCall[Req, Resp proto.Message](ctx context.Context, config Config, req Req, method string, streaming bool) *Call[Req, Resp] {
	callCtx := newQuorumCallContext[Req, Resp](ctx, config, req, method, streaming)
	return &Call[Req, Resp]{Responses: newResponses(callCtx), ctx: callCtx}
}
