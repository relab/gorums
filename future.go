package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Future struct {
	reply protoreflect.ProtoMessage
	err   error
	c     chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *Future) Get() (protoreflect.ProtoMessage, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *Future) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

func FutureCall(ctx context.Context, d QuorumCallData) *Future {
	msgID := d.Manager.nextMsgID()
	// set up channel to collect replies to this call.
	replyChan := make(chan *gorumsStreamResult, len(d.Nodes))
	d.Manager.putChan(msgID, replyChan)

	md := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  d.MethodID,
	}

	expected := len(d.Nodes)
	for _, n := range d.Nodes {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			nodeArg := d.PerNodeArgFn(d.Message, n.id)
			if nodeArg != nil {
				expected--
				continue
			}
		}
		n.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}
	}

	fut := &Future{c: make(chan struct{}, 1)}

	go func() {
		defer d.Manager.deleteChan(msgID)
		defer close(fut.c)

		var (
			resp    protoreflect.ProtoMessage
			errs    []Error
			quorum  bool
			replies = make(map[uint32]protoreflect.ProtoMessage)
		)

		for {
			select {
			case r := <-replyChan:
				if r.err != nil {
					errs = append(errs, Error{r.nid, r.err})
					break
				}
				reply := r.reply
				replies[r.nid] = reply
				if resp, quorum = d.QuorumFunction(d.Message, replies); quorum {
					fut.reply, fut.err = resp, nil
					return
				}
			case <-ctx.Done():
				fut.reply, fut.err = resp, QuorumCallError{"incomplete call", len(replies), errs}
				return
			}
			if len(errs)+len(replies) == expected {
				fut.reply, fut.err = resp, QuorumCallError{"incomplete call", len(replies), errs}
				return
			}
		}
	}()

	return fut
}
