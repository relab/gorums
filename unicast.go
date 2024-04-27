package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
)

// Unicast is a one-way call; no replies are processed.
// By default this function returns once the message has been sent.
// Providing the call option WithNoSendWaiting, the function may return
// before the message has been sent.
func (n *RawNode) Unicast(ctx context.Context, d CallData, opts ...CallOption) {
	o := getCallOptions(E_Unicast, opts)

	md := &ordering.Metadata{MessageID: n.mgr.getMsgID(), Method: d.Method, BroadcastMsg: &ordering.BroadcastMsg{BroadcastID: d.BroadcastID}}
	req := request{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	if o.noSendWaiting {
		n.channel.enqueue(req, nil, false)
		return // don't wait for message to be sent
	}

	// newReply must be called before adding req to sendQ
	replyChan := make(chan response, 1)
	n.channel.enqueue(req, replyChan, false)
	// channel sends an empty reply on replyChan when the message has been sent
	// wait until the message has been sent
	<-replyChan
}
