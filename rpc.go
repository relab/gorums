package gorums

import (
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// RPCCall executes a remote procedure call on the node.
//
// This method should be used by generated code only.
func RPCCall[Req, Resp proto.Message](ctx *NodeContext, req Req, method string) (Resp, error) {
	md := ordering.NewGorumsMetadata(ctx, ctx.nextMsgID(), method)
	replyChan := make(chan NodeResponse[msg], 1)
	ctx.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, req), responseChan: replyChan})

	select {
	case r := <-replyChan:
		if r.Err != nil {
			var zero Resp
			return zero, r.Err
		}
		return r.Value.(Resp), r.Err
	case <-ctx.Done():
		var zero Resp
		return zero, ctx.Err()
	}
}
