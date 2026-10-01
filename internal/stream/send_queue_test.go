package stream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSendQueuePush(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name        string
		closeDone   bool // close the done channel before pushing
		closeQueue  bool // close the queue before pushing
		ctx         context.Context
		wait        bool
		noReplyChan bool
		wantErr     error
		wantDropped int64
	}{
		{name: "FullNoWaitFailsFast", ctx: context.Background(), wantErr: ErrSendQueueFull},
		{name: "FullNoWaitDropsReply", ctx: context.Background(), noReplyChan: true, wantDropped: 1},
		{name: "FullWaitEndsWithContext", ctx: cancelled, wait: true, wantErr: context.Canceled},
		{name: "FullWaitEndsWithDone", ctx: context.Background(), wait: true, closeDone: true, wantErr: ErrNodeClosed},
		{name: "ClosedFails", ctx: context.Background(), closeQueue: true, wantErr: ErrNodeClosed},
		{name: "ClosedDropsReply", ctx: context.Background(), closeQueue: true, noReplyChan: true, wantDropped: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan struct{})
			q := newSendQueue(1, 1, done)
			// Fill the queue with a request that has its own response channel.
			q.push(Request{Ctx: context.Background(), Msg: &Message{}, ResponseChan: make(chan response, 1)}, false)
			if tt.closeDone {
				close(done)
			}
			if tt.closeQueue {
				q.close()
			}
			replyCh := make(chan response, 1)
			req := Request{Ctx: tt.ctx, Msg: &Message{}, ResponseChan: replyCh}
			if tt.noReplyChan {
				req.ResponseChan = nil
			}
			q.push(req, tt.wait)

			if tt.wantErr != nil {
				select {
				case resp := <-replyCh:
					if !errors.Is(resp.Err, tt.wantErr) {
						t.Errorf("reply error = %v, want %v", resp.Err, tt.wantErr)
					}
				default:
					t.Errorf("no reply, want %v", tt.wantErr)
				}
			}
			if got := q.dropped.Load(); got != tt.wantDropped {
				t.Errorf("dropped = %d, want %d", got, tt.wantDropped)
			}
		})
	}
}

// TestSendQueueWaitsForSpace verifies that a waiting push completes once the
// consumer makes room.
func TestSendQueueWaitsForSpace(t *testing.T) {
	q := newSendQueue(1, 1, nil)
	q.push(Request{Ctx: context.Background(), Msg: &Message{}}, false)
	pushed := make(chan struct{})
	go func() {
		q.push(Request{Ctx: context.Background(), Msg: &Message{}}, true)
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
		replies := make(chan response, pushers*perPusher)
		var wg sync.WaitGroup
		for range pushers {
			wg.Go(func() {
				for range perPusher {
					q.push(Request{Ctx: context.Background(), Msg: &Message{}, ResponseChan: replies}, false)
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
		if n := len(replies); n != pushers*perPusher {
			t.Fatalf("got %d replies, want %d", n, pushers*perPusher)
		}
	}
}
