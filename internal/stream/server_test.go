package stream

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
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
func (*fakeNodeStream) SetHeader(metadata.MD) error  { return nil }
func (*fakeNodeStream) SendHeader(metadata.MD) error { return nil }
func (*fakeNodeStream) SetTrailer(metadata.MD)       {}
func (*fakeNodeStream) SendMsg(any) error            { return nil }
func (*fakeNodeStream) RecvMsg(any) error            { return nil }

var _ Gorums_NodeStreamServer = (*fakeNodeStream)(nil)

// channelAcceptor is a [PeerAcceptor] that returns a prepared channel.
type channelAcceptor struct {
	ch *InboundChannel
}

func (a channelAcceptor) AcceptPeer(context.Context, BidiStream) (*InboundChannel, func(), error) {
	return a.ch, func() {}, nil
}

// inboundAcceptor is a [PeerAcceptor] that accepts each stream on a new
// inbound channel, or refuses it if refuse is set.
type inboundAcceptor struct {
	refuse bool
}

func (a inboundAcceptor) AcceptPeer(ctx context.Context, stream BidiStream) (*InboundChannel, func(), error) {
	if a.refuse {
		return nil, func() {}, status.Error(codes.Unauthenticated, "stream refused by test acceptor")
	}
	ch := NewInboundChannel(ctx, 0, stream, InboundOptions{RecvBufferSize: testRecvBufferSize})
	return ch, func() { _ = ch.Close() }, nil
}

// TestServerNodeStreamAcceptance verifies that a dialer's stream counts as up
// once the server's acceptor accepts it, with no request sent, and never if
// the acceptor refuses it.
func TestServerNodeStreamAcceptance(t *testing.T) {
	tests := []struct {
		name   string
		refuse bool
	}{
		{name: "Accepted"},
		{name: "Refused", refuse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var changes atomic.Int32
			opts := OutboundOptions{RecvBufferSize: testRecvBufferSize, EagerReconnect: true, OnStreamChange: func() { changes.Add(1) }}
			tc := setupChannelOptions(t, opts, NewServer(nil, inboundAcceptor{refuse: tt.refuse}).NodeStream)
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
	ch.Enqueue(Request{
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
