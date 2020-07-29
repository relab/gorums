package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type CallData struct {
	Manager        *Manager
	Nodes          []*Node
	Message        protoreflect.ProtoMessage
	MethodID       int32
	PerNodeArgFn   func(protoreflect.ProtoMessage, uint32) protoreflect.ProtoMessage
	QuorumFunction func(protoreflect.ProtoMessage, map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)
}

func QuorumCall(ctx context.Context, d CallData) (resp protoreflect.ProtoMessage, err error) {
	msgID := d.Manager.nextMsgID()
	// set up channel to collect replies to this call.
	replyChan := make(chan *orderingResult, len(d.Nodes))
	d.Manager.putChan(msgID, replyChan)
	// and remove it when the call it scomplete
	defer d.Manager.deleteChan(msgID)

	md := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  d.MethodID,
	}

	expected := len(d.Nodes)
	for _, n := range d.Nodes {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			nodeArg := d.PerNodeArgFn(d.Message, n.id)
			if nodeArg != nil {
				expected--
				continue
			}
		}
		n.sendQ <- &Message{Metadata: md, Message: msg}
	}

	var (
		errs    []GRPCError
		quorum  bool
		replies = make(map[uint32]protoreflect.ProtoMessage)
	)

	for {
		select {
		case r := <-replyChan:
			if r.err != nil {
				errs = append(errs, GRPCError{r.nid, r.err})
				break
			}
			reply := r.reply
			replies[r.nid] = reply
			if resp, quorum = d.QuorumFunction(d.Message, replies); quorum {
				return resp, nil
			}
		case <-ctx.Done():
			return resp, QuorumCallError{"incomplete call", len(replies), errs}
		}
		if len(errs)+len(replies) == expected {
			return resp, QuorumCallError{"incomplete call", len(replies), errs}
		}
	}
}
