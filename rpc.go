package gorums

import (
	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// RPCCall executes a remote procedure call on the node.
//
// This method should be used by generated code only.
func RPCCall[T NodeID](ctx *NodeContext[T], msg proto.Message, method string) (proto.Message, error) {
	md := ordering.NewGorumsMetadata(ctx, ctx.nextMsgID(), method)
	replyChan := make(chan NodeResponse[T, proto.Message], 1)
	ctx.enqueue(request[T]{ctx: ctx, msg: NewRequestMessage(md, msg), responseChan: replyChan})

	select {
	case r := <-replyChan:
		return r.Value, r.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
