package impl

import (
	"context"
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
		router := stream.NewMessageRouter(recorders[i])
		transport := stream.NewTransport(id, clientGen, router)
		transport.StoreChannel(stream.NewLocalChannel(id, router))
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
		oneway:  true, // fire-and-forget: no reply channel needed
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
