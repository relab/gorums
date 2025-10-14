package gorums

import (
	"testing"

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
	if !opts.mustWaitSendDone() {
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
	// Create call options with waitSendDone=false
	opts := callOptions{
		callType:     &protoimpl.ExtensionInfo{}, // non-nil
		waitSendDone: false,                      // don't wait for send completion
	}
	// Verify mustWaitSendDone returns false
	if opts.mustWaitSendDone() {
		t.Fatal("mustWaitSendDone should return false with waitSendDone=false")
	}
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

// TestMustWaitSendDoneWithNilCallType tests that mustWaitSendDone returns false
// when callType is nil (edge case for non-oneway calls).
func TestMustWaitSendDoneWithNilCallType(t *testing.T) {
	opts := callOptions{callType: nil, waitSendDone: true}

	if opts.mustWaitSendDone() {
		t.Error("mustWaitSendDone should return false when callType is nil")
	}
}
