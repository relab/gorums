package stream

import (
	"context"
	"errors"
	"slices"
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
// returns immediately and is dropped; the handler then returns and the
// channel's request dispatcher starts the next request.
// The test fails if the reply blocks.
func TestReceiverDispatchNotWedgedByReentrantReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const nodeID = uint32(1)

		// The handler answers every request on the same channel it was
		// dispatched on: the reentrant back-channel reply.
		dispatched := make(chan uint64, 8)
		replied := make(chan uint64, 8)
		handler := requestHandlerFunc(func(_ context.Context, msg *Message, release func(), send func(*Message)) {
			defer release()
			dispatched <- msg.GetMessageSeqNo()
			send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo(), Method: mock.TestMethod}.Build())
			replied <- msg.GetMessageSeqNo()
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
		first := ServerSequenceNumber(1)
		c.dispatchInbound(Message_builder{MessageSeqNo: first, Method: mock.TestMethod}.Build())
		synctest.Wait()

		// The reply returned on the full queue and was dropped.
		if got := drain(replied); !slices.Equal(got, []uint64{first}) {
			t.Fatalf("replied = %v, want [%d]: the reply blocked on a full send queue", got, first)
		}
		if got := c.DroppedReplies(); got != 1 {
			t.Fatalf("DroppedReplies() = %d, want 1: the reply did not reach the full send queue", got)
		}

		// The first handler has returned, so the dispatcher runs the next
		// back-channel request, whose reply is dropped the same way.
		second := ServerSequenceNumber(2)
		c.dispatchInbound(Message_builder{MessageSeqNo: second, Method: mock.TestMethod}.Build())
		synctest.Wait()

		if got, want := drain(dispatched), []uint64{first, second}; !slices.Equal(got, want) {
			t.Fatalf("dispatched = %v, want %v", got, want)
		}
		if got := drain(replied); !slices.Equal(got, []uint64{second}) {
			t.Fatalf("replied = %v, want [%d]", got, second)
		}
		if got := c.DroppedReplies(); got != 2 {
			t.Errorf("DroppedReplies() = %d, want 2", got)
		}
	})
}

// drain returns the values buffered in ch without blocking.
func drain(ch <-chan uint64) []uint64 {
	var got []uint64
	for {
		select {
		case v := <-ch:
			got = append(got, v)
		default:
			return got
		}
	}
}

// TestChannelTrySendDoesNotBlockOnFullQueue verifies that the non-blocking
// enqueue used for back-channel replies returns immediately on a full send
// queue, even with a background (deadline-free) context. A back-channel reply
// carries only the connection context, so it must make progress without
// context cancellation to keep a reply from wedging the receiver. The one-way
// [Channel.Enqueue] path waits until the request context is done (see
// TestChannelEnqueueRespectsRequestContext).
//
// [Channel.TrySend] is the entry point outside this package, used by the
// drain goroutine in [Server.NodeStream]; [Channel.trySend] is used by the
// client-side back-channel reply (see
// TestReceiverDispatchNotWedgedByReentrantReply).
func TestChannelTrySendDoesNotBlockOnFullQueue(t *testing.T) {
	tests := []struct {
		name    string
		trySend func(*Channel, Request)
	}{
		{name: "trySend", trySend: (*Channel).trySend},
		{name: "TrySend", trySend: (*Channel).TrySend},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
					tt.trySend(c, Request{
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
					t.Fatalf("%s blocked on a full send queue with a background context", tt.name)
				}
				select {
				case resp := <-reply:
					if !errors.Is(resp.Err, ErrSendQueueFull) {
						t.Errorf("%s reply error = %v, want ErrSendQueueFull", tt.name, resp.Err)
					}
				default:
					t.Fatalf("%s did not fail the request when the queue was full", tt.name)
				}
			})
		})
	}
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
