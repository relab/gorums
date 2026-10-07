package benchkit

import (
	"sync"
	"time"
)

// Ticker drives the per-interval event stream. On each tick it calls
// Stats.TickInterval to snapshot the per-interval Welford accumulator and
// emits ThroughputInterval and LatencyInterval events to its event buffer.
//
// The Ticker owns its event buffer: NewTicker allocates one when interval > 0,
// and Events returns the buffered events for attachment to the Result. When the
// configured interval is zero, the buffer is nil, no background goroutine is
// started, all emission is a no-op, and Events returns nil.
type Ticker struct {
	interval time.Duration
	stats    *Stats
	buffer   *eventBuffer

	done chan struct{}
	wg   sync.WaitGroup
}

// NewTicker returns a Ticker that samples stats every interval. stats must not
// be nil. When interval > 0 the Ticker allocates an event buffer and starts a
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

// RateStep emits a RATE_STEP phase marker with the new target rate. Rate
// ramping calls it to annotate each step in the event log.
func (t *Ticker) RateStep(rate int64) {
	t.buffer.emitPhase(time.Now(), PhaseMarker_RATE_STEP, rate)
}

// Stop signals the background goroutine to exit, waits for it to finish, and
// emits a STOP phase marker.
func (t *Ticker) Stop() {
	if t.interval > 0 {
		close(t.done)
		t.wg.Wait()
	}
	t.buffer.emitPhase(time.Now(), PhaseMarker_STOP, 0)
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
	t.buffer.emitThroughput(now, opDelta, now.Sub(prev))
	if count > 0 {
		t.buffer.emitLatency(now, mean, stddev, count)
	}
}
