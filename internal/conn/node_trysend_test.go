package conn

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
)

// blockingBidiStream is a [stream.BidiStream] whose Send blocks until
// release() is called, simulating a backpressured or unresponsive peer link.
// It signals entered once a Send call is in progress, so a test can wait
// until the channel's sender goroutine is durably occupied before proceeding.
type blockingBidiStream struct {
	entered chan struct{}
	release chan struct{}
	closed  chan struct{}
}

func newBlockingBidiStream() *blockingBidiStream {
	return &blockingBidiStream{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (s *blockingBidiStream) Send(*stream.Message) error {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	select {
	case <-s.release:
		return nil
	case <-s.closed:
		return context.Canceled
	}
}

func (s *blockingBidiStream) Recv() (*stream.Message, error) {
	<-s.closed
	return nil, context.Canceled
}

func (s *blockingBidiStream) close() { close(s.closed) }

// TestPeerNodeTrySendDoesNotBlockOnStuckTransport verifies that
// peerNode.TrySend returns while the peer's transport is stuck. In
// [stream.Server.NodeStream], a handler hands its reply to a goroutine that
// drains a bounded "finished" channel by calling peerNode.TrySend, and the
// next handler's reply waits on that channel until the call returns.
//
// This exercises the real chain the drain goroutine depends on: peerNode ->
// Node.trySend -> stream.Transport.TrySend -> stream.Channel.TrySend. The test
// hangs if any link in that chain blocks.
func TestPeerNodeTrySendDoesNotBlockOnStuckTransport(t *testing.T) {
	transportStream := newBlockingBidiStream()
	t.Cleanup(transportStream.close)

	router := stream.NewMessageRouter()
	ch := stream.NewInboundChannel(t.Context(), 1, 0, transportStream, router)
	n := newTestNode(1, router, ch)

	// Occupy the sender: this one-way request is handed to the channel's
	// sender goroutine, which then blocks in Send on a transport that never
	// drains (a full or backpressured link during a teardown broadcast).
	n.loadTransport().Enqueue(stream.Request{
		Ctx:    t.Context(),
		Oneway: true,
		Msg:    stream.Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
	})
	select {
	case <-transportStream.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("sender never entered Send")
	}

	// TrySend is exactly what the NodeStream drain goroutine calls to
	// deliver a handler's reply; it must return promptly even though the
	// sender above is durably stuck.
	p := peerNode{n: n}
	done := make(chan struct{})
	go func() {
		p.TrySend(stream.Request{
			Ctx: t.Context(),
			Msg: stream.Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("peerNode.TrySend blocked on a stuck send queue")
	}
}

// TestPeerNodeTrySendReportsSendQueueFull verifies that a two-way request
// enqueued via TrySend on a stuck transport fails fast with ErrSendQueueFull
// through the real peerNode adapter, matching the two-way trySend contract of
// [stream.Channel].
func TestPeerNodeTrySendReportsSendQueueFull(t *testing.T) {
	transportStream := newBlockingBidiStream()
	t.Cleanup(transportStream.close)

	router := stream.NewMessageRouter()
	ch := stream.NewInboundChannel(t.Context(), 1, 0, transportStream, router)
	n := newTestNode(1, router, ch)

	n.loadTransport().Enqueue(stream.Request{
		Ctx:    t.Context(),
		Oneway: true,
		Msg:    stream.Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
	})
	select {
	case <-transportStream.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("sender never entered Send")
	}

	p := peerNode{n: n}
	reply := make(chan stream.NodeResponse[*stream.Message], 1)
	p.TrySend(stream.Request{
		Ctx:          t.Context(),
		ResponseChan: reply,
		Msg:          stream.Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
	})

	select {
	case r := <-reply:
		if !errors.Is(r.Err, stream.ErrSendQueueFull) {
			t.Errorf("TrySend reply error = %v, want ErrSendQueueFull", r.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TrySend did not fail the request when the queue was full")
	}
}
