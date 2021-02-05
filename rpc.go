package gorums

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type CallData struct {
	Node    *Node
	Message protoreflect.ProtoMessage
	Method  string
}

func RPCCall(ctx context.Context, d CallData) (resp protoreflect.ProtoMessage, err error) {
	md := d.Node.newCall(d.Method)
	replyChan, callDone := d.Node.newReply(md, 1)
	defer callDone()

	d.Node.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}}

	select {
	case r := <-replyChan:
		if r.err != nil {
			return nil, err
		}
		return r.reply, nil
	case <-ctx.Done():
		return resp, ctx.Err()
	}
}
