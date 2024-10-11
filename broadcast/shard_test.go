package broadcast

import (
	"context"
	"errors"
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

func (r *slowRouter) Send(broadcastID uint64, addr, method string, originDigest, originSignature []byte, originPubKey string, req msg) error {
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

func (r *slowRouter) Connect(addr string) {}

func TestShard(t *testing.T) {
	snowflake := NewSnowflake(0)
	broadcastID := snowflake.NewBroadcastID()
	router := &slowRouter{
		returnError: false,
	}
	shardBuffer := 100
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shard := &shard{
		id:        0,
		parentCtx: ctx,
		procs:     make(map[uint64]*BroadcastProcessor, shardBuffer),
		router:    router,
		reqTTL:    5 * time.Minute,
	}

	var tests = []struct {
		in  *Content
		out error
	}{
		{
			in: &Content{
				BroadcastID:       broadcastID,
				OriginAddr:        "127.0.0.1:8080",
				OriginMethod:      "testMethod",
				IsBroadcastClient: true,
				ReceiveChan:       make(chan shardResponse),
				Ctx:               context.Background(),
			},
			out: nil,
		},
		{
			in: &Content{
				BroadcastID:       broadcastID,
				OriginAddr:        "127.0.0.1:8080",
				OriginMethod:      "testMethod",
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse),
				Ctx:               context.Background(),
			},
			out: nil,
		},
		{
			in: &Content{
				BroadcastID:       broadcastID,
				OriginAddr:        "127.0.0.1:8080",
				OriginMethod:      "testMethod",
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse),
				Ctx:               context.Background(),
			},
			out: nil,
		},
	}

	for _, tt := range tests {
		resp := shard.handleMsg(tt.in)
		if resp.err != tt.out {
			t.Fatalf("wrong error returned.\n\tgot: %v, want: %v", tt.out, resp.err)
		}
	}

	shard.handleBMsg(&Msg{
		MsgType: ReplyMsg,
		Reply: &reply{
			Response: mockResp{},
			Err:      nil,
		},
		BroadcastID: broadcastID,
	})

	clientMsg := &Content{
		BroadcastID:       broadcastID,
		OriginAddr:        "127.0.0.1:8080",
		OriginMethod:      "testMethod",
		IsBroadcastClient: true,
		ReceiveChan:       make(chan shardResponse, 1),
		Ctx:               context.Background(),
	}
	resp := shard.handleMsg(clientMsg)
	if !errors.Is(resp.err, AlreadyProcessedErr{}) {
		t.Fatalf("the request should have been stopped. SendToClient has been called.")
	}

	// wait for the request to finish
	msgShouldBeDropped := &Content{
		BroadcastID:       broadcastID,
		OriginAddr:        "127.0.0.1:8080",
		OriginMethod:      "testMethod",
		IsBroadcastClient: true,
		ReceiveChan:       make(chan shardResponse, 1),
		Ctx:               context.Background(),
	}
	// this will panic if the request sendChan is closed
	resp = shard.handleMsg(msgShouldBeDropped)
	if !errors.Is(resp.err, AlreadyProcessedErr{}) {
		t.Fatalf("the request should have been stopped. SendToClient has been called.")
	}
}
