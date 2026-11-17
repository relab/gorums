package stream

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"pgregory.net/rapid"
)

func TestSendQueueProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		capacity := rapid.IntRange(1, 8).Draw(t, "capacity")
		done := make(chan struct{})
		q := newSendQueue(7, uint(capacity), done)
		var model []NodeRequest
		responsesByMessage := map[*Message]chan response{}
		closed := false
		var dropped int64
		actions := rapid.SliceOfN(rapid.IntRange(0, 4), 1, 80).Draw(t, "actions")
		for i, action := range actions {
			switch action {
			case 0, 1, 2:
				responses := make(chan response, 1)
				req := NodeRequest{Ctx: t.Context(), Msg: NewRequestFromPayload(t.Context(), uint64(i+1), "test", nil), ResponseChan: responses}
				kind := rapid.IntRange(0, 2).Draw(t, "kind")
				if kind == 1 {
					req.Oneway = true
				}
				if kind == 2 {
					req.ResponseChan = nil
				} // handler reply
				var want error
				if closed {
					want = ErrNodeClosed
				} else if len(model) == capacity {
					want = ErrSendQueueFull
				}
				if got := q.tryPush(req); !errors.Is(got, want) {
					t.Fatalf("tryPush = %v, want %v", got, want)
				}
				if want == nil {
					model = append(model, req)
					responsesByMessage[req.Msg] = responses
				}
				if len(responses) != 0 {
					t.Fatal("admission delivered a response")
				}
			case 3:
				if len(model) > 0 {
					got := <-q.ch
					if got.Msg != model[0].Msg {
						t.Fatal("queue violated FIFO")
					}
					if len(responsesByMessage[got.Msg]) != 0 {
						t.Fatal("dequeued request received a failure")
					}
					delete(responsesByMessage, got.Msg)
					model = model[1:]
				}
			case 4:
				if !closed {
					close(done)
					closed = true
				}
				q.close()
				for _, req := range model {
					if req.ResponseChan == nil {
						dropped++
						continue
					}
					responses := responsesByMessage[req.Msg]
					if len(responses) != 1 {
						t.Fatalf("close delivered %d errors, want one", len(responses))
					}
					got := <-responses
					if got.NodeID != 7 || !errors.Is(got.Err, ErrNodeClosed) {
						t.Fatalf("close response = %v", got)
					}
				}
				model = nil
				clear(responsesByMessage)
			}
			if len(q.ch) != len(model) || q.dropped.Load() != dropped {
				t.Fatalf("queue length/drops = %d/%d, want %d/%d", len(q.ch), q.dropped.Load(), len(model), dropped)
			}
		}
	})
}

func TestSendQueueBlockedPushSynctest(t *testing.T) {
	for _, event := range []string{"space", "cancel", "deadline", "stop"} {
		t.Run(event, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				done := make(chan struct{})
				q := newSendQueue(1, 1, done)
				first := NodeRequest{Ctx: t.Context(), Oneway: true}
				if err := q.tryPush(first); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
				defer cancel()
				result := make(chan error, 1)
				go func() { result <- q.push(NodeRequest{Ctx: ctx, Oneway: true}) }()
				synctest.Wait()
				if len(result) != 0 {
					t.Fatal("push completed while full")
				}
				var want error
				switch event {
				case "space":
					<-q.ch
				case "cancel":
					cancel()
					want = context.Canceled
				case "deadline":
					time.Sleep(time.Hour)
					want = context.DeadlineExceeded
				case "stop":
					close(done)
					want = ErrNodeClosed
				}
				synctest.Wait()
				select {
				case err := <-result:
					if !errors.Is(err, want) {
						t.Fatalf("push = %v, want %v", err, want)
					}
				default:
					t.Fatal("push remained blocked")
				}
				if event != "stop" {
					close(done)
				}
				q.close()
				q.close()
			})
		})
	}
}
