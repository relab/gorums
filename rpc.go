package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type CallData struct {
	Manager  *Manager
	Node     *Node
	Message  protoreflect.ProtoMessage
	MethodID int32
}

func RPCCall(ctx context.Context, d CallData) (resp protoreflect.ProtoMessage, err error) {
	msgID := d.Manager.nextMsgID()
	// set up channel to collect replies to this call.
	replyChan := make(chan *orderingResult, 1)
	d.Manager.putChan(msgID, replyChan)
	// and remove it when the call it scomplete
	defer d.Manager.deleteChan(msgID)

	md := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  d.MethodID,
	}

	d.Node.sendQ <- &Message{Metadata: md, Message: d.Message}

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
