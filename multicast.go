package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
)

// Multicast is a one-way call; no replies are processed.
// By default this function returns once the message has been sent to all nodes.
// Providing the call option WithNoSendWaiting, the function may return
// before the message has been sent.
//
// This method should be used by generated code only.
func (c RawConfiguration) Multicast(ctx context.Context, d QuorumCallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), d.Method)
	sentMsgs := 0

	var replyChan chan response
	if !o.noSendWaiting {
		replyChan = make(chan response, c.Size())
	}
	for _, n := range c.Nodes() {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			msg = d.PerNodeArgFn(d.Message, n.id)
			if !msg.ProtoReflect().IsValid() {
				continue // don't send if no msg
			}
		}
		n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}, opts: o}, replyChan, false)
		sentMsgs++
	}

	// if noSendWaiting is set, we will not wait for confirmation from the channel before returning.
	if o.noSendWaiting {
		return
	}

	// nodeStream sends an empty reply on replyChan when the message has been sent
	// wait until the message has been sent
	for ; sentMsgs > 0; sentMsgs-- {
		<-replyChan
	}
}
