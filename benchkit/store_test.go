package benchkit

import (
	"math"
	"testing"
	"time"
)

// hist builds a LatencyHistogram from the given nanosecond samples on the
// canonical HDR layout, so tests construct histogram inputs the same way a
// server-measured StatsMode_HDR run does. With no samples it returns nil.
func hist(samples ...int64) *LatencyHistogram {
	h := newHDRHistogram()
	for _, v := range samples {
		_ = h.RecordValue(v)
	}
	return h.snapshot()
}

// TestOffsetHistogram verifies that offsetHistogram shifts every bucket value by
// the delta (a per-server clock offset), preserves the sample count, clamps a
// shift that would drive values below zero rather than dropping samples, and
// returns nil for a nil or empty input.
func TestOffsetHistogram(t *testing.T) {
	if got := offsetHistogram(nil, 100); got != nil {
		t.Errorf("offsetHistogram(nil) = %v, want nil", got)
	}
	if got := offsetHistogram(hist(), 100); got != nil {
		t.Errorf("offsetHistogram(empty) = %v, want nil", got)
	}

	src := hist(10_000, 10_000, 10_000, 10_000) // 4 samples at 10µs

	shifted := offsetHistogram(src, 5_000) // +5µs
	if got := totalCount(shifted); got != 4 {
		t.Fatalf("count after shift = %d, want 4", got)
	}
	if got := p50(shifted); math.Abs(float64(got-15*time.Microsecond)) > 50 {
		t.Errorf("p50 after +5µs shift = %v, want ≈15µs", got)
	}

	// A negative shift larger than the samples clamps to >= 0 and never drops a
	// sample, whose count feeds throughput and the distribution.
	clamped := offsetHistogram(src, -20_000)
	if got := totalCount(clamped); got != 4 {
		t.Errorf("count after clamping shift = %d, want 4", got)
	}
}

// TestMergeHistograms verifies that mergeHistograms sums the sample counts of
// its inputs onto one canonical histogram, ignores nil and empty inputs, and
// returns nil when nothing carries samples.
func TestMergeHistograms(t *testing.T) {
	a := hist(1_000, 1_000, 1_000) // 3 samples at 1µs
	b := hist(1_000, 2_000)        // 1µs, 2µs

	merged := mergeHistograms(a, nil, b, hist()) // nil and empty inputs ignored
	if got := totalCount(merged); got != 5 {
		t.Fatalf("merged count = %d, want 5", got)
	}
	// Median of {1,1,1,1,2}µs is 1µs.
	if got := p50(merged); math.Abs(float64(got-1*time.Microsecond)) > 50 {
		t.Errorf("merged p50 = %v, want ≈1µs", got)
	}

	if got := mergeHistograms(nil, hist()); got != nil {
		t.Errorf("mergeHistograms(all empty) = %v, want nil", got)
	}
}
