package gorums

import (
	"testing"
	"time"

	"google.golang.org/protobuf/runtime/protoimpl"
)

// TestMustWaitSendDoneDefaultBehavior tests that by default (mustWaitSendDone=true),
// the caller blocks until the message is sent and the responseRouter is cleaned up.
func TestMustWaitSendDoneDefaultBehavior(t *testing.T) {
	node := newNodeWithServer(t, 0)

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

	// Should receive empty response (from defer in sendMsg) indicating send completed
	resp := sendRequest(t, node, t.Context(), msgID, opts, 3*time.Second)
	if resp.err != nil {
		t.Errorf("expected no error from send confirmation, got: %v", resp.err)
	}
	if resp.msg != nil {
		t.Error("expected empty response from send confirmation")
	}

	// Verify responseRouter was cleaned up
	node.channel.responseMut.Lock()
	_, exists := node.channel.responseRouters[msgID]
	node.channel.responseMut.Unlock()

	if exists {
		t.Error("responseRouter should have been cleaned up after send")
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

	// Should receive unavailable error
	resp := sendRequest(t, node, t.Context(), msgID, opts, 3*time.Second)
	if resp.err == nil {
		t.Error("expected unavailable error when stream is nil")
	}
}
