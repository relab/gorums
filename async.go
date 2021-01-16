package gorums

import (
	"context"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type Async struct {
	reply protoreflect.ProtoMessage
	err   error
	c     chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *Async) Get() (protoreflect.ProtoMessage, error) {
	<-f.c
	return f.reply, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *Async) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}

func AsyncCall(ctx context.Context, d QuorumCallData) *Async {
	expectedReplies := len(d.Nodes)
	md, replyChan, callDone := d.Manager.newCall(d.Method, expectedReplies, true)

	for _, n := range d.Nodes {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			nodeArg := d.PerNodeArgFn(d.Message, n.id)
			if nodeArg != nil {
				expectedReplies--
				continue
			}
		}
		n.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}
	}

	fut := &Async{c: make(chan struct{}, 1)}

	go func() {
		defer callDone()
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
				replies[r.nid] = r.reply
				if resp, quorum = d.QuorumFunction(d.Message, replies); quorum {
					fut.reply, fut.err = resp, nil
					return
				}
			case <-ctx.Done():
				fut.reply, fut.err = resp, QuorumCallError{"incomplete call", len(replies), errs}
				return
			}
			if len(errs)+len(replies) == expectedReplies {
				fut.reply, fut.err = resp, QuorumCallError{"incomplete call", len(replies), errs}
				return
			}
		}
	}()

	return fut
}
