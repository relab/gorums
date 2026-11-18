package stream

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fakeNodeStream is a minimal Gorums_NodeStreamServer for driving
// Server.NodeStream directly. Its gated mock stream holds every Send until
// Release, simulating a backpressured or unresponsive link, and its Recv
// returns the messages queued with Deliver.
type fakeNodeStream struct {
	*mock.BidiStream[*Message]
	ctx context.Context
}

func newFakeNodeStream(ctx context.Context) *fakeNodeStream {
	return &fakeNodeStream{BidiStream: mock.NewGatedBidiStream[*Message](), ctx: ctx}
}

func (f *fakeNodeStream) Context() context.Context { return f.ctx }

// The remaining methods satisfy grpc.ServerStream; NodeStream never calls them.
func (*fakeNodeStream) SetHeader(metadata.MD) error { return nil }
func (*fakeNodeStream) SetTrailer(metadata.MD)      {}
func (*fakeNodeStream) SendMsg(any) error           { return nil }
func (*fakeNodeStream) RecvMsg(any) error           { return nil }

var _ Gorums_NodeStreamServer = (*fakeNodeStream)(nil)

// channelAcceptor is a [PeerAcceptor] that returns a prepared channel.
type channelAcceptor struct {
	ch *InboundChannel
}

func (a channelAcceptor) AcceptPeer(context.Context, InboundStream) (*InboundChannel, func(), error) {
	return a.ch, func() {}, nil
}

// inboundAcceptor is a [PeerAcceptor] that accepts each stream on a new
// inbound channel, or refuses it if refuse is set.
type inboundAcceptor struct {
	refuse bool
}

func (a inboundAcceptor) AcceptPeer(ctx context.Context, stream InboundStream) (*InboundChannel, func(), error) {
	if a.refuse {
		return nil, func() {}, status.Error(codes.Unauthenticated, "stream refused by test acceptor")
	}
	ch := NewInboundChannel(ctx, 0, stream, InboundOptions{RecvBufferSize: testRecvBufferSize})
	return ch, func() { _ = ch.Close() }, nil
}

// headerInterceptor sends response headers before the handler runs, as
// tracing middleware may do, so the handler cannot send headers of its own.
func headerInterceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := ss.SendHeader(metadata.Pairs("x-trace-id", "test")); err != nil {
		return err
	}
	return handler(srv, ss)
}

// TestServerNodeStreamAcceptance verifies that a dialer's stream counts as up
// once the server's acceptor accepts it, with no request sent, and never if
// the acceptor refuses it. This holds also when an interceptor has already
// sent the response headers.
func TestServerNodeStreamAcceptance(t *testing.T) {
	tests := []struct {
		name    string
		refuse  bool
		srvOpts []grpc.ServerOption
	}{
		{name: "Accepted"},
		{name: "Refused", refuse: true},
		{name: "AcceptedAfterInterceptorHeaders", srvOpts: []grpc.ServerOption{grpc.StreamInterceptor(headerInterceptor)}},
		{name: "RefusedAfterInterceptorHeaders", refuse: true, srvOpts: []grpc.ServerOption{grpc.StreamInterceptor(headerInterceptor)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var changes atomic.Int32
			opts := OutboundOptions{RecvBufferSize: testRecvBufferSize, EagerReconnect: true, OnStreamChange: func() { changes.Add(1) }}
			tc := setupChannelOptions(t, opts, NewServer(nil, inboundAcceptor{refuse: tt.refuse}).NodeStream, tt.srvOpts...)
			if !tt.refuse {
				if !waitUntil(defaultTestTimeout, tc.StreamUp) {
					t.Fatal("StreamUp() = false for a stream the server accepted, want true")
				}
				return
			}
			if !waitUntil(defaultTestTimeout, func() bool { return status.Code(tc.LastErr()) == codes.Unauthenticated }) {
				t.Fatalf("LastErr = %v, want code %v", tc.LastErr(), codes.Unauthenticated)
			}
			if tc.StreamUp() || changes.Load() != 0 {
				t.Errorf("StreamUp() = %t after %d changes for a refused stream, want false after 0", tc.StreamUp(), changes.Load())
			}
		})
	}
}

// TestServerNodeStreamReplyDoesNotWedgeReceiveLoop checks the server-side half of
// the teardown deadlock against [Server.NodeStream]: with the send queue full,
// each handler's reply is dropped instead of waiting, so every inbound request
// is still dispatched in turn.
func TestServerNodeStreamReplyDoesNotWedgeReceiveLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := newFakeNodeStream(ctx)
	t.Cleanup(fs.Close)

	dispatched := make(chan uint64, 8)
	echo := requestHandlerFunc(func(_ context.Context, msg *Message, release func(), send func(*Message)) {
		defer release()
		dispatched <- msg.GetMessageSeqNo()
		send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo(), Method: mock.TestMethod}.Build())
	})
	// Capacity 0: once the send loop is occupied in Send, the queue has no
	// slack, so a reply must be dropped rather than wait for space.
	ch := NewInboundChannel(ctx, 1, fs, InboundOptions{RecvBufferSize: testRecvBufferSize, Handler: echo})
	t.Cleanup(func() { _ = ch.Close() })
	srv := NewServer(nil, channelAcceptor{ch: ch})

	done := make(chan error, 1)
	go func() { done <- srv.NodeStream(fs) }()

	// Occupy the send loop: this one-way request blocks in Send on a
	// transport that never drains.
	ch.queue.push(NodeRequest{
		Ctx:    ctx,
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 100, Method: mock.TestMethod}.Build(),
	})
	select {
	case <-fs.Entered():
	case <-time.After(2 * time.Second):
		t.Fatal("sender never entered Send")
	}

	// Each request starts only after the previous handler returned, which it
	// does only if its reply did not block.
	if err := fs.Deliver(Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build()); err != nil {
		t.Fatalf("Deliver() error: %v", err)
	}
	if err := fs.Deliver(Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build()); err != nil {
		t.Fatalf("Deliver() error: %v", err)
	}
	if err := fs.Deliver(Message_builder{MessageSeqNo: 3, Method: mock.TestMethod}.Build()); err != nil {
		t.Fatalf("Deliver() error: %v", err)
	}

	got := make(map[uint64]bool)
	for len(got) < 3 {
		select {
		case id := <-dispatched:
			got[id] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("dispatched = %v; want all three inbound requests dispatched", got)
		}
	}

	fs.Release()
	fs.Close() // NodeStream's Recv now returns an error and it exits.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("NodeStream did not return after the stream closed")
	}
}

// handlerAcceptor is a [PeerAcceptor] that accepts each stream on a new
// inbound channel that serves requests with handler. It stores the latest
// channel in last.
type handlerAcceptor struct {
	handler RequestHandler
	last    atomic.Pointer[InboundChannel]
}

func (a *handlerAcceptor) AcceptPeer(ctx context.Context, stream InboundStream) (*InboundChannel, func(), error) {
	ch := NewInboundChannel(ctx, 0, stream, InboundOptions{SendBufferSize: 10, RecvBufferSize: testRecvBufferSize, Handler: a.handler})
	a.last.Store(ch)
	return ch, func() { _ = ch.Close() }, nil
}

// gracefulStop runs srv.GracefulStop and returns a channel that is closed
// when it returns.
func gracefulStop(srv *grpc.Server) <-chan struct{} {
	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	return stopped
}

// TestServerNodeStreamWaitsForHandlers verifies that gRPC's GracefulStop,
// which waits for NodeStream, waits for the stream's handlers. After a GOAWAY
// the dialer closes its side of the stream, and the server sends the replies
// of the handlers that finish later. If the dialer disconnects instead, the
// server still waits for the running handler. If the dialer disconnects after
// it closed its side, the server still runs every request it received.
func TestServerNodeStreamWaitsForHandlers(t *testing.T) {
	t.Run("Reply", func(t *testing.T) {
		gate := make(chan struct{})
		started := make(chan struct{}, 1)
		acceptor := &handlerAcceptor{handler: requestHandlerFunc(func(_ context.Context, msg *Message, _ func(), send func(*Message)) {
			started <- struct{}{}
			<-gate
			send(NewResponse(msg, nil, nil))
		})}
		tc := setupChannel(t, NewServer(nil, acceptor).NodeStream)
		reply := make(chan response, 1)
		go func() {
			resp, _ := trySendRequest(tc.OutboundChannel, NodeRequest{}, 1)
			reply <- resp
		}()
		<-started

		stopped := gracefulStop(tc.srv)
		select {
		case <-stopped:
			t.Fatal("GracefulStop returned while a handler was blocked")
		case <-time.After(200 * time.Millisecond):
		}
		close(gate)
		if resp := <-reply; resp.Err != nil {
			t.Errorf("reply after GOAWAY: %v", resp.Err)
		}
		select {
		case <-stopped:
		case <-time.After(defaultTestTimeout):
			t.Fatal("GracefulStop did not return after the handler returned")
		}
	})
	t.Run("Disconnect", func(t *testing.T) {
		gate := make(chan struct{})
		started := make(chan struct{}, 1)
		var finished atomic.Bool
		acceptor := &handlerAcceptor{handler: requestHandlerFunc(func(context.Context, *Message, func(), func(*Message)) {
			started <- struct{}{}
			<-gate
			finished.Store(true)
		})}
		tc := setupChannel(t, NewServer(nil, acceptor).NodeStream)
		if resp := sendRequest(t, tc.OutboundChannel, NodeRequest{Oneway: true}, 1); resp.Err != nil {
			t.Fatalf("one-way send: %v", resp.Err)
		}
		<-started
		if err := tc.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		stopped := gracefulStop(tc.srv)
		select {
		case <-stopped:
			t.Fatal("GracefulStop returned while a disconnected stream's handler was blocked")
		case <-time.After(200 * time.Millisecond):
		}
		close(gate)
		select {
		case <-stopped:
		case <-time.After(defaultTestTimeout):
			t.Fatal("GracefulStop did not return after the handler returned")
		}
		if !finished.Load() {
			t.Error("GracefulStop returned before the handler finished")
		}
	})
	t.Run("DisconnectAfterHalfClose", func(t *testing.T) {
		gate := make(chan struct{})
		started := make(chan struct{}, 2)
		var finished atomic.Int32
		acceptor := &handlerAcceptor{handler: requestHandlerFunc(func(context.Context, *Message, func(), func(*Message)) {
			started <- struct{}{}
			<-gate
			finished.Add(1)
		})}
		tc := setupChannel(t, NewServer(nil, acceptor).NodeStream)
		for id := range uint64(2) {
			if resp := sendRequest(t, tc.OutboundChannel, NodeRequest{Oneway: true}, id+1); resp.Err != nil {
				t.Fatalf("one-way send %d: %v", id+1, resp.Err)
			}
		}
		<-started

		stopped := gracefulStop(tc.srv)
		ch := acceptor.last.Load()
		if !waitUntil(defaultTestTimeout, ch.session.peerClosed.Load) {
			t.Fatal("the server did not see the client close its side of the stream")
		}
		if err := tc.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if !waitUntil(defaultTestTimeout, func() bool { return ch.ctx.Err() != nil }) {
			t.Fatal("the server did not see the client disconnect")
		}
		close(gate)
		select {
		case <-stopped:
		case <-time.After(defaultTestTimeout):
			t.Fatal("GracefulStop did not return after the handlers returned")
		}
		if got := finished.Load(); got != 2 {
			t.Errorf("finished handlers = %d, want 2", got)
		}
	})
}

// TestServerNodeStreamFailsCallsToClosedDialer verifies that a call that the
// server sent to the dialer fails with ErrStreamDown once the dialer has
// closed its side of the stream, since the dialer can no longer reply. A
// handler that waits for such a call therefore does not stall GracefulStop.
func TestServerNodeStreamFailsCallsToClosedDialer(t *testing.T) {
	callErr := make(chan error, 1)
	started := make(chan struct{})
	acceptor := &handlerAcceptor{}
	acceptor.handler = requestHandlerFunc(func(ctx context.Context, _ *Message, _ func(), _ func(*Message)) {
		// The dialer has no handler, so it never answers this call.
		respCh := make(chan response, 1)
		acceptor.last.Load().Enqueue(NodeRequest{
			Ctx:          ctx,
			Msg:          NewRequestFromPayload(ctx, ServerSequenceNumber(1), mock.TestMethod, nil),
			ResponseChan: respCh,
		})
		close(started)
		callErr <- (<-respCh).Err
	})
	tc := setupChannel(t, NewServer(nil, acceptor).NodeStream)
	if resp := sendRequest(t, tc.OutboundChannel, NodeRequest{Oneway: true}, 1); resp.Err != nil {
		t.Fatalf("one-way send: %v", resp.Err)
	}
	<-started

	stopped := gracefulStop(tc.srv)
	select {
	case err := <-callErr:
		if !errors.Is(err, ErrStreamDown) {
			t.Errorf("call to the closed dialer: err = %v, want %v", err, ErrStreamDown)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("the call to the closed dialer did not fail")
	}
	select {
	case <-stopped:
	case <-time.After(defaultTestTimeout):
		t.Fatal("GracefulStop did not return")
	}
}

// senderStream is a stream that a sender opened to srv, served by
// [Server.NodeStream] on a fake stream.
type senderStream struct {
	fs     *fakeNodeStream
	result chan error // NodeStream's error
}

// openSenderStream serves a stream with the sender metadata id, as
// "<token>.<n>", and draining, which may be empty.
func openSenderStream(t *testing.T, srv *Server, id, draining string) *senderStream {
	t.Helper()
	md := metadata.Pairs(streamKey, id)
	if draining != "" {
		md.Append(drainingKey, draining)
	}
	ctx, cancel := context.WithCancel(metadata.NewIncomingContext(t.Context(), md))
	ss := &senderStream{
		fs:     &fakeNodeStream{BidiStream: mock.NewBidiStream[*Message](), ctx: ctx},
		result: make(chan error, 1),
	}
	go func() { ss.result <- srv.NodeStream(ss.fs) }()
	t.Cleanup(func() {
		ss.fs.Close()
		cancel()
	})
	return ss
}

// deliver delivers a request with message ID id on ss.
func (ss *senderStream) deliver(t *testing.T, id uint64) {
	t.Helper()
	if err := ss.fs.Deliver(Message_builder{MessageSeqNo: id, Method: mock.TestMethod}.Build()); err != nil {
		t.Fatalf("Deliver(%d): %v", id, err)
	}
}

// waitResult returns the error that ss's NodeStream returned.
func (ss *senderStream) waitResult(t *testing.T) error {
	t.Helper()
	select {
	case err := <-ss.result:
		return err
	case <-time.After(defaultTestTimeout):
		t.Fatal("NodeStream did not return")
		return nil
	}
}

// orderHandler records the requests it starts and holds each one whose
// message ID has a gate until the gate closes. A handler whose gate is in
// releases instead releases at once and holds until the gate closes.
type orderHandler struct {
	started  chan uint64
	gates    map[uint64]chan struct{}
	releases map[uint64]bool
}

func newOrderHandler() *orderHandler {
	return &orderHandler{started: make(chan uint64, 16), gates: make(map[uint64]chan struct{}), releases: make(map[uint64]bool)}
}

// hold makes request id wait for its gate; release makes it release first.
func (h *orderHandler) hold(id uint64, release bool) chan struct{} {
	h.gates[id] = make(chan struct{})
	h.releases[id] = release
	return h.gates[id]
}

func (h *orderHandler) HandleRequest(_ context.Context, _ ID, msg *Message, release func(), _ func(*Message)) {
	id := msg.GetMessageSeqNo()
	h.started <- id
	if gate, ok := h.gates[id]; ok {
		if h.releases[id] {
			release()
		}
		<-gate
	}
}

// expectStart fails t unless request want starts next.
func (h *orderHandler) expectStart(t *testing.T, want uint64) {
	t.Helper()
	select {
	case got := <-h.started:
		if got != want {
			t.Fatalf("request %d started, want %d", got, want)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatalf("request %d did not start", want)
	}
}

// expectNoStart fails t if a request starts within a short time.
func (h *orderHandler) expectNoStart(t *testing.T) {
	t.Helper()
	select {
	case got := <-h.started:
		t.Fatalf("request %d started, want none", got)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestServerNodeStreamRefusesStaleStream verifies that the server refuses a
// stream whose number is not above the sender's latest stream, with
// codes.Aborted and before any of its requests run. Another sender's streams
// are independent.
func TestServerNodeStreamRefusesStaleStream(t *testing.T) {
	h := newOrderHandler()
	acceptor := &handlerAcceptor{handler: h}
	srv := NewServer(nil, acceptor)

	openSenderStream(t, srv, "tok.2", "").deliver(t, 1)
	h.expectStart(t, 1)
	for _, id := range []string{"tok.1", "tok.2"} {
		if err := openSenderStream(t, srv, id, "").waitResult(t); status.Code(err) != codes.Aborted {
			t.Errorf("stream %s: NodeStream() = %v, want code %v", id, err, codes.Aborted)
		}
	}
	openSenderStream(t, srv, "other.1", "").deliver(t, 2)
	h.expectStart(t, 2)
}

// TestServerNodeStreamSupersedes verifies that a newer stream closes the
// sender's older channel, which drops the requests queued there, and that the
// newer stream's first request starts only after the older channel's running
// request returns or releases.
func TestServerNodeStreamSupersedes(t *testing.T) {
	t.Run("Return", func(t *testing.T) {
		h := newOrderHandler()
		gate := h.hold(1, false)
		srv := NewServer(nil, &handlerAcceptor{handler: h})

		s1 := openSenderStream(t, srv, "tok.1", "")
		s1.deliver(t, 1)
		h.expectStart(t, 1)
		s1.deliver(t, 2) // queued behind request 1

		openSenderStream(t, srv, "tok.2", "").deliver(t, 3)
		h.expectNoStart(t)
		close(gate)
		h.expectStart(t, 3)
		h.expectNoStart(t) // request 2 was dropped
	})
	t.Run("Release", func(t *testing.T) {
		h := newOrderHandler()
		gate := h.hold(1, true)
		t.Cleanup(func() { close(gate) })
		srv := NewServer(nil, &handlerAcceptor{handler: h})

		openSenderStream(t, srv, "tok.1", "").deliver(t, 1)
		h.expectStart(t, 1)
		openSenderStream(t, srv, "tok.2", "").deliver(t, 3)
		h.expectStart(t, 3) // request 1 released, though it still runs
	})
}

// TestServerNodeStreamKeepsDrainingStream verifies that a newer stream keeps
// an older stream that it lists as draining: the older stream runs all its
// requests, and the newer stream's requests start only after the older stream
// has ended and its last request has returned.
func TestServerNodeStreamKeepsDrainingStream(t *testing.T) {
	h := newOrderHandler()
	gate := h.hold(1, false)
	srv := NewServer(nil, &handlerAcceptor{handler: h})

	s1 := openSenderStream(t, srv, "tok.1", "")
	s1.deliver(t, 1)
	h.expectStart(t, 1)
	s1.deliver(t, 2)

	s2 := openSenderStream(t, srv, "tok.2", "1")
	s2.deliver(t, 3)
	close(gate)
	h.expectStart(t, 2)
	h.expectNoStart(t) // stream 1 has not ended

	s1.fs.Close() // the sender half-closes the draining stream
	h.expectStart(t, 3)
	if err := s1.waitResult(t); err != nil {
		t.Errorf("draining stream: NodeStream() = %v, want nil", err)
	}
}

// TestServerNodeStreamSenderGrace verifies that the server keeps a sender's
// record for its grace period after the sender's last stream ended, so a late
// stale stream is still refused, and forgets it afterwards.
func TestServerNodeStreamSenderGrace(t *testing.T) {
	srv := NewServer(nil, &handlerAcceptor{handler: newOrderHandler()})
	srv.grace = 100 * time.Millisecond

	s2 := openSenderStream(t, srv, "tok.2", "")
	s2.fs.Close()
	if err := s2.waitResult(t); err != nil {
		t.Fatalf("NodeStream() = %v, want nil", err)
	}
	if err := openSenderStream(t, srv, "tok.1", "").waitResult(t); status.Code(err) != codes.Aborted {
		t.Fatalf("stale stream within the grace period: NodeStream() = %v, want code %v", err, codes.Aborted)
	}
	if !waitUntil(defaultTestTimeout, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return len(srv.senders) == 0
	}) {
		t.Fatal("the server kept the sender's record after the grace period")
	}
}

// TestServerRunsRequestsInOrderUnderStreamChurn verifies that the server runs
// one channel's requests in the order the channel sent them, while the
// channel's streams break and reopen. A request may be lost with its stream,
// but no request runs after a later one.
func TestServerRunsRequestsInOrderUnderStreamChurn(t *testing.T) {
	var mu sync.Mutex
	var ran []uint64  // request IDs, in the order they started
	var from []string // the stream of each request in ran
	acceptor := &handlerAcceptor{handler: requestHandlerFunc(func(ctx context.Context, msg *Message, _ func(), _ func(*Message)) {
		md, _ := metadata.FromIncomingContext(ctx)
		mu.Lock()
		ran = append(ran, msg.GetMessageSeqNo())
		from = append(from, strings.Join(md.Get(streamKey), ","))
		mu.Unlock()
		time.Sleep(50 * time.Microsecond)
	})}
	tc := setupChannel(t, NewServer(nil, acceptor).NodeStream)
	if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
		t.Fatal("channel never connected")
	}

	churnDone := make(chan struct{})
	go func() {
		defer close(churnDone)
		for start := time.Now(); time.Since(start) < 200*time.Millisecond; {
			time.Sleep(time.Millisecond)
			tc.endSessions()
		}
	}()
	var last uint64
	for id := uint64(1); ; id++ {
		select {
		case <-churnDone:
		default:
			_ = tc.Enqueue(NodeRequest{
				Ctx:    t.Context(),
				Oneway: true,
				Msg:    Message_builder{MessageSeqNo: id, Method: mock.TestMethod}.Build(),
			})
			continue
		}
		last = id
		break
	}

	// After the churn, a final request runs once the server has run every
	// earlier request that it will run.
	final := NodeRequest{
		Ctx:    t.Context(),
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: last, Method: mock.TestMethod}.Build(),
	}
	if !waitUntil(defaultTestTimeout, func() bool { return tc.Enqueue(final) == nil }) {
		t.Fatal("the send queue did not admit the final request")
	}
	if !waitUntil(defaultTestTimeout, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ran) > 0 && ran[len(ran)-1] == last
	}) {
		t.Fatal("the final request did not run")
	}

	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(ran); i++ {
		if ran[i] <= ran[i-1] {
			t.Fatalf("request %d (stream %s) ran after request %d (stream %s)", ran[i], from[i], ran[i-1], from[i-1])
		}
	}
}
