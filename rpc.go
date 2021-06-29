package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type CallData struct {
	Message protoreflect.ProtoMessage
	Method  string
}

func (n *Node) RPCCall(ctx context.Context, d CallData) (resp protoreflect.ProtoMessage, err error) {
	md := &ordering.Metadata{MessageID: n.mgr.getMsgID(), Method: d.Method}
	replyChan := make(chan response, 1)
	n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}}, replyChan, false)

	select {
	case r := <-replyChan:
		if r.err != nil {
			return nil, err
		}
		return r.msg, nil
	case <-ctx.Done():
		return resp, ctx.Err()
	}
}
