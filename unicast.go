package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// Unicast is a one-way call; no replies are returned to the client.
//
// By default, this method blocks until the message has been sent to the node.
// This ensures that send operations complete before the caller proceeds, which can
// be useful for observing context cancellation or for pacing message sends.
//
// With the WithNoSendWaiting call option, the method returns immediately after
// enqueueing the message (fire-and-forget semantics).
//
// This method should be used by generated code only.
func (n *RawNode) Unicast(ctx context.Context, msg proto.Message, method string, opts ...CallOption) {
	o := getCallOptions(E_Unicast, opts)

	md := ordering.NewGorumsMetadata(ctx, n.mgr.getMsgID(), method)

	if !o.waitSendDone {
		n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), opts: o})
		return // fire-and-forget: don't wait for send completion
	}

	// Default: block until send completes
	replyChan := make(chan NodeResponse[proto.Message], 1)
	n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), opts: o, responseChan: replyChan})
	<-replyChan
}
