package stream

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
)

func TestRouterRegisterAndRoute(t *testing.T) {
	r := NewMessageRouter()
	replyChan := make(chan response, 1)
	r.Register(42, Request{
		Ctx:          context.Background(),
		Msg:          &Message{},
		ResponseChan: replyChan,
	})

	resp := response{NodeID: 1, Value: nil}
	if !r.RouteResponse(42, resp) {
		t.Fatal("RouteResponse should return true for registered msgID")
	}

	// The response should be delivered on the channel.
	select {
	case got := <-replyChan:
		if got.NodeID != 1 {
			t.Errorf("NodeID = %d, want 1", got.NodeID)
		}
	default:
		t.Fatal("expected response on channel")
	}

	// After routing a non-streaming request, it should be removed.
	if r.RouteResponse(42, resp) {
		t.Error("RouteResponse should return false for already-consumed msgID")
	}
}

func TestRouterRouteUnknown(t *testing.T) {
	r := NewMessageRouter()

	if r.RouteResponse(999, response{NodeID: 1}) {
		t.Error("RouteResponse should return false for unknown msgID")
	}
}

// TestRouterRouteResponseServerInitiated verifies that RouteResponse absorbs
// unmatched server-initiated IDs and rejects unmatched client-initiated IDs.
func TestRouterRouteResponseServerInitiated(t *testing.T) {
	t.Run("ServerInitiatedReturnsTrue", func(t *testing.T) {
		r := NewMessageRouter()
		if !r.RouteResponse(ServerSequenceNumber(1), response{NodeID: 1}) {
			t.Error("RouteResponse should return true for server-initiated msgID")
		}
	})

	t.Run("ClientInitiatedUnknownReturnsFalse", func(t *testing.T) {
		r := NewMessageRouter()
		if r.RouteResponse(1, response{NodeID: 1}) {
			t.Error("RouteResponse should return false for unmatched client-initiated msgID")
		}
	})
}

func TestRouterStreamingKeepsEntry(t *testing.T) {
	r := NewMessageRouter()
	replyChan := make(chan response, 3)
	r.Register(10, Request{
		Ctx:          context.Background(),
		Msg:          &Message{},
		Streaming:    true,
		ResponseChan: replyChan,
	})

	// First route should succeed and keep the entry.
	resp := response{NodeID: 1}
	if !r.RouteResponse(10, resp) {
		t.Fatal("first RouteResponse should succeed")
	}
	<-replyChan // drain

	// Second route should also succeed (streaming keeps entry alive).
	if !r.RouteResponse(10, resp) {
		t.Fatal("second RouteResponse should succeed for streaming entry")
	}
	<-replyChan // drain

	// Third route should also succeed.
	if !r.RouteResponse(10, resp) {
		t.Fatal("third RouteResponse should succeed for streaming entry")
	}
	<-replyChan // drain
}

func TestRouterCancelPending(t *testing.T) {
	r := NewMessageRouter()
	for i := range 5 {
		r.Register(uint64(i), Request{
			Ctx:          context.Background(),
			Msg:          &Message{},
			ResponseChan: make(chan response, 1),
		})
	}

	cancelled := r.CancelPending()
	if len(cancelled) != 5 {
		t.Errorf("CancelPending returned %d requests, want 5", len(cancelled))
	}

	// Map should be empty now.
	if r.RouteResponse(0, response{}) {
		t.Error("pending map should be empty after CancelPending")
	}
}

func TestRouterRequeuePending(t *testing.T) {
	r := NewMessageRouter()

	// Register 3 non-streaming and 2 streaming requests.
	for i := range 3 {
		msg, _ := NewMessage(context.Background(), uint64(i), mock.TestMethod, nil)
		r.Register(uint64(i), Request{
			Ctx:          context.Background(),
			Msg:          msg,
			Streaming:    false,
			ResponseChan: make(chan response, 1),
		})
	}
	for i := 3; i < 5; i++ {
		msg, _ := NewMessage(context.Background(), uint64(i), mock.TestMethod, nil)
		r.Register(uint64(i), Request{
			Ctx:          context.Background(),
			Msg:          msg,
			Streaming:    true,
			ResponseChan: make(chan response, 1),
		})
	}

	requeue, cancel := r.RequeuePending()
	if len(requeue) != 3 {
		t.Errorf("requeue: got %d requests, want 3", len(requeue))
	}
	if len(cancel) != 2 {
		t.Errorf("cancel: got %d requests, want 2", len(cancel))
	}

	// Verify all streaming requests are in the cancel list.
	for _, req := range cancel {
		if !req.Streaming {
			t.Error("cancel list should only contain streaming requests")
		}
	}
	for _, req := range requeue {
		if req.Streaming {
			t.Error("requeue list should only contain non-streaming requests")
		}
	}

	// Map should be empty.
	if r.RouteResponse(0, response{}) {
		t.Error("pending map should be empty after RequeuePending")
	}
}

// TestRouterRouteInboundMessage verifies RouteInboundMessage demultiplexes
// inbound server-side messages: server-initiated IDs (high bit set) are routed
// to the pending map; client-initiated IDs (low bit) are passed to the caller.
func TestRouterRouteInboundMessage(t *testing.T) {
	t.Run("ClientInitiatedReturnsFalse", func(t *testing.T) {
		r := NewMessageRouter()
		msg := Message_builder{MessageSeqNo: 1}.Build() // low-bit ID
		if r.RouteInboundMessage(1, msg) {
			t.Error("RouteInboundMessage should return false for client-initiated ID")
		}
	})

	t.Run("ServerInitiatedPendingDelivered", func(t *testing.T) {
		r := NewMessageRouter()
		replyChan := make(chan response, 1)
		msgID := ServerSequenceNumber(5)
		r.Register(msgID, Request{
			Ctx:          context.Background(),
			Msg:          &Message{},
			ResponseChan: replyChan,
		})
		msg := Message_builder{MessageSeqNo: msgID}.Build()
		if !r.RouteInboundMessage(42, msg) {
			t.Fatal("RouteInboundMessage should return true for server-initiated ID")
		}
		select {
		case got := <-replyChan:
			if got.NodeID != 42 {
				t.Errorf("NodeID = %d, want 42", got.NodeID)
			}
		default:
			t.Fatal("expected response on channel")
		}
		// Entry consumed — routing again returns true (silently absorbed).
		if !r.RouteInboundMessage(42, msg) {
			t.Error("RouteInboundMessage should return true (absorbed) for stale server-initiated ID")
		}
	})

	t.Run("ServerInitiatedStaleAbsorbed", func(t *testing.T) {
		r := NewMessageRouter()
		msgID := ServerSequenceNumber(7)
		msg := Message_builder{MessageSeqNo: msgID}.Build()
		// No pending entry — should still return true (silently absorbed).
		if !r.RouteInboundMessage(1, msg) {
			t.Error("RouteInboundMessage should return true (absorbed) for unmatched server-initiated ID")
		}
	})
}

type mockRequestHandler struct {
	called atomic.Bool
	done   chan struct{}
}

func newMockRequestHandler() *mockRequestHandler {
	return &mockRequestHandler{done: make(chan struct{})}
}

func (m *mockRequestHandler) HandleRequest(_ context.Context, _ *Message, release func(), _ func(*Message)) {
	m.called.Store(true)
	release()
	if m.done != nil {
		close(m.done)
	}
}

func TestRouterRouteResponseDoesNotBlockOnCanceledRequest(t *testing.T) {
	r := NewMessageRouter()
	ctx, cancel := context.WithCancel(context.Background())
	replyChan := make(chan response, 1)
	replyChan <- response{NodeID: 99} // fill the channel
	r.Register(42, Request{
		Ctx:          ctx,
		Msg:          &Message{},
		ResponseChan: replyChan,
	})
	cancel()

	done := make(chan struct{})
	go func() {
		if !r.RouteResponse(42, response{NodeID: 1}) {
			t.Error("RouteResponse should return true for registered msgID")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RouteResponse blocked on a canceled request with a full reply channel")
	}
}

func TestRouterRouteResponsePrefersDeliveryWhenCanceledAndReplyChanReady(t *testing.T) {
	r := NewMessageRouter()
	ctx, cancel := context.WithCancel(context.Background())
	replyChan := make(chan response, 1)
	r.Register(42, Request{
		Ctx:          ctx,
		Msg:          &Message{},
		ResponseChan: replyChan,
	})
	cancel()

	if !r.RouteResponse(42, response{NodeID: 1, Err: ErrStreamDown}) {
		t.Fatal("RouteResponse should return true for registered msgID")
	}

	select {
	case got := <-replyChan:
		if got.NodeID != 1 {
			t.Fatalf("NodeID = %d, want 1", got.NodeID)
		}
		if !errors.Is(got.Err, ErrStreamDown) {
			t.Fatalf("reply error = %v, want ErrStreamDown", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("RouteResponse dropped a ready delivery on canceled context")
	}
}

func TestReplyErrorDoesNotBlockOnCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	replyChan := make(chan response, 1)
	replyChan <- response{NodeID: 99} // fill the channel
	req := Request{
		Ctx:          ctx,
		ResponseChan: replyChan,
	}
	cancel()

	done := make(chan struct{})
	go func() {
		req.replyError(7, ErrStreamDown)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("replyError blocked on a canceled request with a full reply channel")
	}
}

func TestReplyErrorPrefersDeliveryWhenCanceledAndReplyChanReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	replyChan := make(chan response, 1)
	req := Request{
		Ctx:          ctx,
		ResponseChan: replyChan,
	}
	cancel()

	req.replyError(7, ErrStreamDown)

	select {
	case got := <-replyChan:
		if !errors.Is(got.Err, ErrStreamDown) {
			t.Fatalf("reply error = %v, want ErrStreamDown", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("replyError dropped a ready delivery on canceled context")
	}
}

// TestRouterRouteMessage verifies the three dispatch branches of RouteMessage.
func TestRouterRouteMessage(t *testing.T) {
	const nodeID = uint32(1)
	connCtx := context.Background()

	t.Run("RoutesToPendingCall", func(t *testing.T) {
		r := NewMessageRouter()
		replyChan := make(chan response, 1)
		r.Register(42, Request{
			Ctx:          connCtx,
			Msg:          &Message{},
			ResponseChan: replyChan,
		})

		msg := Message_builder{MessageSeqNo: 42, Method: mock.TestMethod}.Build()
		r.RouteMessage(connCtx, nodeID, msg, nil)
		select {
		case got := <-replyChan:
			if got.NodeID != nodeID {
				t.Errorf("NodeID = %d, want %d", got.NodeID, nodeID)
			}
		default:
			t.Fatal("expected response on channel")
		}
	})

	t.Run("ServerInitiatedDispatchesHandler", func(t *testing.T) {
		handler := newMockRequestHandler()
		r := NewMessageRouter(handler)

		enqueueCalled := false
		enqueue := func(Request) { enqueueCalled = true }

		msg := Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build()
		r.RouteMessage(connCtx, nodeID, msg, enqueue)
		// Wait for the handler goroutine to complete before reading handler.called.
		select {
		case <-handler.done:
		case <-time.After(time.Second):
			t.Fatal("handler was not called within timeout")
		}
		if !handler.called.Load() {
			t.Error("handler was not called for server-initiated message")
		}
		// enqueue should not be triggered by the dispatch itself (only by send closure).
		if enqueueCalled {
			t.Error("enqueue should not be called during request dispatch")
		}
	})

	t.Run("ServerInitiatedNoHandlerIsSilentlyDropped", func(_ *testing.T) {
		r := NewMessageRouter()
		msg := Message_builder{MessageSeqNo: ServerSequenceNumber(1), Method: mock.TestMethod}.Build()
		r.RouteMessage(connCtx, nodeID, msg, nil) // must not panic
	})

	t.Run("ClientInitiatedUnknownIsSilentlyDropped", func(_ *testing.T) {
		r := NewMessageRouter()
		msg := Message_builder{MessageSeqNo: 999, Method: mock.TestMethod}.Build()
		r.RouteMessage(connCtx, nodeID, msg, nil) // must not panic
	})
}
