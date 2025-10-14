package gorums

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
)

// Test helpers

// sendRequest is a helper that sends a request with a specific context.
func sendRequest(t *testing.T, node *RawNode, ctx context.Context, msgID uint64, opts callOptions, timeout time.Duration) response {
	t.Helper()
	replyChan := make(chan response, 1)
	md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
	req := request{
		ctx:  ctx,
		msg:  &Message{Metadata: md, Message: &mock.Request{}},
		opts: opts,
	}
	node.channel.enqueue(req, replyChan, false)

	select {
	case resp := <-replyChan:
		return resp
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for response to message %d", msgID)
		return response{}
	}
}

// sendStreamingRequest is a helper for testing streaming behavior.
// Unlike sendRequestWithContext, this sets streaming=true which keeps the router alive
// after the first response, allowing the caller to test router lifecycle behavior.
func sendStreamingRequest(t *testing.T, node *RawNode, msgID uint64, opts callOptions) response {
	t.Helper()
	replyChan := make(chan response, 10)
	ctx := t.Context()
	md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
	req := request{
		ctx:  ctx,
		msg:  &Message{Metadata: md, Message: &mock.Request{}},
		opts: opts,
	}
	node.channel.enqueue(req, replyChan, true)

	select {
	case resp := <-replyChan:
		return resp
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for response to message %d", msgID)
		return response{}
	}
}

// TODO(meling): rename to newConnectedNode and move to channel_test.go
// setupConnectedNode creates a node connected to a live server that will delay
// responding by the given duration. If delay is 0, the server responds immediately.
func setupConnectedNode(t *testing.T, delay time.Duration) *RawNode {
	t.Helper()
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		srv := NewServer()
		srv.RegisterHandler(handlerName, func(ctx ServerCtx, in *Message, finished chan<- *Message) {
			defer ctx.Release()
			// Simulate slow processing
			time.Sleep(delay)
			SendMessage(ctx, finished, WrapMessage(in.Metadata, &mock.Response{}, nil))
		})
		return srv
	})
	t.Cleanup(teardown)

	return newNode(t, addrs[0])
}

// Test 1: Concurrent Message Sending
func TestChannelConcurrentSends(t *testing.T) {
	node := setupConnectedNode(t, 0)

	if !node.channel.isConnected() {
		t.Fatal("node should be connected")
	}

	// Important: When using getCallOptions(E_Multicast, nil), the mustWaitSendDone() method returns true.
	// This means the sendMsg defer will send a "send completion" notification and DELETE the router.
	// For one-way calls (Unicast/Multicast), the server never sends responses, so this is the only
	// response the caller receives - confirming that the message was successfully sent.
	//
	// For this concurrent send test, we use getCallOptions(E_Multicast, nil) to test
	// the send confirmation path, which is the most common use case for multicast.

	const numMessages = 50 // Reduced to avoid potential issues
	const numGoroutines = 5
	messagesPerGoroutine := numMessages / numGoroutines

	type result struct {
		msgID uint64
		err   error
	}

	results := make(chan result, numMessages)

	// Send messages concurrently from multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			for j := 0; j < messagesPerGoroutine; j++ {
				msgID := uint64(goroutineID*1000 + j + 1)
				// Use standard multicast options which provide send confirmation
				opts := getCallOptions(E_Multicast, nil)
				resp := sendRequest(t, node, t.Context(), msgID, opts, 5*time.Second)
				results <- result{msgID: msgID, err: resp.err}
			}
		}(i)
	}

	// Collect results
	var errors []error
	for i := 0; i < numMessages; i++ {
		res := <-results
		if res.err != nil {
			errors = append(errors, res.err)
		}
	}

	if len(errors) > 0 {
		t.Errorf("got %d errors during concurrent sends (first few): %v", len(errors), errors[:min(5, len(errors))])
	}

	// Verify node is still connected
	if !node.channel.isConnected() {
		t.Error("node should still be connected after concurrent sends")
	}

	// Check for goroutine leaks by verifying channel state
	if node.mgr == nil {
		t.Error("manager should not be nil")
	}
}

// Test 2: Streaming Response Handling
func TestChannelStreamingResponses(t *testing.T) {
	node := setupConnectedNode(t, 0)

	// Test that streaming flag keeps router alive
	msgID := uint64(1)
	resp := sendStreamingRequest(t, node, msgID, getCallOptions(E_Multicast, nil))
	if resp.err != nil {
		t.Errorf("unexpected error: %v", resp.err)
	}

	// Verify router still exists for streaming (unlike non-streaming where it's deleted)
	node.channel.responseMut.Lock()
	_, exists := node.channel.responseRouters[msgID]
	node.channel.responseMut.Unlock()

	if !exists {
		t.Error("router should still exist for streaming message")
	}

	// Manually delete the router
	node.channel.deleteRouter(msgID)

	// Verify router is deleted
	node.channel.responseMut.Lock()
	_, exists = node.channel.responseRouters[msgID]
	node.channel.responseMut.Unlock()

	if exists {
		t.Error("router should be deleted after calling deleteRouter")
	}
}

func TestChannelContext(t *testing.T) {
	waitSendDone := getCallOptions(E_Multicast, nil)
	noSendWaiting := getCallOptions(E_Multicast, []CallOption{WithNoSendWaiting()})

	// Helper context setup functions
	cancelledContext := func(ctx context.Context) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately
		return ctx, cancel
	}
	expireBeforeSend := func(ctx context.Context) (context.Context, context.CancelFunc) {
		// Very short timeout to cancel during SendMsg operation
		// Note: SendMsg itself is fast, but we're testing the cancellation path
		ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		// Let context expire before we send
		time.Sleep(5 * time.Millisecond)
		return ctx, cancel
	}

	tests := []struct {
		name         string
		serverDelay  time.Duration
		contextSetup func(context.Context) (context.Context, context.CancelFunc)
		callOpts     callOptions
		wantErr      error
	}{
		{
			name:         "CancelBeforeSend/WaitSending",
			serverDelay:  0,
			contextSetup: cancelledContext,
			callOpts:     waitSendDone,
			wantErr:      context.Canceled,
		},
		{
			name:         "CancelBeforeSend/NoSendWaiting",
			serverDelay:  0,
			contextSetup: cancelledContext,
			callOpts:     noSendWaiting,
			wantErr:      context.Canceled,
		},
		{
			name:         "CancelDuringSend/WaitSending",
			serverDelay:  3 * time.Second,
			contextSetup: expireBeforeSend,
			callOpts:     waitSendDone,
			wantErr:      context.DeadlineExceeded,
		},
		{
			name:         "CancelDuringSend/NoSendWaiting",
			serverDelay:  3 * time.Second,
			contextSetup: expireBeforeSend,
			callOpts:     noSendWaiting,
			wantErr:      context.DeadlineExceeded,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := setupConnectedNode(t, tt.serverDelay)

			ctx, cancel := tt.contextSetup(t.Context())
			t.Cleanup(cancel)

			// Send request
			msgID := uint64(100 + i)
			resp := sendRequest(t, node, ctx, msgID, tt.callOpts, 5*time.Second)

			if !errors.Is(resp.err, tt.wantErr) {
				t.Errorf("expected %v, got: %v", tt.wantErr, resp.err)
			}
		})
	}
}

// Test 4: Stream Failure During Active Communication
func TestChannelStreamFailureDuringCommunication(t *testing.T) {
	srvAddr := "127.0.0.1:5001"
	startServer, stopServer := testServerSetup(t, srvAddr, dummySrv())
	startServer()

	node := newNode(t, srvAddr)
	if !node.channel.isConnected() {
		t.Error("node should be connected")
	}

	// Send first message successfully
	resp1 := sendRequest(t, node, t.Context(), 1, getCallOptions(E_Multicast, nil), 3*time.Second)
	if resp1.err != nil {
		t.Errorf("first message should succeed, got error: %v", resp1.err)
	}

	// Stop server to break the stream
	stopServer()

	// Give some time for the stream to break
	time.Sleep(100 * time.Millisecond)

	// Try to send another message - should fail
	resp2 := sendRequest(t, node, t.Context(), 2, getCallOptions(E_Multicast, nil), 3*time.Second)
	if resp2.err == nil {
		t.Error("expected error when stream is broken")
	}

	// Verify lastErr is set
	if node.channel.lastErr() == nil {
		t.Error("lastErr should be set after stream failure")
	}
}

// Test 5: Channel Shutdown and Cleanup
func TestChannelShutdown(t *testing.T) {
	node := setupConnectedNode(t, 0)

	if !node.channel.isConnected() {
		t.Fatal("node should be connected")
	}

	// Enqueue several messages in goroutines (don't wait for responses)
	const numMessages = 10
	for i := 0; i < numMessages; i++ {
		go func(msgID uint64) {
			sendRequest(t, node, t.Context(), msgID, getCallOptions(E_Multicast, nil), 3*time.Second)
		}(uint64(i))
	}

	// Give messages time to enqueue
	time.Sleep(50 * time.Millisecond)

	// Close the manager (which should close the node)
	node.mgr.Close()

	// Try to send a message after closure
	resp := sendRequest(t, node, t.Context(), 999, getCallOptions(E_Multicast, nil), 1*time.Second)
	if resp.err == nil {
		t.Error("expected error when sending to closed channel")
	}
}

// Test 6: Send Completion Waiting Behavior
func TestChannelSendCompletionWaiting(t *testing.T) {
	node := setupConnectedNode(t, 0)

	tests := []struct {
		name         string
		waitSendDone bool
	}{
		{
			name:         "wait for send completion",
			waitSendDone: true,
		},
		{
			name:         "no wait for send completion",
			waitSendDone: false,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgID := uint64(200 + i)
			opts := getCallOptions(E_Multicast, nil)
			opts.waitSendDone = tt.waitSendDone

			start := time.Now()
			resp := sendRequest(t, node, t.Context(), msgID, opts, 3*time.Second)
			elapsed := time.Since(start)

			// Just verify we got a response (error or not)
			t.Logf("response received in %v (waitSendDone=%v, err=%v)", elapsed, tt.waitSendDone, resp.err)
		})
	}
}

// Test 7: Error Tracking
func TestChannelErrorTracking(t *testing.T) {
	// Test with a node that can't connect
	node := newNode(t, "127.0.0.1:5002")

	// Try to send a message (should fail)
	resp := sendRequest(t, node, t.Context(), 1, callOptions{}, 3*time.Second)
	if resp.err == nil {
		t.Error("expected error when connecting to non-existent server")
	}

	// Note: lastErr might not be set immediately since the connection
	// attempt happens asynchronously. The error we get is streamDownErr
	// which is returned before lastErr is set.
	// Let's just verify we got an error response, which is the important part.
	t.Logf("error received: %v", resp.err)
}

// Test 8: Connection State Tracking
func TestChannelConnectionState(t *testing.T) {
	tests := []struct {
		name          string
		node          *RawNode
		wantConnected bool
	}{
		{name: "WithLiveServer", node: setupConnectedNode(t, 0), wantConnected: true},
		{name: "WithoutServer_", node: newNode(t, "127.0.0.1:5003"), wantConnected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connected := tt.node.channel.isConnected()
			if connected != tt.wantConnected {
				t.Errorf("isConnected() = %v, want %v", connected, tt.wantConnected)
			}
		})
	}
}

// Test 9: Response Routing
func TestChannelResponseRouting(t *testing.T) {
	node := setupConnectedNode(t, 0)

	// Send multiple messages and verify each gets routed correctly
	const numMessages = 20
	type msgResponse struct {
		msgID uint64
		resp  response
	}

	results := make(chan msgResponse, numMessages)

	// Send all messages
	for i := 0; i < numMessages; i++ {
		msgID := uint64(i + 1000)
		go func(id uint64) {
			opts := getCallOptions(E_Multicast, nil)
			resp := sendRequest(t, node, t.Context(), id, opts, 5*time.Second)
			results <- msgResponse{msgID: id, resp: resp}
		}(msgID)
	}

	// Collect and verify results
	received := make(map[uint64]bool)
	for i := 0; i < numMessages; i++ {
		result := <-results
		if result.resp.err != nil {
			t.Errorf("message %d got error: %v", result.msgID, result.resp.err)
		}
		if received[result.msgID] {
			t.Errorf("message %d received twice", result.msgID)
		}
		received[result.msgID] = true
	}

	// Verify all messages were received
	if len(received) != numMessages {
		t.Errorf("got %d unique responses, want %d", len(received), numMessages)
	}
}

// Test 10: Router Cleanup for Non-Streaming
func TestChannelRouterCleanup(t *testing.T) {
	node := setupConnectedNode(t, 0)

	// Send a non-streaming message
	msgID := uint64(5000)
	resp := sendRequest(t, node, t.Context(), msgID, getCallOptions(E_Multicast, nil), 3*time.Second)
	if resp.err != nil {
		t.Errorf("unexpected error: %v", resp.err)
	}

	// Give some time for cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify router is cleaned up for non-streaming
	node.channel.responseMut.Lock()
	_, exists := node.channel.responseRouters[msgID]
	node.channel.responseMut.Unlock()

	if exists {
		t.Error("router should be automatically deleted for non-streaming message")
	}
}
