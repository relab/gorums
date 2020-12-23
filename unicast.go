package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
)

func Unicast(ctx context.Context, d CallData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)

	msgID := d.Manager.nextMsgID()

	var replyChan chan *gorumsStreamResult
	if !o.sendAsync {
		replyChan = make(chan *gorumsStreamResult, 1)
		d.Manager.putChan(msgID, replyChan)
		// and remove it when the call is complete
		defer d.Manager.deleteChan(msgID)
	}

	md := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  d.MethodID,
	}

	d.Node.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	// wait until the message has been sent (nodeStream will give an empty reply when this happens)
	if !o.sendAsync {
		<-replyChan
	}
}
