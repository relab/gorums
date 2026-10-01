package stream

import (
	"context"
	"maps"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
)

// gatedSendStream is a stream whose Send blocks while the gate is closed, as a
// send stalled by the peer's flow control does. Recv blocks until done closes.
type gatedSendStream struct {
	gate    chan struct{}
	entered chan struct{}
	done    chan struct{}
	sent    atomic.Int32
}

func newGatedSendStream() *gatedSendStream {
	return &gatedSendStream{
		gate:    make(chan struct{}),
		entered: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
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
	<-s.done
	return nil, context.Canceled
}

// TestChannelCancelledSendKeepsStream verifies that cancelling the context of a
// request whose Send is stalled leaves the stream in place.
func TestChannelCancelledSendKeepsStream(t *testing.T) {
	t.Run("Inbound", func(t *testing.T) {
		st := newGatedSendStream()
		c := NewInboundChannel(t.Context(), 1, st, InboundOptions{SendBufferSize: 4})
		t.Cleanup(func() {
			close(st.done)
			_ = c.Close()
		})

		ctx, cancel := context.WithCancel(t.Context())
		c.Enqueue(Request{
			Ctx:          ctx,
			Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
			ResponseChan: make(chan response, 1),
		})
		select {
		case <-st.entered:
		case <-time.After(defaultTestTimeout):
			t.Fatal("Send was not reached")
		}
		cancel()
		time.Sleep(50 * time.Millisecond)
		if c.session.ended() {
			t.Fatal("cancelling a stalled send ended the stream")
		}

		// Once the stall ends, later requests are sent on the same stream.
		close(st.gate)
		c.Enqueue(Request{
			Ctx:          t.Context(),
			Msg:          Message_builder{MessageSeqNo: 2, Method: mock.TestMethod}.Build(),
			ResponseChan: make(chan response, 1),
		})
		deadline := time.Now().Add(defaultTestTimeout)
		for st.sent.Load() < 2 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if got := st.sent.Load(); got < 2 {
			t.Fatalf("sends after the stalled one: got %d total, want 2", got)
		}
	})

	t.Run("Outbound", func(t *testing.T) {
		// The server never reads, so a message larger than the flow-control
		// window stalls the client's Send.
		tc := setupChannel(t, holdServer)
		if !waitForConnection(tc.OutboundChannel, streamConnectTimeout) {
			t.Fatal("channel never connected")
		}
		tc.mu.Lock()
		sessions := slices.Collect(maps.Keys(tc.sessions))
		tc.mu.Unlock()
		if len(sessions) != 1 {
			t.Fatalf("sessions = %d, want 1", len(sessions))
		}

		ctx, cancel := context.WithCancel(t.Context())
		tc.Enqueue(Request{
			Ctx:          ctx,
			Msg:          Message_builder{MessageSeqNo: 1, Method: mock.TestMethod, Payload: make([]byte, 2<<20)}.Build(),
			ResponseChan: make(chan response, 1),
		})
		time.Sleep(100 * time.Millisecond)
		cancel()
		time.Sleep(50 * time.Millisecond)
		if sessions[0].ended() {
			t.Fatal("cancelling a stalled send ended the stream")
		}
	})
}
