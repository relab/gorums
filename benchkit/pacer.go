package benchkit

import (
	"context"
	"sync"
	"time"
)

// Pacer paces a single sending goroutine to an open-loop, fixed-rate schedule.
// Sends are scheduled at absolute deadlines start, start+interval, ... so the
// cadence does not drift even if individual sends are briefly delayed. This
// keeps a one-way latency benchmark below saturation, avoiding the queue
// backlog that otherwise dominates the measured latency.
type Pacer struct {
	interval time.Duration
	next     time.Time
}

// NewPacer returns a pacer that, together with the other workers, sustains a
// combined rate of ratePerNode sends per second. Each of the workers gets an
// equal share of the rate, staggered within one inter-send interval so their
// sends do not all fire at the same instant. NewPacer returns nil when
// ratePerNode <= 0, signalling unlimited (saturating) sends.
func NewPacer(ratePerNode, workers, worker int, start time.Time) *Pacer {
	if ratePerNode <= 0 || workers <= 0 {
		return nil
	}
	perWorker := float64(ratePerNode) / float64(workers)
	interval := time.Duration(float64(time.Second) / perWorker)
	offset := interval * time.Duration(worker) / time.Duration(workers)
	return &Pacer{interval: interval, next: start.Add(offset)}
}

// Wait blocks until this worker's next scheduled send time, then advances the
// schedule. It returns false if ctx is cancelled while waiting. A nil pacer
// never waits, so unlimited senders call Wait without a branch at the call site.
func (p *Pacer) Wait(ctx context.Context) bool {
	if p == nil {
		return true
	}
	if d := time.Until(p.next); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return false
		}
	}
	p.next = p.next.Add(p.interval)
	return true
}

// RatedGate is a concurrency-safe, open-loop rate limiter shared by many
// sending goroutines. Unlike Pacer, which is owned by a single worker, one
// RatedGate enforces a combined rate across an arbitrary, dynamically sized set
// of senders. This is used by the async benchmark, where new sends are fired
// from completion callbacks rather than a fixed worker pool, so a per-worker
// pacer cannot pace them. Slots are handed out at absolute deadlines start,
// start+interval, ... so the cadence does not drift.
type RatedGate struct {
	interval time.Duration
	mu       sync.Mutex
	next     time.Time
}

// NewRatedGate returns a gate that hands out slots at a combined rate sends per
// second. It returns nil when rate <= 0, signalling unlimited (saturating)
// sends, mirroring NewPacer.
func NewRatedGate(rate int, start time.Time) *RatedGate {
	if rate <= 0 {
		return nil
	}
	interval := time.Duration(float64(time.Second) / float64(rate))
	return &RatedGate{interval: interval, next: start}
}

// Wait claims the next slot in the shared schedule and blocks until its
// deadline, then returns true. It returns false if ctx is cancelled while
// waiting. A nil gate never waits, so unlimited senders call Wait without a
// branch at the call site. The schedule is advanced under the lock, but the
// wait itself happens unlocked so concurrent senders are not serialized.
func (g *RatedGate) Wait(ctx context.Context) bool {
	if g == nil {
		return true
	}
	g.mu.Lock()
	when := g.next
	g.next = g.next.Add(g.interval)
	g.mu.Unlock()
	if d := time.Until(when); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			return false
		}
	}
	return true
}
