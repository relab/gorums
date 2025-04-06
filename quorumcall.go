package gorums

import (
	"context"
	"iter"

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
}

// QuorumCall performs a quorum call on the configuration.
//
// This method should be used by generated code only.
func QuorumCall[responseType protoreflect.ProtoMessage](ctx context.Context, c RawConfiguration, d QuorumCallData) iter.Seq2[responseType, error] {
	expectedReplies := len(c)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), d.Method)

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

	replies := int(0)
	var noResponse responseType

	return func(yield func(K responseType, V error) bool) {
		for {
			select {
			case r := <-replyChan:
				replies++
				if !yield(r.msg.(responseType), r.err) {
					return
				}
			case <-ctx.Done():
				yield(noResponse, QuorumCallError{cause: ctx.Err()})
				return
			}
			if replies >= expectedReplies {
				yield(noResponse, QuorumCallError{cause: Incomplete})
				return
			}
		}
	}
}
