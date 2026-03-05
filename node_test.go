package gorums

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

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
	nodes := []*Node{
		makeNode(100, nil),
		makeNode(101, errors.New("some error")),
		makeNode(42, nil),
		makeNode(99, errors.New("some error")),
	}

	n := len(nodes)

	OrderedBy(ID).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].id < nodes[i-1].id {
			t.Error("by id: not sorted")
			printNodes(t, nodes)
		}
	}

	OrderedBy(LastNodeError).Sort(nodes)
	for i := n - 1; i > 0; i-- {
		if nodes[i].LastErr() == nil && nodes[i-1].LastErr() != nil {
			t.Error("by error: not sorted")
			printNodes(t, nodes)
		}
	}
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

// TestNodeRouteResponse verifies that Node.RouteResponse correctly routes
// an incoming response message to a pending server-initiated call.
func TestNodeRouteResponse(t *testing.T) {
	n := newInboundNode(42, "127.0.0.1:9000", func() uint64 { return 0 })
	replyChan := make(chan NodeResponse[*stream.Message], 1)

	// Register a pending call with msgID=7 on the node's router.
	n.router.Register(7, stream.Request{
		Ctx:          context.Background(),
		Msg:          &stream.Message{},
		ResponseChan: replyChan,
	})

	// Build a response message with matching msgID.
	respMsg := stream.Message_builder{MessageSeqNo: 7}.Build()

	// RouteResponse should find and deliver the pending call.
	if !n.RouteResponse(respMsg) {
		t.Fatal("RouteResponse should return true for a pending msgID")
	}

	// Verify the response was delivered.
	select {
	case got := <-replyChan:
		if got.NodeID != 42 {
			t.Errorf("NodeID = %d, want 42", got.NodeID)
		}
	default:
		t.Fatal("expected response on channel")
	}

	// Routing the same msgID again should return false (already consumed).
	if n.RouteResponse(respMsg) {
		t.Error("RouteResponse should return false for consumed msgID")
	}

	// Unknown msgID should return false.
	unknownMsg := stream.Message_builder{MessageSeqNo: 999}.Build()
	if n.RouteResponse(unknownMsg) {
		t.Error("RouteResponse should return false for unknown msgID")
	}
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
		n := newInboundNode(1, "127.0.0.1:9081", func() uint64 { return 0 })
		b.ResetTimer()
		for range b.N {
			n.Enqueue(req)
		}
	})

	b.Run("AtomicLoadNonNil", func(b *testing.B) {
		// Stub channel attached; measures atomic.Pointer.Load() + non-nil branch
		// without going through Channel.Enqueue (which requires a running goroutine).
		n := newInboundNode(1, "127.0.0.1:9081", func() uint64 { return 0 })
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
	n := newInboundNode(1, lis.Addr().String(), func() uint64 { return 0 })
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
