package gorums

import (
	"github.com/relab/gorums/ordering"
)

func Unicast(d CallData) {
	msgID := d.Manager.nextMsgID()

	md := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  d.MethodID,
	}

	d.Node.sendQ <- gorumsStreamRequest{nil, &Message{Metadata: md, Message: d.Message}}
}
