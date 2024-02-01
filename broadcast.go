package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// QuorumCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
//
// This struct should be used by generated code only.
type BroadcastCallData struct {
	Message     protoreflect.ProtoMessage
	Method      string
	Sender      string
	BroadcastID string // a unique identifier for the current message
}

// BroadcastCall performs a multicast call on the configuration.
//
// This method should be used by generated code only.
func (c RawConfiguration) BroadcastCall(ctx context.Context, d BroadcastCallData) {
	md := &ordering.Metadata{MessageID: c.getMsgID(), Method: d.Method, BroadcastMsg: &ordering.BroadcastMsg{
		Sender: d.Sender, BroadcastID: d.BroadcastID,
	}}

	for _, n := range c {
		msg := d.Message
		n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}, nil, false)
	}
}
