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
				ReceiveChan:       make(chan shardResponse),
			},
			out: nil,
		},
		{
			in: Content{
				BroadcastID:       broadcastID,
				OriginAddr:        "127.0.0.1:8080",
				OriginMethod:      "testMethod",
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse),
			},
			out: nil,
		},
		{
			in: Content{
				BroadcastID:       broadcastID,
				OriginAddr:        "127.0.0.1:8080",
				OriginMethod:      "testMethod",
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse),
			},
			out: nil,
		},
	}

	for _, tt := range tests {
		shard.sendChan <- tt.in
		resp := <-tt.in.ReceiveChan
		if resp.err != tt.out {
			t.Fatalf("wrong error returned.\n\tgot: %v, want: %v", tt.out, resp.err)
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
		ReceiveChan:       make(chan shardResponse, 1),
		Ctx:               context.Background(),
	}
	shard.sendChan <- clientMsg

	// wait for the request to finish
	time.Sleep(1 * time.Second)
	msgShouldBeDropped := Content{
		BroadcastID:       broadcastID,
		OriginAddr:        "127.0.0.1:8080",
		OriginMethod:      "testMethod",
		IsBroadcastClient: true,
		ReceiveChan:       make(chan shardResponse, 1),
		Ctx:               context.Background(),
	}
	// this will panic if the request sendChan is closed
	shard.sendChan <- msgShouldBeDropped
	select {
	case resp := <-msgShouldBeDropped.ReceiveChan:
		if resp.err == nil {
			t.Fatalf("the request should have been stopped. SendToClient has been called.")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("a deadlock has most probably occured due to NOT buffering the receiveChan on the message.")
	}

	select {
	case resp := <-clientMsg.ReceiveChan:
		if resp.err == nil {
			t.Fatalf("the request should have been stopped. SendToClient has been called.")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("a deadlock has most probably occured due to buffering the sendChan on a request and not cleaning up afterwards.")
	}
}

func BenchmarkShard(b *testing.B) {
	snowflake := NewSnowflake("127.0.0.1:8080")
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

	originMethod := "test"
	originAddr := "127.0.0.1:8080"
	msgs := make([]Content, 10)
	for i := 0; i < 10; i++ {
		msg := Content{
			BroadcastID:       snowflake.NewBroadcastID(),
			IsBroadcastClient: false,
			OriginAddr:        originAddr,
			OriginMethod:      originMethod,
			ReceiveChan:       make(chan shardResponse, 1),
			Ctx:               context.Background(),
		}
		msgs[i] = msg
		shard.sendChan <- msg
		<-msg.ReceiveChan
	}
	//resp := Msg{
	//Reply: &reply{
	//Response: mockResp{},
	//Err:      nil,
	//},
	//BroadcastID: broadcastID,
	//}

	b.ResetTimer()
	b.Run("run", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var msg Content
			// every 5 msgs is a new broadcast request
			if i%5 == 0 {
				msg = Content{
					BroadcastID:       snowflake.NewBroadcastID(),
					IsBroadcastClient: true,
					OriginAddr:        originAddr,
					OriginMethod:      originMethod,
					ReceiveChan:       make(chan shardResponse, 1),
					Ctx:               context.Background(),
				}
			} else {
				msg = msgs[i%10]
			}

			shard.sendChan <- msg
			<-msg.ReceiveChan
		}
	})
	b.StopTimer()
	shard.Close()
}
