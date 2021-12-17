package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
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

type asyncCallState struct {
	md              *ordering.Metadata
	data            QuorumCallData
	replyChan       <-chan response
	expectedReplies int
}

func (c Configuration) AsyncCall(ctx context.Context, d QuorumCallData) *Async {
	expectedReplies := len(c)
	md := &ordering.Metadata{MessageID: c.getMsgID(), Method: d.Method}
	replyChan := make(chan response, expectedReplies)

	for _, n := range c {
		msg := d.Message
		if d.PerNodeArgFn != nil {
			msg = d.PerNodeArgFn(d.Message, n.id)
			if !msg.ProtoReflect().IsValid() {
				expectedReplies--
				continue // don't send if no msg
			}
		}
		n.channel.enqueue(request{ctx: ctx, msg: &Message{Metadata: md, Message: msg}}, replyChan, false)
	}

	fut := &Async{c: make(chan struct{}, 1)}

	go c.handleAsyncCall(ctx, fut, asyncCallState{
		md:              md,
		data:            d,
		replyChan:       replyChan,
		expectedReplies: expectedReplies,
	})

	return fut
}

func (c Configuration) handleAsyncCall(ctx context.Context, fut *Async, state asyncCallState) {
	defer close(fut.c)

	var (
		resp    protoreflect.ProtoMessage
		errs    []Error
		quorum  bool
		replies = make(map[uint32]protoreflect.ProtoMessage)
	)

	for {
		select {
		case r := <-state.replyChan:
			if r.err != nil {
				errs = append(errs, Error{r.nid, r.err})
				break
			}
			replies[r.nid] = r.msg
			if resp, quorum = state.data.QuorumFunction(state.data.Message, replies); quorum {
				fut.reply, fut.err = resp, nil
				return
			}
		case <-ctx.Done():
			fut.reply, fut.err = resp, QuorumCallError{ctx.Err().Error(), len(replies), errs}
			return
		}
		if len(errs)+len(replies) == state.expectedReplies {
			fut.reply, fut.err = resp, QuorumCallError{"incomplete call", len(replies), errs}
			return
		}
	}
}
