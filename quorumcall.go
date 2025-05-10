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
) Responses[responseType] {
	nodes := len(c)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), d.Method)

	replyChan := make(chan response, nodes)

	for _, n := range c {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			msg = d.PerNodeArgFn(d.Message, n.id)
			if !msg.ProtoReflect().IsValid() {
				nodes--
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

	return func(yield func(Response[responseType]) bool) {
		replies := int(0)
		errors := int(0)

		defer close(replyChan)
		for _, n := range c {
			defer n.channel.deleteRouter(md.GetMessageID())
		}

		for {
			if d.ServerStream {
				if errors >= nodes {
					return
				}
			} else {
				if replies >= nodes {
					return
				}
			}
			select {
			case r := <-replyChan:
				replies++
				if r.err != nil {
					errors++
				}
				var msg responseType
				if r.msg != nil {
					msg = r.msg.(responseType)
				}
				if !yield(NewResponse(msg, r.err, r.nid)) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
}
