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
// handler can reply on a full send queue and still let the next request run:
// the reply is dropped instead of waiting for space, the handler returns, and
// the dispatcher starts the next request.
func TestReceiverDispatchNotWedgedByReentrantReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// The handler answers every request on the session it was dispatched
		// on: the reentrant back-channel reply.
		dispatched := make(chan uint64, 8)
		replied := make(chan uint64, 8)
		handler := requestHandlerFunc(func(_ context.Context, msg *Message, release func(), send func(*Message)) {
			defer release()
			dispatched <- msg.GetMessageSeqNo()
			send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo(), Method: mock.TestMethod}.Build())
			replied <- msg.GetMessageSeqNo()
		})

		// Capacity 0: once the send loop is occupied in Send, the queue has
		// no slack, so a reply would have to wait for space.
		stream := newBlockingSendStream()
		e := newEndpoint(context.Background(), 1, 0, 0, handler, nil)
		ctx, cancel := context.WithCancel(e.ctx)
		s := newSession(ctx, cancel, &e, stream, true, true)
		go s.sendLoop(nil)
		defer func() {
			stream.close()
			e.cancel()
			synctest.Wait()
		}()

		// Occupy the send loop: this one-way request is handed to it, and it
		// then blocks in Send on a transport that never drains.
		e.Enqueue(Request{
			Ctx:    context.Background(),
			Oneway: true,
			Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()

		// Route a back-channel request as the receive loop does. The handler
		// replies on the same, now-full queue.
		first := ServerSequenceNumber(1)
		s.handle(Message_builder{MessageSeqNo: first, Method: mock.TestMethod}.Build())
		synctest.Wait()

		if got := drain(replied); !slices.Equal(got, []uint64{first}) {
			t.Fatalf("replied = %v, want [%d]: the reply blocked on a full send queue", got, first)
		}
		if got := e.DroppedReplies(); got != 1 {
			t.Fatalf("DroppedReplies() = %d, want 1: the reply did not reach the full send queue", got)
		}

		// The first handler has returned, so the dispatcher runs the next
		// request, whose reply is dropped the same way.
		second := ServerSequenceNumber(2)
		s.handle(Message_builder{MessageSeqNo: second, Method: mock.TestMethod}.Build())
		synctest.Wait()

		if got, want := drain(dispatched), []uint64{first, second}; !slices.Equal(got, want) {
			t.Fatalf("dispatched = %v, want %v", got, want)
		}
		if got := drain(replied); !slices.Equal(got, []uint64{second}) {
			t.Fatalf("replied = %v, want [%d]", got, second)
		}
		if got := e.DroppedReplies(); got != 2 {
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

// TestChannelReplyOnFullQueue verifies that a handler reply on a full send
// queue returns at once and is counted as dropped, even though it carries the
// channel's never-cancelled context. With WaitingReplies, the reply instead
// waits and is sent once the queue drains.
func TestChannelReplyOnFullQueue(t *testing.T) {
	tests := []struct {
		name           string
		waitingReplies bool
	}{
		{name: "Dropped", waitingReplies: false},
		{name: "Waiting", waitingReplies: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				stream := newBlockingSendStream()
				c := NewInboundChannel(context.Background(), 1, stream, InboundOptions{WaitingReplies: tt.waitingReplies})
				defer func() {
					stream.close()
					_ = c.Close()
					synctest.Wait()
				}()

				// Occupy the send loop so the queue is full and cannot drain.
				c.Enqueue(Request{
					Ctx:    context.Background(),
					Oneway: true,
					Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
				})
				synctest.Wait()

				returned := make(chan struct{})
				go func() {
					c.reply(Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build())
					close(returned)
				}()
				synctest.Wait()

				select {
				case <-returned:
					if tt.waitingReplies {
						t.Fatal("waiting reply returned on a full send queue")
					}
				default:
					if !tt.waitingReplies {
						t.Fatal("reply blocked on a full send queue")
					}
				}
				wantDropped := int64(1)
				if tt.waitingReplies {
					wantDropped = 0
				}
				if got := c.DroppedReplies(); got != wantDropped {
					t.Errorf("DroppedReplies() = %d, want %d", got, wantDropped)
				}
				if !tt.waitingReplies {
					return
				}
				stream.release()
				synctest.Wait()
				<-returned
				if got := drain(stream.sends); !slices.Equal(got, []uint64{1, 2}) {
					t.Errorf("sent = %v, want [1 2]", got)
				}
			})
		})
	}
}

// TestChannelDroppedRepliesCountsOnlyUnreportableDrops verifies that
// DroppedReplies counts a reply dropped on a full queue, but not a two-way
// request that fails on the same full queue: its caller already observes
// ErrSendQueueFull directly.
func TestChannelDroppedRepliesCountsOnlyUnreportableDrops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stream := newBlockingSendStream()
		c := NewInboundChannel(context.Background(), 1, stream, InboundOptions{})
		defer func() {
			stream.close()
			_ = c.Close()
			synctest.Wait()
		}()

		// Occupy the send loop so the queue is full and cannot drain.
		c.Enqueue(Request{
			Ctx:    context.Background(),
			Oneway: true,
			Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		})
		synctest.Wait()

		if got := c.DroppedReplies(); got != 0 {
			t.Fatalf("DroppedReplies() = %d before any drop, want 0", got)
		}

		c.reply(Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build())
		synctest.Wait()
		if got := c.DroppedReplies(); got != 1 {
			t.Errorf("DroppedReplies() = %d after a dropped reply, want 1", got)
		}

		reply := make(chan response, 1)
		c.Enqueue(Request{
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

// TestInboundCloseReturnsWhileSendBlocked verifies that inbound cleanup does
// not wait for a transport send whose completion depends on the RPC returning.
func TestInboundCloseReturnsWhileSendBlocked(t *testing.T) {
	stream := newBlockingSendStream()
	t.Cleanup(stream.close)
	c := NewInboundChannel(context.Background(), 1, stream, InboundOptions{})
	t.Cleanup(func() { _ = c.Close() })

	c.Enqueue(Request{
		Ctx:    context.Background(),
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
	})
	select {
	case <-stream.entered:
	case <-time.After(defaultTestTimeout):
		t.Fatal("sender never entered Send")
	}

	closed := make(chan struct{})
	go func() {
		_ = c.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(defaultTestTimeout):
		t.Fatal("Close waited for the blocked transport send")
	}
	stream.release()
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

// channelAcceptor is a [PeerAcceptor] that returns a prepared channel.
type channelAcceptor struct {
	ch *InboundChannel
}

func (a channelAcceptor) AcceptPeer(context.Context, BidiStream) (*InboundChannel, func(), error) {
	return a.ch, func() {}, nil
}

// TestNodeStreamReplyDoesNotWedgeReceiveLoop checks the server-side half of
// the teardown deadlock against [Server.NodeStream]: with the send queue full,
// each handler's reply is dropped instead of waiting, so every inbound request
// is still dispatched in turn.
func TestNodeStreamReplyDoesNotWedgeReceiveLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := newFakeNodeStream(ctx)
	t.Cleanup(fs.close)

	dispatched := make(chan uint64, 8)
	echo := requestHandlerFunc(func(_ context.Context, msg *Message, release func(), send func(*Message)) {
		defer release()
		dispatched <- msg.GetMessageSeqNo()
		send(Message_builder{MessageSeqNo: msg.GetMessageSeqNo(), Method: mock.TestMethod}.Build())
	})
	// Capacity 0: once the send loop is occupied in Send, the queue has no
	// slack, so a reply must be dropped rather than wait for space.
	ch := NewInboundChannel(ctx, 1, fs, InboundOptions{Handler: echo})
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
	case <-fs.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("sender never entered Send")
	}

	// Each request starts only after the previous handler returned, which it
	// does only if its reply did not block.
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
