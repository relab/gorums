package benchkit

import (
	"time"

	"golang.org/x/exp/stats"
)

// Summary is one Result reduced over a trimmed read-time window. The benchmark
// binary records the whole run; presentation tools call Summarize to exclude
// the startup transient without re-running anything.
// The validity flags state which fields carry meaningful data, so callers never
// infer validity from zero values.
type Summary struct {
	// Throughput is the ops/s over the kept window: recomputed from the kept
	// ThroughputInterval events, or the stored whole-run value when the Result
	// carries no events. Always valid, and additive across nodes.
	Throughput float64

	// CV is the coefficient of variation (σ/μ) of the kept per-interval
	// throughputs; meaningful iff CVValid (at least two kept intervals).
	CV      float64
	CVValid bool

	// Latencies holds the kept raw per-op samples in nanoseconds; meaningful
	// iff LatencyValid (false for HDR runs, which retain no raw samples).
	// The trim cut is applied only for client-measured exact runs, where one
	// op yields one in-order sample; server-measured runs keep the whole-run
	// samples (their order does not map to the op count).
	Latencies    []int64
	LatencyValid bool

	// Histogram is the whole-run latency distribution for HDR runs, which
	// retain no raw samples; nil otherwise. The trim does not apply to it —
	// the histogram has no time dimension — so statistics derived from it
	// describe the whole run.
	Histogram *LatencyHistogram

	// Metrics holds the named metrics of the Result over the kept window,
	// sorted by name.
	Metrics []SummaryMetric
}

// SummaryMetric is one named metric of a Result over the read-time window that
// [Summarize] keeps. Exactly one of Dist and Counter is set.
type SummaryMetric struct {
	// Name is the metric name, which matches [a-z][a-z0-9_]*.
	Name string

	// Unit is "ns" for a series and the recorded unit for a counter.
	Unit string

	// MeasurementMode is SERVER_MEASURED for a one-way latency series taken
	// across two clocks, and CLIENT_MEASURED for a quantity that one node
	// measured on its own clock.
	MeasurementMode MeasurementMode

	// Dist is the distribution of a series. A client-measured series in exact
	// mode is cut at the same ticker interval as the primary series, using the
	// series' tick counts; a series without tick counts, a server-measured
	// series, and an hdr histogram describe the whole run.
	Dist *LatencyDist

	// HDR is true for a series recorded as an hdr histogram, and false for a
	// series of raw samples and for a counter. An exact series that the trim
	// empties stays exact.
	HDR bool

	// Counter is the value of a counter over the whole run.
	Counter *CounterValue
}

// CheckMerge returns an error naming the metric and both descriptions when
// other has another unit, kind, measurement mode, or stats mode than m. Merge
// the distributions of two series with [LatencyDist.Merge] and the values of
// two counters with [CounterValue.Merge] only when it returns nil.
func (m SummaryMetric) CheckMerge(other SummaryMetric) error {
	return checkMetricMerge(m.Name, m.description(), other.description())
}

// description returns the kind, measurement mode, and unit of m, and the stats
// mode of a series, for error messages.
func (m SummaryMetric) description() string {
	return metricDescription(m.Dist != nil, m.HDR, m.MeasurementMode, m.Unit)
}

// Summarize derives one Result's throughput, throughput coefficient of
// variation, and latency samples over the window that excludes the first trim
// of the run. When the Result carries an interval event stream, throughput is
// recomputed over the intervals at or after trim and the CV is the σ/μ of
// those per-interval throughputs. The latency slice is cut at the sample index
// implied by the dropped intervals' cumulative op counts only for
// client-measured exact runs, where one op yields one in-order sample;
// server-measured samples arrive out of band and clock-corrected, so they keep
// whole-run percentiles. When there are no events, the stored whole-run
// throughput and latencies are used and the CV is invalid. HDR runs carry no
// raw samples; their whole-run distribution passes through as Histogram.
// Named metrics pass through as Metrics, with the cut of [SummaryMetric.Dist].
func Summarize(r *Result, trim time.Duration) Summary {
	cfg := r.GetConfig()
	exact := cfg.GetStatsMode() == StatsMode_EXACT
	clientMeasured := cfg.GetMeasurementMode() == MeasurementMode_CLIENT_MEASURED
	latencies := r.GetLatencies()
	events := r.GetEvents()
	if len(events) == 0 {
		return Summary{Throughput: r.GetThroughput(), Latencies: latencies,
			LatencyValid: exact, Histogram: r.GetHistogram(),
			Metrics: summarizeMetrics(r.GetMetrics(), 0)}
	}

	trimNs := trim.Nanoseconds()
	var keptOps uint64
	var keptDurNs int64
	var cutOps uint64 // ops in dropped intervals -> latency sample-index cut
	var cutTicks int  // dropped intervals -> named-series tick-count cut
	var tputs []float64
	for _, ev := range events {
		tp := ev.GetThroughput()
		if tp == nil {
			continue
		}
		if ev.GetOffset() < trimNs {
			cutOps += tp.GetOps()
			cutTicks++
			continue
		}
		keptOps += tp.GetOps()
		keptDurNs += tp.GetDuration()
		if tp.GetDuration() > 0 {
			tputs = append(tputs, float64(tp.GetOps())/(float64(tp.GetDuration())/1e9))
		}
	}

	throughput := r.GetThroughput()
	if keptDurNs > 0 {
		throughput = float64(keptOps) / (float64(keptDurNs) / 1e9)
	}
	// The index-map cut is exact only when each op produced one in-order sample.
	// Compare in uint64: converting cutOps to int first can wrap negative on a
	// 32-bit int, which would pass the bound and then panic on the slice.
	if clientMeasured && exact && cutOps > 0 && cutOps <= uint64(len(latencies)) {
		latencies = latencies[cutOps:]
	}
	return Summary{
		Throughput:   throughput,
		CV:           coeffVar(tputs),
		CVValid:      len(tputs) >= 2,
		Latencies:    latencies,
		LatencyValid: exact,
		Histogram:    r.GetHistogram(),
		Metrics:      summarizeMetrics(r.GetMetrics(), cutTicks),
	}
}

// summarizeMetrics returns metrics as SummaryMetrics, each client-measured
// series with raw samples cut after the samples of the first cutTicks ticker
// intervals. Each throughput event is one tick, and the ticks of a series
// count its samples in the same order, so the cut drops the same intervals as
// the cut of the primary series.
func summarizeMetrics(metrics []*Metric, cutTicks int) []SummaryMetric {
	var out []SummaryMetric
	for _, m := range metrics {
		sm := SummaryMetric{Name: m.GetName(), Unit: m.GetUnit(), MeasurementMode: m.GetMeasurementMode()}
		if d := m.GetDistribution(); d != nil {
			samples := d.GetSamples()
			ticks := d.GetTickCounts()
			if m.GetMeasurementMode() == MeasurementMode_CLIENT_MEASURED && cutTicks > 0 && cutTicks <= len(ticks) {
				// Compare in uint64, as for the primary cut.
				if cut := ticks[cutTicks-1]; cut <= uint64(len(samples)) {
					samples = samples[cut:]
				}
			}
			sm.Dist = &LatencyDist{}
			sm.HDR = d.GetHistogram() != nil
			sm.Dist.addSamples(samples)
			sm.Dist.addHistogram(d.GetHistogram())
		} else {
			sm.Counter = m.GetCounter()
		}
		out = append(out, sm)
	}
	return out
}

// coeffVar returns the coefficient of variation (sample stddev / mean) of xs,
// or 0 when fewer than two samples exist or the mean is zero.
func coeffVar(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	mean, stddev := stats.MeanAndStdDev(xs)
	if mean == 0 {
		return 0
	}
	return stddev / mean
}
