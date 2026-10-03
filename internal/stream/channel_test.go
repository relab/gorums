package stream

import (
	"context"
	"errors"
	"maps"
	"net"
	"slices"
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

// isConnected reports whether the channel's connection is Ready and a stream
// is accepting requests.
func (c *OutboundChannel) isConnected() bool {
	return c.conn.GetState() == connectivity.Ready && c.StreamUp()
}

// endSessions ends the channel's current sessions, as a broken stream does.
func (c *OutboundChannel) endSessions() {
	c.mu.Lock()
	sessions := slices.Collect(maps.Keys(c.sessions))
	c.mu.Unlock()
	for _, s := range sessions {
		s.end()
	}
}

// pendingExists reports whether a call with msgID is pending on any of the
// channel's sessions.
func (c *OutboundChannel) pendingExists(msgID uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for s := range c.sessions {
		s.pending.mu.Lock()
		_, ok := s.pending.calls[msgID]
		s.pending.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}

// testChannel holds the channel and cleanup function.
type testChannel struct {
	*OutboundChannel
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
func waitForLastErr(t testing.TB, c Channel, wantErr bool, what string) {
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

	c := NewOutboundChannel(t.Context(), 1, conn, OutboundOptions{
		SendBufferSize: 10,
		Latency:        NewLatency(),
		EagerReconnect: eagerReconnect,
	})
	tc := &testChannel{
		OutboundChannel: c,
		srv:             srv,
		lis:             lis,
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
	c := NewOutboundChannel(ctx, 1, conn, OutboundOptions{SendBufferSize: 10})
	t.Cleanup(func() {
		cancel()
		if err := c.Close(); err != nil {
			t.Errorf("failed to close channel: %v", err)
		}
	})
	return &testChannel{OutboundChannel: c}
}

// waitForConnection polls until the node is connected or timeout expires.
// Returns true if connected, false if timeout expired.
func waitForConnection(c *OutboundChannel, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.isConnected() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return c.isConnected()
}

// waitForDisconnection polls until the channel is disconnected or timeout expires.
// Returns true if disconnected, false if timeout expired.
func waitForDisconnection(c *OutboundChannel, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !c.isConnected() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !c.isConnected()
}

func sendRequest(t testing.TB, c Channel, req Request, msgID uint64) response {
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

func sendReq(t testing.TB, results chan<- msgResponse, c Channel, goroutineID, msgsToSend int, req Request) {
	for j := range msgsToSend {
		msgID := uint64(goroutineID*1000 + j)
		resp := sendRequest(t, c, req, msgID)
		results <- msgResponse{msgID: msgID, resp: resp}
	}
}

func TestChannelCreation(t *testing.T) {
	tc := setupChannelWithoutServer(t)

	// send message when server is down
	resp := sendRequest(t, tc.OutboundChannel, Request{Oneway: true}, 1)
	if resp.Err == nil {
		t.Error("response err: got <nil>, want error")
	}
}

func TestChannelShutdown(t *testing.T) {
	tc := setupChannel(t, echoServer)

	if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	// enqueue several messages to confirm normal operation
	const numMessages = 10
	var wg sync.WaitGroup
	for i := range numMessages {
		wg.Go(func() {
			resp := sendRequest(t, tc.OutboundChannel, Request{}, uint64(i))
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
	resp := sendRequest(t, tc.OutboundChannel, Request{}, 999)
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
	if latency := tc.latency.Load(); latency != -1*time.Second {
		t.Errorf("Initial latency = %v, expected -1s", latency)
	}

	// Send a few requests to update latency
	for i := range 10 {
		sendRequest(t, tc.OutboundChannel, Request{Oneway: false}, uint64(i))
	}

	latency := tc.latency.Load()
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
			resp := sendRequest(t, tc.OutboundChannel, Request{Oneway: tt.oneway}, uint64(i))
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
				resp := sendRequest(t, tc.OutboundChannel, Request{Oneway: true}, 1)
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

			resp := sendRequest(t, tc.OutboundChannel, Request{Oneway: true}, uint64(i))
			if resp.Err == nil {
				t.Errorf("expected error containing %q but got nil", tt.wantErr)
			} else if !strings.Contains(resp.Err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, resp.Err)
			}
		})
	}
}

// TestChannelStreamFailureRecordsLastErr verifies that a request the channel
// cannot deliver because no stream could be established leaves the reason in
// LastErr, even when the request has no response channel to report it on.
func TestChannelStreamFailureRecordsLastErr(t *testing.T) {
	tc := setupChannelWithoutServer(t)

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
// health: a channel whose stream failed once reports the failure, and reports
// nil again once traffic flows over a new stream, so a node with one transient
// failure sorts ahead of a node that is down right now.
func TestChannelLastErrClearsOnRecovery(t *testing.T) {
	tc := setupChannel(t, rejectFirstStreamServer())

	// The channel's eager connect creates the stream the server rejects.
	waitForLastErr(t, tc.OutboundChannel, true, "the rejected stream to be recorded")

	// A completed round trip over the replacement stream proves the channel
	// usable again.
	if resp := sendRequest(t, tc.OutboundChannel, Request{}, 1); resp.Err != nil {
		t.Fatalf("sendRequest after recovery: %v", resp.Err)
	}
	waitForLastErr(t, tc.OutboundChannel, false, "LastErr to clear after recovery")
}

// TestChannelEagerReconnectRecordsStreamFailure verifies that an idle channel
// with eager reconnect records its failed stream creations.
func TestChannelEagerReconnectRecordsStreamFailure(t *testing.T) {
	conn := newUnavailableClientConn(t)
	ctx, cancel := context.WithCancel(context.Background())
	c := NewOutboundChannel(ctx, 1, conn, OutboundOptions{SendBufferSize: 10, EagerReconnect: true})
	t.Cleanup(func() {
		cancel()
		if err := c.Close(); err != nil {
			t.Errorf("failed to close channel: %v", err)
		}
	})

	// No request is ever enqueued: only the redial loop runs.
	waitForLastErr(t, c, true, "the failed redial to be recorded")
}

// TestChannelCloseCancelsOnlyOwnedPendingRequests verifies that closing one
// of a node's inbound channels fails only the calls pending on that channel.
func TestChannelCloseCancelsOnlyOwnedPendingRequests(t *testing.T) {
	oldStream := newMockBidiStream()
	newStream := newMockBidiStream()
	t.Cleanup(oldStream.close)
	t.Cleanup(newStream.close)
	oldChannel := NewInboundChannel(t.Context(), 1, oldStream, InboundOptions{SendBufferSize: 1})
	newChannel := NewInboundChannel(t.Context(), 1, newStream, InboundOptions{SendBufferSize: 1})
	t.Cleanup(func() { _ = oldChannel.Close() })
	t.Cleanup(func() { _ = newChannel.Close() })

	oldReply := make(chan response, 1)
	newReply := make(chan response, 1)
	oldMessage := Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build()
	newMessage := Message_builder{MessageSeqNo: ServerSequenceNumber(2), Method: mock.TestMethod}.Build()
	oldChannel.Enqueue(Request{Ctx: t.Context(), Msg: oldMessage, ResponseChan: oldReply})
	newChannel.Enqueue(Request{Ctx: t.Context(), Msg: newMessage, ResponseChan: newReply})

	pending := func() int { return oldChannel.PendingCount() + newChannel.PendingCount() }
	deadline := time.Now().Add(time.Second)
	for pending() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := pending(); got != 2 {
		t.Fatalf("pending count = %d, want 2", got)
	}

	if err := oldChannel.Close(); err != nil {
		t.Fatalf("old channel Close: %v", err)
	}
	select {
	case got := <-oldReply:
		if !errors.Is(got.Err, ErrStreamDown) {
			t.Fatalf("old request error = %v, want ErrStreamDown", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("old request was not cancelled")
	}
	select {
	case got := <-newReply:
		t.Fatalf("new request was cancelled by old channel: %v", got.Err)
	default:
	}

	newChannel.session.handle(newMessage)
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
			name: "RequiresStream",
			setup: func(t *testing.T) *testChannel {
				tc := setupChannel(t, echoServer)
				if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
					t.Fatal("node should be connected before ending its stream")
				}
				tc.endSessions()
				if !waitForDisconnection(tc.OutboundChannel, streamConnectTimeout) {
					t.Fatal("node should be disconnected after its stream ended")
				}
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
				if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
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
			resp := sendRequest(t, tc.OutboundChannel, Request{Ctx: ctx, Oneway: tt.oneway}, uint64(i))
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
// cancelled, so a per-call deadline unblocks a worker stuck behind a peer that
// stopped reading.
func TestChannelEnqueueRespectsRequestContext(t *testing.T) {
	stream := newBlockingSendStream()
	// Capacity 0: the queue has no slack, so a second request blocks in
	// Enqueue as soon as the sender goroutine is occupied in Send.
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 0})
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
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 1})
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

// TestChannelEnqueueOnewayBlocksWhenFull verifies that one-way requests wait
// on a full queue: with no reply to await, backpressure paces a one-way
// producer, so the producer waits (cancellable via the request context) and
// the message is kept.
func TestChannelEnqueueOnewayBlocksWhenFull(t *testing.T) {
	stream := newBlockingSendStream()
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 1})
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

// TestChannelFirstRequestLatency verifies that the first request does not wait
// for anything beyond the stream's creation.
func TestChannelFirstRequestLatency(t *testing.T) {
	tc := setupChannel(t, echoServer)

	start := time.Now()
	resp := sendRequest(t, tc.OutboundChannel, Request{}, 1)
	firstLatency := time.Since(start)

	if resp.Err != nil {
		t.Fatalf("unexpected error on first request: %v", resp.Err)
	}

	start = time.Now()
	resp = sendRequest(t, tc.OutboundChannel, Request{}, 2)
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

// TestChannelReconnectAfterServerDrop verifies that a request after the server
// dropped the stream opens a new stream and completes.
func TestChannelReconnectAfterServerDrop(t *testing.T) {
	tc := setupChannel(t, breakStreamServer)

	// First request succeeds; the server then drops the stream.
	resp := sendRequest(t, tc.OutboundChannel, Request{}, 1)
	if resp.Err != nil {
		t.Fatalf("unexpected error on initial request: %v", resp.Err)
	}

	// Wait for the channel to detect the server-side disconnect so the next
	// request triggers the reconnection.
	if !waitForDisconnection(tc.OutboundChannel, streamConnectTimeout) {
		t.Fatal("channel should be disconnected after server drop")
	}

	// Second request: the channel re-establishes the stream.
	resp = sendRequest(t, tc.OutboundChannel, Request{}, 2)
	if resp.Err != nil {
		t.Fatalf("unexpected error after reconnect: %v", resp.Err)
	}
}

// TestChannelConcurrentStreamReconnect verifies correct handling of concurrent
// requests during stream reconnection. The server breaks the stream after echoing
// the first message, then the test fires multiple concurrent requests without
// waiting for the channel to detect the disconnect.
//
// Requests that race with the broken stream's teardown must be sent again on
// the new stream rather than fail.
func TestChannelConcurrentStreamReconnect(t *testing.T) {
	tc := setupChannel(t, breakStreamServer)
	if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	// Request 1 causes the server to echo and immediately break the stream.
	resp := sendRequest(t, tc.OutboundChannel, Request{}, 1)
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
			resp := sendRequest(t, tc.OutboundChannel, Request{}, uint64(i+2))
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

// TestChannelRequestsSurviveStreamChurn verifies that two-way requests issued
// while the current stream is repeatedly torn down never fail: a request
// pending on an ended stream, or taken from the queue as the stream ends, is
// sent again on the next stream.
func TestChannelRequestsSurviveStreamChurn(t *testing.T) {
	tc := setupChannel(t, echoServer)
	if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	churnDone := make(chan struct{})
	go func() {
		defer close(churnDone)
		for range 2000 {
			tc.endSessions()
		}
	}()

	const concurrency = 8
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for i := range concurrency {
		wg.Go(func() {
			// Issue requests until the churn ends, recording the first failure.
			for msgID := uint64(1); ; msgID++ {
				resp := sendRequest(t, tc.OutboundChannel, Request{}, uint64(i+1)*100000+msgID)
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

// TestChannelCancelImmediatelyAfterSendRecovers verifies that callers which
// cancel their request context the instant the response arrives never strand
// the channel. The churn is most valuable under -race.
func TestChannelCancelImmediatelyAfterSendRecovers(t *testing.T) {
	tc := setupChannel(t, echoServer)
	if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
		t.Fatal("channel never connected")
	}

	// Each iteration cancels the request context the moment the response is in hand.
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

	// After the churn a fresh request must still complete.
	if resp := sendRequest(t, tc.OutboundChannel, Request{}, iterations+1); resp.Err != nil {
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
// request round trip; see [NewOutboundChannel] for why a peer depends on
// that.
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
// an outbound session drops late responses that no longer have a pending call,
// while still dispatching server-initiated requests to the handler.
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
			stream := newMockBidiStream()
			handler := &signalingRequestHandler{called: make(chan *Message, 1)}
			e := newEndpoint(t.Context(), 1, 1, 0, handler, nil)
			ctx, cancel := context.WithCancel(e.ctx)
			s := newSession(ctx, cancel, &e, stream, true, true)
			t.Cleanup(e.cancel)

			done := make(chan struct{})
			go func() {
				_ = s.receive()
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
			select {
			case <-done:
			case <-time.After(defaultTestTimeout):
				t.Fatal("receive did not return after the stream closed")
			}
		})
	}
}

func TestChannelRouterLifecycle(t *testing.T) {
	tc := setupChannel(t, echoServer)

	if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
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
			resp := sendRequest(t, tc.OutboundChannel, Request{Oneway: tt.oneway, Streaming: tt.streaming}, msgID)
			if resp.Err != nil {
				t.Errorf("unexpected error: %v", resp.Err)
			}
			if exists := tc.pendingExists(msgID); exists != tt.wantRouter {
				t.Errorf("pending call exists = %v, want %v", exists, tt.wantRouter)
			}
			if tt.wantPanic && !panicRecovered {
				t.Errorf("expected panic but none occurred")
			}
		})
	}
}

func TestChannelResponseRouting(t *testing.T) {
	tc := setupChannel(t, echoServer)

	const numMessages = 20
	results := make(chan msgResponse, numMessages)

	for i := range numMessages {
		go sendReq(t, results, tc.OutboundChannel, i, 1, Request{Oneway: true})
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
			sendReq(t, results, tc.OutboundChannel, goID, msgsPerGoroutine, Request{Oneway: true})
			sendReq(t, results, tc.OutboundChannel, goID, msgsPerGoroutine, Request{Oneway: false})
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

// TestChannelDeadlock verifies that requests can still be queued while a
// broken stream is torn down and replaced (issue #235).
func TestChannelDeadlock(t *testing.T) {
	tc := setupChannel(t, breakStreamServer)

	if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
		t.Fatal("channel should be connected")
	}

	// Send message to activate stream
	sendRequest(t, tc.OutboundChannel, Request{Oneway: true}, 1)

	// Break the stream, forcing a reconnection on next send
	tc.endSessions()
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
			case tc.queue.ch <- req:
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

// TestChannelSessionEndWithFullQueue verifies that ending a session whose
// pending calls exceed the send queue's free space neither blocks nor strands
// a call: each is either queued again or fails with ErrSendQueueFull.
func TestChannelSessionEndWithFullQueue(t *testing.T) {
	const sendBufSize = 2
	stream := newBlockingSendStream()
	t.Cleanup(stream.close)
	e := newEndpoint(t.Context(), 1, sendBufSize, 0, nil, nil)
	t.Cleanup(e.cancel)
	ctx, cancel := context.WithCancel(e.ctx)
	s := newSession(ctx, cancel, &e, stream, true, true)

	const numPending = sendBufSize + 2
	replies := make(chan response, numPending)
	for i := range numPending {
		msg := Message_builder{MessageSeqNo: uint64(1000 + i), Method: mock.TestMethod}.Build()
		s.pending.add(msg.GetMessageSeqNo(), Request{Ctx: t.Context(), Msg: msg, ResponseChan: replies})
	}

	done := make(chan struct{})
	go func() {
		s.end()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ending the session blocked on a full send queue")
	}
	if got := len(e.queue.ch); got != sendBufSize {
		t.Errorf("requeued calls = %d, want %d", got, sendBufSize)
	}
	for range numPending - sendBufSize {
		if got := <-replies; !errors.Is(got.Err, ErrSendQueueFull) {
			t.Errorf("overflow call error = %v, want ErrSendQueueFull", got.Err)
		}
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

// TestInboundChannel verifies that an inbound channel sends a one-way request
// and confirms it without a routed response.
func TestInboundChannel(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 10})
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

// TestInboundChannelClose verifies that a closed inbound channel fails later
// requests with ErrNodeClosed.
func TestInboundChannelClose(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 10})

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

	if !c.session.ended() {
		t.Error("session still running after close")
	}
}

// TestInboundChannelStreamDown verifies that an inbound channel whose stream
// ended fails later requests with ErrNodeClosed instead of reconnecting.
func TestInboundChannelStreamDown(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 10})

	// Verify initial send works.
	resp := sendRequest(t, c, Request{Oneway: true}, 1)
	if resp.Err != nil {
		t.Fatalf("initial send failed: %v", resp.Err)
	}

	// The stream ends, as Serve observes, and the channel is closed, as the
	// NodeStream cleanup does.
	stream.close()
	if err := c.Serve(); err == nil {
		t.Fatal("Serve returned nil after the stream ended")
	}
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

	if !c.session.ended() {
		t.Error("inbound session still running after its stream ended")
	}
}

// BenchmarkChannelFirstRequest measures the latency of the first request,
// which includes stream creation.
//
// This benchmark creates a new server and node per iteration to measure true
// "cold start" latency. Due to TCP port exhaustion on macOS (ephemeral ports
// enter TIME_WAIT state and take time to recycle), this benchmark should be
// run with limited iterations (e.g., -benchtime=100x).
//
// Note: This benchmark includes server setup overhead, so absolute numbers
// should be interpreted with caution. The goal is to detect regressions.
func BenchmarkChannelFirstRequest(b *testing.B) {
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

// BenchmarkChannelReconnect measures the latency of a request that must
// reconnect after the stream ended.
func BenchmarkChannelReconnect(b *testing.B) {
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
		tc.endSessions()

		// Now send a request, which opens a new stream.
		ctx := context.Background()
		reqMsg, _ := NewMessage(ctx, uint64(i+1), mock.TestMethod, nil)
		req := Request{Ctx: ctx, Msg: reqMsg}
		replyChan := make(chan response, 1)
		req.ResponseChan = replyChan
		tc.Enqueue(req)

		select {
		case <-replyChan:
			// errors are ignored in benchmarks.
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
