package benchkit

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"golang.org/x/exp/stats"
)

// TestHistogramInvalidParameters verifies that NewHistogram rejects parameters
// outside the supported ranges, mirroring the HdrHistogram constructors.
func TestHistogramInvalidParameters(t *testing.T) {
	tests := []struct {
		name            string
		lowest, highest int64
		sigfigs         int
	}{
		{"LowestZero", 0, 1000, 3},
		{"HighestBelowTwiceLowest", 1000, 1500, 3},
		{"SigfigsZero", 1, 1000, 0},
		{"SigfigsTooLarge", 1, 1000, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("NewHistogram(%d, %d, %d) did not panic", tt.lowest, tt.highest, tt.sigfigs)
				}
			}()
			NewHistogram(tt.lowest, tt.highest, tt.sigfigs)
		})
	}
}

// TestHistogramRecordValue verifies basic recording: total count, exact
// min/max, range errors for untrackable values, and Reset.
func TestHistogramRecordValue(t *testing.T) {
	h := NewHistogram(1, 1_000_000, 3)
	for _, v := range []int64{1, 500, 999_999, 42, 42} {
		if err := h.RecordValue(v); err != nil {
			t.Fatalf("RecordValue(%d): %v", v, err)
		}
	}
	if got := h.TotalCount(); got != 5 {
		t.Errorf("TotalCount() = %d, want 5", got)
	}
	if got := h.Min(); got != 1 {
		t.Errorf("Min() = %d, want 1", got)
	}
	if got := h.Max(); got != 999_999 {
		t.Errorf("Max() = %d, want 999999", got)
	}
	if err := h.RecordValue(-1); err == nil {
		t.Error("RecordValue(-1) = nil, want error")
	}
	if err := h.RecordValue(2_000_000); err == nil {
		t.Error("RecordValue(2000000) = nil, want error")
	}

	h.Reset()
	if h.TotalCount() != 0 || h.Min() != 0 || h.Max() != 0 || h.ValueAtQuantile(50) != 0 {
		t.Errorf("after Reset: count=%d min=%d max=%d p50=%d, want all zero",
			h.TotalCount(), h.Min(), h.Max(), h.ValueAtQuantile(50))
	}
}

// TestHistogramRecordValueAtHighestBoundary verifies that recording exactly at
// the configured highest value never panics, including when highest lands
// exactly on a bucket-count doubling boundary (a power-of-two multiple of the
// sub-bucket range), which previously undercounted the needed buckets by one.
func TestHistogramRecordValueAtHighestBoundary(t *testing.T) {
	for _, tc := range []struct {
		name            string
		lowest, highest int64
		sigfigs         int
	}{
		{"boundary-2sigfigs", 1, 1 << 20, 2},
		{"boundary-3sigfigs", 1, 2048, 3},
		{"non-boundary", 1, 1_000_000, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHistogram(tc.lowest, tc.highest, tc.sigfigs)
			if err := h.RecordValue(tc.highest); err != nil {
				t.Fatalf("RecordValue(highest=%d): %v", tc.highest, err)
			}
			if got := h.Max(); got != tc.highest {
				t.Errorf("Max() = %d, want %d", got, tc.highest)
			}
		})
	}
}

// TestHistogramRecordValueN verifies that RecordValueN records the given number
// of occurrences in one call, equivalent to that many RecordValue calls, that a
// zero count is a no-op, and that an out-of-range value errors without recording.
func TestHistogramRecordValueN(t *testing.T) {
	bulk := NewHistogram(1, 1_000_000, 3)
	one := NewHistogram(1, 1_000_000, 3)

	if err := bulk.RecordValueN(500, 4); err != nil {
		t.Fatalf("RecordValueN(500, 4): %v", err)
	}
	for range 4 {
		if err := one.RecordValue(500); err != nil {
			t.Fatalf("RecordValue(500): %v", err)
		}
	}
	if got := bulk.TotalCount(); got != 4 {
		t.Errorf("TotalCount() = %d, want 4", got)
	}
	if bulk.Min() != one.Min() || bulk.Max() != one.Max() {
		t.Errorf("bulk min/max = %d/%d, want %d/%d", bulk.Min(), bulk.Max(), one.Min(), one.Max())
	}
	if !slices.Equal(bulk.counts, one.counts) {
		t.Error("RecordValueN(v, n) counts differ from n * RecordValue(v)")
	}

	if err := bulk.RecordValueN(700, 0); err != nil {
		t.Fatalf("RecordValueN(700, 0): %v", err)
	}
	if got := bulk.TotalCount(); got != 4 {
		t.Errorf("TotalCount() after zero-count record = %d, want 4", got)
	}
	if err := bulk.RecordValueN(2_000_000, 3); err == nil {
		t.Error("RecordValueN(2000000, 3) = nil, want out-of-range error")
	}
	if got := bulk.TotalCount(); got != 4 {
		t.Errorf("TotalCount() after out-of-range record = %d, want 4", got)
	}
}

// TestHistogramAccuracy verifies that quantiles, mean, and stddev computed
// from the histogram match the exact statistics of the recorded samples within
// the configured significant figures (3 sigfigs → 0.1% relative error per
// value, plus quantile granularity of one bucket).
func TestHistogramAccuracy(t *testing.T) {
	const n = 100_000
	h := NewHistogram(1, 60_000_000_000, 3)
	rng := rand.New(rand.NewPCG(1, 2))
	samples := make([]float64, n)
	for i := range samples {
		// Log-normal-ish latencies spanning ~1µs to ~100ms.
		v := int64(1000 * math.Exp(rng.NormFloat64()*1.5+3))
		samples[i] = float64(v)
		if err := h.RecordValue(v); err != nil {
			t.Fatalf("RecordValue(%d): %v", v, err)
		}
	}

	for _, q := range []float64{50, 90, 99, 99.9} {
		exact := stats.Quantiles(samples, q/100)[0]
		got := float64(h.ValueAtQuantile(q))
		if relErr := math.Abs(got-exact) / exact; relErr > 0.005 {
			t.Errorf("ValueAtQuantile(%v) = %.0f, exact %.0f (rel err %.4f > 0.005)", q, got, exact, relErr)
		}
	}
	exactMean, exactStdDev := stats.MeanAndStdDev(samples)
	if relErr := math.Abs(h.Mean()-exactMean) / exactMean; relErr > 0.001 {
		t.Errorf("Mean() = %.0f, exact %.0f (rel err %.4f > 0.001)", h.Mean(), exactMean, relErr)
	}
	if relErr := math.Abs(h.StdDev()-exactStdDev) / exactStdDev; relErr > 0.005 {
		t.Errorf("StdDev() = %.0f, exact %.0f (rel err %.4f > 0.005)", h.StdDev(), exactStdDev, relErr)
	}
}

// TestHistogramBuckets verifies that buckets yields (value, count) pairs in
// ascending value order, that the counts sum to the total, and that each
// recorded value is represented within the configured precision.
func TestHistogramBuckets(t *testing.T) {
	h := NewHistogram(1, 1_000_000_000, 2)
	recorded := []int64{100, 100, 5_000, 123_456, 999_999_999}
	for _, v := range recorded {
		if err := h.RecordValue(v); err != nil {
			t.Fatalf("RecordValue(%d): %v", v, err)
		}
	}
	var values []int64
	var total uint64
	for v, c := range h.buckets() {
		values = append(values, v)
		total += c
	}
	if !slices.IsSorted(values) {
		t.Errorf("bucket values not ascending: %v", values)
	}
	if total != h.TotalCount() {
		t.Errorf("bucket counts sum = %d, want %d", total, h.TotalCount())
	}
	// Every recorded value must have a representative within 1% (2 sigfigs).
	for _, want := range recorded {
		ok := slices.ContainsFunc(values, func(v int64) bool {
			return math.Abs(float64(v-want)) <= max(0.01*float64(want), 1)
		})
		if !ok {
			t.Errorf("no bucket value within 1%% of recorded %d: %v", want, values)
		}
	}
}
