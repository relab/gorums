package impl

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// QuorumCall returns a lazy [Call] for the nodes in config.
// Consuming a call on an empty config sends nothing and reports
// [ErrEmptyConfig].
func QuorumCall[Req, Resp proto.Message](ctx context.Context, config Config, req Req, method string) *Call[Req, Resp] {
	return invokeQuorumCall[Req, Resp](ctx, config, req, method, false)
}

// QuorumCallStream returns a lazy [Call] that accepts multiple responses per
// node until ctx ends. Consuming a call on an empty config sends nothing and
// reports [ErrEmptyConfig].
func QuorumCallStream[Req, Resp proto.Message](ctx context.Context, config Config, req Req, method string) *Call[Req, Resp] {
	return invokeQuorumCall[Req, Resp](ctx, config, req, method, true)
}

// invokeQuorumCall is the internal implementation shared by QuorumCall and QuorumCallStream.
func invokeQuorumCall[Req, Resp proto.Message](ctx context.Context, config Config, req Req, method string, streaming bool) *Call[Req, Resp] {
	callCtx := newQuorumCallContext[Req, Resp](ctx, config, req, method, streaming)
	return &Call[Req, Resp]{Responses: newResponses(callCtx), ctx: callCtx}
}
