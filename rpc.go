package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// CallData contains data needed to make a remote procedure call.
//
// This struct should be used by generated code only.
type CallData struct {
	Message     protoreflect.ProtoMessage
	Method      string
	BroadcastID uint64
}

// RPCCall executes a remote procedure call on the node.
//
// This method should be used by generated code only.
func (n *RawNode) RPCCall(ctx context.Context, d CallData) (protoreflect.ProtoMessage, error) {
	md := &ordering.Metadata{MessageID: n.mgr.getMsgID(), Method: d.Method, BroadcastMsg: &ordering.BroadcastMsg{BroadcastID: d.BroadcastID}}

	//md := &ordering.Metadata{MessageID: n.mgr.getMsgID(), Method: d.Method}
	replyChan := make(chan response, 1)
	n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}}, replyChan, false)

	select {
	case r := <-replyChan:
		return r.msg, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
