package benchkit

import (
	"testing"
	"time"
)

func TestTickerNilBufferSafe(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	tk := NewTicker(0, s) // interval=0: no buffer, no goroutine
	tk.Start(100)
	tk.RateStep(200)
	cv := tk.Stop()
	if cv != 0 {
		t.Errorf("cv = %v, want 0 (no ticks)", cv)
	}
	if tk.Events() != nil {
		t.Error("Events() should be nil when interval == 0")
	}
}

func TestTickerCVComputation(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	tk := NewTicker(0, s) // interval=0: no background goroutine
	// Inject synthetic throughput samples directly to test CV arithmetic.
	tk.updateCV(100)
	tk.updateCV(200)
	tk.updateCV(150)
	cv := tk.cv()
	// mean=150, stddev(sample)=50, cv=50/150≈0.333
	const wantCV = 50.0 / 150.0
	const eps = 1e-9
	diff := cv - wantCV
	if diff < -eps || diff > eps {
		t.Errorf("cv = %v, want %v", cv, wantCV)
	}
}

func TestTickerCVZeroWhenFewTicks(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	tk := NewTicker(0, s)
	tk.updateCV(100) // only one sample
	if cv := tk.cv(); cv != 0 {
		t.Errorf("cv = %v, want 0 with single sample", cv)
	}
}

// TestTickerFlushesFinalPartialInterval verifies that ops recorded between the
// last tick and Stop are emitted as a final partial interval rather than lost:
// the summed interval ops must equal the recorded ops, and STOP must remain the
// last event.
func TestTickerFlushesFinalPartialInterval(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	tk := NewTicker(time.Hour, s) // no tick fires during the test
	tk.Start(0)
	for range 5 {
		s.AddLatency(time.Millisecond)
	}
	tk.Stop()

	events := tk.Events()
	var ops uint64
	var latencyIntervals int
	for _, ev := range events {
		if tp := ev.GetThroughput(); tp != nil {
			ops += tp.GetOps()
		}
		if ev.GetLatency() != nil {
			latencyIntervals++
		}
	}
	if ops != 5 {
		t.Errorf("summed interval ops = %d, want 5", ops)
	}
	if latencyIntervals != 1 {
		t.Errorf("latency intervals = %d, want 1", latencyIntervals)
	}
	last := events[len(events)-1]
	if ph := last.GetPhase(); ph == nil || ph.GetPhase() != PhaseMarker_STOP {
		t.Errorf("last event is not STOP: %v", last)
	}
}

// TestTickerSkipsEmptyFinalInterval verifies that an empty tail (no ops, no
// samples) does not emit a trailing zero interval.
func TestTickerSkipsEmptyFinalInterval(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	tk := NewTicker(time.Hour, s)
	tk.Start(0)
	tk.Stop()
	for _, ev := range tk.Events() {
		if ev.GetThroughput() != nil || ev.GetLatency() != nil {
			t.Errorf("unexpected interval event in empty run: %v", ev)
		}
	}
}

func TestTickerWithIntervalEmitsEvents(t *testing.T) {
	// Use a short interval to verify the Ticker emits events into its buffer.
	s := NewStats(StatsMode_EXACT)
	tk := NewTicker(50*time.Millisecond, s)

	tk.Start(0)
	// Add some ops so the throughput interval is non-zero.
	for range 100 {
		s.AddOp()
	}
	time.Sleep(120 * time.Millisecond) // allow at least two ticks
	tk.Stop()

	events := tk.Events()
	// Should have at least: START + ≥2 throughput + STOP
	if len(events) < 4 {
		t.Errorf("len(events) = %d, want ≥4", len(events))
	}
	// First event must be the start marker.
	first := events[0]
	if ph := first.GetPhase(); ph == nil || ph.GetPhase() != PhaseMarker_START {
		t.Errorf("events[0] is not the start marker; phase = %v, throughput = %v, latency = %v",
			first.GetPhase(), first.GetThroughput(), first.GetLatency())
	}
}

// TestTickerRateStepConcurrentWithTicksIsRaceFree exercises the rate-ramping
// path (T4): RateStep is called from the caller's goroutine while the
// background ticker goroutine concurrently emits throughput/latency events
// into the same eventBuffer. Run with -race to catch regressions.
func TestTickerRateStepConcurrentWithTicksIsRaceFree(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	tk := NewTicker(time.Millisecond, s)

	tk.Start(100)
	for step := int64(200); step <= 1000; step += 200 {
		s.AddOp()
		tk.RateStep(step)
		time.Sleep(2 * time.Millisecond)
	}
	tk.Stop()

	events := tk.Events()
	rateSteps := 0
	for _, ev := range events {
		if ph := ev.GetPhase(); ph != nil && ph.GetPhase() == PhaseMarker_RATE_STEP {
			rateSteps++
		}
	}
	if rateSteps != 5 {
		t.Errorf("rateSteps = %d, want 5", rateSteps)
	}
}
