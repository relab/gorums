package benchkit

import (
	"iter"
	"maps"
	"math"
	"runtime"
	"slices"
	"sync"
	"time"
)

// totalCount sums the counts of a LatencyHistogram.
func totalCount(h *LatencyHistogram) uint64 {
	var n uint64
	// h.pairs()
	for _, c := range h.GetCount() {
		n += c
	}
	return n
}

// p50 returns the median of h as a time.Duration, via the Result percentile path.
func p50(h *LatencyHistogram) time.Duration {
	pcts := Result_builder{Histogram: h}.Build().Percentiles(0.5)
	if pcts == nil {
		return -1
	}
	return pcts[0]
}

// pairs yields the histogram's stored (value, count) pairs in recorded order,
// stopping if a malformed message carries fewer counts than values. The
// sequence is re-iterable, so weightedMeanStdDev may range over it twice.
func (h *LatencyHistogram) pairs() iter.Seq2[int64, uint64] {
	return func(yield func(int64, uint64) bool) {
		values, counts := h.GetValue(), h.GetCount()
		for i, v := range values {
			if i >= len(counts) {
				return
			}
			if !yield(v, counts[i]) {
				return
			}
		}
	}
}

// weightedMeanStdDev returns the mean and population standard deviation over
// the weighted (value, count) pairs, in the values' units; both are zero when
// the pairs are empty. It ranges over pairs twice, so the sequence must be
// re-iterable (Histogram.buckets and LatencyHistogram.pairs both are).
func weightedMeanStdDev(pairs iter.Seq2[int64, uint64]) (mean, stddev float64) {
	var total uint64
	var sum float64
	for v, c := range pairs {
		total += c
		sum += float64(v) * float64(c)
	}
	if total == 0 {
		return 0, 0
	}
	mean = sum / float64(total)
	var sqSum float64
	for v, c := range pairs {
		d := float64(v) - mean
		sqSum += d * d * float64(c)
	}
	return mean, math.Sqrt(sqSum / float64(total))
}

// quantileRank returns the 1-based cumulative-count rank on which the
// q-quantile falls over total samples, with q clamped to [0, 1]. The rank is
// at least 1, so any non-empty distribution yields a value.
func quantileRank(total uint64, q float64) uint64 {
	return max(uint64(min(max(q, 0), 1)*float64(total)+0.5), 1)
}

// Stats records the raw data of a benchmark. Each AddLatency call forwards
// the sample to the configured SampleStore and updates the per-interval
// Welford accumulator. All derived statistics (mean, stddev, percentiles)
// are exposed through the *Result returned by GetResult so there is a
// single source of truth.
//
// The default zero value (&Stats{}) uses StatsMode_EXACT (exact sample
// storage). Use NewStats to select a different store mode.
type Stats struct {
	mut       sync.Mutex
	startTime time.Time
	endTime   time.Time
	startMs   runtime.MemStats
	endMs     runtime.MemStats

	mode     StatsMode              // backing store mode for the aggregate and per-sender stores
	store    SampleStore            // aggregate store; nil → lazily initialized as StatsMode_EXACT
	bySender map[uint32]SampleStore // per-sender one-way latency stores keyed by sender node ID

	// Per-interval Welford accumulator used by the Ticker. Always O(1).
	iMean    float64
	iM2      float64 // sum of squared deviations
	iCount   uint64  // samples in the current interval
	opCount  uint64  // total ops recorded since the last Reset
	iOpStart uint64  // opCount at the start of the current interval
}

// NewStats creates a Stats with the given sample storage mode, applied to both
// the aggregate store and the per-sender stores. The zero value &Stats{} uses
// StatsMode_EXACT without calling NewStats.
func NewStats(mode StatsMode) *Stats {
	return &Stats{mode: mode, store: newSampleStore(mode)}
}

// sampleStore returns the aggregate SampleStore, lazily initializing it as
// StatsMode_EXACT if it has not been set. Must be called with s.mut held.
func (s *Stats) sampleStore() SampleStore {
	if s.store == nil {
		s.store = newSampleStore(StatsMode_EXACT)
	}
	return s.store
}

// Start records the start time and memory stats.
func (s *Stats) Start() {
	s.mut.Lock()
	defer s.mut.Unlock()

	runtime.ReadMemStats(&s.startMs)
	s.startTime = time.Now()
}

// End records the end time and memory stats.
func (s *Stats) End() {
	s.mut.Lock()
	defer s.mut.Unlock()

	s.endTime = time.Now()
	runtime.ReadMemStats(&s.endMs)
}

// AddLatency records a latency measurement. It forwards the sample to the
// aggregate SampleStore and updates the per-interval Welford accumulator.
func (s *Stats) AddLatency(l time.Duration) {
	ns := l.Nanoseconds()
	s.mut.Lock()
	s.sampleStore().Add(ns)
	s.intervalUpdate(ns)
	s.mut.Unlock()
}

// intervalUpdate advances the per-interval Welford accumulator and increments
// the total op counter. Must be called with s.mut held.
func (s *Stats) intervalUpdate(ns int64) {
	s.opCount++
	s.iCount++
	delta := float64(ns) - s.iMean
	s.iMean += delta / float64(s.iCount)
	delta2 := float64(ns) - s.iMean
	s.iM2 += delta * delta2
}

// AddOp increments the per-interval operation counter without recording a
// latency sample. Use this in server-measured benchmarks where the client
// only sends messages and latency is collected server-side via Stats.AddLatency.
func (s *Stats) AddOp() {
	s.mut.Lock()
	s.opCount++
	s.mut.Unlock()
}

// Ops returns the total number of operations recorded since the last Reset,
// regardless of how they were recorded (AddLatency, AddOp, or
// AddLatencyBySender). Server-measured runners use it to derive client-side
// per-op statistics from the client's own send count.
func (s *Stats) Ops() uint64 {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.opCount
}

// TickInterval atomically snapshots the per-interval Welford accumulator and
// resets it for the next interval. It returns the mean latency in nanoseconds,
// the sample standard deviation in nanoseconds, the number of latency samples
// in the interval, and the total op delta (total ops recorded via AddOp or
// AddLatency since the last call to TickInterval or Reset). mean, stddev, and
// count are zero if no latency samples were recorded in the interval, but
// opDelta reflects op-only intervals (server-measured runs record ops via
// AddOp without a latency sample). Called by the Ticker goroutine on each
// tick.
func (s *Stats) TickInterval() (mean, stddev float64, count, opDelta uint64) {
	s.mut.Lock()
	mean = s.iMean
	if s.iCount > 1 {
		stddev = math.Sqrt(s.iM2 / float64(s.iCount-1))
	}
	count = s.iCount
	opDelta = s.opCount - s.iOpStart
	// Reset interval state for the next tick.
	s.iMean = 0
	s.iM2 = 0
	s.iCount = 0
	s.iOpStart = s.opCount
	s.mut.Unlock()
	return
}

// AddLatencyBySender records a one-way latency sample keyed by sender node ID.
// Used by symmetric benchmarks where a node receives from multiple senders and
// each sender's samples must later be corrected by that sender's clock offset
// (see [Stats.GetResultCorrected]). It also updates the per-interval Welford
// accumulator so the [Ticker] reflects server-measured latency.
func (s *Stats) AddLatencyBySender(id uint32, l time.Duration) {
	ns := l.Nanoseconds()
	s.mut.Lock()
	if s.bySender == nil {
		s.bySender = make(map[uint32]SampleStore)
	}
	st := s.bySender[id]
	if st == nil {
		st = newSampleStore(s.mode)
		s.bySender[id] = st
	}
	st.Add(ns)
	s.intervalUpdate(ns)
	s.mut.Unlock()
}

// GetResult computes and returns the result of the benchmark from samples
// recorded via AddLatency. In StatsMode_EXACT the full latency distribution is
// available via Result.Latencies; in StatsMode_HDR Latencies is nil and the
// distribution is carried by Result.Histogram instead.
func (s *Stats) GetResult() *Result {
	s.mut.Lock()
	defer s.mut.Unlock()

	store := s.sampleStore()
	r := &Result{}
	n := store.Count()
	r.SetTotalOps(n)
	r.SetTotalTime(int64(s.endTime.Sub(s.startTime)))
	if n > 0 {
		r.SetThroughput(float64(n) / time.Duration(r.GetTotalTime()).Seconds())
		r.SetAllocsPerOp((s.endMs.Mallocs - s.startMs.Mallocs) / n)
		r.SetMemPerOp((s.endMs.TotalAlloc - s.startMs.TotalAlloc) / n)
		if samples := store.Samples(); samples != nil {
			r.SetLatencies(slices.Clone(samples))
		} else if hs, ok := store.(*hdrStore); ok {
			r.SetHistogram(hs.snapshot())
		}
	}
	return r
}

// GetResultCorrected computes and returns the result from the per-sender
// latency stores recorded via AddLatencyBySender, adding each sender's clock
// offset (peer clock minus this node's clock, in nanoseconds) to every sample
// so that cross-machine clock skew is removed. A sender absent from offsets is
// treated as zero offset (no correction). Senders are visited in sorted node-ID
// order so the result is deterministic.
//
// In StatsMode_HDR the offset is a per-sender additive constant applied to each
// sender's histogram bucket values; the shifted per-sender histograms are then
// re-quantized onto one canonical histogram (Result.Histogram, Latencies nil),
// bounding memory the same way the aggregate HDR store does.
func (s *Stats) GetResultCorrected(offsets map[uint32]int64) *Result {
	s.mut.Lock()
	defer s.mut.Unlock()

	if s.mode == StatsMode_HDR {
		agg := newHDRHistogram()
		for _, id := range slices.Sorted(maps.Keys(s.bySender)) {
			if hs, ok := s.bySender[id].(*hdrStore); ok {
				agg.recordPairs(hs.buckets(), offsets[id])
			}
		}
		return s.resultFromHistogram(agg)
	}

	var corrected []int64
	for _, id := range slices.Sorted(maps.Keys(s.bySender)) {
		off := offsets[id]
		for _, l := range s.bySender[id].Samples() {
			corrected = append(corrected, l+off)
		}
	}
	return s.resultFromSamples(corrected)
}

// resultFromSamples builds a Result from the given samples and the timing and
// memory window recorded by Start and End. The caller must hold s.mut.
func (s *Stats) resultFromSamples(samples []int64) *Result {
	r := s.resultWindow(uint64(len(samples)))
	if len(samples) > 0 {
		r.SetLatencies(samples)
	}
	return r
}

// resultFromHistogram builds a Result carrying agg as its latency distribution,
// with the timing and memory window recorded by Start and End. Total ops is the
// histogram's sample count. The caller must hold s.mut.
func (s *Stats) resultFromHistogram(agg *Histogram) *Result {
	r := s.resultWindow(agg.TotalCount())
	if agg.TotalCount() > 0 {
		r.SetHistogram(agg.snapshot())
	}
	return r
}

// resultWindow builds a Result with the total ops, throughput, and per-op memory
// stats derived from n and the timing and memory window recorded by Start and
// End, leaving the latency distribution for the caller to attach. Per-op stats
// are set only when n > 0. The caller must hold s.mut.
func (s *Stats) resultWindow(n uint64) *Result {
	r := &Result{}
	r.SetTotalOps(n)
	r.SetTotalTime(int64(s.endTime.Sub(s.startTime)))
	if n > 0 {
		r.SetThroughput(float64(n) / time.Duration(r.GetTotalTime()).Seconds())
		r.SetAllocsPerOp((s.endMs.Mallocs - s.startMs.Mallocs) / n)
		r.SetMemPerOp((s.endMs.TotalAlloc - s.startMs.TotalAlloc) / n)
	}
	return r
}

// MemDelta returns the raw malloc count and total bytes allocated between
// the last Start and End calls. Used by the Stop handler to compute
// per-op memory stats for benchmarks where the server has no latency
// samples of its own (e.g. QuorumCall, where the client measures latency).
func (s *Stats) MemDelta() (mallocs, totalAlloc uint64) {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.endMs.Mallocs - s.startMs.Mallocs,
		s.endMs.TotalAlloc - s.startMs.TotalAlloc
}

// Reset zeroes out all stats, including the aggregate store and the
// per-interval Welford accumulator, and (re)configures the aggregate and
// per-sender stores to mode, so one Stats can back consecutive runs, including
// runs that select a different StatsMode.
func (s *Stats) Reset(mode StatsMode) {
	s.mut.Lock()
	s.resetLocked(mode)
	s.mut.Unlock()
}

// resetLocked zeroes out all stats, sets the store mode, and rebuilds the
// aggregate store; per-sender stores are dropped and lazily rebuilt in the new
// mode on the next AddLatencyBySender. The caller must hold s.mut.
func (s *Stats) resetLocked(mode StatsMode) {
	s.startTime = time.Time{}
	s.endTime = time.Time{}
	s.startMs = runtime.MemStats{}
	s.endMs = runtime.MemStats{}
	s.mode = mode
	s.store = newSampleStore(mode)
	clear(s.bySender)
	// Reset per-interval Welford accumulator and op counters.
	s.iMean = 0
	s.iM2 = 0
	s.iCount = 0
	s.opCount = 0
	s.iOpStart = 0
}
