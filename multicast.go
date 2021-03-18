package gorums

import (
	"context"
)

// Multicast is a one-way call; no replies are processed.
// By default this function returns once the message has been sent to all nodes.
// Providing the call option WithNoSendWaiting, the function may return
// before the message has been sent.
func (c Configuration) Multicast(ctx context.Context, d QuorumCallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)

	md := c.newCall(d.Method)
	sentMsgs := 0
	send := func() {
		channels := o.getChannels(c)
		for i, n := range c {
			msg := d.Message
			if d.PerNodeArgFn != nil {
				msg = d.PerNodeArgFn(d.Message, n.id)
				if !msg.ProtoReflect().IsValid() {
					continue // don't send if no msg
				}
			}
			channels[i].sendQ <- request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}, opts: o}
			sentMsgs++
		}
	}

	if o.noSendWaiting {
		send()
		return // don't wait for messages to be sent
	}

	replyChan, callDone := c.newReply(md, sentMsgs)
	send()

	// nodeStream sends an empty reply on replyChan when the message has been sent
	// wait until the message has been sent
	for sentMsgs > 0 {
		<-replyChan
		sentMsgs--
	}
	callDone()
}
