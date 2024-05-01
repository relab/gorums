package broadcast

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type slowRouter struct {
	returnError bool
	reqType     string
	resp        protoreflect.ProtoMessage
}

func (r *slowRouter) Send(broadcastID uint64, addr, method string, req any) error {
	time.Sleep(1 * time.Second)
	switch val := req.(type) {
	case *broadcastMsg:
		r.reqType = "Broadcast"
	case *reply:
		r.reqType = "SendToClient"
		r.resp = val.Response
	}
	if r.returnError {
		return fmt.Errorf("router: send error")
	}
	return nil
}

func TestShard(t *testing.T) {
	snowflake := NewSnowflake("127.0.0.1:8080")
	broadcastID := snowflake.NewBroadcastID()
	router := &slowRouter{
		returnError: false,
	}
	shardBuffer := 100
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shard := &shard{
		id:            0,
		sendChan:      make(chan Content, shardBuffer),
		broadcastChan: make(chan Msg, shardBuffer),
		ctx:           ctx,
		cancelFunc:    cancel,
		reqs:          make(map[uint64]*BroadcastRequest, shardBuffer),
	}
	go shard.run(router, 5*time.Minute, 5)

	var tests = []struct {
		in  Content
		out error
	}{
		{
			in: Content{
				BroadcastID:       broadcastID,
				OriginAddr:        "127.0.0.1:8080",
				OriginMethod:      "testMethod",
				IsBroadcastClient: true,
				ReceiveChan:       make(chan error),
			},
			out: nil,
		},
		{
			in: Content{
				BroadcastID:       broadcastID,
				OriginAddr:        "127.0.0.1:8080",
				OriginMethod:      "testMethod",
				IsBroadcastClient: false,
				ReceiveChan:       make(chan error),
			},
			out: nil,
		},
		{
			in: Content{
				BroadcastID:       broadcastID,
				OriginAddr:        "127.0.0.1:8080",
				OriginMethod:      "testMethod",
				IsBroadcastClient: false,
				ReceiveChan:       make(chan error),
			},
			out: nil,
		},
	}

	for _, tt := range tests {
		shard.sendChan <- tt.in
		err := <-tt.in.ReceiveChan
		if err != tt.out {
			t.Fatalf("wrong error returned.\n\tgot: %v, want: %v", tt.out, err)
		}
	}

	shard.broadcastChan <- Msg{
		Reply: &reply{
			Response: mockResp{},
			Err:      nil,
		},
		BroadcastID: broadcastID,
	}

	clientMsg := Content{
		BroadcastID:       broadcastID,
		OriginAddr:        "127.0.0.1:8080",
		OriginMethod:      "testMethod",
		IsBroadcastClient: true,
		ReceiveChan:       make(chan error, 1),
	}
	shard.sendChan <- clientMsg

	// wait for the request to finish
	time.Sleep(1 * time.Second)
	msgShouldBeDropped := Content{
		BroadcastID:       broadcastID,
		OriginAddr:        "127.0.0.1:8080",
		OriginMethod:      "testMethod",
		IsBroadcastClient: true,
		ReceiveChan:       make(chan error, 1),
	}
	// this will panic if the request sendChan is closed
	shard.sendChan <- msgShouldBeDropped
	select {
	case err := <-msgShouldBeDropped.ReceiveChan:
		if err == nil {
			t.Fatalf("the request should have been stopped. SendToClient has been called.")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("a deadlock has most probably occured due to NOT buffering the receiveChan on the message.")
	}

	select {
	case err := <-clientMsg.ReceiveChan:
		if err == nil {
			t.Fatalf("the request should have been stopped. SendToClient has been called.")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("a deadlock has most probably occured due to buffering the sendChan on a request and not cleaning up afterwards.")
	}
}

//func TestHandleBroadcastCall(t *testing.T) {
//snowflake := NewSnowflake("127.0.0.1:8080")
//broadcastID := snowflake.NewBroadcastID()

//var tests = []struct {
//in  Content
//out error
//}{
//{
//in: Content{
//BroadcastID:       broadcastID,
//IsBroadcastClient: false,
//ReceiveChan:       make(chan error, 1),
//},
//out: nil,
//},
//{
//in: Content{
//BroadcastID:       snowflake.NewBroadcastID(),
//IsBroadcastClient: false,
//ReceiveChan:       make(chan error, 1),
//},
//out: BroadcastIDErr{},
//},
//{
//in: Content{
//BroadcastID:       broadcastID,
//IsBroadcastClient: false,
//ReceiveChan:       make(chan error, 1),
//},
//out: nil,
//},
//}

//msg := Content{
//BroadcastID:       broadcastID,
//IsBroadcastClient: false,
//OriginAddr:        "127.0.0.1:8080",
//OriginMethod:      "testMethod",
//ReceiveChan:       make(chan error),
//}

//router := &mockRouter{
//returnError: false,
//}

//ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
//defer cancel()
//req := &BroadcastRequest{
//ctx:           ctx,
//cancelFunc:    cancel,
//sendChan:      make(chan Content),
//broadcastChan: make(chan Msg, 5),
//started:       time.Now(),
//}
//go req.handle(router, msg.BroadcastID, msg)

//for _, tt := range tests {
//req.sendChan <- tt.in
//err := <-tt.in.ReceiveChan
//if err != tt.out {
//t.Fatalf("wrong error returned.\n\tgot: %v, want: %v", tt.out, err)
//}
//}

//select {
//case <-time.After(100 * time.Millisecond):
//case <-req.ctx.Done():
//t.Fatalf("the request is not done yet. SendToClient has not been called.")
//}

//req.broadcastChan <- Msg{
//Reply: &reply{
//Response: mockResp{},
//Err:      nil,
//},
//BroadcastID: broadcastID,
//}

//select {
//case <-time.After(1 * time.Second):
//t.Fatalf("the request is done. SendToClient has been called and this is a BroadcastCall, meaning it should respond regardless of the client request.")
//case <-req.ctx.Done():
//}

//clientMsg := Content{
//BroadcastID:       broadcastID,
//IsBroadcastClient: true,
//OriginAddr:        "127.0.0.1:8080",
//OriginMethod:      "testMethod",
//ReceiveChan:       make(chan error),
//}
//select {
//case <-req.ctx.Done():
//case req.sendChan <- clientMsg:
//t.Fatalf("the request is done. SendToClient has been called so this message should be dropped.")
//}
//}

//func BenchmarkHandle(b *testing.B) {
//snowflake := NewSnowflake("127.0.0.1:8080")
//originMethod := "testMethod"
//router := &mockRouter{
//returnError: false,
//}
//// not important to use unique broadcastID because we are
//// not using shards in this test
//broadcastID := snowflake.NewBroadcastID()
//resp := Msg{
//Reply: &reply{
//Response: mockResp{},
//Err:      nil,
//},
//BroadcastID: broadcastID,
//}
//sendFn := func(resp protoreflect.ProtoMessage, err error) {}

//b.ResetTimer()
//b.Run("RequestHandler", func(b *testing.B) {
//for i := 0; i < b.N; i++ {
//msg := Content{
//BroadcastID:       broadcastID,
//IsBroadcastClient: true,
//SendFn:            sendFn,
//OriginMethod:      originMethod,
//ReceiveChan:       make(chan error, 1),
//}

//ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
//req := &BroadcastRequest{
//ctx:           ctx,
//cancelFunc:    cancel,
//sendChan:      make(chan Content),
//broadcastChan: make(chan Msg, 5),
//started:       time.Now(),
//}
//go req.handle(router, msg.BroadcastID, msg)

//req.broadcastChan <- resp

//<-req.ctx.Done()
//cancel()
//}
//})
//}
