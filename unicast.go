package gorums

import (
	"context"
)

// Unicast is a one-way call; no replies are processed.
// By default this function returns once the message has been sent.
// Providing the call option WithAsyncSend, the function may return
// before the message has been sent.
func Unicast(ctx context.Context, d CallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)

	md := d.Node.newCall(d.Method)
	d.Node.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	if o.noSendWaiting {
		// don't wait for message to be sent
		return
	}
	replyChan, callDone := d.Node.newReply(md, 1)
	// wait until the message has been sent (nodeStream sends an empty reply when this happens)
	<-replyChan
	callDone()
}
