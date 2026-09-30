package stream

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
)

// gatedSendStream is a server-side stream whose Send blocks while the gate is
// closed, as a send stalled by the peer's flow control does.
type gatedSendStream struct {
	gate    chan struct{}
	entered chan struct{}
	sent    atomic.Int32
}

func (s *gatedSendStream) Send(*Message) error {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	<-s.gate
	s.sent.Add(1)
	return nil
}

func (s *gatedSendStream) Recv() (*Message, error) {
	select {}
}

// TestChannelInboundCancelledSendKeepsStream verifies that cancelling the
// context of a request whose Send is stalled does not clear an inbound
// channel's stream. An inbound channel cannot open a new stream, so clearing
// it would leave the channel unable to carry any later request.
func TestChannelInboundCancelledSendKeepsStream(t *testing.T) {
	st := &gatedSendStream{gate: make(chan struct{}), entered: make(chan struct{}, 1)}
	c := NewInboundChannel(t.Context(), 1, 4, st, NewMessageRouter())
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	c.Enqueue(Request{
		Ctx:          ctx,
		Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
		ResponseChan: make(chan response, 1),
	})
	select {
	case <-st.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Send was not reached")
	}
	cancel()
	// Give the cancel watcher time to run.
	time.Sleep(50 * time.Millisecond)
	if c.getStream() == nil {
		t.Fatal("cancelling a stalled send cleared the inbound stream")
	}

	close(st.gate)
	r := make(chan response, 1)
	c.Enqueue(Request{
		Ctx:          t.Context(),
		Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
		ResponseChan: r,
	})
	deadline := time.Now().Add(3 * time.Second)
	for st.sent.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := st.sent.Load(); got < 2 {
		t.Fatalf("sends after the stalled one: got %d total, want 2", got)
	}
}
