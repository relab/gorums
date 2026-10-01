package stream

import (
	"context"
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
// request whose Send is stalled leaves the stream in place, and that later
// requests are sent on it once the stall ends.
func TestChannelCancelledSendKeepsStream(t *testing.T) {
	tests := []struct {
		name    string
		channel func(*testing.T, BidiStream) *Channel
	}{
		{name: "Inbound", channel: func(t *testing.T, st BidiStream) *Channel {
			return NewInboundChannel(t.Context(), 1, 4, st, NewMessageRouter())
		}},
		{name: "Outbound", channel: func(t *testing.T, st BidiStream) *Channel {
			return newChannel(t.Context(), 1, 4, newUnavailableClientConn(t), st, NewMessageRouter(), false, nil)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newGatedSendStream()
			c := tt.channel(t, st)
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
			if c.getStream() == nil {
				t.Fatal("cancelling a stalled send cleared the stream")
			}

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
	}
}
