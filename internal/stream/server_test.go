package stream

import (
	"context"
	"errors"
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
	ch.Enqueue(NodeRequest{
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
