package broadcast

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type mockResp struct{}

func (mockResp) ProtoReflect() protoreflect.Message {
	return nil
}

type mockRouter struct {
	returnError bool
	reqType     string
	resp        protoreflect.ProtoMessage
}

func (r *mockRouter) Send(broadcastID uint64, addr, method string, req any) error {
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

func (r *mockRouter) Connect(addr string) {}

func TestHandleBroadcastOption(t *testing.T) {
	snowflake := NewSnowflake(0)
	broadcastID := snowflake.NewBroadcastID()

	var tests = []struct {
		in  Content
		out error
	}{
		{
			in: Content{
				BroadcastID:       broadcastID,
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse),
			},
			out: nil,
		},
		{
			in: Content{
				BroadcastID:       snowflake.NewBroadcastID(),
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse),
			},
			out: BroadcastIDErr{},
		},
		{
			in: Content{
				BroadcastID:       broadcastID,
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse),
			},
			out: nil,
		},
	}

	msg := Content{
		BroadcastID:  broadcastID,
		OriginMethod: "testMethod",
		ReceiveChan:  make(chan shardResponse),
	}

	router := &mockRouter{
		returnError: false,
	}

	cancelCtx, cancelCancel := context.WithTimeout(context.Background(), 1*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	defer cancelCancel()
	req := &BroadcastRequest{
		ctx:                   ctx,
		cancelFunc:            cancel,
		sendChan:              make(chan Content),
		broadcastChan:         make(chan Msg, 5),
		started:               time.Now(),
		cancellationCtx:       cancelCtx,
		cancellationCtxCancel: cancelCancel,
	}
	go req.handle(router, msg.BroadcastID, msg)

	for _, tt := range tests {
		req.sendChan <- tt.in
		resp := <-tt.in.ReceiveChan
		if resp.err != tt.out {
			t.Fatalf("wrong error returned.\n\tgot: %v, want: %v", tt.out, resp.err)
		}
	}

	select {
	case <-time.After(100 * time.Millisecond):
	case <-req.ctx.Done():
		t.Fatalf("the request is not done yet. SendToClient has not been called.")
	}

	req.broadcastChan <- Msg{
		Reply: &reply{
			Response: mockResp{},
			Err:      nil,
		},
		BroadcastID: broadcastID,
	}

	select {
	case <-time.After(100 * time.Millisecond):
	case <-req.ctx.Done():
		t.Fatalf("the request is not done yet. SendToClient has been called, but the client request has not arrived yet.")
	}

	clientMsg := Content{
		BroadcastID:       broadcastID,
		IsBroadcastClient: true,
		SendFn:            func(resp protoreflect.ProtoMessage, err error) error { return nil },
		ReceiveChan:       make(chan shardResponse),
	}
	req.sendChan <- clientMsg
	resp := <-clientMsg.ReceiveChan
	expectedErr := AlreadyProcessedErr{}
	if resp.err != expectedErr {
		t.Fatalf("wrong error returned.\n\tgot: %v, want: %v", resp.err, expectedErr)
	}

	select {
	case <-time.After(3 * time.Second):
		t.Fatalf("the request should have been stopped. Both SendToClient has been called and the client request has arrived.")
	case <-req.ctx.Done():
	}
}

func TestHandleBroadcastCall(t *testing.T) {
	snowflake := NewSnowflake(0)
	broadcastID := snowflake.NewBroadcastID()

	var tests = []struct {
		in  Content
		out error
	}{
		{
			in: Content{
				BroadcastID:       broadcastID,
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse, 1),
			},
			out: nil,
		},
		{
			in: Content{
				BroadcastID:       snowflake.NewBroadcastID(),
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse, 1),
			},
			out: BroadcastIDErr{},
		},
		{
			in: Content{
				BroadcastID:       broadcastID,
				IsBroadcastClient: false,
				ReceiveChan:       make(chan shardResponse, 1),
			},
			out: nil,
		},
	}

	msg := Content{
		BroadcastID:       broadcastID,
		IsBroadcastClient: false,
		OriginAddr:        "127.0.0.1:8080",
		OriginMethod:      "testMethod",
		ReceiveChan:       make(chan shardResponse),
	}

	router := &mockRouter{
		returnError: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	cancelCtx, cancelCancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	defer cancelCancel()
	req := &BroadcastRequest{
		ctx:                   ctx,
		cancelFunc:            cancel,
		sendChan:              make(chan Content),
		broadcastChan:         make(chan Msg, 5),
		started:               time.Now(),
		cancellationCtx:       cancelCtx,
		cancellationCtxCancel: cancelCancel,
	}
	go req.handle(router, msg.BroadcastID, msg)

	for _, tt := range tests {
		req.sendChan <- tt.in
		resp := <-tt.in.ReceiveChan
		if resp.err != tt.out {
			t.Fatalf("wrong error returned.\n\tgot: %v, want: %v", tt.out, resp.err)
		}
	}

	select {
	case <-time.After(100 * time.Millisecond):
	case <-req.ctx.Done():
		t.Fatalf("the request is not done yet. SendToClient has not been called.")
	}

	req.broadcastChan <- Msg{
		Reply: &reply{
			Response: mockResp{},
			Err:      nil,
		},
		BroadcastID: broadcastID,
	}

	select {
	case <-time.After(1 * time.Second):
		t.Fatalf("the request is done. SendToClient has been called and this is a BroadcastCall, meaning it should respond regardless of the client request.")
	case <-req.ctx.Done():
	}

	clientMsg := Content{
		BroadcastID:       broadcastID,
		IsBroadcastClient: true,
		OriginAddr:        "127.0.0.1:8080",
		OriginMethod:      "testMethod",
		ReceiveChan:       make(chan shardResponse),
	}
	select {
	case <-req.ctx.Done():
	case req.sendChan <- clientMsg:
		t.Fatalf("the request is done. SendToClient has been called so this message should be dropped.")
	}
}

func BenchmarkHandle2(b *testing.B) {
	snowflake := NewSnowflake(0)
	originMethod := "testMethod"
	router := &mockRouter{
		returnError: false,
	}
	// not important to use unique broadcastID because we are
	// not using shards in this test
	broadcastID := snowflake.NewBroadcastID()
	resp := Msg{
		Reply: &reply{
			Response: mockResp{},
			Err:      nil,
		},
		BroadcastID: broadcastID,
	}
	sendFn := func(resp protoreflect.ProtoMessage, err error) error { return nil }

	b.ResetTimer()
	b.Run("RequestHandler", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msg := Content{
				BroadcastID:       broadcastID,
				IsBroadcastClient: true,
				SendFn:            sendFn,
				OriginMethod:      originMethod,
				ReceiveChan:       make(chan shardResponse, 1),
			}

			cancelCtx, cancelCancel := context.WithTimeout(context.Background(), 1*time.Minute)
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			req := &BroadcastRequest{
				ctx:                   ctx,
				cancelFunc:            cancel,
				cancellationCtx:       cancelCtx,
				cancellationCtxCancel: cancelCancel,
				sendChan:              make(chan Content),
				broadcastChan:         make(chan Msg, 5),
				started:               time.Now(),
			}
			go req.handle(router, msg.BroadcastID, msg)

			req.broadcastChan <- resp

			<-req.ctx.Done()
			cancel()
			cancelCancel()
		}
	})
}
