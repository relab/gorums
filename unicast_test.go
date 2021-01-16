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
		sendQ: make(chan gorumsStreamRequest, 1),
	}
	go consumeAndPutResult(cd.rq, cd.sendQ)
	b.Run("UnicastSyncSend/NewCall", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			unicastNewCall(context.Background(), cd)
		}
	})
	b.Run("UnicastAsyncSend/NewCall", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			unicastNewCall(context.Background(), cd, WithAsyncSend())
		}
	})
	b.Run("UnicastSyncSend/Basic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			unicastBasic(context.Background(), cd)
		}
	})
	b.Run("UnicastAsyncSend/Basic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			unicastBasic(context.Background(), cd, WithAsyncSend())
		}
	})
}

func TestUnicast(t *testing.T) {
	cd := callData{
		rq:    newReceiveQueue(),
		sendQ: make(chan gorumsStreamRequest, 1),
	}
	go consumeAndPutResult(cd.rq, cd.sendQ)
	unicastBasic(context.Background(), cd)
	unicastBasic(context.Background(), cd, WithAsyncSend())
}

type callData struct {
	rq      *receiveQueue
	sendQ   chan gorumsStreamRequest
	Message protoreflect.ProtoMessage
	Method  string
}

func consumeAndPutResult(rq *receiveQueue, sendQ chan gorumsStreamRequest) {
	for {
		req := <-sendQ
		rq.putResult(req.msg.Metadata.MessageID, &gorumsStreamResult{})
	}
}

func unicastBasic(ctx context.Context, d callData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)
	msgID := d.rq.nextMsgID()
	var replyChan chan *gorumsStreamResult
	if !o.sendAsync {
		replyChan = make(chan *gorumsStreamResult, 1)
		d.rq.putChan(msgID, replyChan)
		defer d.rq.deleteChan(msgID)
	}
	md := &ordering.Metadata{
		MessageID: msgID,
		Method:    d.Method,
	}
	d.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	// wait until the message has been sent (nodeStream will give an empty reply when this happens)
	if !o.sendAsync {
		<-replyChan
	}
}

func unicastNewCall(ctx context.Context, d callData, opts ...CallOption) {
	o := getCallOptions(E_Multicast, opts)
	md, replyChan, callDone := d.rq.newCall(d.Method, 1, !o.sendAsync)
	d.sendQ <- gorumsStreamRequest{ctx: ctx, msg: &Message{Metadata: md, Message: d.Message}, opts: o}

	// wait until the message has been sent (nodeStream will give an empty reply when this happens)
	if !o.sendAsync {
		<-replyChan
		callDone()
	}
}
