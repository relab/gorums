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
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultTestTimeout   = 3 * time.Second
	streamConnectTimeout = 3 * time.Second
)

// isConnected returns true if the channel has an active stream.
// For outbound channels, also requires the gRPC connection to be in Ready state.
// This method is safe for concurrent use. It is only used by tests.
func (c *Channel) isConnected() bool {
	if c.IsInbound() {
		return c.connCtx.Err() == nil && c.getStream() != nil
	}
	return c.conn.GetState() == connectivity.Ready && c.getStream() != nil
}

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

// rejectFirstStreamServer rejects the first stream it accepts and echoes on
// every stream after that, so a channel can record a failure and then recover
// from it on a later stream.
func rejectFirstStreamServer() func(Gorums_NodeStreamServer) error {
	var streams atomic.Int32
	return func(stream Gorums_NodeStreamServer) error {
		if streams.Add(1) == 1 {
			return errors.New("first stream rejected")
		}
		return echoServer(stream)
	}
}

// waitForLastErr polls until the channel's LastErr matches want (nil or
// non-nil) or the timeout expires, and reports what it observed.
func waitForLastErr(t testing.TB, c *Channel, wantErr bool, what string) {
	t.Helper()
	deadline := time.Now().Add(defaultTestTimeout)
	for time.Now().Before(deadline) {
		if (c.LastErr() != nil) == wantErr {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s: LastErr = %v", what, c.LastErr())
}

// setupChannel creates a channel connected to a server.
func setupChannel(t testing.TB, serverFn func(Gorums_NodeStreamServer) error, opts ...grpc.ServerOption) *testChannel {
	t.Helper()
	return setupChannelEager(t, false, serverFn, opts...)
}

// setupChannelEager is [setupChannel] with control over the channel's eager
// stream reconnection (see [NewOutboundChannel]).
func setupChannelEager(t testing.TB, eagerReconnect bool, serverFn func(Gorums_NodeStreamServer) error, opts ...grpc.ServerOption) *testChannel {
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
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	// Create channel
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	c := NewOutboundChannel(t.Context(), 1, 10, conn, NewMessageRouter(), eagerReconnect, nil)
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

// newUnavailableClientConn creates a client connection whose dialer always
// fails, so the connection cannot become ready.
func newUnavailableClientConn(t testing.TB) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(
		"passthrough:///unavailable",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return nil, errors.New("test connection unavailable")
		}),
	)
	if err != nil {
		t.Fatalf("failed to create unavailable client connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// setupChannelWithoutServer creates a channel whose connection cannot reach a server.
func setupChannelWithoutServer(t testing.TB) *testChannel {
	t.Helper()
	conn := newUnavailableClientConn(t)
	ctx, cancel := context.WithCancel(context.Background())
	c := NewOutboundChannel(ctx, 1, 10, conn, NewMessageRouter(), false, nil)
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
	resp := sendRequest(t, tc.Channel, Request{Oneway: true}, 1)
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
	} else if !errors.Is(resp.Err, ErrNodeClosed) {
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
	if latency := tc.router.Latency(); latency != -1*time.Second {
		t.Errorf("Initial latency = %v, expected -1s", latency)
	}

	// Send a few requests to update latency
	for i := range 10 {
		sendRequest(t, tc.Channel, Request{Oneway: false}, uint64(i))
	}

	latency := tc.router.Latency()
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
		name   string
		oneway bool
	}{
		{name: "Oneway", oneway: true},
		{name: "Twoway", oneway: false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			resp := sendRequest(t, tc.Channel, Request{Oneway: tt.oneway}, uint64(i))
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
				resp := sendRequest(t, tc.Channel, Request{Oneway: true}, 1)
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

			resp := sendRequest(t, tc.Channel, Request{Oneway: true}, uint64(i))
			if resp.Err == nil {
				t.Errorf("expected error containing %q but got nil", tt.wantErr)
			} else if !strings.Contains(resp.Err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, resp.Err)
			}
		})
	}
}

// TestChannelStreamFailureRecordsLastErr verifies that a request the sender
// cannot deliver because no stream could be established leaves the reason in
// LastErr. LastErr reports node health, so it records the failure whether or
// not the request itself had somewhere to report the error: here a reply with
// no response channel, the one request shape that reaches the sender without
// one.
func TestChannelStreamFailureRecordsLastErr(t *testing.T) {
	tc := setupChannelWithoutServer(t)
	if err := tc.LastErr(); err != nil {
		t.Fatalf("LastErr = %v, want nil before the first request", err)
	}

	msg, err := NewMessage(context.Background(), 1, mock.TestMethod, nil)
	if err != nil {
		t.Fatalf("NewMessage failed: %v", err)
	}
	tc.Enqueue(Request{Ctx: context.Background(), Msg: msg})

	deadline := time.Now().Add(defaultTestTimeout)
	for tc.LastErr() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	err = tc.LastErr()
	if err == nil {
		t.Fatal("LastErr = nil, want the stream creation error for the undelivered request")
	}
	if !strings.Contains(err.Error(), "connection error") {
		t.Errorf("LastErr = %v, want an error containing %q", err, "connection error")
	}
}

// TestChannelLastErrClearsOnRecovery verifies that LastErr reports current
// health rather than history: a channel whose stream failed once reports the
// failure, and reports nil again once traffic flows over a new stream. Without
// clearing, a node with one transient failure would look permanently unhealthy
// and sort behind a node that is down right now.
func TestChannelLastErrClearsOnRecovery(t *testing.T) {
	tc := setupChannel(t, rejectFirstStreamServer())

	// The channel's eager connect creates the stream the server rejects.
	waitForLastErr(t, tc.Channel, true, "the rejected stream to be recorded")

	// A completed round trip over the replacement stream proves the channel
	// usable again.
	if resp := sendRequest(t, tc.Channel, Request{}, 1); resp.Err != nil {
		t.Fatalf("sendRequest after recovery: %v", resp.Err)
	}
	waitForLastErr(t, tc.Channel, false, "LastErr to clear after recovery")
}

// TestChannelReceiverRecordsStreamFailure verifies that an eager-reconnect
// receiver records its own failed stream creations. The sender records one
// only when it has a request to send, so an idle node would otherwise report
// no error however long it stayed unreachable.
func TestChannelReceiverRecordsStreamFailure(t *testing.T) {
	conn := newUnavailableClientConn(t)
	ctx, cancel := context.WithCancel(context.Background())
	c := NewOutboundChannel(ctx, 1, 10, conn, NewMessageRouter(), true, nil)
	t.Cleanup(func() {
		cancel()
		if err := c.Close(); err != nil {
			t.Errorf("failed to close channel: %v", err)
		}
	})

	// No request is ever enqueued: only the receiver's redial loop runs.
	waitForLastErr(t, c, true, "the receiver's failed redial to be recorded")
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
		tc.Channel = NewOutboundChannel(t.Context(), tc.id, 10, conn, NewMessageRouter(), false, nil)
		return tc
	}

	// Helper to verify stream expectations
	cmpStream := func(t *testing.T, first, second BidiStream, wantSame bool) {
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
		action   func(tc *testChannel) (first, second BidiStream)
		wantSame bool
	}{
		{
			// Use setupChannelWithoutServer so the gRPC connection never reaches
			// connectivity.Ready, making ensureStream fail as expected.
			// newChannelWithoutStream reuses an already-Ready conn (from setupChannel),
			// so ensureStream would succeed there, which is wrong for this sub-case.
			name:  "UnconnectedNodeHasNoStream",
			setup: setupChannelWithoutServer,
			action: func(tc *testChannel) (BidiStream, BidiStream) {
				if _, err := tc.ensureStream(); err == nil {
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
			action: func(tc *testChannel) (BidiStream, BidiStream) {
				if _, err := tc.ensureStream(); err != nil {
					t.Errorf("ensureStream failed: %v", err)
				}
				return tc.getStream(), nil
			},
		},
		{
			name:  "RepeatedCallsReturnSameStream",
			setup: newChannelWithoutStream,
			action: func(tc *testChannel) (BidiStream, BidiStream) {
				if _, err := tc.ensureStream(); err != nil {
					t.Errorf("first ensureStream failed: %v", err)
				}
				first := tc.getStream()
				if _, err := tc.ensureStream(); err != nil {
					t.Errorf("second ensureStream failed: %v", err)
				}
				return first, tc.getStream()
			},
			wantSame: true,
		},
		{
			name:  "StreamDisconnectionCreatesNewStream",
			setup: newChannelWithoutStream,
			action: func(tc *testChannel) (BidiStream, BidiStream) {
				if _, err := tc.ensureStream(); err != nil {
					t.Errorf("initial ensureStream failed: %v", err)
				}
				first := tc.getStream()
				tc.clearStream(first)
				if _, err := tc.ensureStream(); err != nil {
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
	if _, err := tc.ensureStream(); err != nil {
		t.Fatalf("ensureStream failed: %v", err)
	}

	// Break the stream
	tc.clearStream(tc.getStream())

	// Ensure we can get it back
	if _, err := tc.ensureStream(); err != nil {
		t.Fatalf("ensureStream failed after clear: %v", err)
	}
}

// TestChannelEnsureConnectedNodeStreamCancelsAbandonedStream verifies that
// ensureConnectedNodeStream cancels a stream left behind by a previous
// attempt before replacing it with a new one, instead of orphaning it.
//
// Before the fix, when the guard (conn Ready && stream != nil) was false but
// a streamCancel from an earlier attempt was still referenced,
// ensureConnectedNodeStream silently overwrote c.stream and c.streamCancel
// without invoking the previous streamCancel. The abandoned stream then
// stayed alive server-side, and any requests still in flight on it were
// orphaned.
//
// The channel is built directly (bypassing NewOutboundChannel) so no sender
// goroutine runs concurrently and races the manually injected "previous
// attempt" state; ensureConnectedNodeStream is exercised as a plain method
// call, matching how newChannelWithoutStream isolates state in
// TestChannelEnsureStream above.
func TestChannelEnsureConnectedNodeStreamCancelsAbandonedStream(t *testing.T) {
	conn := newUnavailableClientConn(t)
	if state := conn.GetState(); state == connectivity.Ready {
		t.Fatalf("conn state = %v, want anything but Ready", state)
	}

	connCtx, connCancel := context.WithCancel(context.Background())
	t.Cleanup(connCancel)
	c := &Channel{conn: conn, connCtx: connCtx, connCancel: connCancel}

	// Simulate a stream left behind by a previous ensureConnectedNodeStream
	// attempt: a live streamCtx/streamCancel pair and a non-nil stream.
	oldCtx, oldCancel := context.WithCancel(connCtx)
	c.streamCtx, c.streamCancel = oldCtx, oldCancel
	c.stream = newMockBidiStream()
	if oldCtx.Err() != nil {
		t.Fatal("old stream context should not be cancelled yet")
	}

	// conn is not Ready, so the guard is false and ensureConnectedNodeStream
	// takes the replace-stream path.
	_, _ = c.ensureConnectedNodeStream()

	if oldCtx.Err() == nil {
		t.Error("ensureConnectedNodeStream did not cancel the abandoned stream's context before replacing it")
	}
}

func TestChannelCloseCancelsOnlyOwnedPendingRequests(t *testing.T) {
	router := NewMessageRouter()
	oldStream := newMockBidiStream()
	newStream := newMockBidiStream()
	t.Cleanup(oldStream.close)
	t.Cleanup(newStream.close)
	oldChannel := NewInboundChannel(t.Context(), 1, 1, oldStream, router)
	newChannel := NewInboundChannel(t.Context(), 1, 1, newStream, router)
	t.Cleanup(func() { _ = oldChannel.Close() })
	t.Cleanup(func() { _ = newChannel.Close() })

	oldReply := make(chan response, 1)
	newReply := make(chan response, 1)
	oldMessage := Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build()
	newMessage := Message_builder{MessageSeqNo: ServerSequenceNumber(2), Method: mock.TestMethod}.Build()
	oldChannel.Enqueue(Request{Ctx: t.Context(), Msg: oldMessage, ResponseChan: oldReply})
	newChannel.Enqueue(Request{Ctx: t.Context(), Msg: newMessage, ResponseChan: newReply})

	deadline := time.Now().Add(time.Second)
	for router.PendingCount() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := router.PendingCount(); got != 2 {
		t.Fatalf("pending count = %d, want 2", got)
	}

	if err := oldChannel.Close(); err != nil {
		t.Fatalf("old channel Close: %v", err)
	}
	select {
	case got := <-oldReply:
		if !errors.Is(got.Err, ErrNodeClosed) {
			t.Fatalf("old request error = %v, want ErrNodeClosed", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("old request was not cancelled")
	}
	select {
	case got := <-newReply:
		t.Fatalf("new request was cancelled by old channel: %v", got.Err)
	default:
	}

	newID := newMessage.GetMessageSeqNo()
	if !router.deliverPending(newID, response{NodeID: 1, Value: newMessage}) {
		t.Fatal("new request was removed from router")
	}
	select {
	case got := <-newReply:
		if got.Err != nil {
			t.Fatalf("new request response error = %v", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("new request did not receive routed response")
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
		oneway       bool
		wantErr      error
	}{
		{
			name:         "CancelBeforeSend/WaitSending",
			serverFn:     echoServer,
			contextSetup: cancelledContext,
			oneway:       true,
			wantErr:      context.Canceled,
		},
		{
			name:         "CancelBeforeSend/NoSendWaiting",
			serverFn:     echoServer,
			contextSetup: cancelledContext,
			oneway:       false,
			wantErr:      context.Canceled,
		},
		{
			name:         "CancelDuringSend/WaitSending",
			serverFn:     holdServer,
			contextSetup: expireBeforeSend,
			oneway:       true,
			wantErr:      context.DeadlineExceeded,
		},
		{
			name:         "CancelDuringSend/NoSendWaiting",
			serverFn:     holdServer,
			contextSetup: expireBeforeSend,
			oneway:       false,
			wantErr:      context.DeadlineExceeded,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.contextSetup(t.Context())
			t.Cleanup(cancel)

			tc := setupChannel(t, tt.serverFn)
			resp := sendRequest(t, tc.Channel, Request{Ctx: ctx, Oneway: tt.oneway}, uint64(i))
			if !errors.Is(resp.Err, tt.wantErr) {
				t.Errorf("expected %v, got: %v", tt.wantErr, resp.Err)
			}
		})
	}
}

// blockingSendStream blocks every Send until release() is called and blocks
// Recv until the stream is closed. It keeps the channel's sender goroutine
// occupied mid-send so the send queue backs up, simulating a peer that has
// stopped reading (exhausted flow-control windows). Each Send announces its
// message ID on entered when it starts and on sends when it completes, so
// tests can deterministically wait for the sender to be occupied and assert
// FIFO delivery order.
type blockingSendStream struct {
	released chan struct{}
	closed   chan struct{}
	entered  chan uint64
	sends    chan uint64
}

func newBlockingSendStream() *blockingSendStream {
	return &blockingSendStream{
		released: make(chan struct{}),
		closed:   make(chan struct{}),
		entered:  make(chan uint64, 16),
		sends:    make(chan uint64, 16),
	}
}

func (s *blockingSendStream) Send(msg *Message) error {
	s.entered <- msg.GetMessageSeqNo()
	select {
	case <-s.released:
		s.sends <- msg.GetMessageSeqNo()
		return nil
	case <-s.closed:
		return context.Canceled
	}
}

func (s *blockingSendStream) Recv() (*Message, error) {
	<-s.closed
	return nil, context.Canceled
}

func (s *blockingSendStream) release() { close(s.released) }
func (s *blockingSendStream) close()   { close(s.closed) }

// waitID waits for an ID on ch (a blockingSendStream signal channel) and
// fails the test if it does not match want or does not arrive in time.
func waitID(t *testing.T, ch <-chan uint64, want uint64, what string) {
	t.Helper()
	select {
	case id := <-ch:
		if id != want {
			t.Fatalf("%s: message ID = %d, want %d", what, id, want)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatalf("%s: timed out waiting for message %d", what, want)
	}
}

// TestChannelEnqueueRespectsRequestContext verifies that a caller blocked in
// Enqueue on a full send queue is released when its own request context is
// cancelled. Before the fix, Enqueue only watched the channel's connCtx, so a
// worker stuck behind a peer that stopped reading could not be unblocked even
// by a per-call deadline, observed in cluster benchmarks as nodes stalling at
// near-zero throughput for the rest of a run.
func TestChannelEnqueueRespectsRequestContext(t *testing.T) {
	stream := newBlockingSendStream()
	// Capacity 0: the queue has no slack, so a second request blocks in
	// Enqueue as soon as the sender goroutine is occupied in Send.
	c := NewInboundChannel(t.Context(), 1, 0, stream, NewMessageRouter())
	t.Cleanup(func() {
		stream.close()
		_ = c.Close()
	})

	// Occupy the sender: the first request is handed off directly to the
	// sender goroutine, whose Send then blocks on the stream.
	c.Enqueue(Request{
		Ctx:    context.Background(),
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
	})

	// The second request cannot be handed off; its Enqueue must block until
	// the request's own context is cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	reply := make(chan response, 1)
	enqueueReturned := make(chan struct{})
	go func() {
		defer close(enqueueReturned)
		c.Enqueue(Request{
			Ctx:          ctx,
			Oneway:       true,
			ResponseChan: reply,
			Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
		})
	}()

	// Let the goroutine reach the blocking Enqueue before cancelling.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case resp := <-reply:
		if !errors.Is(resp.Err, context.Canceled) {
			t.Errorf("blocked Enqueue reply error = %v, want context.Canceled", resp.Err)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("Enqueue ignored request context cancellation; caller is stuck")
	}
	select {
	case <-enqueueReturned:
	case <-time.After(defaultTestTimeout):
		t.Fatal("Enqueue did not return after request context cancellation")
	}
}

// TestChannelEnqueueTwoWayFailsFastWhenFull verifies that a two-way request
// (one with a waiting local caller) is failed with ErrSendQueueFull instead of
// blocking when the peer's send queue is at capacity, and that requests
// accepted into the queue are still delivered in FIFO order. Quorum calls
// tolerate per-node errors by design, so failing fast lets a call complete
// via the remaining peers instead of stalling the caller behind one peer
// that stopped reading.
func TestChannelEnqueueTwoWayFailsFastWhenFull(t *testing.T) {
	stream := newBlockingSendStream()
	// Capacity 1: one request occupies the sender, one fills the queue.
	c := NewInboundChannel(t.Context(), 1, 1, stream, NewMessageRouter())
	t.Cleanup(func() {
		stream.close()
		_ = c.Close()
	})

	// Occupy the sender with a one-way request; wait until its Send started.
	c.Enqueue(Request{
		Ctx:    context.Background(),
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
	})
	waitID(t, stream.entered, 1, "first send")

	// A two-way request fills the queue's single slot.
	reply2 := make(chan response, 1)
	c.Enqueue(Request{
		Ctx:          context.Background(),
		ResponseChan: reply2,
		Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
	})
	select {
	case resp := <-reply2:
		t.Fatalf("second request should be queued, got early reply: %v", resp.Err)
	default:
	}

	// The next two-way request finds the queue full and must fail fast.
	reply3 := make(chan response, 1)
	c.Enqueue(Request{
		Ctx:          context.Background(),
		ResponseChan: reply3,
		Msg:          Message_builder{MessageSeqNo: 3, Method: mock.TestMethod}.Build(),
	})
	select {
	case resp := <-reply3:
		if !errors.Is(resp.Err, ErrSendQueueFull) {
			t.Errorf("full-queue reply error = %v, want ErrSendQueueFull", resp.Err)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("two-way Enqueue blocked on a full send queue instead of failing fast")
	}

	// FIFO: releasing the stream completes message 1, then message 2 follows.
	stream.release()
	waitID(t, stream.sends, 1, "first send completion")
	waitID(t, stream.sends, 2, "queued send completion")
}

// TestChannelEnqueueOnewayBlocksWhenFull verifies that one-way requests keep
// today's blocking behavior on a full queue: with no reply to await,
// backpressure is the only mechanism pacing a one-way producer, so a full
// queue must make the producer wait (cancellable via the request context)
// rather than drop the message.
func TestChannelEnqueueOnewayBlocksWhenFull(t *testing.T) {
	stream := newBlockingSendStream()
	c := NewInboundChannel(t.Context(), 1, 1, stream, NewMessageRouter())
	t.Cleanup(func() {
		stream.close()
		_ = c.Close()
	})

	// Occupy the sender and fill the queue's single slot.
	c.Enqueue(Request{
		Ctx:    context.Background(),
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
	})
	waitID(t, stream.entered, 1, "first send")
	c.Enqueue(Request{
		Ctx:    context.Background(),
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
	})

	// The third one-way request must block in Enqueue, not fail.
	reply3 := make(chan response, 1)
	enqueueReturned := make(chan struct{})
	go func() {
		defer close(enqueueReturned)
		c.Enqueue(Request{
			Ctx:          context.Background(),
			Oneway:       true,
			ResponseChan: reply3,
			Msg:          Message_builder{MessageSeqNo: 3, Method: mock.TestMethod}.Build(),
		})
	}()
	select {
	case resp := <-reply3:
		t.Fatalf("one-way Enqueue on a full queue returned early with: %v", resp.Err)
	case <-enqueueReturned:
		t.Fatal("one-way Enqueue returned without queue space; expected it to block")
	case <-time.After(100 * time.Millisecond):
		// Still blocked, as intended.
	}

	// Releasing the stream drains the queue; the blocked request completes.
	stream.release()
	select {
	case resp := <-reply3:
		if resp.Err != nil {
			t.Errorf("blocked one-way request failed after release: %v", resp.Err)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("blocked one-way request did not complete after queue drained")
	}
	waitID(t, stream.sends, 1, "first send completion")
	waitID(t, stream.sends, 2, "queued send completion")
	waitID(t, stream.sends, 3, "unblocked send completion")
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

// TestChannelRequestsSurviveStreamChurn verifies that a two-way request
// enqueued while the current stream is concurrently torn down never fails
// with ErrStreamDown. The sender must send on the exact stream its ensure
// step produced: reading the stream again in a separate step races with a
// concurrent clearStream — the receiver observing a broken stream, or a
// cancel watcher — and can observe nil right after a successful ensure,
// failing the request terminally, since a request that was never sent is not
// registered for retry. A request that instead loses the race on Send fails
// with a stream error after registration and is requeued, so under stream
// churn every request must eventually succeed.
func TestChannelRequestsSurviveStreamChurn(t *testing.T) {
	tc := setupChannel(t, echoServer)
	if !waitForConnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	// Churn: keep tearing down whatever stream is current, exactly as the
	// receiver does when it observes a broken stream (clear, then requeue the
	// pending requests stranded on it), forcing every request to race its
	// ensure/send steps against a concurrent stream teardown.
	churnDone := make(chan struct{})
	go func() {
		defer close(churnDone)
		for range 2000 {
			if s := tc.getStream(); s != nil && tc.clearStream(s) {
				tc.requeuePendingMsgs()
			}
		}
	}()

	const concurrency = 8
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for i := range concurrency {
		wg.Go(func() {
			// Issue requests until the churn ends, recording the first failure.
			for msgID := uint64(1); ; msgID++ {
				resp := sendRequest(t, tc.Channel, Request{}, uint64(i+1)*100000+msgID)
				if resp.Err != nil {
					errs[i] = resp.Err
					return
				}
				select {
				case <-churnDone:
					return
				default:
				}
			}
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("requester %d: unexpected error during stream churn: %v", i, err)
		}
	}
}

type recvStartedStream struct {
	*mockBidiStream
	started chan struct{}
	once    sync.Once
}

func newRecvStartedStream() *recvStartedStream {
	return &recvStartedStream{
		mockBidiStream: newMockBidiStream(),
		started:        make(chan struct{}),
	}
}

func (s *recvStartedStream) Recv() (*Message, error) {
	s.once.Do(func() { close(s.started) })
	return s.mockBidiStream.Recv()
}

// TestChannelStaleReceiverDoesNotRequeueCurrentPending verifies that a receiver
// blocked on an old stream instance cannot requeue requests that were already
// registered on a newer stream.
func TestChannelStaleReceiverDoesNotRequeueCurrentPending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stale := newRecvStartedStream()
	current := newMockBidiStream()
	c := NewInboundChannel(ctx, 1, 1, stale, NewMessageRouter())
	t.Cleanup(func() {
		_ = c.Close()
	})

	done := make(chan struct{})
	go func() {
		c.receiver()
		close(done)
	}()

	select {
	case <-stale.started:
	case <-time.After(defaultTestTimeout):
		t.Fatal("receiver did not start reading from stale stream")
	}

	c.streamMut.Lock()
	c.stream = current
	c.streamMut.Unlock()

	const msgID = 42
	msg := Message_builder{
		MessageSeqNo: msgID,
		Method:       mock.TestMethod,
	}.Build()
	c.router.register(c.pendingOwner, msgID, Request{
		Ctx:          ctx,
		Msg:          msg,
		ResponseChan: make(chan response, 1),
	})

	stale.close()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(c.sendQ) > 0 || !routerExists(c, msgID) {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if got := len(c.sendQ); got != 0 {
		t.Fatalf("stale receiver requeued current-stream request: sendQ len = %d, want 0", got)
	}
	if !routerExists(c, msgID) {
		t.Fatal("stale receiver removed pending request for the current stream")
	}

	current.close()
	cancel()
	select {
	case <-done:
	case <-time.After(defaultTestTimeout):
		t.Fatal("receiver did not exit after context cancellation")
	}
}

// lateAfterFuncContext is a test context implementation that gives deterministic
// control over when the AfterFunc callback fires. The trigger() method
// simulates context cancellation and fires the registered callback.
type lateAfterFuncContext struct {
	ready chan struct{}
	mu    sync.Mutex
	err   error
	f     func()
	done  chan struct{}
}

func newLateAfterFuncContext() *lateAfterFuncContext {
	return &lateAfterFuncContext{
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}
}

func (*lateAfterFuncContext) Deadline() (time.Time, bool) { return time.Time{}, false }

func (c *lateAfterFuncContext) Done() <-chan struct{} { return c.done }

func (c *lateAfterFuncContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (*lateAfterFuncContext) Value(any) any { return nil }

func (c *lateAfterFuncContext) AfterFunc(f func()) func() bool {
	c.mu.Lock()
	c.f = f
	close(c.ready)
	c.mu.Unlock()
	return func() bool { return false }
}

func (c *lateAfterFuncContext) trigger() {
	<-c.ready
	c.mu.Lock()
	c.err = context.Canceled
	f := c.f
	close(c.done)
	c.mu.Unlock()
	if f != nil {
		go f()
	}
}

// lateCancelStream is a test stream that records sent message IDs and
// blocks Recv until closed. This lets tests control when the receiver
// goroutine sees a stream error after sends have completed.
type lateCancelStream struct {
	recvStarted chan struct{}
	recvOnce    sync.Once
	closed      chan struct{}
	sends       chan uint64
}

func newLateCancelStream() *lateCancelStream {
	return &lateCancelStream{
		recvStarted: make(chan struct{}),
		closed:      make(chan struct{}),
		sends:       make(chan uint64, 8),
	}
}

func (s *lateCancelStream) Send(msg *Message) error {
	s.sends <- msg.GetMessageSeqNo()
	return nil
}

func (s *lateCancelStream) Recv() (*Message, error) {
	s.recvOnce.Do(func() { close(s.recvStarted) })
	<-s.closed
	return nil, context.Canceled
}

func (s *lateCancelStream) close() {
	close(s.closed)
}

// TestChannelLateCancelWatcherRequeuesPending verifies that a late-running
// per-request cancel watcher cannot strand newer pending requests on a stream
// it clears after the original Send already returned.
func TestChannelLateCancelWatcherRequeuesPending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newLateCancelStream()
	c := NewInboundChannel(ctx, 1, 4, stream, NewMessageRouter())
	t.Cleanup(func() {
		_ = c.Close()
	})

	done := make(chan struct{})
	go func() {
		c.receiver()
		close(done)
	}()

	select {
	case <-stream.recvStarted:
	case <-time.After(defaultTestTimeout):
		t.Fatal("receiver did not start reading from stream")
	}

	ctx1 := newLateAfterFuncContext()
	reply1 := make(chan response, 1)
	c.Enqueue(Request{
		Ctx:          ctx1,
		Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		ResponseChan: reply1,
	})
	select {
	case msgID := <-stream.sends:
		if msgID != 1 {
			t.Fatalf("first send msgID = %d, want 1", msgID)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("first request was not sent")
	}

	reply2 := make(chan response, 1)
	c.Enqueue(Request{
		Ctx:          context.Background(),
		Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
		ResponseChan: reply2,
	})
	select {
	case msgID := <-stream.sends:
		if msgID != 2 {
			t.Fatalf("second send msgID = %d, want 2", msgID)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("second request was not sent")
	}

	if !routerExists(c, 2) {
		t.Fatal("second request should be pending before late cancel watcher runs")
	}

	ctx1.trigger()
	stream.close()

	select {
	case resp := <-reply2:
		if !errors.Is(resp.Err, ErrStreamDown) {
			t.Fatalf("reply2 error = %v, want ErrStreamDown", resp.Err)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("late cancel watcher stranded newer pending request")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(defaultTestTimeout):
		t.Fatal("receiver did not exit after context cancellation")
	}
}

// TestChannelCancelInflightSend verifies the per-request cancel watcher's
// decision logic. The watcher exists solely to unblock a Send stalled by flow
// control (such a Send returns only when its stream dies), so it must clear
// the stream — and requeue the pending requests stranded on it — only while
// the watched Send is still in flight. Once the send has completed, a
// late-running watcher must leave the stream and its pending requests
// untouched: clearing then would sever a healthy stream that later requests
// depend on. A late watcher is routine, not exotic — a caller may cancel its context
// the moment the response arrives (as the benchmark readiness probe does),
// which lands the cancellation between Send returning and the sender's stop
// call, and the watcher goroutine spawned by that cancellation can then run
// arbitrarily late.
func TestChannelCancelInflightSend(t *testing.T) {
	tests := []struct {
		name        string
		sendDone    bool
		wantCleared bool
	}{
		// Send still in flight: the watcher must clear the stream to unblock
		// it, requeueing the pending request for retry on the next stream.
		{name: "InflightSendClearsStream", sendDone: false, wantCleared: true},
		// Send already returned: nothing is blocked, so the watcher must
		// leave the healthy stream and its pending requests untouched.
		{name: "CompletedSendLeavesStream", sendDone: true, wantCleared: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The channel is built directly (bypassing the constructors) so no
			// sender or receiver goroutine races the manually injected state;
			// cancelInflightSend is exercised as a plain method call, matching
			// TestChannelEnsureConnectedNodeStreamCancelsAbandonedStream.
			connCtx, connCancel := context.WithCancel(context.Background())
			t.Cleanup(connCancel)
			c := &Channel{
				sendQ:        make(chan Request, 4),
				id:           1,
				connCtx:      connCtx,
				connCancel:   connCancel,
				router:       NewMessageRouter(),
				pendingOwner: new(pendingOwner),
				streamReady:  make(chan struct{}, 1),
				stream:       newLateCancelStream(),
			}

			// Register a pending two-way request, as the sender does before Send.
			const msgID = 7
			reply := make(chan response, 1)
			req := Request{
				Ctx:          context.Background(),
				Msg:          Message_builder{MessageSeqNo: msgID, Method: mock.TestMethod}.Build(),
				ResponseChan: reply,
			}
			c.router.register(c.pendingOwner, msgID, req)

			sendDone := tc.sendDone
			c.cancelInflightSend(&sendDone, c.getStream())

			if gotCleared := c.getStream() == nil; gotCleared != tc.wantCleared {
				t.Errorf("stream cleared = %t, want %t", gotCleared, tc.wantCleared)
			}
			// The pending request must be requeued exactly when the stream was
			// cleared; a healthy stream keeps its pending entry for the receiver.
			if gotRequeued := len(c.sendQ) == 1; gotRequeued != tc.wantCleared {
				t.Errorf("pending request requeued = %t, want %t", gotRequeued, tc.wantCleared)
			}
			if !tc.wantCleared && !routerExists(c, msgID) {
				t.Error("pending request was removed although the stream was left intact")
			}
		})
	}
}

// TestChannelCancelImmediatelyAfterSendRecovers exercises the residual window
// in the sender's cancel watcher: a caller that cancels its request context the
// instant its response arrives can land the cancellation between Send returning
// and the sender marking the send done, so a late watcher clears an otherwise
// healthy stream. That clear is self-healing — the stream re-establishes on the
// next send and any requeued request retries — so repeated immediate
// cancellation must never permanently strand the channel. The churn is most
// valuable under -race. It is the end-to-end complement to the decision-table
// coverage in [TestChannelCancelInflightSend].
func TestChannelCancelImmediatelyAfterSendRecovers(t *testing.T) {
	tc := setupChannel(t, echoServer)
	if !waitForConnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel never connected")
	}

	// Each iteration cancels the request context the moment the response is in
	// hand, maximizing the chance the watcher fires in the post-Send window.
	const iterations = 200
	for i := range iterations {
		ctx, cancel := context.WithCancel(context.Background())
		reply := make(chan response, 1)
		tc.Enqueue(Request{
			Ctx:          ctx,
			Msg:          Message_builder{MessageSeqNo: uint64(i + 1), Method: mock.TestMethod}.Build(),
			ResponseChan: reply,
		})
		select {
		case resp := <-reply:
			cancel() // cancel the instant the response arrives
			if resp.Err != nil {
				t.Fatalf("request %d failed: %v", i+1, resp.Err)
			}
		case <-time.After(defaultTestTimeout):
			cancel()
			t.Fatalf("request %d never completed", i+1)
		}
	}

	// After the churn a fresh request must still complete: a late watcher that
	// cleared a healthy stream must not have stranded the channel.
	if resp := sendRequest(t, tc.Channel, Request{}, iterations+1); resp.Err != nil {
		t.Fatalf("channel stranded after cancel churn: %v", resp.Err)
	}
}

// killFirstStreamServer returns a NodeStream server function that kills the
// first accepted stream immediately — before the client sends anything — and
// serves echo on every later stream. Each accepted stream's ordinal is sent
// on conns, so a test can await the initial stream and the redial.
func killFirstStreamServer() (serverFn func(Gorums_NodeStreamServer) error, conns chan int32) {
	var connCount atomic.Int32
	conns = make(chan int32, 4)
	serverFn = func(stream Gorums_NodeStreamServer) error {
		n := connCount.Add(1)
		conns <- n
		if n == 1 {
			return errors.New("stream killed by test server")
		}
		return echoServer(stream)
	}
	return serverFn, conns
}

// TestChannelEagerReconnectRedialsWithoutSends verifies that a channel with
// eager reconnection re-establishes a stream the server killed without any
// local send prompting it, and that the replacement stream then carries a
// request round trip. A symmetric peer depends on this stream staying
// registered on its inbound side, so waiting for the next local send — the
// default — would leave that peer dropped indefinitely on a node with nothing
// to send.
func TestChannelEagerReconnectRedialsWithoutSends(t *testing.T) {
	serverFn, conns := killFirstStreamServer()
	tc := setupChannelEager(t, true, serverFn)

	// The sender's initial eager connect creates the first stream with no
	// request enqueued; the server kills it on arrival.
	select {
	case n := <-conns:
		if n != 1 {
			t.Fatalf("first accepted stream ordinal = %d, want 1", n)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("initial stream never reached the server")
	}

	// The channel must redial on its own: no Enqueue happens until the
	// replacement stream is observed server-side.
	select {
	case n := <-conns:
		if n != 2 {
			t.Fatalf("redialed stream ordinal = %d, want 2", n)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("channel did not redial the killed stream without a local send")
	}

	// The replacement stream must carry a request round trip.
	replyCh := make(chan response, 1)
	tc.Enqueue(Request{
		Ctx:          t.Context(),
		Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		ResponseChan: replyCh,
	})
	select {
	case resp := <-replyCh:
		if resp.Err != nil {
			t.Fatalf("echo over redialed stream failed: %v", resp.Err)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("echo over redialed stream never completed")
	}
}

// rejectEveryStreamServer returns a NodeStream server function that rejects
// every accepted stream immediately and counts how many streams it accepted.
func rejectEveryStreamServer() (serverFn func(Gorums_NodeStreamServer) error, count *atomic.Int32) {
	count = new(atomic.Int32)
	serverFn = func(Gorums_NodeStreamServer) error {
		count.Add(1)
		return errors.New("stream rejected by test server")
	}
	return serverFn, count
}

// TestChannelEagerReconnectBacksOffRejectedStreams verifies that a channel with
// eager reconnection paces its redials with capped backoff when the server
// rejects every stream, instead of spinning and creating a new stream per
// iteration. Without backoff the loop produced thousands of accepted streams
// (5,127 in 250 ms during review); with backoff (50 ms base, doubling to a 2 s
// cap) only a handful of attempts fit in the window below.
func TestChannelEagerReconnectBacksOffRejectedStreams(t *testing.T) {
	serverFn, count := rejectEveryStreamServer()
	setupChannelEager(t, true, serverFn)

	const window = 500 * time.Millisecond
	time.Sleep(window)
	// Attempts within the window land at roughly 0, 50, 150, 350 ms plus the
	// sender's initial eager connect: about five. The generous bound tolerates
	// scheduler jitter while still catching an unpaced spin.
	if got := count.Load(); got > 20 {
		t.Fatalf("server accepted %d streams in %v; eager reconnect is not backing off (want <= 20)", got, window)
	}
}

type signalingRequestHandler struct {
	called chan *Message
}

func (h *signalingRequestHandler) HandleRequest(_ context.Context, msg *Message, release func(), _ func(*Message)) {
	defer release()
	select {
	case h.called <- msg:
	default:
	}
}

// TestChannelReceiverDispatchesOnlyServerInitiatedUnknownMessages verifies that
// the client-side receiver drops late client-call responses that no longer have
// a pending router entry, while still dispatching unmatched server-initiated
// requests to the registered back-channel handler.
func TestChannelReceiverDispatchesOnlyServerInitiatedUnknownMessages(t *testing.T) {
	tests := []struct {
		name       string
		msgID      uint64
		wantHandle bool
	}{
		{
			name:       "ClientInitiatedStaleResponseIsDropped",
			msgID:      1,
			wantHandle: false,
		},
		{
			name:       "ServerInitiatedRequestIsDispatched",
			msgID:      ServerSequenceNumber(1),
			wantHandle: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			stream := newMockBidiStream()
			handler := &signalingRequestHandler{called: make(chan *Message, 1)}
			c := NewInboundChannel(ctx, 1, 1, stream, NewMessageRouter(handler))
			t.Cleanup(func() {
				_ = c.Close()
			})

			done := make(chan struct{})
			go func() {
				c.receiver()
				close(done)
			}()

			stream.msgQ <- Message_builder{
				MessageSeqNo: tt.msgID,
				Method:       mock.TestMethod,
			}.Build()

			if tt.wantHandle {
				select {
				case got := <-handler.called:
					if got.GetMessageSeqNo() != tt.msgID {
						t.Fatalf("handler msgID = %d, want %d", got.GetMessageSeqNo(), tt.msgID)
					}
				case <-time.After(defaultTestTimeout):
					t.Fatal("expected handler dispatch")
				}
			} else {
				select {
				case got := <-handler.called:
					t.Fatalf("unexpected handler dispatch for stale response msgID %d", got.GetMessageSeqNo())
				case <-time.After(100 * time.Millisecond):
				}
			}

			stream.close()
			cancel()
			select {
			case <-done:
			case <-time.After(defaultTestTimeout):
				t.Fatal("receiver did not exit after context cancellation")
			}
		})
	}
}

func TestChannelRouterLifecycle(t *testing.T) {
	tc := setupChannel(t, echoServer)

	if !waitForConnection(tc.Channel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	tests := []struct {
		name       string
		oneway     bool
		streaming  bool
		wantRouter bool
		wantPanic  bool
	}{
		{name: "Oneway/NoStreaming/Cleanup", oneway: true, streaming: false, wantRouter: false},
		{name: "Oneway/Streaming/Invalid", oneway: true, streaming: true, wantPanic: true},
		{name: "Twoway/NoStreaming/Cleanup", oneway: false, streaming: false, wantRouter: false},
		{name: "Twoway/Streaming/KeepsRouterAlive", oneway: false, streaming: true, wantRouter: true},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panicRecovered := false
			defer func() {
				if r := recover(); r != nil {
					panicRecovered = true
					if !tt.wantPanic {
						t.Errorf("unexpected panic: %v", r)
					}
				}
			}()
			msgID := uint64(i)
			resp := sendRequest(t, tc.Channel, Request{Oneway: tt.oneway, Streaming: tt.streaming}, msgID)
			if resp.Err != nil {
				t.Errorf("unexpected error: %v", resp.Err)
			}
			if exists := routerExists(tc.Channel, msgID); exists != tt.wantRouter {
				t.Errorf("router exists = %v, want %v", exists, tt.wantRouter)
			}
			if tt.wantPanic && !panicRecovered {
				t.Errorf("expected panic but none occurred")
			}
		})
	}
}

// Helper functions for testing channel response routing and router lifecycle

func routerExists(c *Channel, msgID uint64) bool {
	c.router.mu.Lock()
	defer c.router.mu.Unlock()
	_, exists := c.router.pending[msgID]
	return exists
}

func TestChannelResponseRouting(t *testing.T) {
	tc := setupChannel(t, echoServer)

	const numMessages = 20
	results := make(chan msgResponse, numMessages)

	for i := range numMessages {
		go sendReq(t, results, tc.Channel, i, 1, Request{Oneway: true})
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
			sendReq(t, results, tc.Channel, goID, msgsPerGoroutine, Request{Oneway: true})
			sendReq(t, results, tc.Channel, goID, msgsPerGoroutine, Request{Oneway: false})
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
	sendRequest(t, tc.Channel, Request{Oneway: true}, 1)

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
	srv := grpc.NewServer() // skipcq: GO-S0902
	RegisterGorumsServer(srv, &mockServer{handler: holdServer})
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	c := NewOutboundChannel(t.Context(), 1, sendBufSize, conn, NewMessageRouter(), false, nil)
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

	// Inject sendBufSize+2 non-streaming requests directly into the router's
	// pending map, bypassing the normal sendMsg path. requeuePendingMsgs will
	// attempt to re-enqueue all of them. With sendBufSize=2:
	//  - request 1 is enqueued; sender dequeues it and blocks in ensureStream.
	//  - requests 2-3 fill sendQ to capacity.
	//  - request 4's Enqueue call blocks on a full sendQ while clearStream
	//    still holds streamMut — deadlock.
	const numPending = sendBufSize + 2
	replyChannels := make([]chan response, numPending)
	for i := range numPending {
		replyChannels[i] = make(chan response, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		msg, msgErr := NewMessage(ctx, uint64(1000+i), mock.TestMethod, nil)
		if msgErr != nil {
			t.Fatalf("NewMessage failed: %v", msgErr)
		}
		c.router.register(c.pendingOwner, uint64(1000+i), Request{
			Ctx:          ctx,
			Msg:          msg,
			Streaming:    false,
			Oneway:       false,
			ResponseChan: replyChannels[i],
		})
	}

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

// mockBidiStream is a bidirectional stream for testing inbound channels.
// Send echoes messages back via Recv (echo behavior).
// Call close() to simulate the stream being torn down.
type mockBidiStream struct {
	msgQ   chan *Message
	ctx    context.Context
	cancel context.CancelFunc
}

func newMockBidiStream() *mockBidiStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockBidiStream{
		msgQ:   make(chan *Message, 16),
		ctx:    ctx,
		cancel: cancel,
	}
}

// close simulates the stream being torn down, causing Recv to return an error.
func (m *mockBidiStream) close() {
	m.cancel()
}

func (m *mockBidiStream) Send(msg *Message) error {
	select {
	case m.msgQ <- msg:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *mockBidiStream) Recv() (*Message, error) {
	select {
	case msg := <-m.msgQ:
		return msg, nil
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	}
}

// TestIsInbound verifies IsInbound() for both channel types.
func TestIsInbound(t *testing.T) {
	tests := []struct {
		name     string
		chanFunc func(t *testing.T) *Channel
		want     bool
	}{
		{
			name:     "OutboundWithoutServer",
			chanFunc: func(t *testing.T) *Channel { return setupChannelWithoutServer(t).Channel },
			want:     false,
		},
		{
			name:     "OutboundWithServer",
			chanFunc: func(t *testing.T) *Channel { return setupChannel(t, echoServer).Channel },
			want:     false,
		},
		{
			name: "Inbound",
			chanFunc: func(t *testing.T) *Channel {
				stream := newMockBidiStream()
				c := NewInboundChannel(t.Context(), 1, 10, stream, NewMessageRouter())
				t.Cleanup(func() { _ = c.Close() })
				return c
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.chanFunc(t)
			if got := c.IsInbound(); got != tt.want {
				t.Errorf("IsInbound() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestInboundChannel verifies that an inbound channel can send messages.
// No receiver goroutine is started for inbound channels; the caller's NodeStream
// Recv loop is the sole reader. Oneway confirms successful delivery to the
// stream without requiring a routed response.
func TestInboundChannel(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, 10, stream, NewMessageRouter())
	t.Cleanup(func() {
		_ = c.Close()
	})

	// Send a message and verify it is delivered to the stream.
	resp := sendRequest(t, c, Request{Oneway: true}, 1)
	if resp.Err != nil {
		t.Errorf("unexpected error: %v", resp.Err)
	}
	if resp.NodeID != 1 {
		t.Errorf("NodeID = %d, want 1", resp.NodeID)
	}
}

// TestInboundChannelClose verifies that close does not close the underlying
// server connection (conn is nil for inbound channels).
func TestInboundChannelClose(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, 10, stream, NewMessageRouter())

	// Close the channel; should succeed without touching any grpc.ClientConn.
	if err := c.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Subsequent sends should fail with ErrNodeClosed.
	resp := sendRequest(t, c, Request{Oneway: true}, 2)
	if resp.Err == nil {
		t.Error("expected error after close, got nil")
	} else if !errors.Is(resp.Err, ErrNodeClosed) {
		t.Errorf("expected 'node closed' error, got: %v", resp.Err)
	}

	// Channel should not be connected after close.
	if c.isConnected() {
		t.Error("isConnected() = true after close, want false")
	}
}

// TestInboundChannelStreamDown verifies that an inbound channel does not
// reconnect when the stream goes down. In production, when NodeStream.Recv()
// returns an error, the deferred UnregisterPeer calls detachStream() → channel.Close().
// This test mirrors that path: close the channel to simulate stream-down, then
// verify sends return ErrNodeClosed rather than silently retrying on a new stream.
func TestInboundChannelStreamDown(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, 10, stream, NewMessageRouter())

	// Verify initial send works.
	resp := sendRequest(t, c, Request{Oneway: true}, 1)
	if resp.Err != nil {
		t.Fatalf("initial send failed: %v", resp.Err)
	}

	// Simulate NodeStream detecting stream-down: NodeStream.Recv() returns an
	// error → deferred UnregisterPeer() runs → detachStream() → channel.Close().
	stream.close()
	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// Sends after close must fail with ErrNodeClosed, not silently reconnect.
	resp = sendRequest(t, c, Request{Oneway: true}, 2)
	if resp.Err == nil {
		t.Error("expected error after stream down, got nil")
	} else if !errors.Is(resp.Err, ErrNodeClosed) {
		t.Errorf("expected 'node closed' error, got: %v", resp.Err)
	}

	// Verify channel remains disconnected (did not reconnect).
	if c.isConnected() {
		t.Error("inbound channel reconnected, but it should not")
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
		cancel()
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
				req := Request{Ctx: context.Background(), Msg: msg, Oneway: true, ResponseChan: replyChan}
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
				for pb.Next() {
					replyChan := make(chan response, 1)
					id := msgID.Add(1)
					msg := Message_builder{
						MessageSeqNo: id,
						Method:       mock.TestMethod,
						Payload:      payload,
					}.Build()
					req := Request{Ctx: context.Background(), Msg: msg, Oneway: true, ResponseChan: replyChan}
					tc.Enqueue(req)
					<-replyChan
				}
			})
		})
	}
}
