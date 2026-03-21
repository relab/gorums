package stream

import (
	"context"
	"errors"
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

// TestRouterRouteResponseServerInitiated verifies the three dispatch branches
// for server-initiated sequence numbers in RouteResponse.
func TestRouterRouteResponseServerInitiated(t *testing.T) {
	t.Run("DispatchCalledAndReturnsTrue", func(t *testing.T) {
		r := NewMessageRouter()
		dispatched := false
		if !r.RouteResponse(ServerSequenceNumber(1), response{NodeID: 1}, func() {
			dispatched = true
		}) {
			t.Error("RouteResponse should return true for server-initiated msgID")
		}
		if !dispatched {
			t.Error("dispatch function should be called for server-initiated msgID")
		}
	})

	t.Run("NoDispatchArgReturnsTrue", func(t *testing.T) {
		r := NewMessageRouter()
		if !r.RouteResponse(ServerSequenceNumber(1), response{NodeID: 1}) {
			t.Error("RouteResponse should return true for server-initiated msgID with no dispatch arg")
		}
	})

	t.Run("NilDispatchArgReturnsTrue", func(t *testing.T) {
		r := NewMessageRouter()
		if !r.RouteResponse(ServerSequenceNumber(1), response{NodeID: 1}, nil) {
			t.Error("RouteResponse should return true for server-initiated msgID with nil dispatch")
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

type mockRequestHandler struct {
	called bool
}

func (m *mockRequestHandler) HandleRequest(_ context.Context, _ *Message, release func(), _ func(*Message)) {
	m.called = true
	release()
}

func TestRouterRequestHandler(t *testing.T) {
	t.Run("NoHandler", func(t *testing.T) {
		r := NewMessageRouter()
		if _, ok := r.RequestHandler(); ok {
			t.Error("RequestHandler should return false when no handler is set")
		}
	})

	t.Run("WithHandler", func(t *testing.T) {
		mockHandler := &mockRequestHandler{}
		r := NewMessageRouter(mockHandler)

		h, ok := r.RequestHandler()
		if !ok {
			t.Fatal("RequestHandler should return true for registered handler")
		}
		if h == nil {
			t.Fatal("RequestHandler returned nil handler")
		}

		// Verify the handler is callable.
		h.HandleRequest(context.Background(), &Message{}, func() {}, func(*Message) {})
		if !mockHandler.called {
			t.Error("handler was not called")
		}
	})
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
		c := &Channel{id: 7}
		c.replyError(req, ErrStreamDown)
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

	c := &Channel{id: 7}
	c.replyError(req, ErrStreamDown)

	select {
	case got := <-replyChan:
		if !errors.Is(got.Err, ErrStreamDown) {
			t.Fatalf("reply error = %v, want ErrStreamDown", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("replyError dropped a ready delivery on canceled context")
	}
}
