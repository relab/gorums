package gorums

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type CallData struct {
	Message protoreflect.ProtoMessage
	Method  string
}

func (n *Node) RPCCall(ctx context.Context, d CallData) (resp protoreflect.ProtoMessage, err error) {
	md := n.newCall(d.Method)
	replyChan, callDone := n.newReply(md, 1)
	defer callDone()

	n.sendQ <- request{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}}

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
