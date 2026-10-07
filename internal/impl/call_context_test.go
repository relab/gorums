package impl

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/stream"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// seqNoRecorder records the message sequence numbers of requests dispatched to it.
type seqNoRecorder struct {
	mu     sync.Mutex
	seqNos []uint64
}

func (h *seqNoRecorder) HandleRequest(_ context.Context, msg *stream.Message, release func(), _ func(*stream.Message)) {
	h.mu.Lock()
	h.seqNos = append(h.seqNos, msg.GetMessageSeqNo())
	h.mu.Unlock()
	release()
}

// first returns the first recorded sequence number, waiting briefly for the
// asynchronous handler dispatch to complete.
func (h *seqNoRecorder) first(t *testing.T) uint64 {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		h.mu.Lock()
		if len(h.seqNos) > 0 {
			seqNo := h.seqNos[0]
			h.mu.Unlock()
			return seqNo
		}
		h.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for request dispatch")
	return 0
}

// wait returns the recorded sequence numbers once n requests have been
// dispatched, waiting briefly for the asynchronous handler dispatch.
func (h *seqNoRecorder) wait(t *testing.T, n int) []uint64 {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		h.mu.Lock()
		if len(h.seqNos) >= n {
			seqNos := slices.Clone(h.seqNos)
			h.mu.Unlock()
			return seqNos
		}
		h.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d dispatched requests", n)
	return nil
}

// TestCallContextSendSharedMessageIDs verifies that sendShared reuses a
// single message with one client-initiated ID for all regular nodes, while a
// shared dedup node gets its own message with a server-initiated ID.
func TestCallContextSendSharedMessageIDs(t *testing.T) {
	var clientID, serverID atomic.Uint64
	clientGen := func() uint64 { return clientID.Add(1) }
	serverGen := func() uint64 { return stream.ServerSequenceNumber(serverID.Add(1)) }

	recorders := make([]*seqNoRecorder, 3)
	config := make(Config, 3)
	for i := range config {
		recorders[i] = &seqNoRecorder{}
		id := uint32(i + 1)
		transport := stream.NewTransport(id, clientGen)
		transport.StoreChannel(stream.NewLocalChannel(id, recorders[i]))
		// The third node reuses an inbound stream and must use server-initiated
		// IDs; wrap its transport as a shared transport over the same channel.
		if i == 2 {
			transport = stream.NewSharedTransportWithGen(transport, serverGen)
		}
		config[i] = conn.NewNodeForTest(id, transport)
	}

	c := &CallContext[*pb.StringValue, *pb.StringValue]{
		Context: t.Context(),
		config:  config,
		request: pb.String("hello"),
		method:  "test.Method",
		oneway:  true, // fire-and-forget: no response channel needed
	}
	c.sendShared()

	regular1, regular2 := recorders[0].first(t), recorders[1].first(t)
	shared := recorders[2].first(t)
	if regular1 != regular2 {
		t.Errorf("regular nodes got IDs %d and %d, want one shared message ID", regular1, regular2)
	}
	if shared == regular1 {
		t.Errorf("shared node got ID %d, want its own message ID", shared)
	}
	if shared != stream.ServerSequenceNumber(1) {
		t.Errorf("shared node ID = %d, want server-initiated ID %d", shared, stream.ServerSequenceNumber(1))
	}
}

// TestCallContextSendSharedFanOutStart verifies that sendShared reaches every
// node once per call and starts its fan-out at varying nodes, so that no node
// is reached last in every call. Each node is a shared node that draws its
// message ID from one counter when it is visited, so the node holding a call's
// lowest ID was visited first.
func TestCallContextSendSharedFanOutStart(t *testing.T) {
	const nodes, calls = 4, 200
	var serverID atomic.Uint64
	serverGen := func() uint64 { return stream.ServerSequenceNumber(serverID.Add(1)) }
	clientGen := func() uint64 { return 0 }

	recorders := make([]*seqNoRecorder, nodes)
	config := make(Config, nodes)
	for i := range config {
		recorders[i] = &seqNoRecorder{}
		id := uint32(i + 1)
		transport := stream.NewTransport(id, clientGen)
		transport.StoreChannel(stream.NewLocalChannel(id, recorders[i]))
		config[i] = conn.NewNodeForTest(id, stream.NewSharedTransportWithGen(transport, serverGen))
	}

	for range calls {
		c := &CallContext[*pb.StringValue, *pb.StringValue]{
			Context: t.Context(),
			config:  config,
			request: pb.String("hello"),
			method:  "test.Method",
			oneway:  true,
		}
		c.sendShared()
	}

	// Call k holds the IDs nodes*k+1 through nodes*k+nodes, in visit order.
	serverBit := stream.ServerSequenceNumber(0)
	for i, r := range recorders {
		seqNos := r.wait(t, calls)
		if len(seqNos) != calls {
			t.Fatalf("node %d got %d requests, want %d", i+1, len(seqNos), calls)
		}
		first := 0
		for _, seqNo := range seqNos {
			if (seqNo&^serverBit-1)%nodes == 0 {
				first++
			}
		}
		if first == 0 {
			t.Errorf("node %d was never visited first in %d calls", i+1, calls)
		}
	}
}
