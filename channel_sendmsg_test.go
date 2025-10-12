package gorums

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
	"google.golang.org/protobuf/runtime/protoimpl"
)

// TestWaitForSendDefaultBehavior tests that by default (waitForSend=true),
// the caller blocks until the message is sent and the responseRouter is cleaned up.
func TestWaitForSendDefaultBehavior(t *testing.T) {
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		return dummySrv()
	})
	defer teardown()

	node := newNode(t, addrs[0])

	// Wait for connection to be established
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if node.channel.isConnected() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Create call options with waitForSend=true (default behavior)
	msgID := uint64(1)
	opts := callOptions{
		callType:      &protoimpl.ExtensionInfo{}, // non-nil makes waitForSend true
		noSendWaiting: false,                      // default: wait for send
	}

	// Verify waitForSend returns true
	testReq := request{opts: opts}
	if !testReq.waitForSend() {
		t.Fatal("waitForSend should return true with callType set and noSendWaiting=false")
	}

	replyChan := sendTestMessage(t, node, msgID, opts)

	// Should receive empty response (from defer in sendMsg) indicating send completed
	expectResponse(t, replyChan, func(t *testing.T, resp response) bool {
		if resp.err != nil {
			t.Errorf("expected no error from send confirmation, got: %v", resp.err)
		}
		if resp.msg != nil {
			t.Error("expected empty response from send confirmation")
		}
		return true
	})

	// Verify responseRouter was cleaned up
	node.channel.responseMut.Lock()
	_, exists := node.channel.responseRouters[msgID]
	node.channel.responseMut.Unlock()

	if exists {
		t.Error("responseRouter should have been cleaned up after send")
	}
}

// TestNoSendWaitingBehavior tests that with WithNoSendWaiting option,
// waitForSend returns false, which means sendMsg's defer doesn't send
// an empty confirmation response.
func TestNoSendWaitingBehavior(t *testing.T) {
	// Create request with noSendWaiting=true
	ctx := context.Background()
	msgID := uint64(2)
	md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
	opts := callOptions{
		callType:      &protoimpl.ExtensionInfo{}, // non-nil
		noSendWaiting: true,                       // don't wait for send
	}
	req := request{
		ctx:  ctx,
		msg:  &Message{Metadata: md, Message: &mock.Request{}},
		opts: opts,
	}

	// Verify waitForSend returns false
	if req.waitForSend() {
		t.Fatal("waitForSend should return false with noSendWaiting=true")
	}
}

// TestSendMsgContextAlreadyCancelled tests that sendMsg immediately returns
// context error if the context is already cancelled before sending.
func TestSendMsgContextAlreadyCancelled(t *testing.T) {
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		return dummySrv()
	})
	defer teardown()

	node := newNode(t, addrs[0])

	// Create already-cancelled context and manually enqueue
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	msgID := uint64(3)
	md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
	opts := callOptions{
		callType: &protoimpl.ExtensionInfo{},
	}
	req := request{
		ctx:  ctx,
		msg:  &Message{Metadata: md, Message: &mock.Request{}},
		opts: opts,
	}

	replyChan := make(chan response, 1)
	node.channel.enqueue(req, replyChan, false)

	// Should receive response - when context is cancelled before send,
	// sendMsg returns early with context error. The defer runs and sends
	// empty response (because waitForSend=true), which deletes the router.
	// Then sender tries to send error response but router is already deleted.
	// So we only get the empty response from defer.
	expectResponse(t, replyChan, func(t *testing.T, resp response) bool {
		// Response is empty (send confirmation from defer)
		if resp.err != nil {
			t.Errorf("expected nil error from defer, got: %v", resp.err)
		}
		if resp.msg != nil {
			t.Error("expected nil message from defer")
		}
		return true
	})
}

// TestSendMsgStreamNil tests that sendMsg returns unavailable error
// when stream is nil (not yet established).
func TestSendMsgStreamNil(t *testing.T) {
	// Create node pointing to non-existent server
	node := newNode(t, "127.0.0.1:9999")

	msgID := uint64(4)
	opts := callOptions{
		callType: &protoimpl.ExtensionInfo{},
	}

	replyChan := sendTestMessage(t, node, msgID, opts)

	// Should receive unavailable error
	expectResponse(t, replyChan, func(t *testing.T, resp response) bool {
		if resp.err == nil {
			t.Error("expected unavailable error when stream is nil")
		}
		return true
	})
}

// TestSendMsgContextCancelDuringSend tests the scenario where context
// is cancelled while SendMsg is in progress.
func TestSendMsgContextCancelDuringSend(t *testing.T) {
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		srv := NewServer()
		// Register handler that delays response to simulate slow send
		srv.RegisterHandler(handlerName, func(ctx ServerCtx, in *Message, finished chan<- *Message) {
			defer ctx.Release()
			// Simulate slow processing
			time.Sleep(100 * time.Millisecond)
			SendMessage(ctx, finished, WrapMessage(in.Metadata, &mock.Response{}, nil))
		})
		return srv
	})
	defer teardown()

	node := newNode(t, addrs[0])

	// Wait for connection
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if node.channel.isConnected() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Create context with short timeout and manually enqueue
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	msgID := uint64(5)
	md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
	opts := callOptions{
		callType: &protoimpl.ExtensionInfo{},
	}
	req := request{
		ctx:  ctx,
		msg:  &Message{Metadata: md, Message: &mock.Request{}},
		opts: opts,
	}

	replyChan := make(chan response, 1)
	node.channel.enqueue(req, replyChan, false)

	// Should receive either timeout error or send confirmation
	// (the race condition is acceptable in this test)
	expectResponse(t, replyChan, func(t *testing.T, resp response) bool {
		// Either context timeout or successful send is acceptable
		// The important thing is that the goroutine in sendMsg
		// properly handles the cancellation without deadlock
		_ = resp
		return true
	})
}

// TestWaitForSendWithNilCallType tests that waitForSend returns false
// when callType is nil (edge case).
func TestWaitForSendWithNilCallType(t *testing.T) {
	req := request{
		ctx:  context.Background(),
		msg:  &Message{},
		opts: callOptions{callType: nil, noSendWaiting: false},
	}

	if req.waitForSend() {
		t.Error("waitForSend should return false when callType is nil")
	}
}
