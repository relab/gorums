package gorums

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
	"google.golang.org/protobuf/runtime/protoimpl"
)

// TestMustWaitSendDoneDefaultBehavior tests that by default (mustWaitSendDone=true),
// the caller blocks until the message is sent and the responseRouter is cleaned up.
func TestMustWaitSendDoneDefaultBehavior(t *testing.T) {
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		return dummySrv()
	})
	defer teardown()

	node := newNode(t, addrs[0])
	if !node.channel.isConnected() {
		t.Fatal("node should be connected")
	}

	// Create call options with mustWaitSendDone=true (default behavior)
	msgID := uint64(1)
	opts := callOptions{
		callType:     &protoimpl.ExtensionInfo{}, // non-nil makes mustWaitSendDone check callType
		waitSendDone: true,                       // default: wait for send completion
	}

	// Verify mustWaitSendDone returns true
	testReq := request{opts: opts}
	if !testReq.mustWaitSendDone() {
		t.Fatal("mustWaitSendDone should return true with callType set and waitSendDone=true")
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
// mustWaitSendDone returns false, which means sendMsg's defer doesn't send
// an empty confirmation response.
func TestNoSendWaitingBehavior(t *testing.T) {
	// Create request with waitSendDone=false
	ctx := context.Background()
	msgID := uint64(2)
	md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
	opts := callOptions{
		callType:     &protoimpl.ExtensionInfo{}, // non-nil
		waitSendDone: false,                      // don't wait for send completion
	}
	req := request{
		ctx:  ctx,
		msg:  &Message{Metadata: md, Message: &mock.Request{}},
		opts: opts,
	}

	// Verify mustWaitSendDone returns false
	if req.mustWaitSendDone() {
		t.Fatal("mustWaitSendDone should return false with waitSendDone=false")
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

	// Should receive error response - when context is cancelled before send,
	// sendMsg returns early with context error. With the bug fix, the defer only sends
	// empty response if err == nil. Since err != nil, the sender() goroutine sends the
	// error response to the caller.
	expectResponse(t, replyChan, func(t *testing.T, resp response) bool {
		if resp.err == nil {
			t.Error("expected context.Canceled error, got nil")
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
	if !node.channel.isConnected() {
		t.Fatal("node should be connected")
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

// TestMustWaitSendDoneWithNilCallType tests that mustWaitSendDone returns false
// when callType is nil (edge case for non-oneway calls).
func TestMustWaitSendDoneWithNilCallType(t *testing.T) {
	req := request{
		ctx:  context.Background(),
		msg:  &Message{},
		opts: callOptions{callType: nil, waitSendDone: true},
	}

	if req.mustWaitSendDone() {
		t.Error("mustWaitSendDone should return false when callType is nil")
	}
}
