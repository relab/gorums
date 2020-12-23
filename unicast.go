package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
)

func Unicast(ctx context.Context, d CallData) {
	msgID := d.Manager.nextMsgID()

	md := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  d.MethodID,
	}

	d.Node.sendQ <- gorumsStreamRequest{ctx, &Message{Metadata: md, Message: d.Message}}
}
