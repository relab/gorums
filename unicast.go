package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
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
func (n *RawNode) Unicast(ctx context.Context, d CallData, opts ...CallOption) {
	o := getCallOptions(E_Unicast, opts)

	md := ordering.NewGorumsMetadata(ctx, n.mgr.getMsgID(), d.Method)
	req := request{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	if !o.waitSendDone {
		n.channel.enqueue(req, nil, false)
		return // fire-and-forget: don't wait for send completion
	}

	// Default: block until send completes
	replyChan := make(chan response, 1)
	n.channel.enqueue(req, replyChan, false)
	<-replyChan
}
