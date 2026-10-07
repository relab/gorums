package stream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/relab/gorums/internal/testutils/mock"
)

// TestInboundChannel verifies that an inbound channel sends a one-way request
// and confirms it without a routed response.
func TestInboundChannel(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 10})
	t.Cleanup(func() {
		_ = c.Close()
	})

	// Send a message and verify it is delivered to the stream.
	resp := sendRequest(t, c, Request{Oneway: true}, 1)
	if resp.Err != nil {
		t.Errorf("unexpected error: %v", resp.Err)
	}
	if resp.NodeID != 1 {
		t.Errorf("NodeID = %d, want 1", resp.NodeID)
	}
}

// TestInboundChannelClose verifies that a closed inbound channel fails later
// requests with ErrNodeClosed.
func TestInboundChannelClose(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 10})

	if err := c.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Subsequent sends should fail with ErrNodeClosed.
	resp := sendRequest(t, c, Request{Oneway: true}, 2)
	if resp.Err == nil {
		t.Error("expected error after close, got nil")
	} else if !errors.Is(resp.Err, ErrNodeClosed) {
		t.Errorf("expected 'node closed' error, got: %v", resp.Err)
	}

	if !c.session.ended() {
		t.Error("session still running after close")
	}
}

// TestInboundChannelStreamDown verifies that an inbound channel whose stream
// ended fails later requests with ErrNodeClosed instead of reconnecting.
func TestInboundChannelStreamDown(t *testing.T) {
	stream := newMockBidiStream()
	c := NewInboundChannel(t.Context(), 1, stream, InboundOptions{SendBufferSize: 10})

	// Verify initial send works.
	resp := sendRequest(t, c, Request{Oneway: true}, 1)
	if resp.Err != nil {
		t.Fatalf("initial send failed: %v", resp.Err)
	}

	// The stream ends, as Serve observes, and the channel is closed, as the
	// NodeStream cleanup does.
	stream.close()
	if err := c.Serve(); err == nil {
		t.Fatal("Serve returned nil after the stream ended")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// Sends after close must fail with ErrNodeClosed, not silently reconnect.
	resp = sendRequest(t, c, Request{Oneway: true}, 2)
	if resp.Err == nil {
		t.Error("expected error after stream down, got nil")
	} else if !errors.Is(resp.Err, ErrNodeClosed) {
		t.Errorf("expected 'node closed' error, got: %v", resp.Err)
	}

	if !c.session.ended() {
		t.Error("inbound session still running after its stream ended")
	}
}

// TestInboundChannelCloseReturnsWhileSendBlocked verifies that inbound cleanup does
// not wait for a transport send whose completion depends on the RPC returning.
func TestInboundChannelCloseReturnsWhileSendBlocked(t *testing.T) {
	stream := newBlockingSendStream()
	t.Cleanup(stream.close)
	c := NewInboundChannel(context.Background(), 1, stream, InboundOptions{})
	t.Cleanup(func() { _ = c.Close() })

	c.Enqueue(Request{
		Ctx:    context.Background(),
		Oneway: true,
		Msg:    Message_builder{MessageSeqNo: 1, Method: mock.TestMethod}.Build(),
	})
	select {
	case <-stream.entered:
	case <-time.After(defaultTestTimeout):
		t.Fatal("sender never entered Send")
	}

	closed := make(chan struct{})
	go func() {
		_ = c.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(defaultTestTimeout):
		t.Fatal("Close waited for the blocked transport send")
	}
	stream.release()
}
