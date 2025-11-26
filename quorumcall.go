package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// QuorumCallData holds the message, destination nodes, method identifier,
// and other information necessary to perform the various quorum call types
// supported by Gorums.
//
// This struct should be used by generated code only.
type QuorumCallData struct {
	Message        proto.Message
	Method         string
	PerNodeArgFn   func(proto.Message, uint32) proto.Message
	QuorumFunction func(proto.Message, map[uint32]proto.Message) (proto.Message, bool)
}

// QuorumCall performs a quorum call on the configuration.
//
// This method should be used by generated code only.
func (c RawConfiguration) QuorumCall(ctx context.Context, d QuorumCallData) (resp proto.Message, err error) {
	expectedReplies := len(c)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), d.Method)

	replyChan := make(chan Result[proto.Message], expectedReplies)
	for _, n := range c {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			msg = d.PerNodeArgFn(d.Message, n.id)
			if !msg.ProtoReflect().IsValid() {
				expectedReplies--
				continue // don't send if no msg
			}
		}
		n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg), responseChan: replyChan})
	}

	var (
		errs    []nodeError
		quorum  bool
		replies = make(map[uint32]proto.Message)
	)

	for {
		select {
		case r := <-replyChan:
			if r.Err != nil {
				errs = append(errs, nodeError{nodeID: r.NodeID, cause: r.Err})
				break
			}
			replies[r.NodeID] = r.Value
			if resp, quorum = d.QuorumFunction(d.Message, replies); quorum {
				return resp, nil
			}
		case <-ctx.Done():
			return resp, QuorumCallError{cause: ctx.Err(), errors: errs, replies: len(replies)}
		}
		if len(errs)+len(replies) == expectedReplies {
			return resp, QuorumCallError{cause: Incomplete, errors: errs, replies: len(replies)}
		}
	}
}
