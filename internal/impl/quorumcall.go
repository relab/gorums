package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// QuorumCall is exported and documented as gorumsimpl.QuorumCall.
func QuorumCall[Req, Resp proto.Message](ctx context.Context, config Config, req Req, method string) *Call[Req, Resp] {
	return newCall(newQuorumCallContext[Req, Resp](ctx, config, req, method, false))
}

// QuorumCallStream is exported and documented as gorumsimpl.QuorumCallStream.
func QuorumCallStream[Req, Resp proto.Message](ctx context.Context, config Config, req Req, method string) *Call[Req, Resp] {
	return newCall(newQuorumCallContext[Req, Resp](ctx, config, req, method, true))
}

// QuorumCallScatter is exported and documented as gorumsimpl.QuorumCallScatter.
func QuorumCallScatter[Req, Resp proto.Message](ctx context.Context, config Config, reqs []Req, method string) *Call[Req, Resp] {
	return newCall(newScatterQuorumCallContext[Req, Resp](ctx, config, reqs, method, false))
}

// QuorumCallStreamScatter is exported and documented as gorumsimpl.QuorumCallStreamScatter.
func QuorumCallStreamScatter[Req, Resp proto.Message](ctx context.Context, config Config, reqs []Req, method string) *Call[Req, Resp] {
	return newCall(newScatterQuorumCallContext[Req, Resp](ctx, config, reqs, method, true))
}

// newCall returns the call handle of callCtx.
func newCall[Req, Resp proto.Message](callCtx *CallContext[Req, Resp]) *Call[Req, Resp] {
	return &Call[Req, Resp]{Responses: newResponses(callCtx), ctx: callCtx}
}
