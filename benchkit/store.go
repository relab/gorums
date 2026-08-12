package benchkit

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

// hdrStore backs StatsMode_HDR: it retains no raw samples, only a log-linear
// [Histogram] in constant memory. The histogram is persisted on the Result as
// a LatencyHistogram (see [Stats.GetResult]), from which consumers compute
// approximate percentiles, mean, and stddev.
type hdrStore struct {
	h *Histogram
}

func newHDRStore() *hdrStore {
	return &hdrStore{h: newHDRHistogram()}
}

// Add records ns, clamped into the trackable range: a sample must never be
// dropped, since the op count feeds throughput.
func (s *hdrStore) Add(ns int64) {
	_ = s.h.RecordValue(min(max(ns, 0), hdrHighest))
}

func (s *hdrStore) Count() uint64    { return s.h.TotalCount() }
func (s *hdrStore) Samples() []int64 { return nil }
func (s *hdrStore) Reset()           { s.h.Reset() }

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
