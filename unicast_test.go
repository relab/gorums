package gorums

import (
	"context"
	"testing"

	"github.com/relab/gorums/ordering"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func BenchmarkUnicast(b *testing.B) {
	cd := callData{
		rq:    newReceiveQueue(),
		sendQ: make(chan request, 1),
	}
	go consumeAndPutResult(cd.rq, cd.sendQ)
	b.Run("UnicastSyncSend/NewCall", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			unicastNewCall(context.Background(), cd)
		}
	})
	b.Run("UnicastAsyncSend/NewCall", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			unicastNewCall(context.Background(), cd, WithNoSendWaiting())
		}
	})
	b.Run("UnicastSyncSend/Basic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			unicastBasic(context.Background(), cd)
		}
	})
	b.Run("UnicastAsyncSend/Basic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			unicastBasic(context.Background(), cd, WithNoSendWaiting())
		}
	})
}

func TestUnicast(t *testing.T) {
	cd := callData{
		rq:    newReceiveQueue(),
		sendQ: make(chan request, 1),
	}
	go consumeAndPutResult(cd.rq, cd.sendQ)
	unicastBasic(context.Background(), cd)
	unicastBasic(context.Background(), cd, WithNoSendWaiting())
}

type callData struct {
	rq      *receiveQueue
	sendQ   chan request
	Message protoreflect.ProtoMessage
	Method  string
}

func consumeAndPutResult(rq *receiveQueue, sendQ chan request) {
	for {
		req := <-sendQ
		rq.putResult(req.msg.Metadata.MessageID, &response{})
	}
}

func unicastBasic(ctx context.Context, d callData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)
	msgID := d.rq.nextMsgID()
	var replyChan chan *response
	if !o.noSendWaiting {
		replyChan = make(chan *response, 1)
		d.rq.putChan(msgID, replyChan)
		defer d.rq.deleteChan(msgID)
	}
	md := &ordering.Metadata{
		MessageID: msgID,
		Method:    d.Method,
	}
	d.sendQ <- request{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	// wait until the message has been sent (nodeStream will give an empty reply when this happens)
	if !o.noSendWaiting {
		<-replyChan
	}
}

func unicastNewCall(ctx context.Context, d callData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)
	md := d.rq.newCall(d.Method)
	replyChan, callDone := d.rq.newReply(md, 1)

	d.sendQ <- request{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	// wait until the message has been sent (nodeStream will give an empty reply when this happens)
	if !o.noSendWaiting {
		<-replyChan
		callDone()
	}
}
