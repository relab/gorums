package benchkit

import (
	"math"
	"slices"
	"testing"
)

// closeTo reports whether got is within tol of want.
func closeTo(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}

// latencyHist builds a LatencyHistogram from parallel value and count slices, which
// need not be the same length: a malformed message is exactly what the
// aligned-pair rule exists for.
func latencyHist(values []int64, counts []uint64) *LatencyHistogram {
	return LatencyHistogram_builder{Value: values, Count: counts}.Build()
}

// TestLatencyDistSamples verifies the raw-sample path: the count, the sample
// standard deviation, and the interpolated quantiles.
func TestLatencyDistSamples(t *testing.T) {
	d := Summary{Latencies: []int64{1, 2, 3, 4, 5}, LatencyValid: true}.Dist()

	if got := d.Count(); got != 5 {
		t.Errorf("Count() = %d, want 5", got)
	}
	if d.Empty() {
		t.Error("Empty() = true, want false")
	}
	mean, stddev := d.MeanAndStdDev()
	if !closeTo(mean, 3, 1e-9) {
		t.Errorf("mean = %v, want 3", mean)
	}
	// Sample standard deviation over 1..5: sqrt(10/4).
	if !closeTo(stddev, math.Sqrt(2.5), 1e-9) {
		t.Errorf("stddev = %v, want %v", stddev, math.Sqrt(2.5))
	}
	// R-7 interpolation: p50 of 1..5 lands exactly on 3.
	if qs := d.Quantiles(0.5); len(qs) != 1 || !closeTo(qs[0], 3, 1e-9) {
		t.Errorf("Quantiles(0.5) = %v, want [3]", qs)
	}
}

// TestLatencyDistHistogram verifies the weighted path: the total weight, the
// population standard deviation, and the cumulative-rank quantiles, which
// return a recorded bucket value rather than an interpolated one.
func TestLatencyDistHistogram(t *testing.T) {
	d := Summary{Histogram: latencyHist([]int64{100, 200}, []uint64{5, 15})}.Dist()

	if got := d.Count(); got != 20 {
		t.Errorf("Count() = %d, want 20", got)
	}
	// Weighted mean: (100·5 + 200·15) / 20.
	mean, stddev := d.MeanAndStdDev()
	if !closeTo(mean, 175, 1e-9) {
		t.Errorf("mean = %v, want 175", mean)
	}
	// Population stddev: sqrt((75²·5 + 25²·15) / 20) = sqrt(1875).
	if !closeTo(stddev, math.Sqrt(1875), 1e-9) {
		t.Errorf("stddev = %v, want %v", stddev, math.Sqrt(1875))
	}
	// The 10th of 20 samples falls in the 200 bucket.
	if qs := d.Quantiles(0.5); len(qs) != 1 || qs[0] != 200 {
		t.Errorf("Quantiles(0.5) = %v, want [200]", qs)
	}
}

// TestLatencyDistPrefersSamples verifies that a distribution holding both raw
// samples and histogram pairs answers from the samples, which are the exact
// record; the histogram is present only because some other contributing node
// retained nothing better.
func TestLatencyDistPrefersSamples(t *testing.T) {
	d := Summary{
		Latencies:    []int64{10, 10, 10},
		LatencyValid: true,
		Histogram:    latencyHist([]int64{500}, []uint64{97}),
	}.Dist()

	if got := d.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3 (raw samples), not 100", got)
	}
	if mean, _ := d.MeanAndStdDev(); !closeTo(mean, 10, 1e-9) {
		t.Errorf("mean = %v, want 10", mean)
	}
	if qs := d.Quantiles(0.5); len(qs) != 1 || !closeTo(qs[0], 10, 1e-9) {
		t.Errorf("Quantiles(0.5) = %v, want [10]", qs)
	}
}

// TestLatencyDistInvalidLatenciesIgnored verifies that a summary whose latency
// samples are not valid (an HDR run retains none) contributes only its
// histogram, so the two never mix.
func TestLatencyDistInvalidLatenciesIgnored(t *testing.T) {
	d := Summary{
		Latencies: []int64{1, 2, 3}, // stale field; LatencyValid says otherwise
		Histogram: latencyHist([]int64{400}, []uint64{7}),
	}.Dist()

	if got := d.Count(); got != 7 {
		t.Errorf("Count() = %d, want 7 (histogram only)", got)
	}
	if qs := d.Quantiles(0.5); len(qs) != 1 || qs[0] != 400 {
		t.Errorf("Quantiles(0.5) = %v, want [400]", qs)
	}
}

// TestLatencyDistMerge verifies that merging aggregates samples and histogram
// pairs across nodes, that a merge of a mixed run answers from the raw samples,
// and that merging leaves the source distribution and its sample slices
// unchanged.
func TestLatencyDistMerge(t *testing.T) {
	t.Run("samples", func(t *testing.T) {
		nodeA := []int64{1, 2, 3}
		a := Summary{Latencies: nodeA, LatencyValid: true}.Dist()
		b := Summary{Latencies: []int64{4, 5}, LatencyValid: true}.Dist()

		var merged LatencyDist
		merged.Merge(a)
		merged.Merge(b)

		if got := merged.Count(); got != 5 {
			t.Errorf("Count() = %d, want 5", got)
		}
		if mean, _ := merged.MeanAndStdDev(); !closeTo(mean, 3, 1e-9) {
			t.Errorf("mean = %v, want 3", mean)
		}
		if !slices.Equal(nodeA, []int64{1, 2, 3}) {
			t.Errorf("source samples = %v, want them left unchanged", nodeA)
		}
		if got := a.Count(); got != 3 {
			t.Errorf("source Count() = %d, want it left unchanged at 3", got)
		}
	})

	t.Run("histograms", func(t *testing.T) {
		a := Summary{Histogram: latencyHist([]int64{100, 200}, []uint64{5, 15})}.Dist()
		b := Summary{Histogram: latencyHist([]int64{200, 300}, []uint64{5, 5})}.Dist()

		var merged LatencyDist
		merged.Merge(a)
		merged.Merge(b)

		if got := merged.Count(); got != 30 {
			t.Errorf("Count() = %d, want 30", got)
		}
		// Merged weights: 100×5, 200×20, 300×5. The 15th of 30 is in the 200 bucket.
		if qs := merged.Quantiles(0.5); len(qs) != 1 || qs[0] != 200 {
			t.Errorf("Quantiles(0.5) = %v, want [200]", qs)
		}
		// p99 rank 30 falls on the last bucket.
		if qs := merged.Quantiles(0.99); len(qs) != 1 || qs[0] != 300 {
			t.Errorf("Quantiles(0.99) = %v, want [300]", qs)
		}
	})

	t.Run("mixed", func(t *testing.T) {
		exact := Summary{Latencies: []int64{7, 7}, LatencyValid: true}.Dist()
		hdr := Summary{Histogram: latencyHist([]int64{900}, []uint64{50})}.Dist()

		var merged LatencyDist
		merged.Merge(exact)
		merged.Merge(hdr)

		if got := merged.Count(); got != 2 {
			t.Errorf("Count() = %d, want 2 (raw samples win over the merged histogram)", got)
		}
		if mean, _ := merged.MeanAndStdDev(); !closeTo(mean, 7, 1e-9) {
			t.Errorf("mean = %v, want 7", mean)
		}
	})

	t.Run("nil", func(t *testing.T) {
		var merged LatencyDist
		merged.Merge(nil)
		if !merged.Empty() {
			t.Error("Empty() = false after merging nil, want true")
		}
	})
}

// TestLatencyDistAlignedPairs verifies that only the aligned (value, count)
// pairs of a malformed histogram carry weight, in both directions. An unmatched
// value must not become a zero-weight reading and an unmatched count must not
// become a fabricated zero-nanosecond one, since a report cannot tell either
// from a real measurement.
func TestLatencyDistAlignedPairs(t *testing.T) {
	tests := []struct {
		name      string
		values    []int64
		counts    []uint64
		wantCount uint64
		wantP50   float64
	}{
		{"aligned", []int64{100, 200}, []uint64{5, 15}, 20, 200},
		{"more counts than values", []int64{100, 200}, []uint64{5, 15, 99}, 20, 200},
		{"more values than counts", []int64{100, 200, 300}, []uint64{5, 15}, 20, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Summary{Histogram: latencyHist(tt.values, tt.counts)}.Dist()
			if got := d.Count(); got != tt.wantCount {
				t.Errorf("Count() = %d, want %d", got, tt.wantCount)
			}
			if qs := d.Quantiles(0.5); len(qs) != 1 || qs[0] != tt.wantP50 {
				t.Errorf("Quantiles(0.5) = %v, want [%v]", qs, tt.wantP50)
			}
		})
	}
}

// TestLatencyDistEmpty verifies that an empty distribution reports itself as
// empty and yields no statistics, rather than a zero mean or a zero-filled
// quantile slice a caller could mistake for a measurement. A nil distribution
// behaves the same, so a consumer holding one for a node that recorded nothing
// needs no presence check.
func TestLatencyDistEmpty(t *testing.T) {
	for name, d := range map[string]*LatencyDist{
		"zero value":     {},
		"nil":            nil,
		"empty summary":  Summary{}.Dist(),
		"empty samples":  Summary{Latencies: []int64{}, LatencyValid: true}.Dist(),
		"zero-count bin": Summary{Histogram: latencyHist([]int64{100}, []uint64{0})}.Dist(),
	} {
		t.Run(name, func(t *testing.T) {
			if !d.Empty() {
				t.Error("Empty() = false, want true")
			}
			if got := d.Count(); got != 0 {
				t.Errorf("Count() = %d, want 0", got)
			}
			if mean, stddev := d.MeanAndStdDev(); mean != 0 || stddev != 0 {
				t.Errorf("MeanAndStdDev() = (%v, %v), want (0, 0)", mean, stddev)
			}
			if qs := d.Quantiles(0.5); qs != nil {
				t.Errorf("Quantiles(0.5) = %v, want nil", qs)
			}
		})
	}
}

// TestLatencyDistMergeAfterQuery verifies that a distribution queried before it
// is fully merged still answers from everything it holds afterwards, so the
// cached sample conversion cannot go stale.
func TestLatencyDistMergeAfterQuery(t *testing.T) {
	var d LatencyDist
	d.Merge(Summary{Latencies: []int64{1, 1}, LatencyValid: true}.Dist())
	if mean, _ := d.MeanAndStdDev(); !closeTo(mean, 1, 1e-9) {
		t.Fatalf("mean = %v, want 1", mean)
	}

	d.Merge(Summary{Latencies: []int64{7, 7}, LatencyValid: true}.Dist())
	if got := d.Count(); got != 4 {
		t.Errorf("Count() = %d, want 4", got)
	}
	if mean, _ := d.MeanAndStdDev(); !closeTo(mean, 4, 1e-9) {
		t.Errorf("mean = %v, want 4", mean)
	}
}
