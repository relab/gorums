package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
)

func Multicast(ctx context.Context, d CallData) {
	msgID := d.Manager.nextMsgID()
	// set up channel to collect replies to this call.
	replyChan := make(chan *orderingResult, len(d.Nodes))
	d.Manager.putChan(msgID, replyChan)
	// and remove it when the call it scomplete
	defer d.Manager.deleteChan(msgID)

	md := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  d.MethodID,
	}

	for _, n := range d.Nodes {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			nodeArg := d.PerNodeArgFn(d.Message, n.id)
			if nodeArg != nil {
				continue
			}
		}
		n.sendQ <- &Message{Metadata: md, Message: msg}
	}
}
