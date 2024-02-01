package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// QuorumCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
//
// This struct should be used by generated code only.
type QuorumCallData struct {
	Message        protoreflect.ProtoMessage
	Method         string
	PerNodeArgFn   func(protoreflect.ProtoMessage, uint32) protoreflect.ProtoMessage
	QuorumFunction func(protoreflect.ProtoMessage, map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)
	Sender         string
	BroadcastID    string // a unique identifier for the current message
}

// QuorumCall performs a quorum call on the configuration.
//
// This method should be used by generated code only.
func (c RawConfiguration) QuorumCall(ctx context.Context, d QuorumCallData) (resp protoreflect.ProtoMessage, err error) {
	expectedReplies := len(c)
	md := &ordering.Metadata{MessageID: c.getMsgID(), Method: d.Method, Sender: d.Sender, BroadcastID: d.BroadcastID}

	replyChan := make(chan response, expectedReplies)
	for _, n := range c {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			msg = d.PerNodeArgFn(d.Message, n.id)
			if !msg.ProtoReflect().IsValid() {
				expectedReplies--
				continue // don't send if no msg
			}
		}
		n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}, replyChan, false)
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
			return resp, QuorumCallError{ctx.Err().Error(), len(replies), errs}
		}
		if len(errs)+len(replies) == expectedReplies {
			return resp, QuorumCallError{"incomplete call", len(replies), errs}
		}
	}
}
