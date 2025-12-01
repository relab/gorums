package gorums

import (
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
func Unicast[Req proto.Message](ctx *NodeContext, msg Req, method string, opts ...CallOption) {
	n := ctx.node
	o := getCallOptions(E_Unicast, opts...)

	md := ordering.NewGorumsMetadata(ctx, n.mgr.getMsgID(), method)

	waitSendDone := o.mustWaitSendDone()

	if !waitSendDone {
		// Fire-and-forget: enqueue and return immediately
		n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg)})
		return
	}

	// Default: block until send completes
	replyChan := make(chan NodeResponse[proto.Message], 1)
	n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), waitSendDone: true, responseChan: replyChan})

	// Wait for send confirmation
	select {
	case <-replyChan:
	case <-ctx.Done():
	}
}
