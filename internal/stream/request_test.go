package stream

import (
	"context"
	"errors"
	"testing"
	"time"
)

func (f requestHandlerFunc) HandleRequest(ctx context.Context, _ ID, msg *Message, release func(), send func(*Message)) {
	f(ctx, msg, release, send)
}

func TestRequestSendErrorResponseDoesNotBlockOnCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	responseChan := make(chan response, 1)
	responseChan <- response{NodeID: 99} // fill the channel
	req := Request{
		Ctx:          ctx,
		ResponseChan: responseChan,
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
	responseChan := make(chan response, 1)
	req := Request{
		Ctx:          ctx,
		ResponseChan: responseChan,
	}
	cancel()

	req.sendErrorResponse(7, ErrStreamDown)

	select {
	case got := <-responseChan:
		if !errors.Is(got.Err, ErrStreamDown) {
			t.Fatalf("response error = %v, want ErrStreamDown", got.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("sendErrorResponse dropped a ready delivery on canceled context")
	}
}
