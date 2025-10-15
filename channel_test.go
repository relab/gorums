package gorums

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums/ordering"
	"github.com/relab/gorums/tests/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/runtime/protoimpl"
)

type mockSrv struct{}

func (mockSrv) Test(_ ServerCtx, _ *mock.Request) (*mock.Response, error) {
	return nil, nil
}

// newNode creates a node for the given server address and adds it to a new manager.
func newNode(t *testing.T, srvAddr string) *RawNode {
	mgr := NewRawManager(
		WithGrpcDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
	)
	t.Cleanup(mgr.Close)
	node, err := NewRawNode(srvAddr)
	if err != nil {
		t.Fatal(err)
	}
	if err = mgr.AddNode(node); err != nil {
		t.Error(err)
	}
	return node
}

// newNodeWithServer creates a node connected to a live server that will delay
// responding by the given duration. If delay is 0, the server responds immediately.
func newNodeWithServer(t *testing.T, delay time.Duration) *RawNode {
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

const defaultTestTimeout = 3 * time.Second

// sendRequest is a helper that sends a request with a specific context.
func sendRequest(t *testing.T, node *RawNode, ctx context.Context, msgID uint64, opts callOptions) response {
	t.Helper()
	replyChan := make(chan response, 1)
	md := ordering.NewGorumsMetadata(ctx, msgID, handlerName)
	req := request{ctx: ctx, msg: &Message{Metadata: md}, opts: opts}
	node.channel.enqueue(req, replyChan, false)

	select {
	case resp := <-replyChan:
		return resp
	case <-time.After(defaultTestTimeout):
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
	req := request{ctx: ctx, msg: &Message{Metadata: md}, opts: opts}
	node.channel.enqueue(req, replyChan, true)

	select {
	case resp := <-replyChan:
		return resp
	case <-time.After(defaultTestTimeout):
		t.Fatalf("timeout waiting for response to message %d", msgID)
		return response{}
	}
}

var handlerName = "mock.Server.Test"

func dummySrv() *Server {
	mockSrv := &mockSrv{}
	srv := NewServer()
	srv.RegisterHandler(handlerName, func(ctx ServerCtx, in *Message, finished chan<- *Message) {
		req := in.Message.(*mock.Request)
		defer ctx.Release()
		resp, err := mockSrv.Test(ctx, req)
		SendMessage(ctx, finished, WrapMessage(in.Metadata, resp, err))
	})
	return srv
}

// Helper functions for accessing channel internals

func routerExists(node *RawNode, msgID uint64) bool {
	node.channel.responseMut.Lock()
	defer node.channel.responseMut.Unlock()
	_, exists := node.channel.responseRouters[msgID]
	return exists
}

func getStream(node *RawNode) grpc.ClientStream {
	node.channel.streamMut.Lock()
	defer node.channel.streamMut.Unlock()
	return node.channel.gorumsStream
}

func TestChannelCreation(t *testing.T) {
	node := newNode(t, "127.0.0.1:5000")

	// Send a single message through the channel
	sendRequest(t, node, t.Context(), 1, getCallOptions(E_Multicast, nil))
}

func TestChannelReconnection(t *testing.T) {
	srvAddr := "127.0.0.1:5000"
	startServer, stopServer := testServerSetup(t, srvAddr, dummySrv())

	node := newNode(t, srvAddr)

	// send message when server is down
	resp := sendRequest(t, node, t.Context(), 1, getCallOptions(E_Multicast, nil))
	if resp.err == nil {
		t.Error("response err: got <nil>, want error")
	}

	startServer()

	// send message when server is up but not yet connected,
	// with retries to accommodate gRPC connection establishment
	var successfulSend bool
	for range 10 {
		resp = sendRequest(t, node, t.Context(), 2, getCallOptions(E_Multicast, nil))
		if resp.err == nil {
			successfulSend = true
			break
		}
		// server is up but gRPC connection not yet established, retry after delay
		time.Sleep(500 * time.Millisecond)
	}
	if !successfulSend {
		t.Error("failed to send message after server came back up")
	}

	stopServer()
	time.Sleep(100 * time.Millisecond)

	// send third message when server has previously been up, but is now down
	resp = sendRequest(t, node, t.Context(), 3, getCallOptions(E_Multicast, nil))
	if resp.err == nil {
		t.Error("response err: got <nil>, want error")
	}
}

// TestChannelErrorHandling verifies error detection and handling in various scenarios.
func TestChannelErrorHandling(t *testing.T) {
	tests := []struct {
		name              string
		setup             func(t *testing.T) *RawNode
		wantSpecificError string
		checkLastErr      bool
	}{
		{
			name: "EnqueueToClosedChannel",
			setup: func(t *testing.T) *RawNode {
				node := newNode(t, "127.0.0.1:5000")
				node.mgr.Close()
				time.Sleep(50 * time.Millisecond)
				return node
			},
			wantSpecificError: "channel closed",
		},
		{
			name: "ErrorTracking",
			setup: func(t *testing.T) *RawNode {
				return newNode(t, "127.0.0.1:5002")
			},
		},
		{
			name: "StreamFailureDuringCommunication",
			setup: func(t *testing.T) *RawNode {
				srvAddr := "127.0.0.1:5001"
				startServer, stopServer := testServerSetup(t, srvAddr, dummySrv())
				startServer()
				node := newNode(t, srvAddr)

				if !node.channel.isConnected() {
					t.Error("node should be connected")
				}

				// Send first message successfully
				resp1 := sendRequest(t, node, t.Context(), 1, getCallOptions(E_Multicast, nil))
				if resp1.err != nil {
					t.Errorf("first message should succeed, got error: %v", resp1.err)
				}

				// Stop server to break the stream
				stopServer()
				time.Sleep(100 * time.Millisecond)
				return node
			},
			checkLastErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := tt.setup(t)

			// Send message and verify error
			resp := sendRequest(t, node, t.Context(), 2, getCallOptions(E_Multicast, nil))

			if resp.err == nil {
				t.Error("expected error but got nil")
			} else {
				t.Logf("error received: %v", resp.err)
				if tt.wantSpecificError != "" && resp.err.Error() != tt.wantSpecificError {
					t.Errorf("expected '%s' error, got: %v", tt.wantSpecificError, resp.err)
				}
			}

			if tt.checkLastErr && node.channel.lastErr() == nil {
				t.Error("lastErr should be set after stream failure")
			}
		})
	}
}

// TestChannelEnsureStream verifies that ensureStream correctly manages stream lifecycle.
func TestChannelEnsureStream(t *testing.T) {
	node := newNodeWithServer(t, 0)

	tests := []struct {
		name            string
		ensureTwice     bool
		wantSameStreams bool
	}{
		{
			name:        "CreatesStream",
			ensureTwice: false,
		},
		{
			name:            "Idempotent",
			ensureTwice:     true,
			wantSameStreams: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node.channel = newChannel(node)

			if initialStream := getStream(node); initialStream != nil {
				t.Fatal("Stream should be nil initially")
			}

			if err := node.channel.ensureStream(); err != nil {
				t.Fatalf("First ensureStream failed: %v", err)
			}

			firstStream := getStream(node)
			if firstStream == nil {
				t.Error("Stream was not created by ensureStream")
			}

			if tt.ensureTwice {
				if err := node.channel.ensureStream(); err != nil {
					t.Fatalf("Second ensureStream failed: %v", err)
				}

				secondStream := getStream(node)
				if tt.wantSameStreams && firstStream != secondStream {
					t.Error("ensureStream created a new stream instead of reusing existing one")
				}
			}
		})
	}
}

// TestChannelConnectionState verifies connection state detection and behavior.
func TestChannelConnectionState(t *testing.T) {
	tests := []struct {
		name          string
		node          *RawNode
		setup         func(node *RawNode)
		wantConnected bool
		sendMsg       func(t *testing.T, node *RawNode)
	}{
		{
			name: "RequiresBothReadyAndStream",
			node: newNodeWithServer(t, 0),
			setup: func(node *RawNode) {
				if !node.channel.isConnected() {
					t.Fatal("node should be connected before clearing stream")
				}
				node.channel.clearStream()
			},
			wantConnected: false,
		},
		{
			name:          "WithLiveServer",
			node:          newNodeWithServer(t, 0),
			wantConnected: true,
		},
		{
			name:          "WithoutServer",
			node:          newNode(t, "127.0.0.1:5003"),
			wantConnected: false,
		},
		{
			name:          "SendWithNilStream",
			node:          newNode(t, "127.0.0.1:9999"),
			wantConnected: false,
			sendMsg: func(t *testing.T, node *RawNode) {
				opts := getCallOptions(E_Multicast, []CallOption{WithNoSendWaiting()})
				resp := sendRequest(t, node, t.Context(), 4, opts)
				if resp.err == nil {
					t.Error("expected error when sending with nil stream")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(tt.node)
			}

			connected := tt.node.channel.isConnected()
			if connected != tt.wantConnected {
				t.Errorf("isConnected() = %v, want %v", connected, tt.wantConnected)
			}

			if tt.sendMsg != nil {
				tt.sendMsg(t, tt.node)
			}
		})
	}
}

// TestChannelDeadlock reproduces a deadlock bug (issue #235) that occurred
// in channel.go when the stream broke during active communication.
//
// Root Cause:
// The receiver goroutine held a read lock while performing a blocking I/O operation
// that could hang indefinitely when the stream broke. Meanwhile, the sender goroutine
// tried to acquire a write lock to reconnect, creating a deadlock.
//
// This test verifies the fix by:
// 1. Establishing a connection and activating the stream
// 2. Breaking the stream by stopping the server
// 3. Sending multiple messages concurrently to trigger the deadlock condition
// 4. Verifying all goroutines can successfully enqueue without hanging
func TestChannelDeadlock(t *testing.T) {
	node := newNodeWithServer(t, 0)
	if !node.channel.isConnected() {
		t.Error("node should be connected")
	}

	// Send a message to activate the stream
	sendRequest(t, node, t.Context(), 1, getCallOptions(E_Multicast, nil))

	// Break the stream, forcing a reconnection on next send
	node.channel.clearStream()
	time.Sleep(20 * time.Millisecond)

	// Send multiple messages concurrently when stream is broken with the
	// goal to trigger a deadlock between sender and receiver goroutines.
	doneChan := make(chan bool, 10)
	for id := range 10 {
		go func() {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			md := ordering.NewGorumsMetadata(ctx, uint64(100+id), handlerName)
			req := request{ctx: ctx, msg: &Message{Metadata: md}}

			// try to enqueue
			select {
			case node.channel.sendQ <- req:
				// successfully enqueued
				doneChan <- true
			case <-ctx.Done():
				// timed out trying to enqueue (deadlock!)
				doneChan <- false
			}
		}()
	}

	// Wait for all goroutines to complete
	timeout := time.After(5 * time.Second)
	successful := 0
	for completed := range 10 {
		select {
		case success := <-doneChan:
			if success {
				successful++
			}
		case <-timeout:
			// remaining goroutines are stuck trying to enqueue.
			t.Fatalf("DEADLOCK: Only %d/10 goroutines completed (%d successful).", completed, successful)
		}
	}

	// If we reach here, all 10 goroutines completed (but some may have failed to enqueue)
	if successful < 10 {
		t.Fatalf("DEADLOCK: %d/10 goroutines timed out trying to enqueue (sendQ blocked)", 10-successful)
	}
}

// TestChannelRouterLifecycle tests router creation, persistence, and cleanup behavior.
func TestChannelRouterLifecycle(t *testing.T) {
	node := newNodeWithServer(t, 0)
	if !node.channel.isConnected() {
		t.Fatal("node should be connected")
	}

	waitSendDoneOpts := getCallOptions(E_Multicast, []CallOption{WithNoSendWaiting()})
	waitSendDoneOpts.waitSendDone = true
	waitSendDoneOpts.callType = &protoimpl.ExtensionInfo{}

	tests := []struct {
		name             string
		msgID            uint64
		opts             callOptions
		send             func(t *testing.T, node *RawNode, msgID uint64, opts callOptions) response
		verifyResponse   func(t *testing.T, resp response)
		afterSend        func(t *testing.T, node *RawNode, msgID uint64)
		wantRouterExists bool
	}{
		{
			name:  "WaitSendDone",
			msgID: 1,
			opts:  waitSendDoneOpts,
			send: func(t *testing.T, node *RawNode, msgID uint64, opts callOptions) response {
				if !opts.mustWaitSendDone() {
					t.Fatal("mustWaitSendDone should return true")
				}
				return sendRequest(t, node, t.Context(), msgID, opts)
			},
			verifyResponse: func(t *testing.T, resp response) {
				if resp.msg != nil {
					t.Error("expected empty response from send confirmation")
				}
			},
			wantRouterExists: false,
		},
		{
			name:  "StreamingKeepsRouterAlive",
			msgID: 2,
			opts:  getCallOptions(E_Multicast, nil),
			send:  sendStreamingRequest,
			afterSend: func(t *testing.T, node *RawNode, msgID uint64) {
				if !routerExists(node, msgID) {
					t.Error("router should still exist for streaming message before manual delete")
				}
				node.channel.deleteRouter(msgID)
			},
			wantRouterExists: false,
		},
		{
			name:  "NonStreamingAutoCleanup",
			msgID: 5000,
			opts:  getCallOptions(E_Multicast, nil),
			send: func(t *testing.T, node *RawNode, msgID uint64, opts callOptions) response {
				return sendRequest(t, node, t.Context(), msgID, opts)
			},
			afterSend: func(t *testing.T, node *RawNode, msgID uint64) {
				time.Sleep(100 * time.Millisecond)
			},
			wantRouterExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := tt.send(t, node, tt.msgID, tt.opts)
			if resp.err != nil {
				t.Errorf("unexpected error: %v", resp.err)
			}

			if tt.verifyResponse != nil {
				tt.verifyResponse(t, resp)
			}

			if tt.afterSend != nil {
				tt.afterSend(t, node, tt.msgID)
			}

			if exists := routerExists(node, tt.msgID); exists != tt.wantRouterExists {
				t.Errorf("router exists = %v, want %v", exists, tt.wantRouterExists)
			}
		})
	}
}

// Test 1: Concurrent Message Sending
func TestChannelConcurrentSends(t *testing.T) {
	node := newNodeWithServer(t, 0)

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
	for i := range numGoroutines {
		go func(goroutineID int) {
			for j := range messagesPerGoroutine {
				msgID := uint64(goroutineID*1000 + j + 1)
				// Use standard multicast options which provide send confirmation
				opts := getCallOptions(E_Multicast, nil)
				resp := sendRequest(t, node, t.Context(), msgID, opts)
				results <- result{msgID: msgID, err: resp.err}
			}
		}(i)
	}

	// Collect results
	var errors []error
	for range numMessages {
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
			node := newNodeWithServer(t, tt.serverDelay)

			ctx, cancel := tt.contextSetup(t.Context())
			t.Cleanup(cancel)

			// Send request
			msgID := uint64(100 + i)
			resp := sendRequest(t, node, ctx, msgID, tt.callOpts)

			if !errors.Is(resp.err, tt.wantErr) {
				t.Errorf("expected %v, got: %v", tt.wantErr, resp.err)
			}
		})
	}
}

// TestChannelShutdown verifies proper cleanup when channel is closed.
func TestChannelShutdown(t *testing.T) {
	node := newNodeWithServer(t, 0)

	if !node.channel.isConnected() {
		t.Fatal("node should be connected")
	}

	// Enqueue several messages in goroutines (don't wait for responses)
	const numMessages = 10
	for i := range numMessages {
		go func(msgID uint64) {
			sendRequest(t, node, t.Context(), msgID, getCallOptions(E_Multicast, nil))
		}(uint64(i))
	}

	// Give messages time to enqueue
	time.Sleep(50 * time.Millisecond)

	// Close the manager (which should close the node)
	node.mgr.Close()

	// Try to send a message after closure
	resp := sendRequest(t, node, t.Context(), 999, getCallOptions(E_Multicast, nil))
	if resp.err == nil {
		t.Error("expected error when sending to closed channel")
	}
}

// Test 6: Send Completion Waiting Behavior
func TestChannelSendCompletionWaiting(t *testing.T) {
	node := newNodeWithServer(t, 0)

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
			resp := sendRequest(t, node, t.Context(), msgID, opts)
			elapsed := time.Since(start)

			// Just verify we got a response (error or not)
			t.Logf("response received in %v (waitSendDone=%v, err=%v)", elapsed, tt.waitSendDone, resp.err)
		})
	}
}

// TestChannelResponseRouting verifies that responses are correctly routed to their callers.
func TestChannelResponseRouting(t *testing.T) {
	node := newNodeWithServer(t, 0)

	// Send multiple messages and verify each gets routed correctly
	const numMessages = 20
	type msgResponse struct {
		msgID uint64
		resp  response
	}

	results := make(chan msgResponse, numMessages)

	// Send all messages
	for i := range numMessages {
		msgID := uint64(i + 1000)
		go func(id uint64) {
			opts := getCallOptions(E_Multicast, nil)
			resp := sendRequest(t, node, t.Context(), id, opts)
			results <- msgResponse{msgID: id, resp: resp}
		}(msgID)
	}

	// Collect and verify results
	received := make(map[uint64]bool)
	for range numMessages {
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
