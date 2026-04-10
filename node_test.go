package gorums

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNodeSort(t *testing.T) {
	makeNode := func(id uint32, err error) *Node {
		n := &Node{id: id, router: stream.NewMessageRouter()}
		n.channel.Store(stream.NewChannelWithState(err))
		return n
	}
	makeNodeWithLatency := func(id uint32, lat time.Duration) *Node {
		return &Node{id: id, router: stream.NewMessageRouterWithLatency(lat)}
	}
	someErr := errors.New("some error")
	nodes := []*Node{
		makeNode(100, nil),
		makeNode(101, someErr),
		makeNode(42, nil),
		makeNode(99, someErr),
	}

	t.Run("ByID", func(t *testing.T) {
		ns := slices.Clone(nodes)
		slices.SortFunc(ns, ID)
		for i := 1; i < len(ns); i++ {
			if ns[i].id < ns[i-1].id {
				t.Error("by id: not sorted")
				printNodes(t, ns)
			}
		}
	})

	t.Run("ByLastNodeError", func(t *testing.T) {
		ns := slices.Clone(nodes)
		slices.SortFunc(ns, LastNodeError)
		for i := 1; i < len(ns); i++ {
			if ns[i].LastErr() == nil && ns[i-1].LastErr() != nil {
				t.Error("by error: not sorted")
				printNodes(t, ns)
			}
		}
	})

	t.Run("ByLastNodeErrorThenID", func(t *testing.T) {
		ns := slices.Clone(nodes)
		slices.SortFunc(ns, func(a, b *Node) int {
			if c := LastNodeError(a, b); c != 0 {
				return c
			}
			return ID(a, b)
		})
		// Expect: 42 (no err), 100 (no err), 99 (err), 101 (err).
		wantIDs := []uint32{42, 100, 99, 101}
		for i, n := range ns {
			if n.id != wantIDs[i] {
				t.Errorf("by error then id: position %d: got id %d, want %d", i, n.id, wantIDs[i])
				printNodes(t, ns)
			}
		}
	})

	t.Run("ByLatency", func(t *testing.T) {
		// Node 3 has no measurement (-1s): should sort last.
		// Remaining nodes sort ascending by latency.
		ns := []*Node{
			makeNodeWithLatency(1, 30*time.Millisecond),
			makeNodeWithLatency(2, 10*time.Millisecond),
			makeNodeWithLatency(3, -1*time.Second), // no measurement
			makeNodeWithLatency(4, 20*time.Millisecond),
		}
		slices.SortFunc(ns, Latency)
		// Expected: 2 (10ms), 4 (20ms), 1 (30ms), 3 (no data).
		wantIDs := []uint32{2, 4, 1, 3}
		for i, n := range ns {
			if n.id != wantIDs[i] {
				t.Errorf("by latency: position %d: got id %d, want %d", i, n.id, wantIDs[i])
				printNodes(t, ns)
			}
		}
	})

	t.Run("ByLatency/AllUnmeasured", func(t *testing.T) {
		// All nodes without measurements: stable order must be preserved.
		ns := []*Node{
			makeNodeWithLatency(1, -1*time.Second),
			makeNodeWithLatency(2, -1*time.Second),
			makeNodeWithLatency(3, -1*time.Second),
		}
		slices.SortStableFunc(ns, Latency)
		wantIDs := []uint32{1, 2, 3}
		for i, n := range ns {
			if n.id != wantIDs[i] {
				t.Errorf("by latency (all unmeasured): position %d: got id %d, want %d", i, n.id, wantIDs[i])
			}
		}
	})

	t.Run("ByLatencyThenID", func(t *testing.T) {
		// Two nodes with the same latency: secondary sort by ID breaks ties.
		ns := []*Node{
			makeNodeWithLatency(10, 20*time.Millisecond),
			makeNodeWithLatency(5, 10*time.Millisecond),
			makeNodeWithLatency(7, 20*time.Millisecond),
		}
		slices.SortFunc(ns, func(a, b *Node) int {
			if r := Latency(a, b); r != 0 {
				return r
			}
			return ID(a, b)
		})
		// Expected: 5 (10ms), 7 (20ms, lower id), 10 (20ms, higher id).
		wantIDs := []uint32{5, 7, 10}
		for i, n := range ns {
			if n.id != wantIDs[i] {
				t.Errorf("by latency then id: position %d: got id %d, want %d", i, n.id, wantIDs[i])
				printNodes(t, ns)
			}
		}
	})
}

func TestConfigurationWatch(t *testing.T) {
	makeNodeWithLatency := func(id uint32, lat time.Duration) *Node {
		return &Node{id: id, router: stream.NewMessageRouterWithLatency(lat)}
	}

	// allNodes has five nodes; top-3 by ascending latency are 2(10ms), 3(20ms), 1(30ms).
	allNodes := Configuration{
		makeNodeWithLatency(1, 30*time.Millisecond),
		makeNodeWithLatency(2, 10*time.Millisecond),
		makeNodeWithLatency(3, 20*time.Millisecond),
		makeNodeWithLatency(4, 40*time.Millisecond),
		makeNodeWithLatency(5, 50*time.Millisecond),
	}
	const quorumSize = 3
	fastTop3 := func(c Configuration) Configuration { return c.SortBy(Latency)[:quorumSize] }

	t.Run("EmitsInitialSnapshot", func(t *testing.T) {
		// Use a very long interval so only the initial emission fires.
		updates := allNodes.Watch(t.Context(), time.Hour, fastTop3)
		snap := <-updates
		if len(snap) != quorumSize {
			t.Fatalf("initial snapshot size = %d, want %d", len(snap), quorumSize)
		}
		wantIDs := []uint32{2, 3, 1}
		for i, n := range snap {
			if n.ID() != wantIDs[i] {
				t.Errorf("position %d: got id %d, want %d", i, n.ID(), wantIDs[i])
			}
		}
	})

	t.Run("NoEmissionWhenUnchanged", func(t *testing.T) {
		updates := allNodes.Watch(t.Context(), 10*time.Millisecond, fastTop3)
		<-updates // drain initial emission

		// Latencies are fixed, so no further emission should arrive.
		select {
		case cfg, ok := <-updates:
			if ok {
				t.Errorf("unexpected second emission: got ids %v", cfg.NodeIDs())
			}
		case <-time.After(100 * time.Millisecond):
			// expected: no second emission
		}
	})

	t.Run("EmitsOnOrderChange", func(t *testing.T) {
		n1 := makeNodeWithLatency(1, 10*time.Millisecond)
		n2 := makeNodeWithLatency(2, 30*time.Millisecond)
		n3 := makeNodeWithLatency(3, 20*time.Millisecond)
		cfg := Configuration{n1, n2, n3}
		top2 := func(c Configuration) Configuration { return c.SortBy(Latency)[:2] }

		const interval = 20 * time.Millisecond
		updates := cfg.Watch(t.Context(), interval, top2)
		first := <-updates
		// Initial top-2: [1(10ms), 3(20ms)]
		wantFirst := []uint32{1, 3}
		for i, n := range first {
			if n.ID() != wantFirst[i] {
				t.Errorf("initial: position %d got id %d, want %d", i, n.ID(), wantFirst[i])
			}
		}

		// Swap latencies: node 2 becomes fastest.
		n1.router.SetLatency(40 * time.Millisecond)
		n2.router.SetLatency(5 * time.Millisecond)

		select {
		case second := <-updates:
			// New top-2: [2(5ms), 3(20ms)]
			wantSecond := []uint32{2, 3}
			for i, n := range second {
				if n.ID() != wantSecond[i] {
					t.Errorf("after swap: position %d got id %d, want %d", i, n.ID(), wantSecond[i])
				}
			}
		case <-time.After(5 * interval):
			t.Error("expected a second emission after latency swap, but none arrived")
		}
	})

	t.Run("ChannelClosedOnCtxCancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		updates := allNodes.Watch(ctx, time.Hour, fastTop3)
		<-updates // drain initial
		cancel()
		select {
		case _, ok := <-updates:
			if ok {
				t.Error("channel should be closed after ctx cancel")
			}
		case <-time.After(time.Second):
			t.Error("channel should be closed promptly after ctx cancel")
		}
	})
}

func printNodes(t *testing.T, nodes []*Node) {
	t.Helper()
	for i, n := range nodes {
		nodeStr := fmt.Sprintf(
			"%d: node %d | addr: %s | latency: %v | err: %v",
			i, n.id, n.addr, n.Latency(), n.LastErr())
		t.Logf("%s", nodeStr)
	}
}

// testRequestHandler is a minimal stream.RequestHandler that calls release
// and signals dispatch via a channel.
type testRequestHandler struct {
	done chan struct{}
}

func (h *testRequestHandler) HandleRequest(_ context.Context, _ *stream.Message, release func(), _ func(*stream.Message)) {
	release()
	close(h.done)
}

// TestNodeRouteInbound verifies that Node.RouteInbound correctly routes
// server-initiated responses and dispatches client-initiated requests.
func TestNodeRouteInbound(t *testing.T) {
	t.Run("ServerInitiatedPendingDelivered", func(t *testing.T) {
		n := newInboundNode(42, "127.0.0.1:9000", func() uint64 { return 0 }, nil)
		replyChan := make(chan NodeResponse[*stream.Message], 1)
		msgID := stream.ServerSequenceNumber(7)
		n.router.Register(msgID, stream.Request{
			Ctx:          context.Background(),
			Msg:          &stream.Message{},
			ResponseChan: replyChan,
		})
		respMsg := stream.Message_builder{MessageSeqNo: msgID}.Build()
		released := make(chan struct{}, 1)
		release := func() { released <- struct{}{} }
		n.RouteInbound(context.Background(), respMsg, release, func(*stream.Message) {})

		select {
		case got := <-replyChan:
			if got.NodeID != 42 {
				t.Errorf("NodeID = %d, want 42", got.NodeID)
			}
		default:
			t.Fatal("expected response on channel")
		}
		select {
		case <-released:
		default:
			t.Fatal("release should be called for server-initiated response")
		}
	})

	t.Run("ServerInitiatedStaleAbsorbed", func(t *testing.T) {
		n := newInboundNode(42, "127.0.0.1:9000", func() uint64 { return 0 }, nil)
		msgID := stream.ServerSequenceNumber(7)
		respMsg := stream.Message_builder{MessageSeqNo: msgID}.Build()
		released := make(chan struct{}, 1)
		release := func() { released <- struct{}{} }
		n.RouteInbound(context.Background(), respMsg, release, func(*stream.Message) {})
		select {
		case <-released:
		default:
			t.Fatal("release should be called for stale server-initiated response")
		}
	})

	t.Run("ClientInitiatedNilHandlerCallsRelease", func(t *testing.T) {
		n := newInboundNode(42, "127.0.0.1:9000", func() uint64 { return 0 }, nil)
		clientMsg := stream.Message_builder{MessageSeqNo: 1}.Build()
		released := make(chan struct{}, 1)
		release := func() { released <- struct{}{} }
		n.RouteInbound(context.Background(), clientMsg, release, func(*stream.Message) {})
		select {
		case <-released:
		default:
			t.Fatal("release should be called immediately when no handler is registered")
		}
	})

	t.Run("ClientInitiatedDispatchedToHandler", func(t *testing.T) {
		h := &testRequestHandler{done: make(chan struct{})}
		n := newInboundNode(42, "127.0.0.1:9000", func() uint64 { return 0 }, h)
		clientMsg := stream.Message_builder{MessageSeqNo: 1}.Build()
		n.RouteInbound(context.Background(), clientMsg, func() {}, func(*stream.Message) {})
		select {
		case <-h.done:
		case <-time.After(time.Second):
			t.Fatal("handler should have been called for client-initiated request")
		}
	})
}

// BenchmarkNodeEnqueue measures the overhead that Node.enqueue adds per
// request dispatch: an atomic.Pointer.Load() and a nil guard.
// The cost is ~1-2 ns, which is negligible compared to a full
// Channel.Enqueue round-trip (~50-100 ns).
// See BenchmarkChannelSend in internal/stream and BenchmarkNodeEnqueueSend
// below for the full send-path cost.
func BenchmarkNodeEnqueue(b *testing.B) {
	req := stream.Request{}

	b.Run("ChannelNil", func(b *testing.B) {
		// No stream attached: channel.Load() returns nil → early return.
		// Measures the pure atomic load + nil-guard overhead.
		n := newInboundNode(1, "127.0.0.1:9081", func() uint64 { return 0 }, nil)
		b.ResetTimer()
		for range b.N {
			n.Enqueue(req)
		}
	})

	b.Run("AtomicLoadNonNil", func(b *testing.B) {
		// Stub channel attached; measures atomic.Pointer.Load() + non-nil branch
		// without going through Channel.Enqueue (which requires a running goroutine).
		n := newInboundNode(1, "127.0.0.1:9081", func() uint64 { return 0 }, nil)
		n.channel.Store(stream.NewChannelWithState(nil))
		b.ResetTimer()
		for range b.N {
			_ = n.channel.Load()
		}
	})
}

// BenchmarkNodeEnqueueSend measures the end-to-end send latency going through
// the Node.enqueue path (atomic.Pointer.Load + Channel.Enqueue) against a live
// echo server.
//
// To get a fair comparison with BenchmarkChannelSend in internal/stream, the
// server is set up identically: a raw gRPC echo handler (benchEchoServer) that
// calls Recv/Send in a loop with no proto marshal/unmarshal, no per-request
// goroutines, and a send buffer of 10. This isolates the one structural
// difference: going through Node.enqueue (atomic.Pointer.Load + nil guard)
// versus calling Channel.Enqueue directly.
//
// To run this benchmark together with BenchmarkChannelSend, use:
//
//	go test -run=^$ -bench='BenchmarkChannelSend$|BenchmarkNodeEnqueueSend' -benchmem -count=10 ./internal/stream .
func BenchmarkNodeEnqueueSend(b *testing.B) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("failed to listen: %v", err)
	}
	grpcSrv := grpc.NewServer() // skipcq: GO-S0902
	stream.RegisterGorumsServer(grpcSrv, benchEchoServer{})
	go func() { _ = grpcSrv.Serve(lis) }()
	b.Cleanup(grpcSrv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatalf("failed to dial: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close() })

	// Wrap the outbound channel in a Node, adding the one atomic.Pointer.Load
	// that Node.enqueue performs on every dispatch.
	n := newInboundNode(1, lis.Addr().String(), func() uint64 { return 0 }, nil)
	ch := stream.NewOutboundChannel(context.Background(), 1, 10, conn, n.router)
	b.Cleanup(func() { _ = ch.Close() })
	n.channel.Store(ch)

	tests := []struct {
		name string
		size int // payload size in bytes
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			payload := make([]byte, tt.size)
			b.ResetTimer()
			for i := range b.N {
				replyChan := make(chan NodeResponse[*stream.Message], 1)
				reqMsg := stream.Message_builder{
					MessageSeqNo: uint64(i),
					Method:       mock.TestMethod,
					Payload:      payload,
				}.Build()
				n.Enqueue(stream.Request{
					Ctx:          context.Background(),
					Msg:          reqMsg,
					WaitSendDone: true,
					ResponseChan: replyChan,
				})
				<-replyChan
			}
		})
	}
}

// benchEchoServer is a minimal raw gRPC echo server for BenchmarkNodeEnqueueSend.
// It mirrors echoServer in internal/stream/channel_test.go: Recv and Send in a
// loop with no proto marshal/unmarshal and no per-request goroutines, so the
// server-side cost is identical to what BenchmarkChannelSend measures.
type benchEchoServer struct {
	stream.UnimplementedGorumsServer
}

func (benchEchoServer) NodeStream(srv stream.Gorums_NodeStreamServer) error {
	for {
		in, err := srv.Recv()
		if err != nil {
			return err
		}
		if err := srv.Send(in); err != nil {
			return err
		}
	}
}
