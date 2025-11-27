package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// CallData contains data needed to make a remote procedure call.
//
// This struct should be used by generated code only.
type CallData struct {
	Message proto.Message
	Method  string
}

// RPCCall executes a remote procedure call on the node.
//
// This method should be used by generated code only.
func (n *RawNode) RPCCall(ctx context.Context, d CallData) (proto.Message, error) {
	md := ordering.NewGorumsMetadata(ctx, n.mgr.getMsgID(), d.Method)
	replyChan := make(chan NodeResponse[proto.Message], 1)
	n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, d.Message), responseChan: replyChan})

	select {
	case r := <-replyChan:
		return r.Value, r.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
