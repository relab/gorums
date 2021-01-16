package gorums

import (
	"context"
)

func Multicast(ctx context.Context, d QuorumCallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)
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

	// wait until the messages have been sent (nodeStream will give empty replies when this happens)
	if !o.sendAsync {
		for range d.Nodes {
			<-replyChan
		}
		callDone()
	}
}
