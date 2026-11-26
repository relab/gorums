package impl

import (
	"context"
	"errors"
	"io"
	"maps"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// TestCallAdmission separates queue admission from stream completion under
// backpressure, including a multicast with one saturated destination.
func TestCallAdmission(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "Complete"
		if fail {
			name = "Fail"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				gated := mock.NewGatedBidiStream[*stream.Message]()
				ch := stream.NewInboundChannel(t.Context(), 1, gated, stream.InboundOptions{SendBufferSize: 1, RecvBufferSize: 1})
				t.Cleanup(func() { gated.Close(); _ = ch.Close() })
				tr := stream.NewTransport(1, func() uint64 { return 1 })
				tr.StoreChannel(ch)
				node := conn.NewNodeForTest(1, tr)
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				// Async must return before the gated stream write completes.
				h := Unicast(ctx, node, pb.String("first"), "test").Async()
				select {
				case <-gated.Entered():
				case <-ctx.Done():
					t.Fatal("sender did not start")
				}
				if err := Unicast(ctx, node, pb.String("queued"), "test").Send(); err != nil {
					t.Fatal(err)
				}
				healthy := &seqNoRecorder{}
				local := stream.NewLocalChannel(2, healthy, 1, nil, nil)
				t.Cleanup(local.Wait)
				healthyTr := stream.NewTransport(2, func() uint64 { return 2 })
				healthyTr.StoreChannel(local)
				cfg := Config{node, conn.NewNodeForTest(2, healthyTr)}
				err := Multicast(ctx, cfg, pb.String("overflow"), "test").Send()
				if !errors.Is(err, stream.ErrSendQueueFull) {
					t.Fatalf("Send = %v, want queue full", err)
				}
				healthy.wait(t, 1)
				if aggregate, ok := errors.AsType[conn.QuorumCallError](err); !ok || aggregate.NumErrors() != 1 {
					t.Fatalf("admission failures = %v, want one destination", err)
				}
				overflow := Multicast(ctx, cfg, pb.String("async overflow"), "test").Async()
				healthy.wait(t, 2)
				if err := overflow.Wait(); !errors.Is(err, stream.ErrSendQueueFull) {
					t.Fatalf("async overflow = %v", err)
				}
				select {
				case <-gated.Sent():
					t.Fatal("write completed before release")
				default:
				}
				completed := make(chan error, 1)
				go func() { completed <- h.Wait() }()
				synctest.Wait()
				select {
				case err := <-completed:
					t.Fatalf("Wait returned before write: %v", err)
				default:
				}
				if fail {
					gated.Close()
				} else {
					gated.Release()
				}
				err = <-completed
				if fail && !errors.Is(err, io.EOF) {
					t.Fatalf("Wait = %v, want EOF", err)
				}
				if !fail && err != nil {
					t.Fatal(err)
				}
				if again := h.Wait(); again != err {
					t.Fatalf("repeated Wait = %v, want %v", again, err)
				}
				if got := ch.DroppedReplies(); got != 0 {
					t.Fatalf("request counted as dropped reply: %d", got)
				}
			})
		})
	}
}

// stalledNode is a node whose stream holds every send until the stream is
// released, like a peer that has stopped reading.
type stalledNode struct {
	*Node
	stream  *mock.BidiStream[*stream.Message]
	channel *stream.InboundChannel
}

// newStalledNode returns a stalled node whose send queue holds sendBufferSize
// requests. Its requests get the message IDs 1, 2, 3, and so on.
func newStalledNode(t *testing.T, id ID, sendBufferSize uint) stalledNode {
	t.Helper()
	gated := mock.NewGatedBidiStream[*stream.Message]()
	ch := stream.NewInboundChannel(t.Context(), id, gated, stream.InboundOptions{SendBufferSize: sendBufferSize, RecvBufferSize: 1})
	t.Cleanup(func() { gated.Close(); _ = ch.Close() })
	var msgID atomic.Uint64
	transport := stream.NewTransport(id, func() uint64 { return msgID.Add(1) })
	transport.StoreChannel(ch)
	return stalledNode{Node: conn.NewNodeForTest(id, transport), stream: gated, channel: ch}
}

// occupy sends one request to the node and waits until the stream holds it,
// so that later requests stay in the send queue.
func (n stalledNode) occupy(t *testing.T) {
	t.Helper()
	if err := Unicast(t.Context(), n.Node, pb.String("held"), "test").Send(); err != nil {
		t.Fatal(err)
	}
	<-n.stream.Entered()
}

// sent returns the message IDs of the requests sent on the stream so far.
func (n stalledNode) sent() (msgIDs []uint64) {
	for {
		select {
		case msg := <-n.stream.Sent():
			msgIDs = append(msgIDs, msg.GetMessageSeqNo())
		default:
			return msgIDs
		}
	}
}

// newLocalNode returns a node that passes its requests to the returned recorder.
func newLocalNode(t *testing.T, id ID) (*Node, *seqNoRecorder) {
	t.Helper()
	recorder := &seqNoRecorder{}
	local := stream.NewLocalChannel(id, recorder, conn.DefaultRecvBufferSize, nil, nil)
	t.Cleanup(local.Wait)
	transport := stream.NewTransport(id, func() uint64 { return uint64(id) })
	transport.StoreChannel(local)
	return conn.NewNodeForTest(id, transport), recorder
}

// TestCallAdmissionContextEndsAfterDispatch verifies which requests a context
// that ends after dispatch withdraws from the send queue. Send retains accepted
// requests when the context ends. Async reports the stream writes, so its
// context also bounds the wait for transmission.
func TestCallAdmissionContextEndsAfterDispatch(t *testing.T) {
	tests := []struct {
		name     string
		dispatch func(*OnewayCall[*pb.StringValue]) error
		want     []uint64
	}{
		{name: "Send", dispatch: func(c *OnewayCall[*pb.StringValue]) error { return c.Send() }, want: []uint64{1, 2}},
		{name: "Async", dispatch: func(c *OnewayCall[*pb.StringValue]) error { c.Async(); return nil }, want: []uint64{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				node := newStalledNode(t, 1, 1)
				node.occupy(t)
				ctx, cancel := context.WithCancel(t.Context())
				if err := tt.dispatch(Unicast(ctx, node.Node, pb.String("queued"), "test")); err != nil {
					t.Fatal(err)
				}
				cancel()
				node.stream.Release()
				synctest.Wait()
				if got := node.sent(); !slices.Equal(got, tt.want) {
					t.Errorf("sent messages = %v, want %v", got, tt.want)
				}
			})
		})
	}
}

// TestCallAdmissionContextEndedBeforeDispatch verifies that a call whose
// context has already ended returns the context error and sends nothing. A
// call without a target returns the target error instead, as it does with a
// live context.
func TestCallAdmissionContextEndedBeforeDispatch(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, *Node) error
		want error
	}{
		{name: "Unicast/Send", want: context.Canceled, call: func(ctx context.Context, n *Node) error {
			return Unicast(ctx, n, pb.String("x"), "test").Send()
		}},
		{name: "Unicast/Async", want: context.Canceled, call: func(ctx context.Context, n *Node) error {
			return Unicast(ctx, n, pb.String("x"), "test").Async().Wait()
		}},
		{name: "Multicast/Send", want: context.Canceled, call: func(ctx context.Context, n *Node) error {
			return Multicast(ctx, Config{n}, pb.String("x"), "test").Send()
		}},
		{name: "Multicast/Async", want: context.Canceled, call: func(ctx context.Context, n *Node) error {
			return Multicast(ctx, Config{n}, pb.String("x"), "test").Async().Wait()
		}},
		{name: "Unicast/NilNode/Send", want: ErrNoNode, call: func(ctx context.Context, _ *Node) error {
			return Unicast(ctx, nil, pb.String("x"), "test").Send()
		}},
		{name: "Unicast/NilNode/Async", want: ErrNoNode, call: func(ctx context.Context, _ *Node) error {
			return Unicast(ctx, nil, pb.String("x"), "test").Async().Wait()
		}},
		{name: "Multicast/EmptyConfig/Send", want: ErrEmptyConfig, call: func(ctx context.Context, _ *Node) error {
			return Multicast(ctx, nil, pb.String("x"), "test").Send()
		}},
		{name: "Multicast/EmptyConfig/Async", want: ErrEmptyConfig, call: func(ctx context.Context, _ *Node) error {
			return Multicast(ctx, nil, pb.String("x"), "test").Async().Wait()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				node, recorder := newLocalNode(t, 1)
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				if err := tt.call(ctx, node); err != tt.want {
					t.Errorf("call = %v, want %v", err, tt.want)
				}
				synctest.Wait()
				if got := recorder.wait(t, 0); len(got) != 0 {
					t.Errorf("handler got %d requests, want none", len(got))
				}
			})
		})
	}
}

// TestCallAdmissionWaitContextEnds verifies that Wait, when its context ends,
// still reports the admission failures, and reports the context error for
// each node whose stream write has no outcome.
func TestCallAdmissionWaitContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stalled := newStalledNode(t, 1, 1)
		healthy, recorder := newLocalNode(t, 2)
		down := conn.NewNodeForTest(3, stream.NewTransport(3, func() uint64 { return 3 }))

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		err := Multicast(ctx, Config{stalled.Node, healthy, down}, pb.String("x"), "test").Async().Wait()
		if !errors.Is(err, ErrSendFailure) {
			t.Fatalf("Wait = %v, want cause %v", err, ErrSendFailure)
		}
		qcErr, ok := errors.AsType[conn.QuorumCallError](err)
		if !ok {
			t.Fatalf("Wait = %T, want a QuorumCallError", err)
		}
		got := maps.Collect(qcErr.NodeErrors())
		want := map[ID]error{1: context.DeadlineExceeded, 3: stream.ErrStreamDown}
		if len(got) != len(want) {
			t.Errorf("node errors = %v, want %v", got, want)
		}
		for id, wantErr := range want {
			if !errors.Is(got[id], wantErr) {
				t.Errorf("node %d error = %v, want %v", id, got[id], wantErr)
			}
		}
		recorder.wait(t, 1)

		ctx, cancel = context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := Unicast(ctx, stalled.Node, pb.String("x"), "test").Async().Wait(); err != context.DeadlineExceeded {
			t.Errorf("unicast Wait = %v, want %v", err, context.DeadlineExceeded)
		}
	})
}

// TestCallAdmissionOrder verifies that a full queue rejects a request, and
// that the accepted requests are sent in dispatch order whether or not the
// caller keeps the handle that Async returns.
func TestCallAdmissionOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		node := newStalledNode(t, 1, 2)
		node.occupy(t)
		ctx := t.Context()
		if err := Unicast(ctx, node.Node, pb.String("second"), "test").Send(); err != nil {
			t.Fatal(err)
		}
		Unicast(ctx, node.Node, pb.String("third"), "test").Async() // the handle is dropped
		if err := Unicast(ctx, node.Node, pb.String("rejected"), "test").Send(); !errors.Is(err, stream.ErrSendQueueFull) {
			t.Fatalf("Send = %v, want %v", err, stream.ErrSendQueueFull)
		}
		if err := Unicast(ctx, node.Node, pb.String("rejected"), "test").Async().Wait(); !errors.Is(err, stream.ErrSendQueueFull) {
			t.Fatalf("Wait = %v, want %v", err, stream.ErrSendQueueFull)
		}
		node.stream.Release()
		synctest.Wait()
		if got, want := node.sent(), []uint64{1, 2, 3}; !slices.Equal(got, want) {
			t.Errorf("sent messages = %v, want %v", got, want)
		}
	})
}

// TestCallAdmissionClosedChannel verifies that a closed channel rejects a
// request at admission.
func TestCallAdmissionClosedChannel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		node := newStalledNode(t, 1, 1)
		node.stream.Close()
		if err := node.channel.Close(); err != nil {
			t.Fatal(err)
		}
		if err := Unicast(t.Context(), node.Node, pb.String("x"), "test").Send(); !errors.Is(err, stream.ErrNodeClosed) {
			t.Errorf("Send = %v, want %v", err, stream.ErrNodeClosed)
		}
		err := Multicast(t.Context(), Config{node.Node}, pb.String("x"), "test").Async().Wait()
		if !errors.Is(err, stream.ErrNodeClosed) || !errors.Is(err, ErrSendFailure) {
			t.Errorf("Wait = %v, want %v with cause %v", err, stream.ErrNodeClosed, ErrSendFailure)
		}
	})
}
