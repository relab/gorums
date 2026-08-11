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

// TestReceiverDispatchNotWedgedByReentrantReply reproduces the back-channel
// teardown deadlock: a server-initiated (back-channel) request is dispatched to
// a handler while the router's dispatch lock is held; the handler replies on the
// same channel, whose send queue is full because the transport is not draining.
// If that reply enqueue blocks, the handler never releases the dispatch lock and
// the receiver can no longer dispatch inbound frames — the channel deadlocks.
//
// The fix routes back-channel replies through the non-blocking [Channel.trySend]
// (see [Channel.dispatchInbound]). With the fix the handler's reply fails fast,
// the handler returns, the dispatch lock is freed, and the next request
// dispatches. Reverting dispatchInbound to use the blocking [Channel.Enqueue]
// makes this test fail: the dispatch lock is never released.
//
// This is asserted directly on the dispatch lock rather than via synctest's
// all-goroutines-durably-blocked deadlock detection, because a goroutine waiting
// on sync.Mutex.Lock is not "durably blocked" (see testing/synctest); the mutex
// hand-off at the heart of this deadlock is therefore invisible to that
// detection. synctest.Wait is used only to reach a settled state before the
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
		// dispatchSerialized runs the handler while holding the dispatch lock;
		// the handler replies on the same, now-full channel.
		first := Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build()
		c.dispatchInbound(first)
		synctest.Wait() // let the handler reply and (with the fix) return

		// Invariant: after the handler's reentrant reply, the dispatch lock must
		// be free so the receiver can dispatch the next inbound frame. A held
		// lock means the reply blocked on the full queue and the loop is wedged.
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
// queue even with a background (deadline-free) context, rather than blocking
// unbounded. This is the property that keeps a reply from wedging the receiver:
// unlike the one-way [Channel.Enqueue] path — which blocks until the request
// context is done (see TestChannelEnqueueRespectsRequestContext) — trySend must
// not depend on context cancellation to make progress, because a back-channel
// reply carries only the never-cancelled connection context.
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
// TestTrySendDoesNotBlockOnFullQueue, but against the exported [Channel.TrySend]
// rather than the unexported trySend it wraps. TrySend is the entry point used
// outside this package for replies that must never stall a receive/dispatch
// loop — in particular the drain goroutine in [Server.NodeStream], which hands
// a handler's reply to the peer this way so a stuck or backpressured send
// queue cannot wedge that goroutine (see the invariant in
// TestReceiverDispatchNotWedgedByReentrantReply for the client-side analog).
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
func (f *fakeNodeStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeNodeStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeNodeStream) SetTrailer(metadata.MD)       {}
func (f *fakeNodeStream) SendMsg(any) error            { return nil }
func (f *fakeNodeStream) RecvMsg(any) error            { return nil }

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
	go func() {
		defer release()
		p.dispatched <- msg.GetMessageSeqNo()
		send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo(), Method: mock.TestMethod}.Build())
	}()
}

func (p echoPeerNode) TrySend(req Request) {
	p.ch.TrySend(req)
}

// TestNodeStreamReplyDoesNotWedgeReceiveLoop reproduces the server-side half
// of the teardown deadlock directly against [Server.NodeStream], rather than
// against the individual layers TrySend passes through (as the other tests in
// this file do). A handler's reply is handed off via the drain goroutine's
// call to PeerNode.TrySend; this must never block that goroutine, or
// NodeStream's Recv loop below could never read the next inbound frame — see
// the invariant in TestReceiverDispatchNotWedgedByReentrantReply for the
// client-side analog. Reverting echoPeerNode.TrySend to call ch.Enqueue
// instead of ch.TrySend makes this test hang.
//
// This uses real goroutines and wall-clock timeouts rather than synctest: the
// deadlock's key hand-off is NodeStream's own mut, a plain sync.Mutex, and (as
// documented on TestReceiverDispatchNotWedgedByReentrantReply) a goroutine
// blocked on Mutex.Lock is not "durably blocked" to synctest, so a wedged run
// would hang synctest.Wait itself for the real test timeout instead of failing
// with a clear message.
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
	// regardless of whether the drain goroutine wedges: NodeStream's mut
	// only serializes Recv iterations on release, and release for request 1
	// fires as soon as its reply is handed off to the (unbuffered) finished
	// channel — before the drain goroutine's TrySend call on that reply even
	// starts. Request 2's own reply hand-off is what actually depends on the
	// drain goroutine: it blocks on finished until the drain goroutine loops
	// back to receive again, which happens only once its TrySend call for
	// request 1's reply returns. If that TrySend call wedges (the bug this
	// guards against), request 2's release never fires, mut is never freed
	// again, and request 3 — sitting in fs.inbound — is never read by
	// NodeStream's Recv loop or dispatched. So request 3 is the one that
	// actually exercises the invariant; 1 and 2 only get it there.
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
