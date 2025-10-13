package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
)

// Multicast is a one-way call; no replies are returned to the client.
//
// By default, this method blocks until messages have been sent to all nodes.
// This ensures that send operations complete before the caller proceeds, which can
// be useful for observing context cancellation or for pacing message sends.
//
// With the WithNoSendWaiting call option, the method returns immediately after
// enqueueing messages to all nodes (fire-and-forget semantics).
//
// This method should be used by generated code only.
func (c RawConfiguration) Multicast(ctx context.Context, d QuorumCallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), d.Method)
	sentMsgs := 0

	var replyChan chan response
	if o.waitSendDone {
		replyChan = make(chan response, len(c))
	}
	for _, n := range c {
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

	// Fire-and-forget: return immediately without waiting for send completion
	if !o.waitSendDone {
		return
	}

	// Default: block until all sends complete
	for ; sentMsgs > 0; sentMsgs-- {
		<-replyChan
	}
}
