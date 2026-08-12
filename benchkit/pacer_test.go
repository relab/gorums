package benchkit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewPacerUnlimited(t *testing.T) {
	start := time.Now()
	if p := NewPacer(0, 4, 0, start); p != nil {
		t.Errorf("NewPacer(rate=0) = %v, want nil (unlimited)", p)
	}
	if p := NewPacer(1000, 0, 0, start); p != nil {
		t.Errorf("NewPacer(workers=0) = %v, want nil", p)
	}
	// A nil pacer must not block and must report success.
	var p *Pacer
	if !p.Wait(context.Background()) {
		t.Error("nil pacer Wait() = false, want true")
	}
}

func TestNewPacerInterval(t *testing.T) {
	start := time.Now()
	tests := []struct {
		name                  string
		rate, workers, worker int
		wantInterval          time.Duration
		wantOffset            time.Duration
	}{
		{name: "SingleWorker", rate: 1000, workers: 1, worker: 0, wantInterval: time.Millisecond, wantOffset: 0},
		{name: "FourWorkersW0", rate: 1000, workers: 4, worker: 0, wantInterval: 4 * time.Millisecond, wantOffset: 0},
		{name: "FourWorkersW1", rate: 1000, workers: 4, worker: 1, wantInterval: 4 * time.Millisecond, wantOffset: time.Millisecond},
		{name: "FourWorkersW3", rate: 1000, workers: 4, worker: 3, wantInterval: 4 * time.Millisecond, wantOffset: 3 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPacer(tt.rate, tt.workers, tt.worker, start)
			if p == nil {
				t.Fatal("NewPacer returned nil")
			}
			if p.interval != tt.wantInterval {
				t.Errorf("interval = %v, want %v", p.interval, tt.wantInterval)
			}
			if got := p.next.Sub(start); got != tt.wantOffset {
				t.Errorf("start offset = %v, want %v", got, tt.wantOffset)
			}
		})
	}
}

func TestPacerWaitAdvances(t *testing.T) {
	start := time.Now()
	p := NewPacer(1000, 1, 0, start) // 1ms interval
	next := p.next
	if !p.Wait(context.Background()) {
		t.Fatal("Wait() = false, want true")
	}
	if got := p.next.Sub(next); got != time.Millisecond {
		t.Errorf("schedule advanced by %v, want 1ms", got)
	}
}

func TestPacerWaitCancelled(t *testing.T) {
	// Schedule far in the future so Wait blocks, then cancel.
	p := NewPacer(1, 1, 0, time.Now().Add(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p.Wait(ctx) {
		t.Error("Wait() on cancelled ctx = true, want false")
	}
}

func TestNewRatedGateUnlimited(t *testing.T) {
	if g := NewRatedGate(0, time.Now()); g != nil {
		t.Errorf("NewRatedGate(rate=0) = %v, want nil (unlimited)", g)
	}
	if g := NewRatedGate(-5, time.Now()); g != nil {
		t.Errorf("NewRatedGate(rate=-5) = %v, want nil (unlimited)", g)
	}
	// A nil gate must not block and must report success.
	var g *RatedGate
	if !g.Wait(context.Background()) {
		t.Error("nil RatedGate Wait() = false, want true")
	}
}

func TestNewRatedGateInterval(t *testing.T) {
	tests := []struct {
		name         string
		rate         int
		wantInterval time.Duration
	}{
		{name: "1000/s", rate: 1000, wantInterval: time.Millisecond},
		{name: "500/s", rate: 500, wantInterval: 2 * time.Millisecond},
		{name: "1/s", rate: 1, wantInterval: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewRatedGate(tt.rate, time.Now())
			if g == nil {
				t.Fatal("NewRatedGate returned nil")
			}
			if g.interval != tt.wantInterval {
				t.Errorf("interval = %v, want %v", g.interval, tt.wantInterval)
			}
		})
	}
}

func TestRatedGateWaitAdvances(t *testing.T) {
	// Past start so Wait does not block; each call must hand out the next slot.
	g := NewRatedGate(1000, time.Now().Add(-time.Hour)) // 1ms interval
	first := g.next
	if !g.Wait(context.Background()) {
		t.Fatal("Wait() = false, want true")
	}
	if got := g.next.Sub(first); got != time.Millisecond {
		t.Errorf("schedule advanced by %v, want 1ms", got)
	}
}

func TestRatedGateConcurrentSlots(t *testing.T) {
	// Many goroutines sharing one gate must each receive a distinct slot, so the
	// schedule advances exactly once per Wait even under contention.
	const rate, calls = 100000, 1000
	g := NewRatedGate(rate, time.Now().Add(-time.Hour)) // all slots in the past
	start := g.next
	var wg sync.WaitGroup
	for range calls {
		wg.Go(func() {
			g.Wait(context.Background())
		})
	}
	wg.Wait()
	if got := g.next.Sub(start); got != calls*g.interval {
		t.Errorf("schedule advanced by %v after %d waits, want %v", got, calls, calls*g.interval)
	}
}

func TestRatedGateWaitCancelled(t *testing.T) {
	// Schedule far in the future so Wait blocks, then cancel.
	g := NewRatedGate(1, time.Now().Add(time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if g.Wait(ctx) {
		t.Error("Wait() on cancelled ctx = true, want false")
	}
}
