package gorums

import (
	"context"
)

func Unicast(ctx context.Context, d CallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)
	md, replyChan, callDone := d.Manager.newCall(d.Method, 1, !o.sendAsync)
	d.Node.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	// wait until the message has been sent (nodeStream will give an empty reply when this happens)
	if !o.sendAsync {
		<-replyChan
		callDone()
	}
}
