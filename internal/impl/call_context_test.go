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

// TestCallContextSendSharedMessageIDs verifies that sendShared reuses a single
// message with one client-initiated ID for every node in the configuration.
func TestCallContextSendSharedMessageIDs(t *testing.T) {
	var clientID atomic.Uint64
	clientGen := func() uint64 { return clientID.Add(1) }

	recorders := make([]*seqNoRecorder, 3)
	config := make(Config, 3)
	for i := range config {
		recorders[i] = &seqNoRecorder{}
		id := uint32(i + 1)
		router := stream.NewMessageRouter(recorders[i])
		transport := stream.NewTransport(id, clientGen, router)
		transport.StoreChannel(stream.NewLocalChannel(id, router))
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

	first := recorders[0].first(t)
	for i, r := range recorders[1:] {
		if got := r.first(t); got != first {
			t.Errorf("node %d got ID %d, want the shared message ID %d", i+2, got, first)
		}
	}
}
