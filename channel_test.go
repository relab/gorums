package gorums

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/dynamic"
	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

const defaultTestTimeout = 3 * time.Second

var (
	waitSendDone  = getCallOptions(E_Multicast, nil)
	noSendWaiting = getCallOptions(E_Multicast, []CallOption{WithNoSendWaiting()})
)

// newNode creates a node for the given server address and adds it to a new manager.
func newNode(t testing.TB, srvAddr string) *RawNode {
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
func newNodeWithServer(t testing.TB, delay time.Duration) *RawNode {
	t.Helper()
	node, teardown := newNodeWithStoppableServer(t, delay)
	t.Cleanup(teardown)
	return node
}

const handlerName = "mock.Server.Test"

type mockSrv struct{}

func (mockSrv) Test(_ ServerCtx, req proto.Message) (proto.Message, error) {
	return dynamic.NewResponse(dynamic.GetVal(req) + "-mocked-"), nil
}

func newNodeWithStoppableServer(t testing.TB, delay time.Duration) (*RawNode, func()) {
	t.Helper()
	addrs, teardown := TestSetup(t, 1, func(_ int) ServerIface {
		dynamic.Register(t)
		mockSrv := &mockSrv{}
		srv := NewServer()
		srv.RegisterHandler(handlerName, func(ctx ServerCtx, in *Message) (*Message, error) {
			// Simulate slow processing
			time.Sleep(delay)
			req := AsProto[proto.Message](in)
			resp, err := mockSrv.Test(ctx, req)
			return NewResponseMessage(in.GetMetadata(), resp), err
		})
		return srv
	})

	return newNode(t, addrs[0]), teardown
}

func sendRequest(t testing.TB, node *RawNode, req request, msgID uint64) response {
	t.Helper()
	if req.ctx == nil {
		req.ctx = t.Context()
	}
	req.msg = NewRequestMessage(ordering.NewGorumsMetadata(req.ctx, msgID, handlerName), nil)
	replyChan := make(chan response, 1)
	node.channel.enqueue(req, replyChan)

	select {
	case resp := <-replyChan:
		return resp
	case <-time.After(defaultTestTimeout):
		t.Fatalf("timeout waiting for response to message %d", msgID)
		return response{}
	}
}

type msgResponse struct {
	msgID uint64
	resp  response
}

func send(t testing.TB, results chan<- msgResponse, node *RawNode, goroutineID, msgsToSend int, req request) {
	for j := range msgsToSend {
		msgID := uint64(goroutineID*1000 + j)
		resp := sendRequest(t, node, req, msgID)
		results <- msgResponse{msgID: msgID, resp: resp}
	}
}

// Helper functions for accessing channel internals

func routerExists(node *RawNode, msgID uint64) bool {
	node.channel.responseMut.Lock()
	defer node.channel.responseMut.Unlock()
	_, exists := node.channel.responseRouters[msgID]
	return exists
}

func getStream(node *RawNode) grpc.ClientStream {
	return node.channel.getStream()
}

func TestChannelCreation(t *testing.T) {
	node := newNode(t, "127.0.0.1:5000")

	// send message when server is down
	resp := sendRequest(t, node, request{opts: waitSendDone}, 1)
	if resp.err == nil {
		t.Error("response err: got <nil>, want error")
	}
}

// TestChannelShutdown verifies proper cleanup when channel is closed.
func TestChannelShutdown(t *testing.T) {
	node := newNodeWithServer(t, 0)
	if !node.channel.isConnected() {
		t.Error("node should be connected")
	}

	// enqueue several messages to confirm normal operation
	const numMessages = 10
	var wg sync.WaitGroup
	for i := range numMessages {
		wg.Go(func() {
			resp := sendRequest(t, node, request{}, uint64(i))
			if resp.err != nil {
				t.Errorf("unexpected error for message %d, got error: %v", i, resp.err)
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
	if resp.err == nil {
		t.Error("expected error when sending to closed channel")
	} else if resp.err.Error() != "node closed" {
		t.Errorf("expected 'node closed' error, got: %v", resp.err)
	}

	if node.channel.isConnected() {
		t.Error("channel should not be connected after close")
	}
}

// TestChannelSendCompletionWaiting verifies the behavior of send completion waiting.
func TestChannelSendCompletionWaiting(t *testing.T) {
	node := newNodeWithServer(t, 0)

	tests := []struct {
		name string
		opts callOptions
	}{
		{name: "WaitForSend", opts: waitSendDone},
		{name: "NoSendWaiting", opts: noSendWaiting},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			resp := sendRequest(t, node, request{opts: tt.opts}, uint64(i))
			elapsed := time.Since(start)
			if resp.err != nil {
				t.Errorf("unexpected error: %v", resp.err)
			}
			t.Logf("response received in %v", elapsed)
		})
	}
}

// TestChannelErrors verifies error detection and handling in various scenarios.
func TestChannelErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) *RawNode
		wantErr string
	}{
		{
			name: "EnqueueWithoutServer",
			setup: func(t *testing.T) *RawNode {
				return newNode(t, "127.0.0.1:5002")
			},
			wantErr: "connect: connection refused",
		},
		{
			name: "EnqueueToClosedChannel",
			setup: func(t *testing.T) *RawNode {
				node := newNode(t, "127.0.0.1:5000")
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
			setup: func(t *testing.T) *RawNode {
				node := newNodeWithServer(t, 0)
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
			setup: func(t *testing.T) *RawNode {
				node, stopServer := newNodeWithStoppableServer(t, 0)
				resp := sendRequest(t, node, request{opts: waitSendDone}, 1)
				if resp.err != nil {
					t.Errorf("first message should succeed, got error: %v", resp.err)
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
			resp := sendRequest(t, node, request{opts: waitSendDone}, uint64(i))
			if resp.err == nil {
				t.Errorf("expected error '%s' but got nil", tt.wantErr)
			} else {
				if !strings.Contains(resp.err.Error(), tt.wantErr) {
					t.Errorf("expected error '%s', got: %v", tt.wantErr, resp.err)
				}
			}
		})
	}
}

// TestChannelEnsureStream verifies that ensureStream correctly manages stream lifecycle.
func TestChannelEnsureStream(t *testing.T) {
	// Helper to prepare a fresh node with no stream
	newNodeWithoutStream := func(t *testing.T) *RawNode {
		node := newNodeWithServer(t, 0)
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
		setup    func(t *testing.T) *RawNode
		action   func(node *RawNode) (first, second grpc.ClientStream)
		wantSame bool
	}{
		{
			name:  "UnconnectedNodeHasNoStream",
			setup: func(t *testing.T) *RawNode { return newNode(t, "") },
			action: func(node *RawNode) (grpc.ClientStream, grpc.ClientStream) {
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
			action: func(node *RawNode) (grpc.ClientStream, grpc.ClientStream) {
				if err := node.channel.ensureStream(); err != nil {
					t.Errorf("ensureStream failed: %v", err)
				}
				return getStream(node), nil
			},
		},
		{
			name:  "RepeatedCallsReturnSameStream",
			setup: newNodeWithoutStream,
			action: func(node *RawNode) (grpc.ClientStream, grpc.ClientStream) {
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
			action: func(node *RawNode) (grpc.ClientStream, grpc.ClientStream) {
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
		node          *RawNode
		setup         func(node *RawNode)
		wantConnected bool
	}{
		{
			name:          "WithoutServer",
			node:          newNode(t, "127.0.0.1:5003"),
			wantConnected: false,
		},
		{
			name:          "WithLiveServer",
			node:          newNodeWithServer(t, 0),
			wantConnected: true,
		},
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
		})
	}
}

// TestChannelConcurrentSends tests sending multiple messages concurrently from multiple goroutines.
func TestChannelConcurrentSends(t *testing.T) {
	node := newNodeWithServer(t, 0)

	const numMessages = 1000
	const numGoroutines = 10
	msgsPerGoroutine := numMessages / (2 * numGoroutines)

	results := make(chan msgResponse, numMessages)
	for goID := range numGoroutines {
		go func() {
			send(t, results, node, goID, msgsPerGoroutine, request{opts: waitSendDone})
			send(t, results, node, goID, msgsPerGoroutine, request{opts: noSendWaiting})
		}()
	}

	var errs []error
	for range numMessages {
		res := <-results
		if res.resp.err != nil {
			errs = append(errs, res.resp.err)
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
			ctx, cancel := tt.contextSetup(t.Context())
			t.Cleanup(cancel)

			node := newNodeWithServer(t, tt.serverDelay)
			resp := sendRequest(t, node, request{ctx: ctx, opts: tt.callOpts}, uint64(i))
			if !errors.Is(resp.err, tt.wantErr) {
				t.Errorf("expected %v, got: %v", tt.wantErr, resp.err)
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
	sendRequest(t, node, request{opts: waitSendDone}, 1)

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
	node := newNodeWithServer(t, 0)
	if !node.channel.isConnected() {
		t.Error("node should be connected")
	}

	tests := []struct {
		name       string
		opts       callOptions
		streaming  bool
		afterSend  func(t *testing.T, node *RawNode, msgID uint64)
		wantRouter bool
	}{
		{name: "WaitSendDone/NonStreamingAutoCleanup", opts: waitSendDone, streaming: false, wantRouter: false},
		{name: "WaitSendDone/StreamingKeepsRouterAlive", opts: waitSendDone, streaming: true, wantRouter: true},
		{name: "NoSendWaiting/NonStreamingAutoCleanup", opts: noSendWaiting, streaming: false, wantRouter: false},
		{name: "NoSendWaiting/StreamingKeepsRouterAlive", opts: noSendWaiting, streaming: true, wantRouter: true},
	}
	for i, tt := range tests {
		name := fmt.Sprintf("msgID=%d/%s/streaming=%t", i, tt.name, tt.streaming)
		t.Run(name, func(t *testing.T) {
			msgID := uint64(i)
			resp := sendRequest(t, node, request{opts: tt.opts, streaming: tt.streaming}, msgID)
			if resp.err != nil {
				t.Errorf("unexpected error: %v", resp.err)
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
	node := newNodeWithServer(t, 0)

	const numMessages = 20
	results := make(chan msgResponse, numMessages)

	for i := range numMessages {
		go send(t, results, node, i, 1, request{})
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
