package benchkit

import (
	"math"
	"sync"
	"time"
)

// Ticker drives the per-interval event stream. On each tick it calls
// Stats.TickInterval to snapshot the per-interval Welford accumulator, emits
// ThroughputInterval and LatencyInterval events to its event buffer, and
// accumulates per-interval throughput samples for a final coefficient-of-
// variation (CV) computation.
//
// The Ticker owns its event buffer: NewTicker allocates one when interval > 0,
// and Events returns the buffered events for attachment to the Result. When the
// configured interval is zero, the buffer is nil, no background goroutine is
// started, and all emission is a no-op; Stop still returns 0 and Events nil.
type Ticker struct {
	interval time.Duration
	stats    *Stats
	buffer   *eventBuffer

	done chan struct{}
	wg   sync.WaitGroup

	// Welford accumulators for per-interval throughput (ops/s) samples.
	mu      sync.Mutex
	tpMean  float64
	tpM2    float64
	tpCount uint64
}

// NewTicker returns a Ticker that samples stats every interval. stats must not
// be nil. When interval > 0 the Ticker allocates an EventBuffer and starts a
// background goroutine on Start; when interval == 0 no events are collected and
// Events returns nil.
func NewTicker(interval time.Duration, stats *Stats) *Ticker {
	var buf *eventBuffer
	if interval > 0 {
		buf = newEventBuffer()
	}
	return &Ticker{
		interval: interval,
		stats:    stats,
		buffer:   buf,
		done:     make(chan struct{}),
	}
}

// Start emits a START phase marker (carrying the initial target rate) and
// starts the background ticker goroutine if interval > 0.
func (t *Ticker) Start(rate int64) {
	t.buffer.emitPhase(time.Now(), PhaseMarker_START, rate)
	if t.interval > 0 {
		t.wg.Add(1)
		go t.run()
	}
}

// RateStep emits a RATE_STEP phase marker with the new target rate. Used by T4
// (rate ramping) to annotate each step in the event log.
func (t *Ticker) RateStep(rate int64) {
	t.buffer.emitPhase(time.Now(), PhaseMarker_RATE_STEP, rate)
}

// Stop signals the background goroutine to exit, waits for it to finish, emits
// a STOP phase marker, and returns the coefficient of variation
// (stddev/mean) of the per-interval throughput samples. Returns 0 when fewer
// than two ticks occurred or mean throughput is zero.
func (t *Ticker) Stop() float64 {
	if t.interval > 0 {
		close(t.done)
		t.wg.Wait()
	}
	t.buffer.emitPhase(time.Now(), PhaseMarker_STOP, 0)
	return t.cv()
}

// Events returns the events buffered during the run, in emission order, for
// attachment to the Result via Result.SetEvents. It returns nil when the Ticker
// was created with interval == 0 (event collection disabled).
func (t *Ticker) Events() []*Event {
	return t.buffer.Events()
}

// run is the background ticker goroutine. It fires every t.interval, reads
// the per-interval counters from Stats, and emits the corresponding events.
// On shutdown it flushes the partial interval since the last tick, so the
// summed interval ops match the total recorded ops.
func (t *Ticker) run() {
	defer t.wg.Done()
	tk := time.NewTicker(t.interval)
	defer tk.Stop()
	prev := time.Now()
	for {
		select {
		case now := <-tk.C:
			dur := now.Sub(prev)
			prev = now
			mean, stddev, count, opDelta := t.stats.TickInterval()
			if dur > 0 {
				tp := float64(opDelta) / dur.Seconds()
				t.updateCV(tp)
			}
			t.buffer.emitThroughput(now, opDelta, dur)
			if count > 0 {
				t.buffer.emitLatency(now, mean, stddev, count)
			}
		case <-t.done:
			t.flushFinal(prev)
			return
		}
	}
}

// flushFinal emits the partial interval between the last tick and Stop so that
// trailing ops are not lost from the event stream. An empty tail (no ops and no
// samples) emits nothing. Note that this interval can be much shorter than the
// configured tick interval; consumers must use the recorded duration when
// deriving per-interval throughput. It runs in the ticker goroutine before Stop
// emits the STOP marker, so STOP stays the last event.
func (t *Ticker) flushFinal(prev time.Time) {
	now := time.Now()
	mean, stddev, count, opDelta := t.stats.TickInterval()
	if opDelta == 0 && count == 0 {
		return
	}
	dur := now.Sub(prev)
	if dur > 0 {
		t.updateCV(float64(opDelta) / dur.Seconds())
	}
	t.buffer.emitThroughput(now, opDelta, dur)
	if count > 0 {
		t.buffer.emitLatency(now, mean, stddev, count)
	}
}

// updateCV updates the Welford accumulators with one throughput sample.
func (t *Ticker) updateCV(tp float64) {
	t.mu.Lock()
	t.tpCount++
	delta := tp - t.tpMean
	t.tpMean += delta / float64(t.tpCount)
	delta2 := tp - t.tpMean
	t.tpM2 += delta * delta2
	t.mu.Unlock()
}

// cv returns the coefficient of variation of the throughput samples.
// Returns 0 when fewer than two samples exist or mean is zero.
func (t *Ticker) cv() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tpCount < 2 || t.tpMean == 0 {
		return 0
	}
	stddev := math.Sqrt(t.tpM2 / float64(t.tpCount-1))
	return stddev / t.tpMean
}
