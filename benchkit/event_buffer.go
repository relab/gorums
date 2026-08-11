package benchkit

import (
	"sync"
	"time"
)

// eventBuffer accumulates time-series events in memory during a run. The
// harness attaches the buffered events to the per-benchmark Result (via
// Result.SetEvents); there is no separate events file. All methods are no-ops
// on a nil receiver, so callers can pass a nil *eventBuffer when event
// collection is disabled (-interval=0). The Ticker owns the buffer; its
// background goroutine emits throughput and latency events while RateStep
// emits phase markers from the caller's goroutine, so all access is
// serialized through mu. Consumers that need synthetic event streams
// construct Event values directly via the generated builders.
type eventBuffer struct {
	mu     sync.Mutex
	start  time.Time // monotonic offset base (set on the START phase or first emit)
	events []*Event
}

// newEventBuffer returns an empty eventBuffer ready to accumulate events.
func newEventBuffer() *eventBuffer {
	return &eventBuffer{}
}

// emitOffsetLocked returns the nanosecond offset from the start base. Because
// now carries Go's monotonic clock reading, now.Sub(b.start) is
// non-decreasing; the max(..., 0) is a defensive guard for fabricated
// wall-clock-only times. Callers must hold b.mu.
func (b *eventBuffer) emitOffsetLocked(now time.Time) int64 {
	if b.start.IsZero() {
		b.start = now
	}
	return max(now.Sub(b.start).Nanoseconds(), 0)
}

// emitPhase records a lifecycle phase transition. When phase is START the
// supplied instant is used as the monotonic base for all subsequent offsets.
func (b *eventBuffer) emitPhase(now time.Time, phase PhaseMarker_Phase, rate int64) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if phase == PhaseMarker_START {
		b.start = now
	}
	b.events = append(b.events, Event_builder{
		Offset: b.emitOffsetLocked(now),
		Phase:  PhaseMarker_builder{Phase: phase, Rate: rate}.Build(),
	}.Build())
}

// emitThroughput records an ops-completed count and interval duration for one
// ticker period.
func (b *eventBuffer) emitThroughput(now time.Time, ops uint64, duration time.Duration) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, Event_builder{
		Offset:     b.emitOffsetLocked(now),
		Throughput: ThroughputInterval_builder{Ops: ops, Duration: duration.Nanoseconds()}.Build(),
	}.Build())
}

// emitLatency records the Welford accumulator state (mean, stddev, count) for
// one ticker period, all in nanoseconds.
func (b *eventBuffer) emitLatency(now time.Time, mean, stddev float64, count uint64) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, Event_builder{
		Offset:  b.emitOffsetLocked(now),
		Latency: LatencyInterval_builder{Mean: mean, Stddev: stddev, Count: count}.Build(),
	}.Build())
}

// Events returns the buffered events in emission order, or nil on a nil
// receiver. The harness attaches them to the Result via Result.SetEvents.
func (b *eventBuffer) Events() []*Event {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.events
}
