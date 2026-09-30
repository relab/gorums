package stream

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
)

// TestDispatchInboundKeepsReadingWhileHandlerUnreleased verifies that the
// receiver can enqueue the next request and deliver a reply while a handler
// has not released, and that the second request still waits for that release.
func TestDispatchInboundKeepsReadingWhileHandlerUnreleased(t *testing.T) {
	const nodeID = uint32(1)
	releaseFirst := make(chan struct{})
	started := make(chan uint64, 2)
	handler := requestHandlerFunc(func(_ context.Context, msg *Message, release func(), _ func(*Message)) {
		started <- msg.GetMessageSeqNo()
		if msg.GetMessageSeqNo() == ServerSequenceNumber(1) {
			<-releaseFirst
		}
		release()
	})
	router := NewMessageRouter(handler)
	st := newMockBidiStream()
	t.Cleanup(st.close)
	c := NewInboundChannel(context.Background(), nodeID, 0, st, router)
	t.Cleanup(func() { _ = c.Close() })

	replyCh := make(chan response, 1)
	router.Register(42, Request{
		Ctx:          context.Background(),
		Msg:          &Message{},
		ResponseChan: replyCh,
	})

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		c.dispatchInbound(Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build())
		c.dispatchInbound(Message_builder{MessageSeqNo: ServerSequenceNumber(2), Method: mock.TestMethod}.Build())
		c.dispatchInbound(Message_builder{MessageSeqNo: 42, Method: mock.TestMethod}.Build())
	}()

	select {
	case id := <-started:
		if id != ServerSequenceNumber(1) {
			t.Fatalf("first handler id = %d, want %d", id, ServerSequenceNumber(1))
		}
	case <-time.After(time.Second):
		t.Fatal("first handler did not start")
	}

	select {
	case <-replyCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reply was not delivered while a handler was unreleased")
	}

	select {
	case id := <-started:
		t.Fatalf("handler %d started before the first handler released", id)
	default:
	}

	select {
	case <-readerDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("reader blocked dispatching the next request behind an unreleased handler")
	}

	close(releaseFirst)

	select {
	case id := <-started:
		if id != ServerSequenceNumber(2) {
			t.Fatalf("second handler id = %d, want %d", id, ServerSequenceNumber(2))
		}
	case <-time.After(time.Second):
		t.Fatal("second handler did not start after the first handler released")
	}
}
