package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
)

func Multicast(ctx context.Context, d QuorumCallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)

	msgID := d.Manager.nextMsgID()

	var replyChan chan *gorumsStreamResult
	if !o.sendAsync {
		replyChan = make(chan *gorumsStreamResult, len(d.Nodes))
		d.Manager.putChan(msgID, replyChan)
		// and remove it when the call is complete
		defer d.Manager.deleteChan(msgID)
	}

	md := &ordering.Metadata{
		MessageID: msgID,
		Method:    d.Method,
	}

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
	}
}
