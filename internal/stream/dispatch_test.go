package stream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/metadata"
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

// TestDispatchInboundBackChannel verifies how the receiver routes a
// server-initiated request and an unknown response.
func TestDispatchInboundBackChannel(t *testing.T) {
	t.Run("HandlerSeesMessageMetadata", func(t *testing.T) {
		const key, want = "request-id", "dedup-metadata"
		handlerMD := make(chan metadata.MD, 1)
		handler := requestHandlerFunc(func(ctx context.Context, _ *Message, release func(), _ func(*Message)) {
			defer release()
			md, _ := metadata.FromIncomingContext(ctx)
			handlerMD <- md
		})
		st := newMockBidiStream()
		t.Cleanup(st.close)
		c := NewInboundChannel(t.Context(), 1, 0, st, NewMessageRouter(handler))
		t.Cleanup(func() { _ = c.Close() })

		msgCtx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs(key, want))
		msg, err := NewMessage(msgCtx, ServerSequenceNumber(1), mock.TestMethod, nil)
		if err != nil {
			t.Fatalf("NewMessage: %v", err)
		}
		c.dispatchInbound(msg)
		select {
		case md := <-handlerMD:
			if got := md.Get(key); len(got) != 1 || got[0] != want {
				t.Fatalf("incoming metadata %q = %v, want [%q]", key, got, want)
			}
		case <-time.After(defaultTestTimeout):
			t.Fatal("handler was not called")
		}
	})

	t.Run("NoHandlerOrUnknownIDIsDropped", func(t *testing.T) {
		st := newMockBidiStream()
		t.Cleanup(st.close)
		c := NewInboundChannel(t.Context(), 1, 0, st, NewMessageRouter())
		t.Cleanup(func() { _ = c.Close() })
		c.dispatchInbound(Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build())
		c.dispatchInbound(Message_builder{MessageSeqNo: 999, Method: mock.TestMethod}.Build())
	})
}

// TestDispatcherOrder verifies that handlers start in push order, each after
// the previous one released or returned.
func TestDispatcherOrder(t *testing.T) {
	d := newDispatcher(nil, 0)
	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := range n {
		ok := d.push(t.Context(), func(release func()) {
			defer wg.Done()
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			if i%2 == 0 {
				release() // even handlers release early, odd ones on return
			}
		})
		if !ok {
			t.Fatalf("push %d failed", i)
		}
	}
	wg.Wait()
	for i, got := range order {
		if got != i {
			t.Fatalf("order = %v, want 0..%d", order, n-1)
		}
	}
}

// TestDispatcherReleaseAdmitsNext verifies that a release before return starts
// the next handler, and that without a release the next handler waits.
func TestDispatcherReleaseAdmitsNext(t *testing.T) {
	d := newDispatcher(nil, 0)
	releaseFirst := make(chan func())
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	secondStarted := make(chan struct{})
	d.push(t.Context(), func(release func()) {
		releaseFirst <- release
		<-block
	})
	d.push(t.Context(), func(func()) { close(secondStarted) })

	release := <-releaseFirst
	select {
	case <-secondStarted:
		t.Fatal("second handler started before the first released")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	select {
	case <-secondStarted:
	case <-time.After(defaultTestTimeout):
		t.Fatal("second handler did not start after release")
	}
}

// TestDispatcherBounded verifies that a full queue makes tryPush fail and push
// wait, and that push ends with its context or the dispatcher's done channel.
func TestDispatcherBounded(t *testing.T) {
	done := make(chan struct{})
	d := newDispatcher(done, 1)
	block := make(chan struct{})
	defer close(block)
	running := make(chan struct{})
	d.push(t.Context(), func(func()) { close(running); <-block }) // running
	<-running
	if !d.tryPush(func(func()) {}) { // queued
		t.Fatal("tryPush failed with space in the queue")
	}
	if d.tryPush(func(func()) {}) {
		t.Fatal("tryPush succeeded on a full queue")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if d.push(ctx, func(func()) {}) {
		t.Fatal("push succeeded on a full queue")
	}

	pushed := make(chan bool)
	go func() { pushed <- d.push(t.Context(), func(func()) {}) }()
	close(done)
	select {
	case ok := <-pushed:
		if ok {
			t.Fatal("push succeeded after the dispatcher stopped")
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("push did not end when the dispatcher stopped")
	}
}

// TestLocalChannelReentrantCall verifies that a local handler which calls its
// own node before releasing waits only as long as its context allows, and that
// the nested call runs once the handler returns.
func TestLocalChannelReentrantCall(t *testing.T) {
	var c *Channel
	nestedReply := make(chan response, 1)
	outerDone := make(chan error, 1)
	handler := requestHandlerFunc(func(ctx context.Context, msg *Message, _ func(), send func(*Message)) {
		if msg.GetMessageSeqNo() != 1 {
			send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo()}.Build())
			return
		}
		nestedCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		c.Enqueue(Request{
			Ctx:          context.Background(),
			Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
			ResponseChan: nestedReply,
		})
		select {
		case <-nestedReply:
			outerDone <- errors.New("nested call ran before the outer handler released")
		case <-nestedCtx.Done():
			outerDone <- nil
		}
	})
	c = NewLocalChannel(1, NewMessageRouter(handler))

	c.Enqueue(Request{
		Ctx:          t.Context(),
		Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		ResponseChan: make(chan response, 1),
	})
	select {
	case err := <-outerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTestTimeout):
		t.Fatal("outer handler deadlocked on its nested call")
	}
	select {
	case <-nestedReply:
	case <-time.After(defaultTestTimeout):
		t.Fatal("nested call did not run after the outer handler returned")
	}
}
