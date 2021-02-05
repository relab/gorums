package gorums

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// QuorumCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
type QuorumCallData struct {
	Manager        *Manager
	Nodes          []*Node
	Message        protoreflect.ProtoMessage
	Method         string
	PerNodeArgFn   func(protoreflect.ProtoMessage, uint32) protoreflect.ProtoMessage
	QuorumFunction func(protoreflect.ProtoMessage, map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)
}

func QuorumCall(ctx context.Context, d QuorumCallData) (resp protoreflect.ProtoMessage, err error) {
	expectedReplies := len(d.Nodes)
	md := d.Manager.newCall(d.Method)
	replyChan, callDone := d.Manager.newReply(md, expectedReplies)
	defer callDone()

	for _, n := range d.Nodes {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			nodeArg := d.PerNodeArgFn(d.Message, n.id)
			if nodeArg != nil {
				expectedReplies--
				continue
			}
		}
		n.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}
	}

	var (
		errs    []Error
		quorum  bool
		replies = make(map[uint32]protoreflect.ProtoMessage)
	)

	for {
		select {
		case r := <-replyChan:
			if r.err != nil {
				errs = append(errs, Error{r.nid, r.err})
				break
			}
			replies[r.nid] = r.reply
			if resp, quorum = d.QuorumFunction(d.Message, replies); quorum {
				return resp, nil
			}
		case <-ctx.Done():
			return resp, QuorumCallError{"incomplete call", len(replies), errs}
		}
		if len(errs)+len(replies) == expectedReplies {
			return resp, QuorumCallError{"incomplete call", len(replies), errs}
		}
	}
}
