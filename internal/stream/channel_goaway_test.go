package stream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
)

// holdFirstServer echoes every message immediately, except message 1, which it
// answers only after delay. The delayed send stays off the receive loop and is
// serialized with the other sends.
func holdFirstServer(delay time.Duration) func(Gorums_NodeStreamServer) error {
	return func(stream Gorums_NodeStreamServer) error {
		var mu sync.Mutex
		for {
			in, err := stream.Recv()
			if err != nil {
				return err
			}
			if in.GetMessageSeqNo() == 1 {
				go func() {
					time.Sleep(delay)
					mu.Lock()
					_ = stream.Send(in)
					mu.Unlock()
				}()
				continue
			}
			mu.Lock()
			err = stream.Send(in)
			mu.Unlock()
			if err != nil {
				return err
			}
		}
	}
}

// TestChannelGoAwayDoesNotStrandPendingCall sends a call, lets the server emit
// GOAWAY from MaxConnectionAge while that call is still pending, then sends a
// second call. The pending call must still complete.
func TestChannelGoAwayDoesNotStrandPendingCall(t *testing.T) {
	tc := setupChannel(t, holdFirstServer(800*time.Millisecond),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge:      300 * time.Millisecond,
			MaxConnectionAgeGrace: 30 * time.Second,
		}))
	if !waitForConnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel never connected")
	}

	r1 := make(chan response, 1)
	tc.Enqueue(Request{
		Ctx:          context.Background(),
		Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		ResponseChan: r1,
	})

	deadline := time.Now().Add(3 * time.Second)
	for tc.conn.GetState() == connectivity.Ready && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if tc.conn.GetState() == connectivity.Ready {
		t.Fatal("connection stayed Ready; MaxConnectionAge did not send GOAWAY")
	}

	r2 := make(chan response, 1)
	tc.Enqueue(Request{
		Ctx:          context.Background(),
		Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
		ResponseChan: r2,
	})
	select {
	case resp := <-r2:
		if resp.Err != nil {
			t.Fatalf("second call: %v", resp.Err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second call never completed")
	}

	select {
	case resp := <-r1:
		if resp.Err != nil {
			t.Fatalf("pending call: %v", resp.Err)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("pending call never completed; router pending=%d", tc.router.PendingCount())
	}
}
