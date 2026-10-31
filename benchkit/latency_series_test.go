package benchkit

import (
	"math"
	"testing"
	"time"
)

func checkLatencyInterval(t *testing.T, e *Event, offset time.Duration, samples ...float64) {
	t.Helper()
	lat := e.GetLatency()
	if e.GetOffset() != int64(offset) || lat.GetCount() != uint64(len(samples)) {
		t.Fatalf("interval offset/count = %d/%d, want %d/%d", e.GetOffset(), lat.GetCount(), offset, len(samples))
	}
	var mean, variance float64
	for _, v := range samples {
		mean += v / float64(len(samples))
	}
	if len(samples) > 1 {
		for _, v := range samples {
			variance += (v - mean) * (v - mean) / float64(len(samples)-1)
		}
	}
	if math.Abs(lat.GetMean()-mean) > 1e-9 || math.Abs(lat.GetStddev()-math.Sqrt(variance)) > 1e-9 {
		t.Errorf("mean/stddev = %g/%g, want %g/%g", lat.GetMean(), lat.GetStddev(), mean, math.Sqrt(variance))
	}
}

// Different clock offsets change the between-sender variance, while leaving
// each sender's own variance intact. Bucket placement uses receive time, not
// the skewed timestamp carried by the sender.
func TestLatencySeriesCorrectionAndBoundaries(t *testing.T) {
	start := time.Unix(1, 0)
	s := &latencySeries{start: start, interval: time.Second, buckets: make(map[int64]map[uint32]latencyMoments)}
	s.add(start.Add(-time.Nanosecond), 1, 999) // before START
	s.add(start, 1, -90)
	s.add(start.Add(time.Second-time.Nanosecond), 1, -70)
	s.add(start.Add(time.Second/2), 2, 55)
	s.add(start.Add(time.Second), 7, -5)    // exact boundary, missing clock offset
	s.add(start.Add(3*time.Second), 1, -60) // leave an empty bucket

	offsets := map[uint32]int64{1: 100, 2: -50}
	for range 2 { // correction must not mutate the raw interval accumulators
		events := s.corrected(offsets)
		if len(events) != 3 {
			t.Fatalf("intervals = %d, want 3", len(events))
		}
		checkLatencyInterval(t, events[0], time.Second, 10, 30, 5)
		checkLatencyInterval(t, events[1], 2*time.Second, -5)
		checkLatencyInterval(t, events[2], 4*time.Second, 40)
	}
}

func TestLatencySeriesStatsModesAndReset(t *testing.T) {
	for _, mode := range []StatsMode{StatsMode_EXACT, StatsMode_HDR} {
		t.Run(mode.String(), func(t *testing.T) {
			s := NewStats(mode)
			s.Start()
			m := newMeasurement(Options{Interval: time.Hour})
			m.TrackLatency(s)
			m.start()
			t.Cleanup(m.Abandon)
			s.AddLatencyBySender(1, -90)
			s.AddLatencyBySender(1, -70)
			m.stop()
			s.AddLatencyBySender(2, 55) // a trailing receive after the send window
			s.End()
			r := s.GetResultCorrected(map[uint32]int64{1: 100, 2: -50})
			if len(r.GetEvents()) != 1 {
				t.Fatalf("intervals = %d, want 1", len(r.GetEvents()))
			}
			checkLatencyInterval(t, r.GetEvents()[0], time.Hour, 10, 30, 5)
			if r.GetTotalOps() != r.GetEvents()[0].GetLatency().GetCount() {
				t.Error("interval count differs from aggregate count")
			}
			if mode == StatsMode_HDR && r.GetLatencies() != nil {
				t.Error("HDR mode retained raw samples")
			}
			m.Attach(r)
			var latencyEvents int
			for _, e := range r.GetEvents() {
				if e.GetLatency() != nil {
					latencyEvents++
					if e.GetOffset() <= 0 || e.GetOffset() >= int64(time.Hour) {
						t.Errorf("partial bucket end = %d, want result attachment before 1h", e.GetOffset())
					}
					checkLatencyInterval(t, e, time.Duration(e.GetOffset()), 10, 30, 5)
				}
			}
			if latencyEvents != 1 {
				t.Errorf("attached latency events = %d, want 1", latencyEvents)
			}

			s.Reset(mode)
			s.AddLatencyBySender(1, 20)
			if s.series != nil || len(s.GetResultCorrected(nil).GetEvents()) != 0 {
				t.Error("Reset retained interval collection from the previous run")
			}
		})
	}
}

func TestLatencySeriesDisabled(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	m := newMeasurement(Options{})
	m.TrackLatency(s)
	m.start()
	defer m.Abandon()
	s.AddLatencyBySender(1, 20)
	r := s.GetResultCorrected(nil)
	m.Attach(r)
	if s.series != nil || len(r.GetEvents()) != 0 {
		t.Error("interval=0 collected events or allocated interval state")
	}
}

func TestLatencySeriesAggregateAndAttach(t *testing.T) {
	start := time.Unix(1, 0)
	first := &latencySeries{start: start, interval: time.Second, buckets: make(map[int64]map[uint32]latencyMoments)}
	first.add(start, 1, -90)
	first.add(start, 1, -70)
	first.add(start, 2, 55)
	second := &latencySeries{start: start, interval: time.Second, buckets: make(map[int64]map[uint32]latencyMoments)}
	second.add(start, 7, 0)
	second.add(start.Add(time.Second), 7, 40)
	r, err := AggregateServerResults(map[uint32]*Result{
		2: Result_builder{Events: second.corrected(nil)}.Build(),
		1: Result_builder{Events: first.corrected(map[uint32]int64{1: 100, 2: -50})}.Build(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.GetEvents()) != 2 {
		t.Fatalf("merged intervals = %d, want 2", len(r.GetEvents()))
	}
	checkLatencyInterval(t, r.GetEvents()[0], time.Second, 10, 30, 5, 0)
	checkLatencyInterval(t, r.GetEvents()[1], 2*time.Second, 40)

	// Interleave the corrected series with the original send-window events.
	m := newMeasurement(Options{Interval: time.Second})
	m.ticker.buffer.emitPhase(start, PhaseMarker_START, 100)
	m.ticker.buffer.emitThroughput(start.Add(time.Second), 10, time.Second)
	m.ticker.buffer.emitPhase(start.Add(1500*time.Millisecond), PhaseMarker_STOP, 0)
	m.Attach(r)
	events := r.GetEvents()
	if len(events) != 5 || events[0].GetPhase().GetPhase() != PhaseMarker_START ||
		events[1].GetThroughput().GetOps() != 10 || events[3].GetPhase().GetPhase() != PhaseMarker_STOP {
		t.Fatalf("client events were lost or reordered: %v", events)
	}
	checkLatencyInterval(t, events[2], time.Second, 10, 30, 5, 0)
	checkLatencyInterval(t, events[4], 2*time.Second, 40)
}
