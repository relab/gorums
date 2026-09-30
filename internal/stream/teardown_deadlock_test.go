package stream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc/metadata"
)

// TestReceiverDispatchNotWedgedByReentrantReply verifies that a back-channel
// handler can reply on a full send queue and still let the next request run.
// The reply uses [Channel.trySend] (see [Channel.dispatchInbound]), so it
// returns immediately. The router's dispatch lock stays free: the receiver
// enqueues the request and does not hold that lock across the handler.
// The test fails if the reply blocks.
//
// The test asserts on the dispatch lock directly, since a goroutine waiting on
// sync.Mutex.Lock is not "durably blocked" for synctest's deadlock detection
// (see testing/synctest). synctest.Wait only settles the goroutines before the
// assertion.
func TestReceiverDispatchNotWedgedByReentrantReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const nodeID = uint32(1)

		// The handler answers every request on the same channel it was
		// dispatched on: the reentrant back-channel reply.
		dispatched := make(chan uint64, 8)
		handler := requestHandlerFunc(func(_ context.Context, msg *Message, release func(), send func(*Message)) {
			defer release()
			dispatched <- msg.GetMessageSeqNo()
			send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo(), Method: mock.TestMethod}.Build())
		})
		r := NewMessageRouter(handler)

		// Capacity 0: once the sender goroutine is occupied in Send, the queue
		// has no slack, so a reply would have to wait for space.
		stream := newBlockingSendStream()
		c := NewInboundChannel(context.Background(), nodeID, 0, stream, r)
		defer func() {
			// Release the blocked Send and cancel the connection so the sender
			// and any goroutine still blocked on the queue can exit before the
			// bubble's root returns.
			stream.close()
			_ = c.Close()
			synctest.Wait()
		}()

		// Occupy the sender: this one-way request is handed to the sender
		// goroutine, which then blocks in Send on a transport that never drains
		// (a full or backpressured link during the teardown broadcast).
		c.Enqueue(Request{
			Ctx:    context.Background(),
			Oneway: true,
			Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait() // the sender is now durably blocked in Send

		// Dispatch a back-channel request exactly as the receiver loop does.
		// The handler replies on the same, now-full channel.
		first := Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build()
		c.dispatchInbound(first)
		synctest.Wait() // let the handler reply and (with the fix) return

		// Invariant: after the handler's reentrant reply, the dispatch lock is
		// free and the next request can run.
		if !r.dispatchMu.TryLock() {
			t.Fatal("dispatch lock still held: a back-channel reply blocked on a full send queue while holding it, deadlocking the receiver's dispatch loop")
		}
		r.dispatchMu.Unlock()

		// The lock is free: a second back-channel request must still dispatch,
		// i.e. the next dispatch is acquired in bounded time.
		second := Message_builder{MessageSeqNo: ServerSequenceNumber(2), Method: mock.TestMethod}.Build()
		c.dispatchInbound(second)
		synctest.Wait()

		got := make(map[uint64]bool)
		for {
			select {
			case id := <-dispatched:
				got[id] = true
				continue
			default:
			}
			break
		}
		if !got[ServerSequenceNumber(1)] || !got[ServerSequenceNumber(2)] {
			t.Fatalf("dispatched handlers = %v; want both back-channel requests dispatched", got)
		}
	})
}

// TestTrySendDoesNotBlockOnFullQueue is a focused check that the non-blocking
// enqueue used for back-channel replies returns immediately on a full send
// queue, even with a background (deadline-free) context. A back-channel reply
// carries only the connection context, so trySend makes progress without
// context cancellation, which keeps a reply from wedging the receiver. The
// one-way [Channel.Enqueue] path waits until the request context is done (see
// TestChannelEnqueueRespectsRequestContext).
func TestTrySendDoesNotBlockOnFullQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const nodeID = uint32(1)
		stream := newBlockingSendStream()
		c := NewInboundChannel(context.Background(), nodeID, 0, stream, NewMessageRouter())
		defer func() {
			stream.close()
			_ = c.Close()
			synctest.Wait()
		}()

		// Occupy the sender so the queue is full and cannot drain.
		c.Enqueue(Request{
			Ctx:    context.Background(),
			Oneway: true,
			Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()

		reply := make(chan response, 1)
		returned := make(chan struct{})
		go func() {
			c.trySend(Request{
				Ctx:          context.Background(),
				ResponseChan: reply,
				Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
			})
			close(returned)
		}()
		synctest.Wait()

		select {
		case <-returned:
		default:
			t.Fatal("trySend blocked on a full send queue with a background context")
		}
		select {
		case resp := <-reply:
			if !errors.Is(resp.Err, ErrSendQueueFull) {
				t.Errorf("trySend reply error = %v, want ErrSendQueueFull", resp.Err)
			}
		default:
			t.Fatal("trySend did not fail the request when the queue was full")
		}
	})
}

// TestChannelTrySendDoesNotBlockOnFullQueue is the same check as
// TestTrySendDoesNotBlockOnFullQueue against the exported [Channel.TrySend].
// TrySend is the entry point outside this package for replies sent from a
// receive or dispatch loop, in particular from the drain goroutine in
// [Server.NodeStream], which keeps draining while a send queue is stuck or
// backpressured (see TestReceiverDispatchNotWedgedByReentrantReply for the
// client-side analog).
func TestChannelTrySendDoesNotBlockOnFullQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const nodeID = uint32(1)
		stream := newBlockingSendStream()
		c := NewInboundChannel(context.Background(), nodeID, 0, stream, NewMessageRouter())
		defer func() {
			stream.close()
			_ = c.Close()
			synctest.Wait()
		}()

		// Occupy the sender so the queue is full and cannot drain.
		c.Enqueue(Request{
			Ctx:    context.Background(),
			Oneway: true,
			Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()

		reply := make(chan response, 1)
		returned := make(chan struct{})
		go func() {
			c.TrySend(Request{
				Ctx:          context.Background(),
				ResponseChan: reply,
				Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
			})
			close(returned)
		}()
		synctest.Wait()

		select {
		case <-returned:
		default:
			t.Fatal("TrySend blocked on a full send queue with a background context")
		}
		select {
		case resp := <-reply:
			if !errors.Is(resp.Err, ErrSendQueueFull) {
				t.Errorf("TrySend reply error = %v, want ErrSendQueueFull", resp.Err)
			}
		default:
			t.Fatal("TrySend did not fail the request when the queue was full")
		}
	})
}

// TestChannelDroppedRepliesCountsOnlyUnreportableDrops verifies that
// DroppedReplies counts a back-channel reply (no ResponseChan) dropped on a
// full queue, but not a two-way request that fails on the same full queue —
// the two-way caller already observes ErrSendQueueFull directly, so counting
// it too would double-report the same failure.
func TestChannelDroppedRepliesCountsOnlyUnreportableDrops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const nodeID = uint32(1)
		stream := newBlockingSendStream()
		c := NewInboundChannel(context.Background(), nodeID, 0, stream, NewMessageRouter())
		defer func() {
			stream.close()
			_ = c.Close()
			synctest.Wait()
		}()

		// Occupy the sender so the queue is full and cannot drain.
		c.Enqueue(Request{
			Ctx:    context.Background(),
			Oneway: true,
			Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()

		if got := c.DroppedReplies(); got != 0 {
			t.Fatalf("DroppedReplies() = %d before any drop, want 0", got)
		}

		// A back-channel reply with no ResponseChan: dropped and counted.
		c.trySend(Request{
			Ctx: context.Background(),
			Msg: Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()
		if got := c.DroppedReplies(); got != 1 {
			t.Errorf("DroppedReplies() = %d after a reply with no ResponseChan, want 1", got)
		}

		// A two-way request with a ResponseChan: fails fast but is not counted,
		// since the caller already observes ErrSendQueueFull directly.
		reply := make(chan response, 1)
		c.trySend(Request{
			Ctx:          context.Background(),
			ResponseChan: reply,
			Msg:          Message_builder{MessageSeqNo: 3, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()
		select {
		case resp := <-reply:
			if !errors.Is(resp.Err, ErrSendQueueFull) {
				t.Errorf("reply error = %v, want ErrSendQueueFull", resp.Err)
			}
		default:
			t.Fatal("two-way request did not fail on the full queue")
		}
		if got := c.DroppedReplies(); got != 1 {
			t.Errorf("DroppedReplies() = %d after a two-way failure, want unchanged at 1", got)
		}
	})
}

// fakeNodeStream is a minimal Gorums_NodeStreamServer for driving
// Server.NodeStream directly: Send blocks until release is called (simulating
// a backpressured or unresponsive link), signaling entered once a Send call
// is in progress; Recv yields messages fed via feed, in the order fed,
// blocking when none are queued.
type fakeNodeStream struct {
	ctx       context.Context
	inbound   chan *Message
	entered   chan struct{}
	released  chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakeNodeStream(ctx context.Context) *fakeNodeStream {
	return &fakeNodeStream{
		ctx:      ctx,
		inbound:  make(chan *Message, 8),
		entered:  make(chan struct{}, 1),
		released: make(chan struct{}),
		closed:   make(chan struct{}),
	}
}

func (f *fakeNodeStream) Context() context.Context { return f.ctx }

func (f *fakeNodeStream) Recv() (*Message, error) {
	select {
	case m := <-f.inbound:
		return m, nil
	case <-f.closed:
		return nil, context.Canceled
	}
}

func (f *fakeNodeStream) Send(*Message) error {
	select {
	case f.entered <- struct{}{}:
	default:
	}
	select {
	case <-f.released:
		return nil
	case <-f.closed:
		return context.Canceled
	}
}

func (f *fakeNodeStream) feed(m *Message) { f.inbound <- m }
func (f *fakeNodeStream) release()        { close(f.released) }
func (f *fakeNodeStream) close()          { f.closeOnce.Do(func() { close(f.closed) }) }

// The remaining methods satisfy grpc.ServerStream; NodeStream never calls them.
func (*fakeNodeStream) SetHeader(metadata.MD) error  { return nil }
func (*fakeNodeStream) SendHeader(metadata.MD) error { return nil }
func (*fakeNodeStream) SetTrailer(metadata.MD)       {}
func (*fakeNodeStream) SendMsg(any) error            { return nil }
func (*fakeNodeStream) RecvMsg(any) error            { return nil }

var _ Gorums_NodeStreamServer = (*fakeNodeStream)(nil)

// echoOnSameChannelAcceptor is a [PeerAcceptor] whose [PeerNode] replies to
// every inbound request on the same [Channel] it was dispatched from, via
// TrySend — mirroring the real production peerNode adapter — so the
// channel's own stuck sender backs the reply.
type echoOnSameChannelAcceptor struct {
	ch         *Channel
	dispatched chan uint64
}

func (a *echoOnSameChannelAcceptor) AcceptPeer(context.Context, BidiStream) (PeerNode, func(), error) {
	return echoPeerNode{ch: a.ch, dispatched: a.dispatched}, func() {}, nil
}

type echoPeerNode struct {
	ch         *Channel
	dispatched chan uint64
}

func (p echoPeerNode) RouteInbound(_ context.Context, msg *Message, release func(), send func(*Message)) {
	// The reply is produced off the caller's goroutine on purpose: RouteInbound
	// runs on the receive loop, which this test requires to stay unblocked. The
	// goroutine is the behavior under test, so it cannot move to the caller.
	// skipcq: GO-E1007
	go func() {
		defer release()
		p.dispatched <- msg.GetMessageSeqNo()
		send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo(), Method: mock.TestMethod}.Build())
	}()
}

func (p echoPeerNode) TrySend(req Request) {
	p.ch.TrySend(req)
}

// TestNodeStreamReplyDoesNotWedgeReceiveLoop checks the server-side half of
// the teardown deadlock against [Server.NodeStream] itself; the other tests in
// this file check the layers TrySend passes through. A handler's reply is
// handed to the drain goroutine, which sends it with PeerNode.TrySend while
// the send queue is full, and every inbound request is still dispatched (see
// TestReceiverDispatchNotWedgedByReentrantReply for the client-side analog).
// The test hangs if echoPeerNode.TrySend blocks.
//
// It uses real goroutines and wall-clock timeouts, so a wedged run fails with
// a clear message.
func TestNodeStreamReplyDoesNotWedgeReceiveLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := newFakeNodeStream(ctx)
	t.Cleanup(fs.close)

	// Capacity 0: once the sender goroutine is occupied in Send, the queue
	// has no slack, so a reply must fail fast rather than wait for space.
	ch := NewInboundChannel(ctx, 1, 0, fs, NewMessageRouter())
	t.Cleanup(func() { _ = ch.Close() })

	dispatched := make(chan uint64, 8)
	srv := NewServer(0, nil, &echoOnSameChannelAcceptor{ch: ch, dispatched: dispatched})

	done := make(chan error, 1)
	go func() { done <- srv.NodeStream(fs) }()

	// Occupy the sender: this one-way request is handed to the channel's
	// sender goroutine, which then blocks in Send on a transport that never
	// drains (a full or backpressured link during a teardown broadcast).
	ch.Enqueue(Request{
		Ctx:    ctx,
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 100, Method: mock.TestMethod}.Build(),
	})
	select {
	case <-fs.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("sender never entered Send")
	}

	// Feed three inbound requests. Each handler goroutine reports to
	// dispatched before calling send, so requests 1 and 2 are reported
	// whether or not the drain goroutine wedges. The request dispatcher
	// starts the next request on release, and release for request 1 fires
	// as soon as its reply is handed off to the (unbuffered) finished
	// channel, before the drain goroutine's TrySend call on that reply
	// starts. Request 2's reply hand-off waits on finished until the drain
	// goroutine's TrySend call for request 1's reply returns. If that call
	// wedges, request 2's release never fires and the dispatcher never
	// starts request 3. So request 3 exercises the invariant; requests 1
	// and 2 set it up.
	fs.feed(Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build())
	fs.feed(Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build())
	fs.feed(Message_builder{MessageSeqNo: 3, Method: mock.TestMethod}.Build())

	got := make(map[uint64]bool)
	for len(got) < 3 {
		select {
		case id := <-dispatched:
			got[id] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("dispatched = %v; want all three inbound requests dispatched", got)
		}
	}

	fs.release()
	fs.close() // NodeStream's Recv now returns an error and it exits.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("NodeStream did not return after the stream closed")
	}
}
