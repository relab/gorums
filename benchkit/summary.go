package benchkit

import (
	"time"

	"golang.org/x/exp/stats"
)

// Summary is one Result reduced over a trimmed read-time window. The benchmark
// binary records the whole run; presentation tools call Summarize to exclude
// the startup transient without re-running anything (see doc/benchkit.html §10).
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
func Summarize(r *Result, trim time.Duration) Summary {
	cfg := r.GetConfig()
	exact := cfg.GetStatsMode() == StatsMode_EXACT
	clientMeasured := cfg.GetMeasurementMode() == MeasurementMode_CLIENT_MEASURED
	latencies := r.GetLatencies()
	events := r.GetEvents()
	if len(events) == 0 {
		return Summary{Throughput: r.GetThroughput(), Latencies: latencies,
			LatencyValid: exact, Histogram: r.GetHistogram()}
	}

	trimNs := trim.Nanoseconds()
	var keptOps uint64
	var keptDurNs int64
	var cutOps uint64 // ops in dropped intervals -> latency sample-index cut
	var tputs []float64
	for _, ev := range events {
		tp := ev.GetThroughput()
		if tp == nil {
			continue
		}
		if ev.GetOffset() < trimNs {
			cutOps += tp.GetOps()
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
	if clientMeasured && exact && cutOps > 0 && int(cutOps) <= len(latencies) {
		latencies = latencies[cutOps:]
	}
	return Summary{
		Throughput:   throughput,
		CV:           coeffVar(tputs),
		CVValid:      len(tputs) >= 2,
		Latencies:    latencies,
		LatencyValid: exact,
		Histogram:    r.GetHistogram(),
	}
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
