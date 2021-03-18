package gorums

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// QuorumCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
type QuorumCallData struct {
	Message        protoreflect.ProtoMessage
	Method         string
	PerNodeArgFn   func(protoreflect.ProtoMessage, uint32) protoreflect.ProtoMessage
	QuorumFunction func(protoreflect.ProtoMessage, map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)
}

func (c Configuration) QuorumCall(ctx context.Context, d QuorumCallData, opts ...CallOption) (resp protoreflect.ProtoMessage, err error) {
	o := getCallOptions(E_Quorumcall, opts)
	expectedReplies := len(c)
	md := c.newCall(d.Method)
	replyChan, callDone := c.newReply(md, expectedReplies)
	defer callDone()

	channels := o.getChannels(c)
	for i, n := range c {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			msg = d.PerNodeArgFn(d.Message, n.id)
			if !msg.ProtoReflect().IsValid() {
				expectedReplies--
				continue // don't send if no msg
			}
		}
		channels[i].sendQ <- request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}
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
			replies[r.nid] = r.msg
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
