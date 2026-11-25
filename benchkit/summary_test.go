package benchkit

import (
	"math"
	"slices"
	"testing"
	"testing/synctest"
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
	if pcts := dist.Quantiles(0.5); len(pcts) == 0 || pcts[0] != 200 {
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

// TestSummarizeMetricsTrim verifies that Summarize cuts each client-measured
// series in exact mode at the tick count of the last dropped interval, the
// same interval at which it cuts the primary series, and passes
// server-measured series, histograms, and counters through whole.
func TestSummarizeMetricsTrim(t *testing.T) {
	metrics := []*Metric{
		Metric_builder{
			Name: "deliver", Unit: "ns", MeasurementMode: MeasurementMode_SERVER_MEASURED,
			Distribution: Distribution_builder{Samples: []int64{1, 2, 3}, TickCounts: []uint64{1, 2, 3}}.Build(),
		}.Build(),
		Metric_builder{
			Name: "encode", Unit: "ns",
			Distribution: Distribution_builder{Samples: []int64{1, 2, 3, 4, 5, 6, 7, 8, 9}, TickCounts: []uint64{2, 5, 9}}.Build(),
		}.Build(),
		Metric_builder{
			Name: "hist", Unit: "ns",
			Distribution: Distribution_builder{Histogram: latencyHist([]int64{100}, []uint64{4})}.Build(),
		}.Build(),
		Metric_builder{
			Name: "merged", Unit: "ns",
			Distribution: Distribution_builder{Samples: []int64{1, 2, 3}}.Build(),
		}.Build(),
		counterMetric("uploaded", "B", 42),
	}
	events := []*Event{tputEvent(0, 2), tputEvent(intervalNs, 3), tputEvent(2*intervalNs, 4)}

	tests := []struct {
		name   string
		events []*Event
		trim   time.Duration
		want   map[string]uint64 // sample count of each series
	}{
		{"Trimmed", events, time.Second, map[string]uint64{"deliver": 3, "encode": 7, "hist": 4, "merged": 3}},
		{"TwoIntervals", events, 2 * time.Second, map[string]uint64{"deliver": 3, "encode": 4, "hist": 4, "merged": 3}},
		{"NoTrim", events, 0, map[string]uint64{"deliver": 3, "encode": 9, "hist": 4, "merged": 3}},
		{"NoEvents", nil, time.Second, map[string]uint64{"deliver": 3, "encode": 9, "hist": 4, "merged": 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Result_builder{
				Config:  RunConfig_builder{Name: "B"}.Build(),
				Events:  tt.events,
				Metrics: metrics,
			}.Build()
			s := Summarize(r, tt.trim)
			var names []string
			for _, m := range s.Metrics {
				names = append(names, m.Name)
				if m.Counter != nil {
					if m.Unit != "B" || m.Counter.GetSum() != 42 || m.Dist != nil {
						t.Errorf("%s = (unit %q, sum %d, dist %v), want (\"B\", 42, nil)", m.Name, m.Unit, m.Counter.GetSum(), m.Dist)
					}
					continue
				}
				if got := m.Dist.Count(); got != tt.want[m.Name] {
					t.Errorf("%s sample count = %d, want %d", m.Name, got, tt.want[m.Name])
				}
			}
			if want := []string{"deliver", "encode", "hist", "merged", "uploaded"}; !slices.Equal(names, want) {
				t.Errorf("metric names = %v, want %v", names, want)
			}
			if tt.name == "Trimmed" {
				// The cut drops the first interval, which holds samples 1 and 2.
				if got := s.Metrics[1].Dist.Quantiles(0)[0]; got != 3 {
					t.Errorf("encode smallest kept sample = %v, want 3", got)
				}
			}
		})
	}
}

// TestSummarizeMetricsLateSeries verifies that Summarize cuts a series first
// used after some ticks of a real ticker at the same interval as the primary
// series. Each interval records one primary sample; the last three intervals
// also record one sample of the series.
func TestSummarizeMetricsLateSeries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := StartMeasurement(Options{Interval: time.Second})
		// Record in the middle of each interval, away from the ticks.
		time.Sleep(time.Second / 2)
		for i := range 6 {
			m.Stats.AddLatency(time.Millisecond)
			if i >= 3 {
				m.Stats.Series("encode").Add(time.Duration(i))
			}
			time.Sleep(time.Second)
		}
		r := m.Finish()

		for _, tt := range []struct {
			trim        time.Duration
			wantPrimary int
			wantSeries  uint64
		}{
			{3*time.Second + time.Second/2, 3, 3},
			{4*time.Second + time.Second/2, 2, 2},
		} {
			s := Summarize(r, tt.trim)
			if got := len(s.Latencies); got != tt.wantPrimary {
				t.Errorf("trim %v: kept primary samples = %d, want %d", tt.trim, got, tt.wantPrimary)
			}
			if len(s.Metrics) != 1 {
				t.Fatalf("trim %v: metrics = %v, want encode only", tt.trim, s.Metrics)
			}
			if got := s.Metrics[0].Dist.Count(); got != tt.wantSeries {
				t.Errorf("trim %v: kept encode samples = %d, want %d", tt.trim, got, tt.wantSeries)
			}
		}
	})
}
