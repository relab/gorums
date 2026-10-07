package conn

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/relab/gorums/internal/stream"
	"github.com/relab/gorums/internal/testutils/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newTestNode(id uint32, ch stream.Channel) *Node {
	transport := stream.NewTransport(id, func() uint64 { return 0 })
	transport.StoreChannel(ch)
	return newNode(id, "", nil, transport)
}

func newTestNodeWithLatency(id uint32, latency time.Duration) *Node {
	n := newTestNode(id, nil)
	NodeTransport(n).Latency().Store(latency)
	return n
}

// TestNodeEnqueueWithoutChannel verifies that enqueueing to a node with no
// channel, such as a peer that is currently disconnected, fails fast with
// ErrStreamDown instead of silently dropping the request.
func TestNodeEnqueueWithoutChannel(t *testing.T) {
	peer := stream.NewTransport(1, func() uint64 { return 0 })
	tests := []struct {
		name       string
		node       *Node
		wantNodeID uint32
	}{
		{name: "Owned", node: newNode(1, "", nil, peer), wantNodeID: 1},
		{name: "Shared", node: newNode(1, "", nil, stream.NewSharedTransport(peer)), wantNodeID: 1},
		{name: "NoTransport", node: &Node{}, wantNodeID: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responseChan := make(chan stream.NodeResponse[*stream.Message], 1)
			NodeTransport(tt.node).Enqueue(stream.Request{Ctx: t.Context(), ResponseChan: responseChan})
			select {
			case r := <-responseChan:
				if !errors.Is(r.Err, stream.ErrStreamDown) {
					t.Errorf("Enqueue error = %v, want %v", r.Err, stream.ErrStreamDown)
				}
				if r.NodeID != tt.wantNodeID {
					t.Errorf("Enqueue response NodeID = %d, want %d", r.NodeID, tt.wantNodeID)
				}
			default:
				t.Fatal("expected error response for a node without a channel")
			}
		})
	}
}

func TestNodeCloseCancelsAllPendingRequests(t *testing.T) {
	ch := stream.NewInboundChannel(t.Context(), 1, mock.NewBidiStream[*stream.Message](), stream.InboundOptions{SendBufferSize: 1})
	node := newTestNode(1, ch)
	responseChan := make(chan stream.NodeResponse[*stream.Message], 1)
	NodeTransport(node).Enqueue(stream.Request{
		Ctx:          t.Context(),
		Msg:          stream.Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		ResponseChan: responseChan,
	})
	for deadline := time.Now().Add(time.Second); node.PendingCount() != 1; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("request never became pending")
		}
	}

	if err := node.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case got := <-responseChan:
		if !errors.Is(got.Err, stream.ErrStreamDown) {
			t.Fatalf("pending request error = %v, want ErrStreamDown", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending request was not cancelled")
	}
}

func TestNodeMissingTransportIsSafe(t *testing.T) {
	nodes := map[string]*Node{
		"Nil":       nil,
		"ZeroValue": {},
	}
	for name, node := range nodes {
		t.Run(name, func(t *testing.T) {
			if node.IsInbound() {
				t.Error("IsInbound = true, want false")
			}
			if node.IsOutbound() {
				t.Error("IsOutbound = true, want false")
			}
			if node.IsShared() {
				t.Error("IsShared = true, want false")
			}
			if got := node.PendingCount(); got != 0 {
				t.Errorf("PendingCount = %d, want 0", got)
			}
			if err := node.LastErr(); err != nil {
				t.Errorf("LastErr = %v, want nil", err)
			}
			if got := node.Latency(); got != -1*time.Second {
				t.Errorf("Latency = %v, want -1s", got)
			}
			if err := node.close(); err != nil {
				t.Errorf("close = %v, want nil", err)
			}

		})
	}
}

func TestNodeDetail(t *testing.T) {
	newNodeWithErr := func(id uint32, addr string, err error) *Node {
		transport := stream.NewTransport(id, func() uint64 { return 0 })
		transport.StoreChannel(stream.NewChannelWithState(err))
		return newNode(id, addr, nil, transport)
	}
	tests := []struct {
		name string
		node *Node
		want string
	}{
		{name: "Nil", node: nil, want: nilAngleString},
		{name: "Healthy", node: newNodeWithErr(1, "127.0.0.1:9080", nil), want: "node 1 (127.0.0.1:9080)"},
		{name: "NoTransport", node: newNode(2, "127.0.0.1:9081", nil, nil), want: "node 2 (127.0.0.1:9081)"},
		{name: "Failing", node: newNodeWithErr(3, "127.0.0.1:9082", errors.New("connection refused")), want: "node 3 (127.0.0.1:9082): connection refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.Detail(); got != tt.want {
				t.Errorf("Detail() = %q, want %q", got, tt.want)
			}
		})
	}
}

// BenchmarkNodeEnqueue measures the overhead that Node.Enqueue adds per
// request dispatch: transport and channel atomic loads plus nil guards.
// See BenchmarkChannelSend in internal/stream and BenchmarkNodeEnqueueSend
// below for the full send-path cost.
func BenchmarkNodeEnqueue(b *testing.B) {
	req := stream.Request{}

	b.Run("ChannelNil", func(b *testing.B) {
		// No stream attached, so the channel lookup returns nil immediately.
		n := newInboundNode(1, "127.0.0.1:9081", func() uint64 { return 0 })
		b.ResetTimer()
		for range b.N {
			NodeTransport(n).Enqueue(req)
		}
	})

	b.Run("AtomicLoadNonNil", func(b *testing.B) {
		// Stub channel attached; measures the transport and channel loads
		// without going through Channel.Enqueue, which requires a running goroutine.
		n := newInboundNode(1, "127.0.0.1:9081", func() uint64 { return 0 })
		NodeTransport(n).StoreChannel(stream.NewChannelWithState(nil))
		b.ResetTimer()
		for range b.N {
			_ = n.activeChannel()
		}
	})
}

// BenchmarkNodeEnqueueSend measures the end-to-end send latency going through
// the Node.Enqueue path (transport lookup + Channel.Enqueue) against a live
// echo server.
//
// To get a fair comparison with BenchmarkChannelSend in internal/stream, the
// server is set up identically: a raw gRPC echo handler (benchEchoServer) that
// calls Recv/Send in a loop with no proto marshal/unmarshal, no per-request
// goroutines, and a send buffer of 10. This isolates the one structural
// difference: going through Node.Enqueue (transport and channel lookup)
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

	// Wrap the outbound channel in a Node, adding the transport lookup that
	// Node.Enqueue performs on every dispatch.
	n := newInboundNode(1, lis.Addr().String(), func() uint64 { return 0 })
	ch := stream.NewOutboundChannel(context.Background(), 1, conn, stream.OutboundOptions{SendBufferSize: 10, Latency: NodeTransport(n).Latency()})
	b.Cleanup(func() { _ = ch.Close() })
	NodeTransport(n).StoreChannel(ch)

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
				responseChan := make(chan stream.NodeResponse[*stream.Message], 1)
				reqMsg := stream.Message_builder{
					MessageSeqNo: uint64(i),
					Method:       mock.TestMethod,
					Payload:      payload,
				}.Build()
				NodeTransport(n).Enqueue(stream.Request{
					Ctx:          context.Background(),
					Msg:          reqMsg,
					Oneway:       true,
					ResponseChan: responseChan,
				})
				<-responseChan
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
