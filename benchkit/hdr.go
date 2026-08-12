package benchkit

import (
	"fmt"
	"iter"
	"math"
	"math/bits"
)

// Histogram is a bounded-memory, log-linear histogram of non-negative int64
// values (latencies in nanoseconds), after the HdrHistogram design
// (hdrhistogram.org): bucket widths grow with the value's magnitude so every
// recorded value is resolved to the configured number of significant figures,
// in O(1) record time and constant memory. It backs StatsMode_HDR (see
// store.go).
//
// The API mirrors the common HdrHistogram bindings (RecordValue,
// ValueAtQuantile, Mean, StdDev, TotalCount, Min, Max), so switching to a
// library implementation later is mechanical. Histogram is not thread-safe;
// callers serialize access (Stats records under its mutex).
type Histogram struct {
	lowest, highest             int64
	unitMagnitude               int
	subBucketHalfCountMagnitude int
	subBucketCount              int
	subBucketHalfCount          int
	subBucketMask               int64
	bucketCount                 int
	counts                      []uint64
	totalCount                  uint64
	minValue, maxValue          int64
}

// NewHistogram returns a histogram tracking values in [lowest, highest] with
// sigfigs significant figures (1..5). lowest is the smallest value resolved at
// full precision (1 for nanosecond resolution); smaller recorded values are
// not lost but resolve coarsely toward zero. NewHistogram panics on invalid
// parameters, mirroring the HdrHistogram constructors: the parameters are
// compile-time constants of the caller, not runtime input.
func NewHistogram(lowest, highest int64, sigfigs int) *Histogram {
	if lowest < 1 || highest < 2*lowest {
		panic(fmt.Sprintf("benchkit.NewHistogram: need 1 <= lowest and highest >= 2*lowest, got [%d, %d]", lowest, highest))
	}
	if sigfigs < 1 || sigfigs > 5 {
		panic(fmt.Sprintf("benchkit.NewHistogram: sigfigs must be in [1, 5], got %d", sigfigs))
	}
	// The sub-bucket count is the smallest power of two resolving sigfigs
	// decimal digits across one power-of-two range.
	largestSingleUnit := 2 * math.Pow10(sigfigs)
	subBucketCountMagnitude := int(math.Ceil(math.Log2(largestSingleUnit)))
	subBucketHalfCountMagnitude := max(subBucketCountMagnitude-1, 0)
	unitMagnitude := max(int(math.Floor(math.Log2(float64(lowest)))), 0)
	subBucketCount := 1 << (subBucketHalfCountMagnitude + 1)

	h := &Histogram{
		lowest:                      lowest,
		highest:                     highest,
		unitMagnitude:               unitMagnitude,
		subBucketHalfCountMagnitude: subBucketHalfCountMagnitude,
		subBucketCount:              subBucketCount,
		subBucketHalfCount:          subBucketCount / 2,
		subBucketMask:               int64(subBucketCount-1) << unitMagnitude,
		minValue:                    math.MaxInt64,
	}
	// One bucket spans [0, subBucketCount) units; each further bucket doubles
	// the range, reusing the upper half of the sub-buckets at double width.
	// The loop bound is <=, not <: highest itself must be trackable, so a
	// smallestUntrackable that lands exactly on highest still needs one more
	// bucket to cover it.
	h.bucketCount = 1
	for smallestUntrackable := int64(subBucketCount) << unitMagnitude; smallestUntrackable <= highest; smallestUntrackable <<= 1 {
		h.bucketCount++
	}
	h.counts = make([]uint64, (h.bucketCount+1)*h.subBucketHalfCount)
	return h
}

// bucketIndex returns the power-of-two bucket holding v.
func (h *Histogram) bucketIndex(v int64) int {
	pow2Ceiling := bits.Len64(uint64(v | h.subBucketMask))
	return pow2Ceiling - h.unitMagnitude - (h.subBucketHalfCountMagnitude + 1)
}

// subBucketIndex returns v's linear sub-bucket within bucket bucketIdx.
func (h *Histogram) subBucketIndex(v int64, bucketIdx int) int {
	return int(v >> (bucketIdx + h.unitMagnitude))
}

// countsIndex maps a (bucket, sub-bucket) pair to its slot in counts. Buckets
// beyond the first use only the upper half of their sub-buckets (the lower
// half aliases the previous bucket at half the width), so each contributes
// subBucketHalfCount slots.
func (h *Histogram) countsIndex(bucketIdx, subBucketIdx int) int {
	base := (bucketIdx + 1) << h.subBucketHalfCountMagnitude
	return base + subBucketIdx - h.subBucketHalfCount
}

// valueFromIndex returns the lowest value of a (bucket, sub-bucket) pair.
func (h *Histogram) valueFromIndex(bucketIdx, subBucketIdx int) int64 {
	return int64(subBucketIdx) << (bucketIdx + h.unitMagnitude)
}

// rangeSize returns the width of the buckets at bucketIdx.
func (h *Histogram) rangeSize(bucketIdx int) int64 {
	return int64(1) << (bucketIdx + h.unitMagnitude)
}

// RecordValue records one value. It returns an error when v is outside the
// trackable range [0, highest], mirroring the HdrHistogram bindings; callers
// that must not lose samples clamp first (see [hdrStore]).
func (h *Histogram) RecordValue(v int64) error {
	return h.RecordValueN(v, 1)
}

// RecordValueN records n occurrences of v in O(1), for replaying a weighted
// (value, count) pair when re-quantizing one histogram onto another (see
// [Histogram.recordPairs]). It returns an error when v is outside the trackable
// range [0, highest], mirroring the HdrHistogram bindings; callers that must not
// lose samples clamp first. Recording zero occurrences is a no-op.
func (h *Histogram) RecordValueN(v int64, n uint64) error {
	if v < 0 || v > h.highest {
		return fmt.Errorf("value %d outside trackable range [0, %d]", v, h.highest)
	}
	if n == 0 {
		return nil
	}
	bucketIdx := h.bucketIndex(v)
	h.counts[h.countsIndex(bucketIdx, h.subBucketIndex(v, bucketIdx))] += n
	h.totalCount += n
	h.minValue = min(h.minValue, v)
	h.maxValue = max(h.maxValue, v)
	return nil
}

// recordPairs adds each (value+delta, count) pair of the weighted sequence into
// h, clamping each shifted value into h's trackable range [0, highest]. It is
// the shared core of clock-offset correction and histogram merging: delta is a
// per-source clock offset (0 when merging already-corrected histograms).
// Clamping matches [hdrStore.Add], so a correction that pushes a value below
// zero or above the ceiling never drops the sample, whose count feeds
// throughput and the distribution.
func (h *Histogram) recordPairs(pairs iter.Seq2[int64, uint64], delta int64) {
	for v, c := range pairs {
		_ = h.RecordValueN(min(max(v+delta, 0), h.highest), c)
	}
}

// snapshot renders the histogram's occupied buckets as a LatencyHistogram
// message: ascending (value, count) pairs consumers treat as a weighted sample
// set. Returns nil when empty.
func (h *Histogram) snapshot() *LatencyHistogram {
	if h.totalCount == 0 {
		return nil
	}
	var values []int64
	var counts []uint64
	for v, c := range h.buckets() {
		values = append(values, v)
		counts = append(counts, c)
	}
	return LatencyHistogram_builder{Value: values, Count: counts}.Build()
}

// TotalCount returns the number of recorded values.
func (h *Histogram) TotalCount() uint64 { return h.totalCount }

// Min returns the lowest recorded value, exact (0 when empty).
func (h *Histogram) Min() int64 {
	if h.totalCount == 0 {
		return 0
	}
	return h.minValue
}

// Max returns the highest recorded value, exact (0 when empty).
func (h *Histogram) Max() int64 { return h.maxValue }

// buckets yields each occupied bucket in ascending value order as the pair
// (median-equivalent value, count). The median-equivalent value — the middle
// of the bucket's range, as in HdrHistogram — represents every value recorded
// into the bucket within the configured precision, so consumers can treat the
// pairs as a weighted sample set.
func (h *Histogram) buckets() iter.Seq2[int64, uint64] {
	return func(yield func(int64, uint64) bool) {
		for bucketIdx := range h.bucketCount {
			subLo := 0
			if bucketIdx > 0 {
				subLo = h.subBucketHalfCount
			}
			for subBucketIdx := subLo; subBucketIdx < h.subBucketCount; subBucketIdx++ {
				c := h.counts[h.countsIndex(bucketIdx, subBucketIdx)]
				if c == 0 {
					continue
				}
				median := h.valueFromIndex(bucketIdx, subBucketIdx) + h.rangeSize(bucketIdx)/2
				if !yield(median, c) {
					return
				}
			}
		}
	}
}

// ValueAtQuantile returns the highest value of the bucket below which q
// percent (q in [0, 100], as in the HdrHistogram bindings) of the recorded
// values fall, or 0 when empty. The result is within the configured
// significant figures of the exact quantile.
func (h *Histogram) ValueAtQuantile(q float64) int64 {
	if h.totalCount == 0 {
		return 0
	}
	target := quantileRank(h.totalCount, q/100)
	var cum uint64
	for bucketIdx := range h.bucketCount {
		subLo := 0
		if bucketIdx > 0 {
			subLo = h.subBucketHalfCount
		}
		for subBucketIdx := subLo; subBucketIdx < h.subBucketCount; subBucketIdx++ {
			cum += h.counts[h.countsIndex(bucketIdx, subBucketIdx)]
			if cum >= target {
				// The highest value equivalent to this bucket.
				return h.valueFromIndex(bucketIdx, subBucketIdx) + h.rangeSize(bucketIdx) - 1
			}
		}
	}
	return h.maxValue
}

// Mean returns the mean of the recorded values, computed over the bucket
// median-equivalent values (0 when empty).
func (h *Histogram) Mean() float64 {
	mean, _ := weightedMeanStdDev(h.buckets())
	return mean
}

// StdDev returns the population standard deviation of the recorded values,
// computed over the bucket median-equivalent values (0 when empty).
func (h *Histogram) StdDev() float64 {
	_, stddev := weightedMeanStdDev(h.buckets())
	return stddev
}

// Reset discards all recorded values, keeping the bucket layout.
func (h *Histogram) Reset() {
	clear(h.counts)
	h.totalCount = 0
	h.minValue = math.MaxInt64
	h.maxValue = 0
}
