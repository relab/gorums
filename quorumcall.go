package gorums

import (
	"cmp"
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// QuorumCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
//
// This struct should be used by generated code only.
type QuorumCallData[idType cmp.Ordered] struct {
	Message        protoreflect.ProtoMessage
	Method         string
	PerNodeArgFn   func(protoreflect.ProtoMessage, idType) protoreflect.ProtoMessage
	QuorumFunction func(protoreflect.ProtoMessage, map[idType]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool)
}

// QuorumCall performs a quorum call on the configuration.
//
// This method should be used by generated code only.
func (c RawConfiguration[idType]) QuorumCall(ctx context.Context, d QuorumCallData[idType]) (resp protoreflect.ProtoMessage, err error) {
	expectedReplies := len(c)
	md := ordering.Metadata_builder{MessageID: c.getMsgID(), Method: d.Method}.Build()

	replyChan := make(chan response[idType], expectedReplies)
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
		errs    []nodeError[idType]
		quorum  bool
		replies = make(map[idType]protoreflect.ProtoMessage)
	)

	for {
		select {
		case r := <-replyChan:
			if r.err != nil {
				errs = append(errs, nodeError[idType]{nodeID: r.nid, cause: r.err})
				break
			}
			replies[r.nid] = r.msg
			if resp, quorum = d.QuorumFunction(d.Message, replies); quorum {
				return resp, nil
			}
		case <-ctx.Done():
			return resp, QuorumCallError[idType]{cause: ctx.Err(), errors: errs, replies: len(replies)}
		}
		if len(errs)+len(replies) == expectedReplies {
			return resp, QuorumCallError[idType]{cause: Incomplete, errors: errs, replies: len(replies)}
		}
	}
}
