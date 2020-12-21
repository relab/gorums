package gorums

import (
	"github.com/relab/gorums/ordering"
)

func Multicast(d QuorumCallData) {
	msgID := d.Manager.nextMsgID()
	// set up channel to collect replies to this call.
	replyChan := make(chan *gorumsStreamResult, len(d.Nodes))
	d.Manager.putChan(msgID, replyChan)
	// and remove it when the call is complete
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
		n.sendQ <- gorumsStreamRequest{nil, &Message{Metadata: md, Message: msg}}
	}
}
