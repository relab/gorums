package stream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestSendQueuePush verifies rejection has no asynchronous side effects.
func TestSendQueuePush(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name            string
		push            func(*sendQueue, NodeRequest) error
		ctx             context.Context
		closed, stopped bool
		want            error
	}{
		{name: "Full", push: (*sendQueue).tryPush, ctx: context.Background(), want: ErrSendQueueFull},
		{name: "CanceledWait", push: (*sendQueue).push, ctx: canceled, want: context.Canceled},
		{name: "StoppedWait", push: (*sendQueue).push, ctx: context.Background(), stopped: true, want: ErrNodeClosed},
		{name: "Closed", push: (*sendQueue).tryPush, ctx: context.Background(), closed: true, want: ErrNodeClosed},
		{name: "ClosedWait", push: (*sendQueue).push, ctx: context.Background(), closed: true, want: ErrNodeClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan struct{})
			q := newSendQueue(1, 1, done)
			q.tryPush(NodeRequest{Ctx: context.Background(), Oneway: true})
			if tt.closed {
				q.close()
			}
			if tt.stopped {
				close(done)
			}
			responses := make(chan response, 1)
			err := tt.push(q, NodeRequest{Ctx: tt.ctx, ResponseChan: responses})
			if !errors.Is(err, tt.want) {
				t.Fatalf("admission = %v, want %v", err, tt.want)
			}
			if len(responses) != 0 {
				t.Fatal("rejection also sent on response channel")
			}
			if q.dropped.Load() != 0 {
				t.Fatal("rejection counted as a dropped reply")
			}
		})
	}
}

// TestSendQueueWaitsForSpace verifies that a waiting push completes once the
// consumer makes room.
func TestSendQueueWaitsForSpace(t *testing.T) {
	q := newSendQueue(1, 1, nil)
	q.tryPush(NodeRequest{Ctx: context.Background(), Msg: &Message{}})
	pushed := make(chan struct{})
	go func() {
		q.push(NodeRequest{Ctx: context.Background(), Msg: &Message{}})
		close(pushed)
	}()
	select {
	case <-pushed:
		t.Fatal("push did not wait for space")
	case <-time.After(20 * time.Millisecond):
	}
	<-q.ch
	select {
	case <-pushed:
	case <-time.After(defaultTestTimeout):
		t.Fatal("push did not complete after space was made")
	}
}

// TestSendQueueCloseAnswersEveryRequest verifies that every request pushed
// concurrently with close is either queued and then failed by close, or failed
// by push; none is left in the queue.
func TestSendQueueCloseAnswersEveryRequest(t *testing.T) {
	const pushers = 8
	const perPusher = 100
	for range 20 {
		done := make(chan struct{})
		q := newSendQueue(1, pushers*perPusher, done)
		responseChan := make(chan response, pushers*perPusher)
		var wg sync.WaitGroup
		for range pushers {
			wg.Go(func() {
				for range perPusher {
					if err := q.tryPush(NodeRequest{Ctx: context.Background(), Msg: &Message{}, ResponseChan: responseChan}); err != nil {
						responseChan <- response{Err: err}
					}
				}
			})
		}
		close(done)
		q.close()
		wg.Wait()
		// Requests pushed before close was entered were drained by it.
		if n := len(q.ch); n != 0 {
			t.Fatalf("%d requests left in a closed queue", n)
		}
		if n := len(responseChan); n != pushers*perPusher {
			t.Fatalf("got %d responses, want %d", n, pushers*perPusher)
		}
	}
}
