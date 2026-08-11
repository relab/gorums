package benchkit

import (
	"testing"
	"time"
)

func TestEventBufferRoundTrip(t *testing.T) {
	b := newEventBuffer()
	start := time.Unix(0, 1000)

	b.emitPhase(start, PhaseMarker_START, 100)
	b.emitThroughput(start.Add(500*time.Millisecond), 50, 500*time.Millisecond)
	b.emitLatency(start.Add(500*time.Millisecond), 200.0, 30.5, 50)
	b.emitThroughput(start.Add(1500*time.Millisecond), 100, 500*time.Millisecond)
	b.emitPhase(start.Add(2000*time.Millisecond), PhaseMarker_STOP, 0)

	events := b.Events()
	if len(events) != 5 {
		t.Fatalf("len(events) = %d, want 5", len(events))
	}

	// First event: START phase marker at offset 0.
	ev0 := events[0]
	if ev0.GetOffset() != 0 {
		t.Errorf("events[0].offset = %d, want 0", ev0.GetOffset())
	}
	ph0 := ev0.GetPhase()
	if ph0 == nil {
		t.Fatal("events[0].phase is nil")
	}
	if got, want := ph0.GetPhase(), PhaseMarker_START; got != want {
		t.Errorf("events[0].phase = %v, want %v", got, want)
	}
	if got, want := ph0.GetRate(), int64(100); got != want {
		t.Errorf("events[0].rate = %d, want 100", got)
	}

	// Second event: throughput interval at offset 500ms.
	ev1 := events[1]
	if ev1.GetOffset() != 500_000_000 {
		t.Errorf("events[1].offset = %d, want 500000000", ev1.GetOffset())
	}
	tp := ev1.GetThroughput()
	if tp == nil {
		t.Fatal("events[1].throughput is nil")
	}
	if got, want := tp.GetOps(), uint64(50); got != want {
		t.Errorf("throughput.ops = %d, want 50", got)
	}

	// Third event: latency interval.
	lat := events[2].GetLatency()
	if lat == nil {
		t.Fatal("events[2].latency is nil")
	}
	if got, want := lat.GetMean(), 200.0; got != want {
		t.Errorf("latency.mean = %f, want %f", got, want)
	}
	if got, want := lat.GetCount(), uint64(50); got != want {
		t.Errorf("latency.count = %d, want 50", got)
	}
}

func TestEventBufferNilSafe(t *testing.T) {
	var b *eventBuffer
	// All methods must be no-ops on a nil receiver.
	b.emitPhase(time.Time{}, PhaseMarker_START, 0)
	b.emitThroughput(time.Time{}, 10, time.Microsecond)
	b.emitLatency(time.Time{}, 100.0, 10.0, 10)
	if b.Events() != nil {
		t.Error("nil eventBuffer Events() should return nil")
	}
}

func TestEventBufferOffsetBase(t *testing.T) {
	b := newEventBuffer()
	// Emit without a phase marker first: base anchored to the first call.
	base := time.Unix(0, 1000)
	b.emitThroughput(base, 5, 100)
	b.emitThroughput(base.Add(500), 5, 100)

	events := b.Events()
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if got := events[0].GetOffset(); got != 0 {
		t.Errorf("events[0].offset = %d, want 0", got)
	}
	if got := events[1].GetOffset(); got != 500 {
		t.Errorf("events[1].offset = %d, want 500", got)
	}
}
