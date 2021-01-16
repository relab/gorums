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
	// sendAsync == true => replyChan and callDone are nil and thus cannot be used
	md, replyChan, callDone := d.Manager.newCall(d.Method, len(d.Nodes), !o.sendAsync)

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

	if !o.sendAsync {
		// wait until the messages have been sent
		// (nodeStream will produce empty replies when this happens)
		for range d.Nodes {
			<-replyChan
		}
		callDone()
	}
}
