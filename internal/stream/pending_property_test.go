package stream

import (
	"context"
	"testing"

	"pgregory.net/rapid"
)

func TestPendingCallsProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		type entry struct {
			req    NodeRequest
			cancel context.CancelFunc
		}
		var p pendingCalls
		model := map[uint64]entry{}
		var next uint64
		closed := false
		// Bursts cross the sweep boundary even in short action sequences.
		actions := rapid.SliceOfN(rapid.IntRange(0, 4), 1, 50).Draw(t, "actions")
		for _, action := range actions {
			switch action {
			case 0, 1:
				count := rapid.SampledFrom([]int{1, 2, 63, 64, 65}).Draw(t, "batch")
				for range count {
					next++
					ctx, cancel := context.WithCancel(t.Context())
					t.Cleanup(cancel)
					req := NodeRequest{Ctx: ctx, Msg: NewRequestFromPayload(ctx, next, "test", nil), Streaming: rapid.Bool().Draw(t, "streaming")}
					if p.add(next, req) == closed {
						t.Fatalf("add: closed=%v", closed)
					}
					if !closed {
						model[next] = entry{req, cancel}
					}
				}
			case 2:
				id := rapid.Uint64Range(0, next+1).Draw(t, "take")
				want, exists := model[id]
				got, ok := p.take(id)
				// Expired entries may already have been swept; live entries must remain.
				if ok && !exists || !ok && exists && want.req.Ctx.Err() == nil {
					t.Fatalf("take(%d) = %v, model exists=%v", id, ok, exists)
				}
				if ok && (got.Msg != want.req.Msg || got.Streaming != want.req.Streaming || got.sendTime.IsZero()) {
					t.Fatalf("take(%d) changed request: %v", id, got)
				}
				if !ok || !want.req.Streaming {
					delete(model, id)
				}
			case 3:
				id := rapid.Uint64Range(0, next+1).Draw(t, "cancel")
				if e, ok := model[id]; ok {
					e.cancel()
				}
			case 4:
				for _, req := range p.drain() {
					id := req.Msg.GetMessageSeqNo()
					e, ok := model[id]
					if !ok || req.Msg != e.req.Msg {
						t.Fatalf("unexpected or duplicate drained request %d", id)
					}
					delete(model, id)
				}
				for id, e := range model {
					if e.req.Ctx.Err() == nil {
						t.Fatalf("drain lost live request %d", id)
					}
				}
				clear(model)
				closed = true
			}
			live := 0
			for _, e := range model {
				if e.req.Ctx.Err() == nil {
					live++
				}
			}
			if size := p.len(); size < live || size > len(model) {
				t.Fatalf("pending count %d, live=%d, retained upper bound=%d", size, live, len(model))
			}
		}
	})
}

func TestPendingCallsSweepProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		live := rapid.IntRange(0, 128).Draw(t, "live")
		expired := rapid.IntRange(64, 512).Draw(t, "expired")
		var p pendingCalls
		for i := range live {
			p.add(uint64(i), NodeRequest{Ctx: t.Context()})
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		for i := range expired {
			p.add(uint64(live+i), NodeRequest{Ctx: ctx, Streaming: true})
		}
		if size := p.len(); size > max(minSweepSize, 2*live) {
			t.Fatalf("sweep retained %d calls with %d live", size, live)
		}
		for i := range live {
			if _, ok := p.take(uint64(i)); !ok {
				t.Fatalf("sweep lost live call %d", i)
			}
		}
	})
}
