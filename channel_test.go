package gorums

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

const defaultTestTimeout = 3 * time.Second

type mockSrv struct{}

func (mockSrv) Test(_ ServerCtx, req proto.Message) (proto.Message, error) {
	return pb.String(mock.GetVal(req) + "-mocked-"), nil
}

// delayServerFn returns a server function that delays responses by the given duration.
func delayServerFn(delay time.Duration) func(_ int) ServerIface {
	return func(_ int) ServerIface {
		mockSrv := &mockSrv{}
		srv := NewServer()
		srv.RegisterHandler(mock.TestMethod, func(ctx ServerCtx, in *Message) (*Message, error) {
			// Simulate slow processing
			time.Sleep(delay)
			resp, err := mockSrv.Test(ctx, in.GetProtoMessage())
			return NewResponseMessage(in.GetMetadata(), resp), err
		})
		return srv
	}
}

// newNodeWithStoppableServer creates a node with a server that can be stopped early.
// This is needed for tests that verify behavior when servers go down.
// Note: This uses TestSetup instead of TestServers to avoid goleak checks in
// table-driven tests where subtests may intentionally create nodes without servers.
func newNodeWithStoppableServer(t testing.TB, delay time.Duration) (*Node, func()) {
	t.Helper()
	addrs, teardown := TestSetup(t, 1, delayServerFn(delay))
	return NewTestNode(t, addrs[0]), teardown
}

func sendRequest(t testing.TB, node *Node, req request, msgID uint64) NodeResponse[proto.Message] {
	t.Helper()
	if req.ctx == nil {
		req.ctx = t.Context()
	}
	req.msg = NewRequestMessage(ordering.NewGorumsMetadata(req.ctx, msgID, mock.TestMethod), nil)
	replyChan := make(chan NodeResponse[proto.Message], 1)
	req.responseChan = replyChan
	node.channel.enqueue(req)

	select {
	case resp := <-replyChan:
		return resp
	case <-time.After(defaultTestTimeout):
		t.Fatalf("timeout waiting for response to message %d", msgID)
		return NodeResponse[proto.Message]{}
	}
}

type msgResponse struct {
	msgID uint64
	resp  NodeResponse[proto.Message]
}

func send(t testing.TB, results chan<- msgResponse, node *Node, goroutineID, msgsToSend int, req request) {
	for j := range msgsToSend {
		msgID := uint64(goroutineID*1000 + j)
		resp := sendRequest(t, node, req, msgID)
		results <- msgResponse{msgID: msgID, resp: resp}
	}
}

// Helper functions for accessing channel internals

func routerExists(node *Node, msgID uint64) bool {
	node.channel.responseMut.Lock()
	defer node.channel.responseMut.Unlock()
	_, exists := node.channel.responseRouters[msgID]
	return exists
}

func getStream(node *Node) grpc.ClientStream {
	return node.channel.getStream()
}

func TestChannelCreation(t *testing.T) {
	node := NewTestNode(t, "127.0.0.1:5000")

	// send message when server is down
	resp := sendRequest(t, node, request{waitSendDone: true}, 1)
	if resp.Err == nil {
		t.Error("response err: got <nil>, want error")
	}
}

// TestChannelShutdown verifies proper cleanup when channel is closed.
func TestChannelShutdown(t *testing.T) {
	node := SetupNode(t, delayServerFn(0))
	if !node.channel.isConnected() {
		t.Error("node should be connected")
	}

	// enqueue several messages to confirm normal operation
	const numMessages = 10
	var wg sync.WaitGroup
	for i := range numMessages {
		wg.Go(func() {
			resp := sendRequest(t, node, request{}, uint64(i))
			if resp.Err != nil {
				t.Errorf("unexpected error for message %d, got error: %v", i, resp.Err)
			}
		})
	}
	wg.Wait()

	// shut down the node's channel
	if err := node.close(); err != nil {
		t.Errorf("error closing node: %v", err)
	}

	// try to send a message after node closure
	resp := sendRequest(t, node, request{}, 999)
	if resp.Err == nil {
		t.Error("expected error when sending to closed channel")
	} else if resp.Err.Error() != "node closed" {
		t.Errorf("expected 'node closed' error, got: %v", resp.Err)
	}

	if node.channel.isConnected() {
		t.Error("channel should not be connected after close")
	}
}

// TestChannelSendCompletionWaiting verifies the behavior of send completion waiting.
func TestChannelSendCompletionWaiting(t *testing.T) {
	node := SetupNode(t, delayServerFn(0))

	tests := []struct {
		name         string
		waitSendDone bool
	}{
		{name: "WaitForSend", waitSendDone: true},
		{name: "NoSendWaiting", waitSendDone: false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			resp := sendRequest(t, node, request{waitSendDone: tt.waitSendDone}, uint64(i))
			elapsed := time.Since(start)
			if resp.Err != nil {
				t.Errorf("unexpected error: %v", resp.Err)
			}
			t.Logf("response received in %v", elapsed)
		})
	}
}

// TestChannelErrors verifies error detection and handling in various scenarios.
func TestChannelErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) *Node
		wantErr string
	}{
		{
			name: "EnqueueWithoutServer",
			setup: func(t *testing.T) *Node {
				return NewTestNode(t, "127.0.0.1:5002")
			},
			wantErr: "connect: connection refused",
		},
		{
			name: "EnqueueToClosedChannel",
			setup: func(t *testing.T) *Node {
				node := NewTestNode(t, "127.0.0.1:5000")
				err := node.close()
				if err != nil {
					t.Errorf("failed to close node: %v", err)
				}
				return node
			},
			wantErr: "node closed",
		},
		{
			name: "EnqueueToServerWithClosedNode",
			setup: func(t *testing.T) *Node {
				node := SetupNode(t, delayServerFn(0))
				err := node.close()
				if err != nil {
					t.Errorf("failed to close node: %v", err)
				}
				return node
			},
			wantErr: "node closed",
		},
		{
			name: "ServerFailureDuringCommunication",
			setup: func(t *testing.T) *Node {
				node, stopServer := newNodeWithStoppableServer(t, 0)
				resp := sendRequest(t, node, request{waitSendDone: true}, 1)
				if resp.Err != nil {
					t.Errorf("first message should succeed, got error: %v", resp.Err)
				}
				stopServer()
				return node
			},
			wantErr: "connect: connection refused",
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := tt.setup(t)
			time.Sleep(100 * time.Millisecond)

			// Send message and verify error
			resp := sendRequest(t, node, request{waitSendDone: true}, uint64(i))
			if resp.Err == nil {
				t.Errorf("expected error '%s' but got nil", tt.wantErr)
			} else if !strings.Contains(resp.Err.Error(), tt.wantErr) {
				t.Errorf("expected error '%s', got: %v", tt.wantErr, resp.Err)
			}
		})
	}
}

// TestChannelEnsureStream verifies that ensureStream correctly manages stream lifecycle.
func TestChannelEnsureStream(t *testing.T) {
	// Helper to prepare a fresh node with no stream
	newNodeWithoutStream := func(t *testing.T) *Node {
		node := SetupNode(t, delayServerFn(0))
		node.cancel() // ensure sender and receiver goroutines are stopped
		node.channel = newChannel(node)
		return node
	}

	// Helper to verify stream expectations
	cmpStream := func(t *testing.T, first, second grpc.ClientStream, wantSame bool) {
		t.Helper()
		// If second is nil, skip equality check (covered by UnconnectedNodeHasNoStream action)
		if second == nil {
			return
		}
		// Both streams provided - check equality
		if wantSame && first != second {
			t.Error("expected same stream, but got different stream")
		}
		if !wantSame && first == second {
			t.Error("expected different stream, but got same stream")
		}
	}

	tests := []struct {
		name     string
		setup    func(t *testing.T) *Node
		action   func(node *Node) (first, second grpc.ClientStream)
		wantSame bool
	}{
		{
			name:  "UnconnectedNodeHasNoStream",
			setup: func(t *testing.T) *Node { return NewTestNode(t, "") },
			action: func(node *Node) (grpc.ClientStream, grpc.ClientStream) {
				if err := node.channel.ensureStream(); err == nil {
					t.Error("ensureStream succeeded unexpectedly")
				}
				if getStream(node) != nil {
					t.Error("stream should be nil")
				}
				return nil, nil
			},
		},
		{
			name:  "CreatesStreamWhenConnected",
			setup: newNodeWithoutStream,
			action: func(node *Node) (grpc.ClientStream, grpc.ClientStream) {
				if err := node.channel.ensureStream(); err != nil {
					t.Errorf("ensureStream failed: %v", err)
				}
				return getStream(node), nil
			},
		},
		{
			name:  "RepeatedCallsReturnSameStream",
			setup: newNodeWithoutStream,
			action: func(node *Node) (grpc.ClientStream, grpc.ClientStream) {
				if err := node.channel.ensureStream(); err != nil {
					t.Errorf("first ensureStream failed: %v", err)
				}
				first := getStream(node)
				if err := node.channel.ensureStream(); err != nil {
					t.Errorf("second ensureStream failed: %v", err)
				}
				return first, getStream(node)
			},
			wantSame: true,
		},
		{
			name:  "StreamDisconnectionCreatesNewStream",
			setup: newNodeWithoutStream,
			action: func(node *Node) (grpc.ClientStream, grpc.ClientStream) {
				if err := node.channel.ensureStream(); err != nil {
					t.Errorf("initial ensureStream failed: %v", err)
				}
				first := getStream(node)
				node.channel.clearStream()
				if err := node.channel.ensureStream(); err != nil {
					t.Errorf("ensureStream after disconnect failed: %v", err)
				}
				return first, getStream(node)
			},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := tt.setup(t)
			if stream := getStream(node); stream != nil {
				t.Fatal("stream should be nil initially")
			}
			first, second := tt.action(node)
			cmpStream(t, first, second, tt.wantSame)
		})
	}
}

// TestChannelConnectionState verifies connection state detection and behavior.
func TestChannelConnectionState(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) *Node
		wantConnected bool
	}{
		{
			name:          "WithoutServer",
			setup:         func(t *testing.T) *Node { return NewTestNode(t, "127.0.0.1:5003") },
			wantConnected: false,
		},
		{
			name:          "WithLiveServer",
			setup:         func(t *testing.T) *Node { return SetupNode(t, delayServerFn(0)) },
			wantConnected: true,
		},
		{
			name: "RequiresBothReadyAndStream",
			setup: func(t *testing.T) *Node {
				node := SetupNode(t, delayServerFn(0))
				if !node.channel.isConnected() {
					t.Fatal("node should be connected before clearing stream")
				}
				node.channel.clearStream()
				return node
			},
			wantConnected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := tt.setup(t)
			connected := node.channel.isConnected()
			if connected != tt.wantConnected {
				t.Errorf("isConnected() = %v, want %v", connected, tt.wantConnected)
			}
		})
	}
}

// TestChannelConcurrentSends tests sending multiple messages concurrently from multiple goroutines.
func TestChannelConcurrentSends(t *testing.T) {
	node := SetupNode(t, delayServerFn(0))

	const numMessages = 1000
	const numGoroutines = 10
	msgsPerGoroutine := numMessages / (2 * numGoroutines)

	results := make(chan msgResponse, numMessages)
	for goID := range numGoroutines {
		go func() {
			send(t, results, node, goID, msgsPerGoroutine, request{waitSendDone: true})
			send(t, results, node, goID, msgsPerGoroutine, request{waitSendDone: false})
		}()
	}

	var errs []error
	for range numMessages {
		res := <-results
		if res.resp.Err != nil {
			errs = append(errs, res.resp.Err)
		}
	}

	if len(errs) > 0 {
		t.Errorf("got %d errors during concurrent sends (first few): %v", len(errs), errs[:min(3, len(errs))])
	}
	if !node.channel.isConnected() {
		t.Error("node should still be connected after concurrent sends")
	}
	if node.mgr == nil {
		t.Error("manager should not be nil")
	}
}

func TestChannelContext(t *testing.T) {
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
		waitSendDone bool
		wantErr      error
	}{
		{
			name:         "CancelBeforeSend/WaitSending",
			serverDelay:  0,
			contextSetup: cancelledContext,
			waitSendDone: true,
			wantErr:      context.Canceled,
		},
		{
			name:         "CancelBeforeSend/NoSendWaiting",
			serverDelay:  0,
			contextSetup: cancelledContext,
			waitSendDone: false,
			wantErr:      context.Canceled,
		},
		{
			name:         "CancelDuringSend/WaitSending",
			serverDelay:  3 * time.Second,
			contextSetup: expireBeforeSend,
			waitSendDone: true,
			wantErr:      context.DeadlineExceeded,
		},
		{
			name:         "CancelDuringSend/NoSendWaiting",
			serverDelay:  3 * time.Second,
			contextSetup: expireBeforeSend,
			waitSendDone: false,
			wantErr:      context.DeadlineExceeded,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.contextSetup(t.Context())
			t.Cleanup(cancel)

			node := SetupNode(t, delayServerFn(tt.serverDelay))
			resp := sendRequest(t, node, request{ctx: ctx, waitSendDone: tt.waitSendDone}, uint64(i))
			if !errors.Is(resp.Err, tt.wantErr) {
				t.Errorf("expected %v, got: %v", tt.wantErr, resp.Err)
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
	node := SetupNode(t, delayServerFn(0))
	if !node.channel.isConnected() {
		t.Error("node should be connected")
	}

	// Send a message to activate the stream
	sendRequest(t, node, request{waitSendDone: true}, 1)

	// Break the stream, forcing a reconnection on next send
	node.channel.clearStream()
	time.Sleep(20 * time.Millisecond)

	// Send multiple messages concurrently when stream is broken with the
	// goal to trigger a deadlock between sender and receiver goroutines.
	doneChan := make(chan bool, 10)
	for id := range 10 {
		go func() {
			ctx := TestContext(t, 3*time.Second)
			md := ordering.NewGorumsMetadata(ctx, uint64(100+id), mock.TestMethod)
			req := request{ctx: ctx, msg: NewRequestMessage(md, nil)}

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
	node := SetupNode(t, delayServerFn(0))
	if !node.channel.isConnected() {
		t.Error("node should be connected")
	}

	tests := []struct {
		name         string
		waitSendDone bool
		streaming    bool
		afterSend    func(t *testing.T, node *Node, msgID uint64)
		wantRouter   bool
	}{
		{name: "WaitSendDone/NonStreamingAutoCleanup", waitSendDone: true, streaming: false, wantRouter: false},
		{name: "WaitSendDone/StreamingKeepsRouterAlive", waitSendDone: true, streaming: true, wantRouter: true},
		{name: "NoSendWaiting/NonStreamingAutoCleanup", waitSendDone: false, streaming: false, wantRouter: false},
		{name: "NoSendWaiting/StreamingKeepsRouterAlive", waitSendDone: false, streaming: true, wantRouter: true},
	}
	for i, tt := range tests {
		name := fmt.Sprintf("msgID=%d/%s/streaming=%t", i, tt.name, tt.streaming)
		t.Run(name, func(t *testing.T) {
			msgID := uint64(i)
			resp := sendRequest(t, node, request{waitSendDone: tt.waitSendDone, streaming: tt.streaming}, msgID)
			if resp.Err != nil {
				t.Errorf("unexpected error: %v", resp.Err)
			}
			if exists := routerExists(node, msgID); exists != tt.wantRouter {
				t.Errorf("router exists = %v, want %v", exists, tt.wantRouter)
			}
			node.channel.deleteRouter(msgID) // just for kicks
		})
	}
}

// TestChannelResponseRouting sends multiple messages and verifies that
// responses are correctly routed to their callers.
func TestChannelResponseRouting(t *testing.T) {
	node := SetupNode(t, delayServerFn(0))

	const numMessages = 20
	results := make(chan msgResponse, numMessages)

	for i := range numMessages {
		go send(t, results, node, i, 1, request{})
	}

	// Collect and verify results
	received := make(map[uint64]bool)
	for range numMessages {
		result := <-results
		if result.resp.Err != nil {
			t.Errorf("message %d got error: %v", result.msgID, result.resp.Err)
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

// TestChannelStreamReadySignaling verifies that the receiver goroutine is properly notified
// when a stream becomes available. This tests the stream-ready signaling mechanism
// that replaces the old time.Sleep polling approach.
func TestChannelStreamReadySignaling(t *testing.T) {
	node := SetupNode(t, delayServerFn(0))

	// The first request triggers stream creation. We measure how quickly
	// the receiver starts processing after the stream is ready.
	start := time.Now()
	resp := sendRequest(t, node, request{}, 1)
	firstLatency := time.Since(start)

	if resp.Err != nil {
		t.Fatalf("unexpected error on first request: %v", resp.Err)
	}

	// Second request should be faster since stream is already established
	start = time.Now()
	resp = sendRequest(t, node, request{}, 2)
	secondLatency := time.Since(start)

	if resp.Err != nil {
		t.Fatalf("unexpected error on second request: %v", resp.Err)
	}

	t.Logf("first request latency: %v", firstLatency)
	t.Logf("second request latency: %v", secondLatency)

	// The first request should complete in reasonable time (not waiting for polling timeout).
	// With the old 10ms polling, the first request could take 10-20ms just waiting for the stream.
	// With proper signaling, it should be much faster (sub-millisecond for the signal itself).
	const maxAcceptableLatency = 100 * time.Millisecond // generous bound for CI environments
	if firstLatency > maxAcceptableLatency {
		t.Errorf("first request took %v, expected < %v (possible stream-ready polling delay)", firstLatency, maxAcceptableLatency)
	}
}

// TestChannelStreamReadyAfterReconnect verifies that the receiver is properly notified
// when a stream is re-established after being cleared (simulating reconnection).
func TestChannelStreamReadyAfterReconnect(t *testing.T) {
	node, teardown := newNodeWithStoppableServer(t, 0)
	defer teardown()

	// First request to establish the stream
	resp := sendRequest(t, node, request{}, 1)
	if resp.Err != nil {
		t.Fatalf("unexpected error on first request: %v", resp.Err)
	}

	// Clear the stream to simulate disconnection.
	// Note: In production, clearStream is called by the receiver when it detects an error.
	// The next request will trigger stream re-creation via ensureStream().
	node.channel.clearStream()

	// The sender's ensureStream() will recreate the stream.
	// We may need to retry a few times since there's a race between
	// clearStream and the sender's stream check.
	var reconnectLatency time.Duration
	var lastErr error
	start := time.Now()
	for i := range 5 {
		resp = sendRequest(t, node, request{}, uint64(i+2))
		if resp.Err == nil {
			reconnectLatency = time.Since(start)
			break
		}
		lastErr = resp.Err
		// Give the sender a chance to reconnect
		time.Sleep(5 * time.Millisecond)
	}
	if resp.Err != nil {
		t.Fatalf("failed to reconnect after 5 attempts: %v", lastErr)
	}

	t.Logf("reconnect latency: %v", reconnectLatency)

	// Even after reconnection, the latency should be reasonable
	const maxAcceptableLatency = 200 * time.Millisecond
	if reconnectLatency > maxAcceptableLatency {
		t.Errorf("reconnect took %v, expected < %v", reconnectLatency, maxAcceptableLatency)
	}
}

// BenchmarkChannelStreamReadyFirstRequest measures the latency of the first request,
// which includes stream creation and the stream-ready signaling.
// Note: This benchmark includes server setup overhead, so absolute numbers
// should be interpreted with caution. The goal is to detect regressions.
func BenchmarkChannelStreamReadyFirstRequest(b *testing.B) {
	for b.Loop() {
		node, teardown := newNodeWithStoppableServer(b, 0)

		// Use a fresh context for the benchmark request
		ctx := TestContext(b, defaultTestTimeout)
		req := request{ctx: ctx}
		req.msg = NewRequestMessage(ordering.NewGorumsMetadata(ctx, 1, mock.TestMethod), nil)
		replyChan := make(chan NodeResponse[proto.Message], 1)
		req.responseChan = replyChan
		node.channel.enqueue(req)

		select {
		case resp := <-replyChan:
			if resp.Err != nil {
				b.Logf("request error (may occur during rapid cycles): %v", resp.Err)
			}
		case <-ctx.Done():
			b.Logf("timeout (may occur during rapid cycles)")
		}

		// Close the node before stopping the server to ensure clean shutdown
		_ = node.close()
		teardown()
	}
}

// BenchmarkChannelStreamReadySubsequentRequest measures the latency of requests
// after the stream is already established (steady-state performance).
func BenchmarkChannelStreamReadySubsequentRequest(b *testing.B) {
	node, teardown := newNodeWithStoppableServer(b, 0)
	defer teardown()

	// Warm up: establish the stream
	resp := sendRequest(b, node, request{}, 0)
	if resp.Err != nil {
		b.Fatalf("warmup error: %v", resp.Err)
	}

	b.ResetTimer()
	for i := range b.N {
		resp := sendRequest(b, node, request{}, uint64(i+1))
		if resp.Err != nil {
			b.Fatalf("unexpected error: %v", resp.Err)
		}
	}
}

// BenchmarkChannelStreamReadyReconnect measures the latency of reconnecting
// after the stream has been cleared.
// Note: This benchmark has inherent variability due to the race between
// clearStream and the sender's ensureStream call.
func BenchmarkChannelStreamReadyReconnect(b *testing.B) {
	node, teardown := newNodeWithStoppableServer(b, 0)
	defer teardown()

	// Establish initial stream with a fresh context
	ctx := context.Background()
	req := request{ctx: ctx}
	req.msg = NewRequestMessage(ordering.NewGorumsMetadata(ctx, 0, mock.TestMethod), nil)
	replyChan := make(chan NodeResponse[proto.Message], 1)
	req.responseChan = replyChan
	node.channel.enqueue(req)

	select {
	case resp := <-replyChan:
		if resp.Err != nil {
			b.Fatalf("initial request error: %v", resp.Err)
		}
	case <-time.After(defaultTestTimeout):
		b.Fatal("timeout on initial request")
	}

	b.ResetTimer()
	for i := range b.N {
		node.channel.clearStream()

		// Wait a tiny bit for the receiver to notice the stream is gone
		// and be ready for the signal. This simulates real-world behavior
		// where the receiver detects the error before reconnection.
		time.Sleep(100 * time.Microsecond)

		// Now send a request which will trigger ensureStream -> newNodeStream -> signal
		ctx := context.Background()
		req := request{ctx: ctx}
		req.msg = NewRequestMessage(ordering.NewGorumsMetadata(ctx, uint64(i+1), mock.TestMethod), nil)
		replyChan := make(chan NodeResponse[proto.Message], 1)
		req.responseChan = replyChan
		node.channel.enqueue(req)

		select {
		case resp := <-replyChan:
			if resp.Err != nil {
				// Stream down error is expected sometimes due to race; just continue
				b.Logf("request %d error: %v", i, resp.Err)
			}
		case <-time.After(500 * time.Millisecond):
			b.Fatalf("timeout on request %d", i)
		}
	}
}
