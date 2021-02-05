package gorums

import (
	"context"
)

// Multicast is a one-way call; no replies are processed.
// By default this function returns once the message has been sent to all nodes.
// Providing the call option WithAsyncSend, the function may return
// before the message has been sent.
func Multicast(ctx context.Context, d QuorumCallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)

	md := d.Manager.newCall(d.Method)
	for _, n := range d.Nodes {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			nodeArg := d.PerNodeArgFn(d.Message, n.id)
			if nodeArg != nil {
				continue
			}
		}
		n.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: msg}, opts: o}
	}

	// TODO rename sendAsync to waitSend or waitSendCompletion
	if o.sendAsync {
		// don't wait for messages to be sent
		return
	}
	replyChan, callDone := d.Manager.newReply(md, len(d.Nodes))
	// wait until the messages have been sent (nodeStream sends empty replies when this happens)
	for range d.Nodes {
		<-replyChan
	}
	callDone()
}
