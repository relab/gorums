package impl

import (
	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"google.golang.org/protobuf/proto"
)

// RemoteCall executes a remote procedure call on the node.
//
// This method should be used by generated code only.
func RemoteCall[Req, Resp proto.Message](ctx *NodeContext, req Req, method string) (Resp, error) {
	replyChan := make(chan NodeResponse[*stream.Message], 1)
	node := ctx.Node()
	transport := conn.NodeTransport(node)
	reqMsg, err := stream.NewMessage(ctx, transport.NextMsgID(), method, req)
	if err != nil {
		var zero Resp
		return zero, err
	}
	transport.Enqueue(stream.Request{Ctx: ctx, Msg: reqMsg, ResponseChan: replyChan})

	select {
	case r := <-replyChan:
		var zero Resp
		if r.Err != nil {
			return zero, r.Err
		}
		respMsg, err := unmarshalResponse(r.Value)
		if err != nil {
			return zero, err
		}
		resp, ok := respMsg.(Resp)
		if !ok {
			return zero, stream.ErrTypeMismatch
		}
		return resp, nil
	case <-ctx.Done():
		var zero Resp
		return zero, ctx.Err()
	}
}
