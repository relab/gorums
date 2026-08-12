package benchkit

import (
	"math"
	"testing"
	"time"
)

const intervalNs = int64(time.Second)

func tputEvent(offsetNs int64, ops uint64) *Event {
	return Event_builder{
		Offset:     offsetNs,
		Throughput: ThroughputInterval_builder{Ops: ops, Duration: intervalNs}.Build(),
	}.Build()
}

// TestSummarizeTrim verifies that Summarize trims the startup transient from a
// Result's interval event stream: throughput is recomputed over the kept
// intervals and the CV is the variation of their per-interval throughputs. The
// latency slice is cut at the dropped intervals' cumulative op count only for
// client-measured exact runs; server-measured runs keep whole-run percentiles
// and HDR runs expose no raw samples.
func TestSummarizeTrim(t *testing.T) {
	events := []*Event{
		tputEvent(0, 5), // dropped by trim=1s (5 ops -> sample cut when applicable)
		tputEvent(1*intervalNs, 10),
		tputEvent(2*intervalNs, 20),
		tputEvent(3*intervalNs, 30),
	}
	const wantTput = float64(20) // (10+20+30) ops / 3 s
	const wantCV = 0.5           // mean 20, sample stddev 10

	tests := []struct {
		name         string
		mode         MeasurementMode
		stats        StatsMode
		wantLatLen   int
		wantLatValid bool
	}{
		{"ClientExactCuts", MeasurementMode_CLIENT_MEASURED, StatsMode_EXACT, 60, true},
		{"ServerKeepsAll", MeasurementMode_SERVER_MEASURED, StatsMode_EXACT, 65, true},
		{"HDRNoSamples", MeasurementMode_CLIENT_MEASURED, StatsMode_HDR, 65, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Result_builder{
				Config: RunConfig_builder{
					Name:            "Q",
					MeasurementMode: tt.mode,
					StatsMode:       tt.stats,
				}.Build(),
				Throughput: 999, // stored whole-run value; must be overridden by the trim
				Latencies:  make([]int64, 65),
				Events:     events,
			}.Build()

			s := Summarize(r, time.Second)
			if s.Throughput != wantTput {
				t.Errorf("Throughput = %v, want %v", s.Throughput, wantTput)
			}
			if !s.CVValid || math.Abs(s.CV-wantCV) > 1e-9 {
				t.Errorf("CV = %v (valid %v), want %v (valid)", s.CV, s.CVValid, wantCV)
			}
			if len(s.Latencies) != tt.wantLatLen {
				t.Errorf("Latencies len = %d, want %d", len(s.Latencies), tt.wantLatLen)
			}
			if s.LatencyValid != tt.wantLatValid {
				t.Errorf("LatencyValid = %v, want %v", s.LatencyValid, tt.wantLatValid)
			}
		})
	}
}

// TestSummarizeNoEvents verifies that a Result without an event stream falls
// back to the stored whole-run throughput and latencies, with the CV invalid.
func TestSummarizeNoEvents(t *testing.T) {
	r := Result_builder{
		Config:     RunConfig_builder{Name: "Q"}.Build(),
		Throughput: 123,
		Latencies:  []int64{10, 20, 30},
	}.Build()

	s := Summarize(r, time.Second)
	if s.Throughput != 123 {
		t.Errorf("Throughput = %v, want 123", s.Throughput)
	}
	if s.CVValid {
		t.Error("CVValid = true, want false without events")
	}
	if !s.LatencyValid || len(s.Latencies) != 3 {
		t.Errorf("Latencies = %v (valid %v), want 3 kept samples", s.Latencies, s.LatencyValid)
	}
}

// TestSummarizeSteadyThroughputCV verifies that a perfectly steady run
// (identical per-interval throughput) reports a valid CV of zero rather than
// being marked invalid.
func TestSummarizeSteadyThroughputCV(t *testing.T) {
	r := Result_builder{
		Config: RunConfig_builder{Name: "Q"}.Build(),
		Events: []*Event{tputEvent(0, 10), tputEvent(1*intervalNs, 10), tputEvent(2*intervalNs, 10)},
	}.Build()

	s := Summarize(r, 0)
	if !s.CVValid {
		t.Error("CVValid = false, want true for steady throughput")
	}
	if s.CV != 0 {
		t.Errorf("CV = %v, want 0", s.CV)
	}
}

// TestSummarizeHDRHistogram verifies that an HDR Result's whole-run histogram
// passes through Summarize untrimmed (the histogram has no time dimension),
// and that an exact Result carries no histogram.
func TestSummarizeHDRHistogram(t *testing.T) {
	hist := LatencyHistogram_builder{
		Value: []int64{100, 200},
		Count: []uint64{5, 15},
	}.Build()
	hdr := Result_builder{
		Config: RunConfig_builder{
			Name:      "Q",
			StatsMode: StatsMode_HDR,
		}.Build(),
		Histogram: hist,
		Events:    []*Event{tputEvent(0, 5), tputEvent(1*intervalNs, 15)},
	}.Build()

	s := Summarize(hdr, time.Second)
	if s.LatencyValid {
		t.Error("LatencyValid = true, want false for HDR")
	}
	if s.Histogram == nil {
		t.Fatal("Histogram = nil, want pass-through")
	}
	// Weighted p50 over (100×5, 200×15): the 10th of 20 samples is 200.
	dist := s.Dist()
	if pcts := dist.Quantiles(0.5); pcts == nil || pcts[0] != 200 {
		t.Errorf("histogram p50 = %v, want 200ns", pcts)
	}
	// Weighted mean: (100·5 + 200·15) / 20 = 175.
	if mean, _ := dist.MeanAndStdDev(); mean != 175 {
		t.Errorf("histogram mean = %v, want 175", mean)
	}

	exact := Result_builder{
		Config:    RunConfig_builder{Name: "Q"}.Build(),
		Latencies: []int64{1, 2, 3},
	}.Build()
	if s := Summarize(exact, time.Second); s.Histogram != nil {
		t.Errorf("Histogram = %v, want nil for exact run", s.Histogram)
	}
}
