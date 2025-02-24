package gorums

import (
	"cmp"
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Async encapsulates the state of an asynchronous quorum call,
// and has methods for checking the status of the call or waiting for it to complete.
//
// This struct should only be used by generated code.
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

type asyncCallState[idType cmp.Ordered] struct {
	md              *ordering.Metadata
	data            QuorumCallData[idType]
	replyChan       <-chan response[idType]
	expectedReplies int
}

// AsyncCall starts an asynchronous quorum call, returning an Async object that can be used to retrieve the results.
//
// This function should only be used by generated code.
func (c RawConfiguration[idType]) AsyncCall(ctx context.Context, d QuorumCallData[idType]) *Async {
	expectedReplies := len(c)
	md := ordering.Metadata_builder{MessageID: c.getMsgID(), Method: d.Method}.Build()
	replyChan := make(chan response[idType], expectedReplies)

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

	go c.handleAsyncCall(ctx, fut, asyncCallState[idType]{
		md:              md,
		data:            d,
		replyChan:       replyChan,
		expectedReplies: expectedReplies,
	})

	return fut
}

func (c RawConfiguration[idType]) handleAsyncCall(ctx context.Context, fut *Async, state asyncCallState[idType]) {
	defer close(fut.c)

	var (
		resp    protoreflect.ProtoMessage
		errs    []nodeError[idType]
		quorum  bool
		replies = make(map[idType]protoreflect.ProtoMessage)
	)

	for {
		select {
		case r := <-state.replyChan:
			if r.err != nil {
				errs = append(errs, nodeError[idType]{nodeID: r.nid, cause: r.err})
				break
			}
			replies[r.nid] = r.msg
			if resp, quorum = state.data.QuorumFunction(state.data.Message, replies); quorum {
				fut.reply, fut.err = resp, nil
				return
			}
		case <-ctx.Done():
			fut.reply, fut.err = resp, QuorumCallError[idType]{cause: ctx.Err(), errors: errs, replies: len(replies)}
			return
		}
		if len(errs)+len(replies) == state.expectedReplies {
			fut.reply, fut.err = resp, QuorumCallError[idType]{cause: Incomplete, errors: errs, replies: len(replies)}
			return
		}
	}
}
