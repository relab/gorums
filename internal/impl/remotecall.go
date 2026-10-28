package impl

import (
	"context"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
)

// RemoteCall executes a remote procedure call on node. It returns
// [ErrNoNode] if node is nil.
//
// This method should be used by generated code only.
func RemoteCall[Req, Resp proto.Message](ctx context.Context, node *Node, req Req, method string) (Resp, error) {
	if node == nil {
		var zero Resp
		return zero, ErrNoNode
	}
	responseChan := make(chan NodeResponse[*stream.Message], 1)
	transport := conn.NodeTransport(node)
	reqMsg, err := stream.NewMessage(ctx, transport.NextMsgID(), method, req)
	if err != nil {
		var zero Resp
		return zero, err
	}
	transport.Enqueue(stream.Request{Ctx: ctx, Msg: reqMsg, ResponseChan: responseChan})

	select {
	case r := <-responseChan:
		resp := mapToCallResponse[Resp](r)
		return resp.Value, resp.Err
	case <-ctx.Done():
		var zero Resp
		return zero, ctx.Err()
	}
}
