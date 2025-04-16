package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// QuorumCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
//
// This struct should be used by generated code only.
type QuorumCallData struct {
	Message      proto.Message
	Method       string
	PerNodeArgFn func(proto.Message, uint32) proto.Message
	ServerStream bool
}

// QuorumCall performs a quorum call on the configuration.
//
// This method should be used by generated code only.
func QuorumCall[responseType proto.Message](
	ctx context.Context,
	c RawConfiguration,
	d QuorumCallData,
) Iterator[responseType] {
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
		n.channel.enqueue(
			request{
				ctx: ctx,
				msg: &Message{Metadata: md, Message: msg},
			},
			replyChan, d.ServerStream,
		)
	}

	if d.ServerStream {
		for _, n := range c {
			defer n.channel.deleteRouter(md.GetMessageID())
		}
	}

	replies := int(0)

	return func(yield func(Response[responseType]) bool) {
		for {
			select {
			case r := <-replyChan:
				replies++
				if !yield(NewResponse(r.msg.(responseType), r.err, r.nid)) {
					return
				}
			case <-ctx.Done():
				return
			}
			// for streaming just wait for context timeout/cancel
			if d.ServerStream && replies >= expectedReplies {
				return
			}
		}
	}
}
