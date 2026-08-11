package benchkit

import (
	"iter"
	"maps"
	"slices"

	"golang.org/x/exp/stats"
)

// LatencyDist is a latency distribution accumulated from one or more results.
// It answers the same questions — sample count, mean, standard deviation,
// quantiles — whether the underlying runs retained raw per-op samples (exact
// mode) or only a bucketed distribution (HDR mode), so a consumer merging
// results across the nodes of a cluster does not branch on which it got.
//
// A distribution that holds both raw samples and histogram pairs answers from
// the raw samples: they are the exact record, and the histogram is present only
// because some other contributing node retained nothing better. The zero value
// is an empty distribution ready to merge into.
//
// LatencyDist keeps the units of what it was given, which for every benchkit
// result is nanoseconds. It retains the sample slices it is given rather than
// copying them — a run's merged samples are the largest thing a presentation
// tool holds — and never writes through them, so a caller must not mutate a
// Result's latencies after contributing them.
type LatencyDist struct {
	// batches holds each contributing result's raw samples without copying
	// them; a merge across many nodes appends a batch rather than growing one
	// flat slice, so the merged samples are materialized once, at query time,
	// instead of once per merge step.
	batches [][]int64
	count   uint64           // total samples across batches
	hist    map[int64]uint64 // merged weighted (value, count) pairs
	floats  []float64        // cached flat float64 view of batches; dropped on add
}

// Dist returns the summary's latencies as a distribution: its raw samples when
// they are valid, and its whole-run histogram otherwise. Merge the results to
// aggregate a benchmark across the nodes of a run.
func (s Summary) Dist() *LatencyDist {
	d := &LatencyDist{}
	if s.LatencyValid {
		d.addSamples(s.Latencies)
	}
	d.addHistogram(s.Histogram)
	return d
}

// resultDist returns the whole-run distribution recorded in r, without the
// read-time trim [Summarize] applies. Raw samples are taken as recorded,
// whatever the run's stats mode, so the untrimmed statistics on Result report
// exactly what the run stored.
func resultDist(r *Result) *LatencyDist {
	d := &LatencyDist{}
	d.addSamples(r.GetLatencies())
	d.addHistogram(r.GetHistogram())
	return d
}

// addSamples adds one result's raw samples. It retains ns rather than copying
// it, and never appends into it.
func (d *LatencyDist) addSamples(ns []int64) {
	if len(ns) == 0 {
		return
	}
	d.batches = append(d.batches, ns)
	d.count += uint64(len(ns))
	d.floats = nil
}

// addHistogram adds one result's bucketed distribution. Only the aligned pairs
// carry weight (see [LatencyHistogram.pairs]), so a malformed message with more
// counts than values contributes no unmatched tail weight. Nil-safe.
func (d *LatencyDist) addHistogram(h *LatencyHistogram) {
	d.addPairs(h.pairs())
}

// addPairs adds weighted (value, count) pairs to the histogram side.
func (d *LatencyDist) addPairs(pairs iter.Seq2[int64, uint64]) {
	for v, c := range pairs {
		if d.hist == nil {
			d.hist = make(map[int64]uint64)
		}
		d.hist[v] += c
	}
}

// Merge adds every sample and histogram pair of other into d, leaving other
// unchanged. Use it to aggregate one benchmark across the nodes of a run.
func (d *LatencyDist) Merge(other *LatencyDist) {
	if other == nil {
		return
	}
	for _, batch := range other.batches {
		d.addSamples(batch)
	}
	d.addPairs(maps.All(other.hist))
}

// Count returns the number of samples the distribution answers from: the raw
// sample count when raw samples were contributed, and the total histogram
// weight otherwise.
func (d *LatencyDist) Count() uint64 {
	if d == nil {
		return 0
	}
	if d.count > 0 {
		return d.count
	}
	var total uint64
	for _, c := range d.hist {
		total += c
	}
	return total
}

// Empty reports whether the distribution holds no samples. The statistics of an
// empty distribution are not meaningful, so callers test this rather than
// reading a zero mean or a nil quantile slice as a measurement.
func (d *LatencyDist) Empty() bool {
	return d.Count() == 0
}

// MeanAndStdDev returns the mean and standard deviation of the distribution,
// or (0, 0) when it is empty. Over raw samples this is the sample standard
// deviation; over histogram pairs it is the population standard deviation,
// matching [Histogram.Mean] and [Histogram.StdDev]'s HdrHistogram-mirroring
// convention.
func (d *LatencyDist) MeanAndStdDev() (mean, stddev float64) {
	if d == nil {
		return 0, 0
	}
	if d.count == 0 {
		return weightedMeanStdDev(d.pairs())
	}
	return stats.MeanAndStdDev(d.samples())
}

// Quantiles returns the requested quantile values (in [0, 1]) in the units of
// the recorded samples, or nil when the distribution is empty. Over raw samples
// the quantiles are interpolated; over histogram pairs each is the recorded
// bucket value on which the quantile's cumulative rank falls, so a reported
// quantile is always a value the run actually observed.
func (d *LatencyDist) Quantiles(quantiles ...float64) []float64 {
	if d == nil {
		return nil
	}
	if d.count > 0 {
		return stats.Quantiles(d.samples(), quantiles...)
	}
	total := d.Count()
	if total == 0 {
		return nil
	}
	out := make([]float64, len(quantiles))
	for i, q := range quantiles {
		target := quantileRank(total, q)
		var cum uint64
		for v, c := range d.pairs() {
			cum += c
			if cum >= target {
				out[i] = float64(v)
				break
			}
		}
	}
	return out
}

// samples returns the merged raw samples as one float64 slice, as the
// golang.org/x/exp/stats functions require. The conversion is cached, since
// summarizing one distribution asks several questions of it; adding to the
// distribution drops the cache.
func (d *LatencyDist) samples() []float64 {
	if d.floats != nil {
		return d.floats
	}
	d.floats = make([]float64, 0, d.count)
	for _, batch := range d.batches {
		for _, v := range batch {
			d.floats = append(d.floats, float64(v))
		}
	}
	return d.floats
}

// pairs yields the merged weighted (value, count) pairs in ascending value
// order, which the cumulative-rank quantile scan requires. The sequence is
// re-iterable, so weightedMeanStdDev may range over it twice.
func (d *LatencyDist) pairs() iter.Seq2[int64, uint64] {
	values := slices.Sorted(maps.Keys(d.hist))
	return func(yield func(int64, uint64) bool) {
		for _, v := range values {
			if !yield(v, d.hist[v]) {
				return
			}
		}
	}
}
