package gorums

import (
	"context"
)

// Unicast is a one-way call; no replies are processed.
// By default this function returns once the message has been sent.
// Providing the call option WithNoSendWaiting, the function may return
// before the message has been sent.
func (n *Node) Unicast(ctx context.Context, d CallData, opts ...CallOption) {
	o := getCallOptions(E_Unicast, opts)

	md := n.newCall(d.Method)
	req := request{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	if o.noSendWaiting {
		n.sendQ <- req
		return // don't wait for message to be sent
	}

	// newReply must be called before adding req to sendQ
	replyChan, callDone := n.newReply(md, 1)
	n.sendQ <- req
	// nodeStream sends an empty reply on replyChan when the message has been sent
	// wait until the message has been sent
	<-replyChan
	callDone()
}
