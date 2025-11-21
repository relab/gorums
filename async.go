package gorums

import (
	"context"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/proto"
)

// Async encapsulates the state of an asynchronous quorum call,
// and has methods for checking the status of the call or waiting for it to complete.
//
// This struct should only be used by generated code.
type Async struct {
	reply proto.Message
	err   error
	c     chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *Async) Get() (proto.Message, error) {
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

// AsyncCall starts an asynchronous quorum call, returning an Async object that can be used to retrieve the results.
//
// This function should only be used by generated code.
func (c RawConfiguration) AsyncCall(ctx context.Context, d QuorumCallData) *Async {
	expectedReplies := len(c)
	md := ordering.NewGorumsMetadata(ctx, c.getMsgID(), d.Method)
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
		n.channel.enqueue(request{ctx: ctx, msg: NewRequestMessage(md, msg)}, replyChan)
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

func (RawConfiguration) handleAsyncCall(ctx context.Context, fut *Async, state asyncCallState) {
	defer close(fut.c)

	var (
		resp    proto.Message
		errs    []nodeError
		quorum  bool
		replies = make(map[uint32]proto.Message)
	)

	for {
		select {
		case r := <-state.replyChan:
			if r.err != nil {
				errs = append(errs, nodeError{nodeID: r.nid, cause: r.err})
				break
			}
			replies[r.nid] = r.msg
			if resp, quorum = state.data.QuorumFunction(state.data.Message, replies); quorum {
				fut.reply, fut.err = resp, nil
				return
			}
		case <-ctx.Done():
			fut.reply, fut.err = resp, QuorumCallError{cause: ctx.Err(), errors: errs, replies: len(replies)}
			return
		}
		if len(errs)+len(replies) == state.expectedReplies {
			fut.reply, fut.err = resp, QuorumCallError{cause: Incomplete, errors: errs, replies: len(replies)}
			return
		}
	}
}
