package stream

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultTestTimeout   = 3 * time.Second
	streamConnectTimeout = 3 * time.Second
)

// testChannel holds the channel and cleanup function.
type testChannel struct {
	*Channel
	srv *grpc.Server
	lis net.Listener
}

// echoServer serves as a generic server that echoes back any message.
func echoServer(stream Gorums_NodeStreamServer) error {
	for {
		in, err := stream.Recv()
		if err != nil {
			return err
		}
		// Echo back
		if err := stream.Send(in); err != nil {
			return err
		}
	}
}

// delayServer serves a server that delays each message by delay
func delayServer(delay time.Duration) func(stream Gorums_NodeStreamServer) error {
	return func(stream Gorums_NodeStreamServer) error {
		for {
			in, err := stream.Recv()
			if err != nil {
				return err
			}
			time.Sleep(delay)
			if err := stream.Send(in); err != nil {
				return err
			}
		}
	}
}

// A server that drops the stream after first message
func breakStreamServer(stream Gorums_NodeStreamServer) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	_ = stream.Send(msg)
	return errors.New("stream broken")
}

// holdServer hangs, effectively blocking the stream until context cancellation.
func holdServer(stream Gorums_NodeStreamServer) error {
	<-stream.Context().Done()
	return nil
}

// setupChannel creates a channel connected to a server.
func setupChannel(t testing.TB, serverFn func(Gorums_NodeStreamServer) error, opts ...grpc.ServerOption) *testChannel {
	t.Helper()

	// Start listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	// Start server
	srv := grpc.NewServer(opts...)
	if serverFn == nil {
		t.Fatal("setupChannel: serverFn must be provided; use echoServer for default behavior")
	}
	RegisterGorumsServer(srv, &mockServer{handler: serverFn})
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	// Create channel
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	c := NewChannel(t.Context(), conn, 1, 10)
	tc := &testChannel{
		Channel: c,
		srv:     srv,
		lis:     lis,
	}

	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("failed to close channel: %v", err)
		}
		srv.Stop()
		_ = conn.Close()
	})
	return tc
}

type mockServer struct {
	UnimplementedGorumsServer
	handler func(Gorums_NodeStreamServer) error
}

func (s *mockServer) NodeStream(srv Gorums_NodeStreamServer) error {
	return s.handler(srv)
}

// setupChannelWithoutServer creates a channel that tries to connect to a non-existent server.
func setupChannelWithoutServer(t testing.TB) *testChannel {
	t.Helper()
	// Pick a random port (hopefully unused) or loopback without listener
	conn, err := grpc.NewClient("127.0.0.1:54321", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := NewChannel(ctx, conn, 1, 10)
	t.Cleanup(func() {
		cancel()
		if err := c.Close(); err != nil {
			t.Errorf("failed to close channel: %v", err)
		}
	})
	return &testChannel{
		Channel: c,
	}
}

// waitForConnection polls until the node is connected or timeout expires.
// Returns true if connected, false if timeout expired.
func waitForConnection(c *Channel, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.isConnected() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return c.isConnected()
}

// waitForDisconnection polls until the channel is disconnected (stream is nil) or timeout expires.
// Returns true if disconnected, false if timeout expired.
func waitForDisconnection(c *Channel, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !c.isConnected() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !c.isConnected()
}

func sendRequest(t testing.TB, c *Channel, req Request, msgID uint64) response {
	t.Helper()
	if req.Ctx == nil {
		req.Ctx = context.Background()
	}
	reqMsg, err := NewMessage(req.Ctx, msgID, mock.TestMethod, nil)
	if err != nil {
		t.Fatalf("NewMessage failed: %v", err)
	}
	req.Msg = reqMsg
	replyChan := make(chan response, 1)
	req.ResponseChan = replyChan
	c.Enqueue(req)

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

func sendReq(t testing.TB, results chan<- msgResponse, c *Channel, goroutineID, msgsToSend int, req Request) {
	for j := range msgsToSend {
		msgID := uint64(goroutineID*1000 + j)
		resp := sendRequest(t, c, req, msgID)
		results <- msgResponse{msgID: msgID, resp: resp}
	}
}

func TestChannelCreation(t *testing.T) {
	tc := setupChannelWithoutServer(t)

	// send message when server is down
	resp := sendRequest(t, tc.Channel, Request{WaitSendDone: true}, 1)
	if resp.Err == nil {
		t.Error("response err: got <nil>, want error")
	}
}

func TestChannelShutdown(t *testing.T) {
	tc := setupChannel(t, echoServer)

	if !waitForConnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	// enqueue several messages to confirm normal operation
	const numMessages = 10
	var wg sync.WaitGroup
	for i := range numMessages {
		wg.Go(func() {
			resp := sendRequest(t, tc.Channel, Request{}, uint64(i))
			if resp.Err != nil {
				t.Errorf("unexpected error for message %d, got error: %v", i, resp.Err)
			}
		})
	}
	wg.Wait()

	// shut down the channel
	if err := tc.Close(); err != nil {
		t.Errorf("error closing channel: %v", err)
	}

	// try to send a message after closure
	resp := sendRequest(t, tc.Channel, Request{}, 999)
	if resp.Err == nil {
		t.Error("expected error when sending to closed channel")
	} else if !strings.Contains(resp.Err.Error(), "node closed") {
		t.Errorf("expected 'node closed' error, got: %v", resp.Err)
	}

	if tc.isConnected() {
		t.Error("channel should not be connected after close")
	}
}

func TestChannelLatency(t *testing.T) {
	const minDelay = 20 * time.Millisecond
	tc := setupChannel(t, delayServer(minDelay))

	// Initial latency should be -1
	if latency := tc.Latency(); latency != -1*time.Second {
		t.Errorf("Initial latency = %v, expected -1s", latency)
	}

	// Send a few requests to update latency
	for i := range 10 {
		sendRequest(t, tc.Channel, Request{WaitSendDone: false}, uint64(i))
	}

	latency := tc.Latency()
	if latency <= 0 {
		t.Errorf("Latency = %v, expected > 0", latency)
	}
	if latency < minDelay {
		t.Errorf("Latency = %v, expected >= %v (server delay)", latency, minDelay)
	}
}

func TestChannelSendCompletionWaiting(t *testing.T) {
	tc := setupChannel(t, echoServer)

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
			resp := sendRequest(t, tc.Channel, Request{WaitSendDone: tt.waitSendDone}, uint64(i))
			elapsed := time.Since(start)
			if resp.Err != nil {
				t.Errorf("unexpected error: %v", resp.Err)
			}
			t.Logf("response received in %v", elapsed)
		})
	}
}

func TestChannelErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) *testChannel
		wantErr string
	}{
		{
			name: "EnqueueWithoutServer",
			setup: func(t *testing.T) *testChannel {
				return setupChannelWithoutServer(t)
			},
			wantErr: "connection error",
		},
		{
			name: "EnqueueToClosedChannel",
			setup: func(t *testing.T) *testChannel {
				tc := setupChannelWithoutServer(t)
				if err := tc.Close(); err != nil {
					t.Errorf("failed to close channel: %v", err)
				}
				return tc
			},
			wantErr: "node closed",
		},
		{
			name: "ServerFailureDuringCommunication",
			setup: func(t *testing.T) *testChannel {
				tc := setupChannel(t, echoServer)
				// Send a message to ensure connection is established
				resp := sendRequest(t, tc.Channel, Request{WaitSendDone: true}, 1)
				if resp.Err != nil {
					t.Errorf("initial message send should succeed, got error: %v", resp.Err)
				}
				// Stop the server to simulate failure
				tc.srv.Stop()
				return tc
			},
			wantErr: "connection error",
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := tt.setup(t)
			time.Sleep(100 * time.Millisecond)

			resp := sendRequest(t, tc.Channel, Request{WaitSendDone: true}, uint64(i))
			if resp.Err == nil {
				t.Errorf("expected error containing %q but got nil", tt.wantErr)
			} else if !strings.Contains(resp.Err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, resp.Err)
			}
		})
	}
}

// TestChannelEnsureStream verifies that ensureStream correctly manages stream lifecycle.
func TestChannelEnsureStream(t *testing.T) {
	// Helper to prepare a fresh node with no stream
	newChannelWithoutStream := func(t testing.TB) *testChannel {
		tc := setupChannel(t, echoServer)
		// ensure sender and receiver goroutines are stopped
		tc.connCancel()
		// Extract grpc.ClientConn from existing channel
		conn := tc.conn
		// Create new channel with test context without metadata (real implementation captures metadata)
		tc.Channel = NewChannel(t.Context(), conn, tc.id, 10)
		return tc
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
		setup    func(t testing.TB) *testChannel
		action   func(tc *testChannel) (first, second grpc.ClientStream)
		wantSame bool
	}{
		{
			// Use setupChannelWithoutServer so the gRPC connection never reaches
			// connectivity.Ready, making ensureStream fail as expected.
			// newChannelWithoutStream reuses an already-Ready conn (from setupChannel),
			// so ensureStream would succeed there, which is wrong for this sub-case.
			name:  "UnconnectedNodeHasNoStream",
			setup: setupChannelWithoutServer,
			action: func(tc *testChannel) (grpc.ClientStream, grpc.ClientStream) {
				if err := tc.ensureStream(); err == nil {
					t.Error("ensureStream succeeded unexpectedly")
				}
				if tc.getStream() != nil {
					t.Error("stream should be nil")
				}
				return nil, nil
			},
		},
		{
			name:  "CreatesStreamWhenConnected",
			setup: newChannelWithoutStream,
			action: func(tc *testChannel) (grpc.ClientStream, grpc.ClientStream) {
				if err := tc.ensureStream(); err != nil {
					t.Errorf("ensureStream failed: %v", err)
				}
				return tc.getStream(), nil
			},
		},
		{
			name:  "RepeatedCallsReturnSameStream",
			setup: newChannelWithoutStream,
			action: func(tc *testChannel) (grpc.ClientStream, grpc.ClientStream) {
				if err := tc.ensureStream(); err != nil {
					t.Errorf("first ensureStream failed: %v", err)
				}
				first := tc.getStream()
				if err := tc.ensureStream(); err != nil {
					t.Errorf("second ensureStream failed: %v", err)
				}
				return first, tc.getStream()
			},
			wantSame: true,
		},
		{
			name:  "StreamDisconnectionCreatesNewStream",
			setup: newChannelWithoutStream,
			action: func(tc *testChannel) (grpc.ClientStream, grpc.ClientStream) {
				if err := tc.ensureStream(); err != nil {
					t.Errorf("initial ensureStream failed: %v", err)
				}
				first := tc.getStream()
				tc.clearStream(first)
				if err := tc.ensureStream(); err != nil {
					t.Errorf("ensureStream after disconnect failed: %v", err)
				}
				return first, tc.getStream()
			},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := tt.setup(t)
			first, second := tt.action(tc)
			cmpStream(t, first, second, tt.wantSame)
		})
	}
}

func TestChannelEnsureStreamAfterBroken(t *testing.T) {
	tc := setupChannel(t, echoServer)

	// Ensure we have a stream
	if err := tc.ensureStream(); err != nil {
		t.Fatalf("ensureStream failed: %v", err)
	}

	// Break the stream
	tc.clearStream(tc.getStream())

	// Ensure we can get it back
	if err := tc.ensureStream(); err != nil {
		t.Fatalf("ensureStream failed after clear: %v", err)
	}
}

// TestChannelConnectionState verifies connection state detection and behavior.
func TestChannelConnectionState(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) *testChannel
		wantConnected bool
	}{
		{
			name:          "WithoutServer",
			setup:         func(t *testing.T) *testChannel { return setupChannelWithoutServer(t) },
			wantConnected: false,
		},
		{
			name:          "WithLiveServer",
			setup:         func(t *testing.T) *testChannel { return setupChannel(t, echoServer) },
			wantConnected: true,
		},
		{
			name: "RequiresBothReadyAndStream",
			setup: func(t *testing.T) *testChannel {
				tc := setupChannel(t, echoServer)
				// Wait for stream to be established
				if !waitForConnection(tc.Channel, streamConnectTimeout) {
					t.Fatal("node should be connected before clearing stream")
				}
				tc.clearStream(tc.getStream())
				return tc
			},
			wantConnected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := tt.setup(t)
			if tt.wantConnected {
				// For tests expecting connection, poll until connected or timeout
				if !waitForConnection(tc.Channel, streamConnectTimeout) {
					t.Errorf("isConnected() = false, want true")
				}
			} else {
				// For tests expecting no connection, verify immediately
				if tc.isConnected() {
					t.Errorf("isConnected() = true, want false")
				}
			}
		})
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
		// Very short timeout to cancel during SendMsg operation.
		// Note: SendMsg itself is fast, but we're testing the cancellation path.
		ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		// Let context expire before we send
		time.Sleep(5 * time.Millisecond)
		return ctx, cancel
	}

	tests := []struct {
		name         string
		serverFn     func(Gorums_NodeStreamServer) error
		contextSetup func(context.Context) (context.Context, context.CancelFunc)
		waitSendDone bool
		wantErr      error
	}{
		{
			name:         "CancelBeforeSend/WaitSending",
			serverFn:     echoServer,
			contextSetup: cancelledContext,
			waitSendDone: true,
			wantErr:      context.Canceled,
		},
		{
			name:         "CancelBeforeSend/NoSendWaiting",
			serverFn:     echoServer,
			contextSetup: cancelledContext,
			waitSendDone: false,
			wantErr:      context.Canceled,
		},
		{
			name:         "CancelDuringSend/WaitSending",
			serverFn:     holdServer,
			contextSetup: expireBeforeSend,
			waitSendDone: true,
			wantErr:      context.DeadlineExceeded,
		},
		{
			name:         "CancelDuringSend/NoSendWaiting",
			serverFn:     holdServer,
			contextSetup: expireBeforeSend,
			waitSendDone: false,
			wantErr:      context.DeadlineExceeded,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.contextSetup(t.Context())
			t.Cleanup(cancel)

			tc := setupChannel(t, tt.serverFn)
			resp := sendRequest(t, tc.Channel, Request{Ctx: ctx, WaitSendDone: tt.waitSendDone}, uint64(i))
			if !errors.Is(resp.Err, tt.wantErr) {
				t.Errorf("expected %v, got: %v", tt.wantErr, resp.Err)
			}
		})
	}
}

// TestChannelStreamReadySignaling verifies that the receiver goroutine is properly notified
// when a stream becomes available.
func TestChannelStreamReadySignaling(t *testing.T) {
	tc := setupChannel(t, echoServer)

	start := time.Now()
	resp := sendRequest(t, tc.Channel, Request{}, 1)
	firstLatency := time.Since(start)

	if resp.Err != nil {
		t.Fatalf("unexpected error on first request: %v", resp.Err)
	}

	start = time.Now()
	resp = sendRequest(t, tc.Channel, Request{}, 2)
	secondLatency := time.Since(start)

	if resp.Err != nil {
		t.Fatalf("unexpected error on second request: %v", resp.Err)
	}

	t.Logf("first request latency: %v", firstLatency)
	t.Logf("second request latency: %v", secondLatency)

	const maxAcceptableLatency = 100 * time.Millisecond
	if firstLatency > maxAcceptableLatency {
		t.Errorf("first request took %v, expected < %v", firstLatency, maxAcceptableLatency)
	}
}

// TestChannelStreamReadyAfterReconnect verifies that the receiver is properly notified
// when a stream is re-established after being cleared (simulating reconnection).
// The server drops the stream after the first message, forcing the channel to reconnect.
// The second request must succeed, proving the full reconnect path works end-to-end.
func TestChannelStreamReadyAfterReconnect(t *testing.T) {
	tc := setupChannel(t, breakStreamServer)

	// First request succeeds; the server then drops the stream.
	resp := sendRequest(t, tc.Channel, Request{}, 1)
	if resp.Err != nil {
		t.Fatalf("unexpected error on initial request: %v", resp.Err)
	}

	// Wait for the receiver to detect the server-side disconnect so the channel
	// is in a clean disconnected state before the next request triggers reconnection.
	if !waitForDisconnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel should be disconnected after server drop")
	}

	// Second request: the sender re-establishes the stream and the receiver
	// picks up the streamReady signal to route the response.
	resp = sendRequest(t, tc.Channel, Request{}, 2)
	if resp.Err != nil {
		t.Fatalf("unexpected error after reconnect: %v", resp.Err)
	}
}

// TestChannelConcurrentStreamReconnect verifies correct handling of concurrent
// requests during stream reconnection. The server breaks the stream after echoing
// the first message, then the test fires multiple concurrent requests without
// waiting for the channel to detect the disconnect.
//
// This validates the channel's stream lifecycle management ensures that:
// - Requests are only tracked in the response router when sent on the current stream
// - The clearStream stale-check prevents cancelling a new stream's context
// - Concurrent requests sent during reconnection succeed without spurious errors
func TestChannelConcurrentStreamReconnect(t *testing.T) {
	tc := setupChannel(t, breakStreamServer)
	if !waitForConnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	// Request 1 causes the server to echo and immediately break the stream.
	resp := sendRequest(t, tc.Channel, Request{}, 1)
	if resp.Err != nil {
		t.Fatalf("unexpected error on initial request: %v", resp.Err)
	}

	// Fire concurrent requests without waiting for the channel to notice the
	// disconnect. These requests race with the teardown of the broken stream,
	// validating that the channel correctly routes them to the new stream
	// without spurious cancellation.
	const concurrency = 10
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for i := range concurrency {
		wg.Go(func() {
			resp := sendRequest(t, tc.Channel, Request{}, uint64(i+2))
			errs[i] = resp.Err
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("request %d: unexpected error after reconnect: %v", i+2, err)
		}
	}
}

func TestChannelRouterLifecycle(t *testing.T) {
	tc := setupChannel(t, echoServer)

	if !waitForConnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	tests := []struct {
		name         string
		waitSendDone bool
		streaming    bool
		wantRouter   bool
		wantPanic    bool
	}{
		{name: "WaitSendDone/NoStreaming/Cleanup", waitSendDone: true, streaming: false, wantRouter: false},
		{name: "WaitSendDone/Streaming/Invalid", waitSendDone: true, streaming: true, wantPanic: true},
		{name: "NoSendWaiting/NoStreaming/Cleanup", waitSendDone: false, streaming: false, wantRouter: false},
		{name: "NoSendWaiting/Streaming/KeepsRouterAlive", waitSendDone: false, streaming: true, wantRouter: true},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tt.wantPanic {
						t.Errorf("unexpected panic: %v", r)
					}
				}
			}()
			msgID := uint64(i)
			resp := sendRequest(t, tc.Channel, Request{WaitSendDone: tt.waitSendDone, Streaming: tt.streaming}, msgID)
			if resp.Err != nil {
				t.Errorf("unexpected error: %v", resp.Err)
			}
			if exists := routerExists(tc.Channel, msgID); exists != tt.wantRouter {
				t.Errorf("router exists = %v, want %v", exists, tt.wantRouter)
			}
			deleteRouter(tc.Channel, msgID)
		})
	}
}

// Helper functions for testing channel response routing and router lifecycle

func routerExists(c *Channel, msgID uint64) bool {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	_, exists := c.responseRouters[msgID]
	return exists
}

func deleteRouter(c *Channel, msgID uint64) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	delete(c.responseRouters, msgID)
}

func TestChannelResponseRouting(t *testing.T) {
	tc := setupChannel(t, echoServer)

	const numMessages = 20
	results := make(chan msgResponse, numMessages)

	for i := range numMessages {
		go sendReq(t, results, tc.Channel, i, 1, Request{WaitSendDone: true})
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

	if len(received) != numMessages {
		t.Errorf("got %d unique responses, want %d", len(received), numMessages)
	}
}

func TestChannelConcurrentSends(t *testing.T) {
	tc := setupChannel(t, echoServer)

	const numMessages = 1000
	const numGoroutines = 10
	msgsPerGoroutine := numMessages / (2 * numGoroutines)

	results := make(chan msgResponse, numMessages)
	for goID := range numGoroutines {
		go func() {
			sendReq(t, results, tc.Channel, goID, msgsPerGoroutine, Request{WaitSendDone: true})
			sendReq(t, results, tc.Channel, goID, msgsPerGoroutine, Request{WaitSendDone: false})
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
	if !tc.isConnected() {
		t.Error("channel should still be connected after concurrent sends")
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
	tc := setupChannel(t, breakStreamServer)

	if !waitForConnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	// Send message to activate stream
	sendRequest(t, tc.Channel, Request{WaitSendDone: true}, 1)

	// Break the stream, forcing a reconnection on next send
	tc.clearStream(tc.getStream())
	time.Sleep(20 * time.Millisecond)

	// Send multiple messages concurrently when stream is broken with the
	// goal to trigger a deadlock between sender and receiver goroutines.
	doneChan := make(chan bool, 10)
	for id := range 10 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			reqMsg, _ := NewMessage(ctx, uint64(100+id), mock.TestMethod, nil)
			req := Request{Ctx: ctx, Msg: reqMsg}

			select {
			case tc.sendQ <- req:
				doneChan <- true
			case <-ctx.Done():
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
			t.Fatalf("DEADLOCK: Only %d/10 goroutines completed", completed)
		}
	}
	// If we reach here, all 10 goroutines completed (but some may have failed to enqueue)
	if successful < 10 {
		t.Fatalf("DEADLOCK: %d/10 goroutines timed out", 10-successful)
	}
}

// TestChannelClearStreamDeadlock verifies that clearStream followed by requeuePendingMsgs
// does not deadlock when sendQ is full and responseRouters contains pending non-streaming requests.
//
// Deadlock scenario (original code, where clearStream called requeuePendingMsgs internally):
//  1. clearStream acquires streamMut and calls requeuePendingMsgs.
//  2. requeuePendingMsgs enqueues the first pending request; sender dequeues it and
//     immediately blocks in ensureStream waiting for streamMut.
//  3. requeuePendingMsgs fills sendQ to capacity with the remaining requests.
//  4. requeuePendingMsgs blocks in Enqueue on one final request (sendQ is full).
//  5. Neither goroutine can proceed: receiver holds streamMut while blocked in Enqueue,
//     and sender waits for streamMut in ensureStream — deadlock.
//
// The fix moves requeuePendingMsgs out of clearStream so streamMut is never held
// across the Enqueue calls.
func TestChannelClearStreamDeadlock(t *testing.T) {
	// Use a very small sendQ (capacity 2) so the deadlock is triggered with only
	// sendBufSize+2 = 4 injected pending requests.
	const sendBufSize = 2

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	RegisterGorumsServer(srv, &mockServer{handler: holdServer})
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	c := NewChannel(t.Context(), conn, 1, sendBufSize)
	t.Cleanup(func() {
		if closeErr := c.Close(); closeErr != nil {
			t.Errorf("failed to close channel: %v", closeErr)
		}
		_ = conn.Close()
	})

	if !waitForConnection(c, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}
	staleStream := c.getStream()

	// Inject sendBufSize+2 non-streaming requests directly into responseRouters,
	// bypassing the normal sendMsg path. requeuePendingMsgs will attempt to
	// re-enqueue all of them. With sendBufSize=2:
	//  - request 1 is enqueued; sender dequeues it and blocks in ensureStream.
	//  - requests 2-3 fill sendQ to capacity.
	//  - request 4's Enqueue call blocks on a full sendQ while clearStream
	//    still holds streamMut — deadlock.
	const numPending = sendBufSize + 2
	replyChannels := make([]chan response, numPending)
	c.responseMut.Lock()
	for i := range numPending {
		replyChannels[i] = make(chan response, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		msg, msgErr := NewMessage(ctx, uint64(1000+i), mock.TestMethod, nil)
		if msgErr != nil {
			c.responseMut.Unlock()
			t.Fatalf("NewMessage failed: %v", msgErr)
		}
		c.responseRouters[uint64(1000+i)] = Request{
			Ctx:          ctx,
			Msg:          msg,
			Streaming:    false,
			WaitSendDone: false,
			ResponseChan: replyChannels[i],
		}
	}
	c.responseMut.Unlock()

	// clearStream + requeuePendingMsgs (as the real call sites do) should complete
	// without deadlocking.
	done := make(chan struct{})
	go func() {
		c.clearStream(staleStream)
		c.requeuePendingMsgs()
		close(done)
	}()

	select {
	case <-done:
		// No deadlock.
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK: clearStream+requeuePendingMsgs did not return within 2s")
	}
}

// BenchmarkChannelStreamReadyFirstRequest measures the latency of the first request,
// which includes stream creation and the stream-ready signaling.
//
// This benchmark creates a new server and node per iteration to measure true
// "cold start" latency. Due to TCP port exhaustion on macOS (ephemeral ports
// enter TIME_WAIT state and take time to recycle), this benchmark should be
// run with limited iterations (e.g., -benchtime=100x).
//
// Note: This benchmark includes server setup overhead, so absolute numbers
// should be interpreted with caution. The goal is to detect regressions.
func BenchmarkChannelStreamReadyFirstRequest(b *testing.B) {
	if b.N > 500 {
		b.Skip("Skipping to avoid port exhaustion; use -benchtime=100x")
	}

	for b.Loop() {
		tc := setupChannel(b, echoServer)

		// Use a fresh context for the benchmark request
		ctx, cancel := context.WithTimeout(b.Context(), defaultTestTimeout)
		defer cancel()
		reqMsg, _ := NewMessage(ctx, 1, mock.TestMethod, nil)
		req := Request{Ctx: ctx, Msg: reqMsg}
		replyChan := make(chan response, 1)
		req.ResponseChan = replyChan
		tc.Enqueue(req)

		select {
		case resp := <-replyChan:
			if resp.Err != nil {
				b.Logf("request error (may occur during rapid cycles): %v", resp.Err)
			}
		case <-ctx.Done():
			b.Logf("timeout (may occur during rapid cycles)")
		}

		// Close the node before stopping the server to ensure clean shutdown
		_ = tc.Close()
		tc.srv.Stop()
	}
}

// BenchmarkChannelStreamReadyReconnect measures the latency of reconnecting
// after the stream has been cleared.
// Note: This benchmark has inherent variability due to the race between
// clearStream and the sender's ensureStream call.
func BenchmarkChannelStreamReadyReconnect(b *testing.B) {
	tc := setupChannel(b, echoServer)

	// Establish initial stream with a fresh context
	ctx := context.Background()
	reqMsg, _ := NewMessage(ctx, 0, mock.TestMethod, nil)
	req := Request{Ctx: ctx, Msg: reqMsg}
	replyChan := make(chan response, 1)
	req.ResponseChan = replyChan
	tc.Enqueue(req)

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
		tc.clearStream(tc.getStream())

		// Wait a tiny bit for the receiver to notice the stream is gone
		// and be ready for the signal. This simulates real-world behavior
		// where the receiver detects the error before reconnection.
		time.Sleep(100 * time.Microsecond)

		// Now send a request which will trigger ensureStream -> newNodeStream -> signal
		ctx := context.Background()
		reqMsg, _ := NewMessage(ctx, uint64(i+1), mock.TestMethod, nil)
		req := Request{Ctx: ctx, Msg: reqMsg}
		replyChan := make(chan response, 1)
		req.ResponseChan = replyChan
		tc.Enqueue(req)

		select {
		case <-replyChan:
			// stream down errors are sometimes expected here due to a race between
			// clearStream and ensureStream; we ignore errors in benchmarks.
		case <-time.After(500 * time.Millisecond):
			b.Fatalf("timeout on request %d", i)
		}
	}
}

func BenchmarkChannelSend(b *testing.B) {
	tc := setupChannel(b, echoServer)

	tests := []struct {
		name string
		size int // payload size in bytes
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			payload := make([]byte, tt.size)
			b.ResetTimer()
			for i := range b.N {
				// Optimization: reuse chan if we know it's 1-buffered and read.
				replyChan := make(chan response, 1)
				msg := Message_builder{
					MessageSeqNo: uint64(i),
					Method:       mock.TestMethod,
					Payload:      payload,
				}.Build()
				req := Request{Ctx: context.Background(), Msg: msg, WaitSendDone: true, ResponseChan: replyChan}
				tc.Enqueue(req)
				<-replyChan
			}
		})
	}
}

var msgID atomic.Uint64

func BenchmarkChannelSendParallel(b *testing.B) {
	tc := setupChannel(b, echoServer)

	tests := []struct {
		name string
		size int
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			payload := make([]byte, tt.size)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				replyChan := make(chan response, 1)
				for pb.Next() {
					id := msgID.Add(1)
					msg := Message_builder{
						MessageSeqNo: id,
						Method:       mock.TestMethod,
						Payload:      payload,
					}.Build()
					req := Request{Ctx: context.Background(), Msg: msg, WaitSendDone: true, ResponseChan: replyChan}
					tc.Enqueue(req)
					<-replyChan
				}
			})
		})
	}
}
