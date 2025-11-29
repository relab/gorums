package gorums

import (
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// RPCCall executes a remote procedure call on the node.
//
// This method should be used by generated code only.
func RPCCall(ctx *NodeContext, msg proto.Message, method string) (proto.Message, error) {
	n := ctx.node
	md := ordering.NewGorumsMetadata(ctx, n.mgr.getMsgID(), method)
	replyChan := make(chan NodeResponse[proto.Message], 1)
	n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), responseChan: replyChan})

	select {
	case r := <-replyChan:
		return r.Value, r.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
