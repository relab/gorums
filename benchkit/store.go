package benchkit

import (
	"iter"
	"maps"
	"slices"
)

// SampleStore accumulates latency samples for one benchmark run.
// Implementations are not thread-safe; callers must serialize access.
//
// The store provides the raw material for the [Result]: the sample count
// and, when retained, the raw samples. All derived statistics (mean, stddev,
// percentiles) are computed by the Result layer — from Samples() in exact
// mode, and from the persisted LatencyHistogram in HDR mode (see
// [Result.Percentiles] and [Summarize]).
type SampleStore interface {
	// Add records one latency sample in nanoseconds.
	Add(ns int64)
	// Count returns the total number of samples recorded.
	Count() uint64
	// Samples returns the raw samples in nanoseconds. Returns nil in HDR
	// mode, where raw samples are not retained.
	Samples() []int64
	// Reset discards all recorded samples, making the store ready for reuse.
	Reset()
}

// newSampleStore returns a SampleStore for the given mode.
func newSampleStore(mode StatsMode) SampleStore {
	switch mode {
	case StatsMode_HDR:
		return newHDRStore()
	default:
		return &exactStore{}
	}
}

// exactStore retains every latency sample in a slice.
type exactStore struct {
	samples []int64
}

func (e *exactStore) Add(ns int64)     { e.samples = append(e.samples, ns) }
func (e *exactStore) Count() uint64    { return uint64(len(e.samples)) }
func (e *exactStore) Samples() []int64 { return e.samples }
func (e *exactStore) Reset()           { e.samples = e.samples[:0] }

// The StatsMode_HDR histogram layout: nanosecond resolution floor, a one
// minute ceiling (samples above it are clamped; a per-op latency near the
// ceiling would already have hit the benchSlack timeout), and three
// significant figures (~0.1% relative error, ~216 KiB of buckets).
const (
	hdrLowest  = 1
	hdrHighest = int64(60_000_000_000)
	hdrSigfigs = 3
)

// newHDRHistogram returns a Histogram with benchkit's standard HDR layout: the
// canonical bucket set shared by every StatsMode_HDR result, so a merged or
// clock-offset-corrected histogram keeps the same bounded memory and precision
// as one recorded directly.
func newHDRHistogram() *Histogram {
	return NewHistogram(hdrLowest, hdrHighest, hdrSigfigs)
}

// hdrStore backs StatsMode_HDR: it retains no raw samples, only log-linear
// [Histogram]s in constant memory. The histogram is persisted on the Result as
// a LatencyHistogram (see [Stats.GetResult]), from which consumers compute
// approximate percentiles, mean, and stddev.
//
// A server-measured raw sample is negative when the sender's clock runs ahead
// of the receiver's by more than the transit time. Its sign must survive until
// the clock-offset correction, so neg records the magnitudes of negative
// samples and h records the rest.
type hdrStore struct {
	h   *Histogram
	neg *Histogram
}

func newHDRStore() *hdrStore {
	return &hdrStore{h: newHDRHistogram(), neg: newHDRHistogram()}
}

// Add records ns, its magnitude clamped to the trackable ceiling: a sample
// must never be dropped, since the op count feeds throughput.
func (s *hdrStore) Add(ns int64) {
	if ns < 0 {
		_ = s.neg.RecordValue(min(-ns, hdrHighest))
		return
	}
	_ = s.h.RecordValue(min(ns, hdrHighest))
}

func (s *hdrStore) Count() uint64    { return s.h.TotalCount() + s.neg.TotalCount() }
func (s *hdrStore) Samples() []int64 { return nil }

func (s *hdrStore) Reset() {
	s.h.Reset()
	s.neg.Reset()
}

// buckets yields each occupied bucket of both histograms in ascending value
// order as the pair (median-equivalent value, count); see [Histogram.buckets].
func (s *hdrStore) buckets() iter.Seq2[int64, uint64] {
	return func(yield func(int64, uint64) bool) {
		var values []int64
		var counts []uint64
		for v, c := range s.neg.buckets() {
			values = append(values, -v)
			counts = append(counts, c)
		}
		for i, value := range slices.Backward(values) {
			if !yield(value, counts[i]) {
				return
			}
		}
		for v, c := range s.h.buckets() {
			if !yield(v, c) {
				return
			}
		}
	}
}

// snapshot renders the store's occupied buckets as a LatencyHistogram message;
// see [Histogram.snapshot]. Returns nil when empty.
func (s *hdrStore) snapshot() *LatencyHistogram {
	if s.Count() == 0 {
		return nil
	}
	var values []int64
	var counts []uint64
	for v, c := range s.buckets() {
		values = append(values, v)
		counts = append(counts, c)
	}
	return LatencyHistogram_builder{Value: values, Count: counts}.Build()
}

// offsetHistogram returns src re-quantized onto the canonical HDR layout with
// delta added to every value, for clock-offset correction of a server-measured
// histogram (delta is the negated per-peer offset, mirroring [CorrectLatencies]
// on the raw-sample path). Returns nil when src is nil or empty.
func offsetHistogram(src *LatencyHistogram, delta int64) *LatencyHistogram {
	if src == nil {
		return nil
	}
	h := newHDRHistogram()
	h.recordPairs(src.pairs(), delta)
	return h.snapshot()
}

// mergeHistograms combines hists onto one canonical HDR histogram, summing their
// (value, count) pairs. It is the HDR counterpart to concatenating raw latency
// slices (see [AggregateServerResults]): re-quantizing onto the shared layout
// keeps the merged histogram at constant bucket count no matter how many
// per-sender or per-server histograms are combined. Returns nil when no input
// carries samples.
func mergeHistograms(hists ...*LatencyHistogram) *LatencyHistogram {
	h := newHDRHistogram()
	for _, src := range hists {
		if src != nil {
			h.recordPairs(src.pairs(), 0)
		}
	}
	return h.snapshot()
}

// storeDist returns the distribution recorded in st: a copy of its raw samples
// in exact mode, or its histogram in hdr mode. Both are nil when st is empty.
func storeDist(st SampleStore) (samples []int64, hist *LatencyHistogram) {
	if raw := st.Samples(); len(raw) > 0 {
		return slices.Clone(raw), nil
	}
	if hs, ok := st.(*hdrStore); ok {
		return nil, hs.snapshot()
	}
	return nil, nil
}

// correctedDist combines the per-sender stores in node-ID order, adding each
// sender's clock offset (sender clock minus this node's clock, in nanoseconds)
// to its samples. A sender absent from offsets has zero offset. In
// StatsMode_HDR the shifted per-sender histograms are re-quantized onto one
// canonical histogram, so samples is nil; otherwise hist is nil.
func correctedDist(mode StatsMode, bySender map[uint32]SampleStore, offsets map[uint32]int64) (samples []int64, hist *LatencyHistogram) {
	ids := slices.Sorted(maps.Keys(bySender))
	if mode == StatsMode_HDR {
		agg := newHDRHistogram()
		for _, id := range ids {
			if hs, ok := bySender[id].(*hdrStore); ok {
				agg.recordPairs(hs.buckets(), offsets[id])
			}
		}
		return nil, agg.snapshot()
	}
	for _, id := range ids {
		off := offsets[id]
		for _, l := range bySender[id].Samples() {
			samples = append(samples, l+off)
		}
	}
	return samples, nil
}

// offsetDist adds delta to every value of a distribution. It shifts samples in
// place when there are any and returns hist unchanged; otherwise it returns
// hist re-quantized onto the canonical HDR layout (see [offsetHistogram]).
func offsetDist(samples []int64, hist *LatencyHistogram, delta int64) *LatencyHistogram {
	if len(samples) > 0 {
		for i := range samples {
			samples[i] += delta
		}
		return hist
	}
	return offsetHistogram(hist, delta)
}

// distMerger concatenates raw samples and merges histograms from several
// distributions into one. The zero value is an empty merger.
type distMerger struct {
	samples []int64
	hists   []*LatencyHistogram
}

// add adds one distribution. It copies samples, so the caller keeps ownership.
func (m *distMerger) add(samples []int64, hist *LatencyHistogram) {
	m.samples = append(m.samples, samples...)
	if hist != nil {
		m.hists = append(m.hists, hist)
	}
}

// dist returns the merged distribution: the concatenated samples, nil when
// there are none, and the histograms merged onto one canonical histogram (see
// [mergeHistograms]), nil when there are none.
func (m *distMerger) dist() (samples []int64, hist *LatencyHistogram) {
	if len(m.samples) > 0 {
		samples = m.samples
	}
	if len(m.hists) > 0 {
		hist = mergeHistograms(m.hists...)
	}
	return samples, hist
}
