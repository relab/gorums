package stream

import (
	"context"
	"errors"
	"testing"
	"time"
)

func (f requestHandlerFunc) HandleRequest(ctx context.Context, msg *Message, release func(), send func(*Message)) {
	f(ctx, msg, release, send)
}

func TestRequestSendErrorResponseDoesNotBlockOnCanceledRequest(t *testing.T) {
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
		req.sendErrorResponse(7, ErrStreamDown)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sendErrorResponse blocked on a canceled request with a full response channel")
	}
}

func TestRequestSendErrorResponsePrefersDeliveryWhenCanceledAndResponseChanReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	replyChan := make(chan response, 1)
	req := Request{
		Ctx:          ctx,
		ResponseChan: replyChan,
	}
	cancel()

	req.sendErrorResponse(7, ErrStreamDown)

	select {
	case got := <-replyChan:
		if !errors.Is(got.Err, ErrStreamDown) {
			t.Fatalf("reply error = %v, want ErrStreamDown", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("sendErrorResponse dropped a ready delivery on canceled context")
	}
}
