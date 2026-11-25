package benchkit

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// metricNamePattern matches a valid metric name.
var metricNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// seriesUnit is the unit of every [Series]: durations in nanoseconds.
const seriesUnit = "ns"

// Series records a named series of durations beyond the primary latency
// series, such as the encode time of each block. It follows the stats mode of
// its [Stats] like the primary series: raw samples in exact mode, a histogram
// in hdr mode. Get one with [Stats.Series]. A Series is safe for concurrent
// use.
//
// A series is either client-measured or server-measured. [Series.Add] records
// a duration that one node measured on its own clock. [Series.AddBySender]
// records a one-way latency across two clocks, and marks the series
// server-measured, so that clock-offset correction shifts it like the primary
// series. One series uses only one of the two methods between resets.
type Series struct {
	mu       sync.Mutex
	mode     StatsMode
	store    SampleStore            // samples from Add; nil until the first Add
	bySender map[uint32]SampleStore // samples from AddBySender, keyed by sender node ID
	server   bool                   // samples come from AddBySender
	count    uint64                 // samples since the last reset
	ticks    []uint64               // count at each tick, exact mode only
}

// Add records one duration that this node measured on its own clock. It panics
// when the series holds samples from [Series.AddBySender] since the last reset.
func (s *Series) Add(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server {
		panic("benchkit: Series.Add on a series that holds samples from AddBySender")
	}
	if s.store == nil {
		s.store = newSampleStore(s.mode)
	}
	s.store.Add(d.Nanoseconds())
	s.count++
}

// AddBySender records one one-way latency sample from the sender with node ID
// id, and marks the series server-measured. [Stats.GetResultCorrected] adds
// each sender's clock offset to its samples, and [CorrectLatencies] corrects
// the samples of a server's reply, as for the primary series. It panics when
// the series holds samples from [Series.Add] since the last reset.
func (s *Series) AddBySender(id uint32, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.server && s.count > 0 {
		panic("benchkit: Series.AddBySender on a series that holds samples from Add")
	}
	s.server = true
	if s.bySender == nil {
		s.bySender = make(map[uint32]SampleStore)
	}
	st := s.bySender[id]
	if st == nil {
		st = newSampleStore(s.mode)
		s.bySender[id] = st
	}
	st.Add(d.Nanoseconds())
	s.count++
}

// reset discards the samples and tick counts and sets the stats mode.
func (s *Series) reset(mode StatsMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
	s.store = nil
	clear(s.bySender)
	s.server = false
	s.count = 0
	s.ticks = nil
}

// tick records the cumulative sample count for the trim cut. It records
// nothing in hdr mode, where the trim does not apply.
func (s *Series) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode == StatsMode_EXACT {
		s.ticks = append(s.ticks, s.count)
	}
}

// metric returns the series as a Metric with the given name, or nil when it
// holds no samples. The samples of a server-measured series are uncorrected;
// a sender's negative samples keep their sign, so that [CorrectLatencies] can
// correct them.
func (s *Series) metric(name string) *Metric {
	return s.snapshot(name, nil, false)
}

// correctedMetric returns the series as a Metric like [Series.metric], with
// each sender's clock offset (sender clock minus this node's clock, in
// nanoseconds) added to the samples of a server-measured series, as
// [Stats.GetResultCorrected] does for the primary series.
func (s *Series) correctedMetric(name string, offsets map[uint32]int64) *Metric {
	return s.snapshot(name, offsets, true)
}

// snapshot returns the series as a Metric with the given name, or nil when it
// holds no samples. When correct is true, it adds each sender's offset to the
// samples of a server-measured series.
func (s *Series) snapshot(name string, offsets map[uint32]int64, correct bool) *Metric {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.count == 0 {
		return nil
	}
	var samples []int64
	var hist *LatencyHistogram
	switch {
	case !s.server:
		samples, hist = storeDist(s.store)
	case correct || s.mode != StatsMode_HDR:
		samples, hist = correctedDist(s.mode, s.bySender, offsets)
	default:
		// correctedDist clamps negative values to zero, so combine the
		// per-sender stores in a store that keeps the sign.
		combined := newHDRStore()
		for _, st := range s.bySender {
			if hs, ok := st.(*hdrStore); ok {
				for v, c := range hs.buckets() {
					combined.addN(v, c)
				}
			}
		}
		hist = combined.snapshot()
	}
	return s.build(name, samples, hist)
}

// build returns a Metric with the given name and distribution, and the
// measurement mode and tick counts of s. The caller must hold s.mu.
func (s *Series) build(name string, samples []int64, hist *LatencyHistogram) *Metric {
	mode := MeasurementMode_CLIENT_MEASURED
	if s.server {
		mode = MeasurementMode_SERVER_MEASURED
	}
	return Metric_builder{
		Name:            name,
		Unit:            seriesUnit,
		MeasurementMode: mode,
		Distribution: Distribution_builder{
			Samples:    samples,
			Histogram:  hist,
			TickCounts: slices.Clone(s.ticks),
		}.Build(),
	}.Build()
}

// Counter records a named count, such as the bytes that a node uploaded. Get
// one with [Stats.Counter]. A Counter is safe for concurrent use.
type Counter struct {
	unit  string
	mu    sync.Mutex // guards value and used as one pair
	value uint64
	used  bool // Add was called since the last reset
}

// Add adds n to the counter.
func (c *Counter) Add(n uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += n
	c.used = true
}

// reset sets the counter to zero.
func (c *Counter) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value = 0
	c.used = false
}

// metric returns the counter as a Metric with the given name, or nil when Add
// was not called since the last reset.
func (c *Counter) metric(name string) *Metric {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.used {
		return nil
	}
	v := c.value
	return Metric_builder{
		Name:    name,
		Unit:    c.unit,
		Counter: CounterValue_builder{Sum: v, Max: v, Nodes: 1}.Build(),
	}.Build()
}

// Series returns the duration series with the given name, registering it on
// first use. The name must match [a-z][a-z0-9_]* and must not name a
// [Counter] of s. Series panics on an invalid name, like [NewHistogram]: names
// are constants of the caller. Every call with the same name returns the same
// handle, and [Stats.Reset] keeps it valid, so a caller can look it up once
// and record into it for every run. In exact mode, a series registered after
// some ticks of the run counts zero samples at each of those ticks, so that
// its tick counts align with the throughput intervals of the run.
func (s *Stats) Series(name string) *Series {
	s.mut.Lock()
	defer s.mut.Unlock()
	if ser, ok := s.namedSeries[name]; ok {
		return ser
	}
	checkMetricName(name)
	if _, ok := s.counters[name]; ok {
		panic(fmt.Sprintf("benchkit: metric %q is registered as a counter, not a series", name))
	}
	if s.namedSeries == nil {
		s.namedSeries = make(map[string]*Series)
	}
	ser := &Series{mode: s.mode}
	if s.mode == StatsMode_EXACT && s.ticks > 0 {
		ser.ticks = make([]uint64, s.ticks)
	}
	s.namedSeries[name] = ser
	return ser
}

// Counter returns the counter with the given name and unit, registering it on
// first use. The name must match [a-z][a-z0-9_]* and must not name a [Series]
// of s; the unit is free text, such as "B". Counter panics on an invalid name
// or when the name is registered with another unit, like [NewHistogram]: names
// and units are constants of the caller. Every call with the same name returns
// the same handle, and [Stats.Reset] keeps it valid. The Result holds a counter
// once [Counter.Add] was called on it since the last Reset, so Add(0) makes a
// counter report a zero.
func (s *Stats) Counter(name, unit string) *Counter {
	s.mut.Lock()
	defer s.mut.Unlock()
	if c, ok := s.counters[name]; ok {
		if c.unit != unit {
			panic(fmt.Sprintf("benchkit: counter %q is registered with unit %q, not %q", name, c.unit, unit))
		}
		return c
	}
	checkMetricName(name)
	if _, ok := s.namedSeries[name]; ok {
		panic(fmt.Sprintf("benchkit: metric %q is registered as a series, not a counter", name))
	}
	if s.counters == nil {
		s.counters = make(map[string]*Counter)
	}
	c := &Counter{unit: unit}
	s.counters[name] = c
	return c
}

// checkMetricName panics when name is not a valid metric name.
func checkMetricName(name string) {
	if !metricNamePattern.MatchString(name) {
		panic(fmt.Sprintf("benchkit: invalid metric name %q: want [a-z][a-z0-9_]*", name))
	}
}

// metrics returns the series that hold samples and the counters that were
// added to since the last reset, sorted by name. The caller must hold s.mut.
func (s *Stats) metrics() []*Metric {
	return s.collectMetrics(func(name string, ser *Series) *Metric { return ser.metric(name) })
}

// lockedMetrics returns the metrics like [Stats.metrics], holding s.mut.
func (s *Stats) lockedMetrics() []*Metric {
	s.mut.Lock()
	defer s.mut.Unlock()
	return s.metrics()
}

// collectMetrics returns the metric of each series, as built by seriesMetric,
// and of each counter, sorted by name, leaving out nil metrics. The caller must
// hold s.mut.
func (s *Stats) collectMetrics(seriesMetric func(string, *Series) *Metric) []*Metric {
	var out []*Metric
	names := slices.AppendSeq(slices.Collect(maps.Keys(s.namedSeries)), maps.Keys(s.counters))
	slices.Sort(names)
	for _, name := range names {
		var m *Metric
		if ser, ok := s.namedSeries[name]; ok {
			m = seriesMetric(name, ser)
		} else {
			m = s.counters[name].metric(name)
		}
		if m != nil {
			out = append(out, m)
		}
	}
	return out
}

// tickMetrics records the cumulative sample count of every series. The caller
// must hold s.mut.
func (s *Stats) tickMetrics() {
	for _, ser := range s.namedSeries {
		ser.tick()
	}
}

// resetMetrics clears every metric in place and sets the stats mode of every
// series, so that cached handles stay valid. The caller must hold s.mut.
func (s *Stats) resetMetrics(mode StatsMode) {
	for _, ser := range s.namedSeries {
		ser.reset(mode)
	}
	for _, c := range s.counters {
		c.reset()
	}
}

// correctedMetrics returns the metrics like [Stats.metrics], with the
// server-measured series corrected by offsets. The caller must hold s.mut.
func (s *Stats) correctedMetrics(offsets map[uint32]int64) []*Metric {
	return s.collectMetrics(func(name string, ser *Series) *Metric { return ser.correctedMetric(name, offsets) })
}

// correctMetrics subtracts offset from every value of the server-measured
// series in metrics, as [CorrectLatencies] does for the primary series.
func correctMetrics(metrics []*Metric, offset int64) {
	for _, m := range metrics {
		d := m.GetDistribution()
		if d == nil || m.GetMeasurementMode() != MeasurementMode_SERVER_MEASURED {
			continue
		}
		samples := d.GetSamples()
		d.SetHistogram(offsetDist(samples, d.GetHistogram(), -offset))
		d.SetSamples(samples)
	}
}

// describeMetric returns the kind, measurement mode, and unit of m, and the
// stats mode of a series, for error messages. Two metrics of the same name
// merge only when their descriptions are equal. Exact samples and an hdr
// histogram do not merge into one distribution, so the stats mode is part of
// the description.
func describeMetric(m *Metric) string {
	d := m.GetDistribution()
	kind := "counter"
	if d != nil {
		kind = "series"
	}
	if m.GetMeasurementMode() == MeasurementMode_SERVER_MEASURED {
		kind = "server-measured " + kind
	}
	desc := fmt.Sprintf("%s in %q", kind, m.GetUnit())
	switch {
	case d == nil:
		return desc
	case d.GetHistogram() != nil:
		return desc + " (hdr)"
	default:
		return desc + " (exact)"
	}
}

// metricSet holds metrics keyed by name.
type metricSet map[string]*Metric

// metricMerger merges the metrics of several sources by name. It owns the
// metrics it returns: it clones each metric that it takes from a source, so
// merging never changes a source. The zero value is an empty merger.
type metricMerger struct {
	set   metricSet
	dists map[string]*distMerger // series of a name seen in more than one source
}

// merge merges each metric of src into the metric of the same name: series
// collect their samples and histograms for [metricMerger.sorted], and counters
// add their sums and node counts and keep the larger max. It returns an error
// when a metric's unit, kind, measurement mode, or stats mode differs from the
// metric of the same name.
func (mm *metricMerger) merge(src []*Metric) error {
	if mm.set == nil {
		mm.set = make(metricSet)
		mm.dists = make(map[string]*distMerger)
	}
	for _, m := range src {
		name := m.GetName()
		cur, ok := mm.set[name]
		if !ok {
			mm.set[name] = proto.CloneOf(m)
			continue
		}
		if a, b := describeMetric(cur), describeMetric(m); a != b {
			return fmt.Errorf("metric %q: %s does not match %s", name, a, b)
		}
		if c := m.GetCounter(); c != nil {
			acc := cur.GetCounter()
			acc.SetSum(acc.GetSum() + c.GetSum())
			acc.SetMax(max(acc.GetMax(), c.GetMax()))
			acc.SetNodes(acc.GetNodes() + c.GetNodes())
			continue
		}
		dists := mm.dists[name]
		if dists == nil {
			dists = &distMerger{}
			acc := cur.GetDistribution()
			dists.add(acc.GetSamples(), acc.GetHistogram())
			mm.dists[name] = dists
		}
		d := m.GetDistribution()
		dists.add(d.GetSamples(), d.GetHistogram())
	}
	return nil
}

// sorted returns the merged metrics sorted by name. A series merged from
// several sources concatenates their samples in merge order, merges their
// histograms, and has no tick counts, since the cut index of one source does
// not apply to the others. Call it once, after the last merge.
func (mm *metricMerger) sorted() []*Metric {
	for name, dists := range mm.dists {
		acc := mm.set[name].GetDistribution()
		samples, hist := dists.dist()
		acc.SetSamples(samples)
		acc.SetHistogram(hist)
		acc.SetTickCounts(nil)
	}
	return mm.set.sorted()
}

// union adds each metric of src to set. The metrics of src and set come from
// the client and the servers of one run, so it returns an error when set
// already holds a metric of the same name.
func (set metricSet) union(src []*Metric) error {
	for _, m := range src {
		if _, ok := set[m.GetName()]; ok {
			return fmt.Errorf("metric %q is recorded both on the client and on the servers", m.GetName())
		}
		set[m.GetName()] = m
	}
	return nil
}

// sorted returns the metrics of set sorted by name.
func (set metricSet) sorted() []*Metric {
	out := make([]*Metric, 0, len(set))
	for _, name := range slices.Sorted(maps.Keys(set)) {
		out = append(out, set[name])
	}
	return out
}

// newMetricSet returns a set holding metrics, which must have distinct names.
// The set takes the metrics without cloning them.
func newMetricSet(metrics []*Metric) metricSet {
	set := make(metricSet, len(metrics))
	for _, m := range metrics {
		set[m.GetName()] = m
	}
	return set
}

// serverMeasuredMetric returns an error naming the first server-measured
// series of metrics, or nil when there is none; where names the origin of
// metrics, whose lifecycle has no clock offsets to correct the series.
func serverMeasuredMetric(metrics []*Metric, where string) error {
	for _, m := range metrics {
		if m.GetDistribution() != nil && m.GetMeasurementMode() == MeasurementMode_SERVER_MEASURED {
			return fmt.Errorf("metric %q is a server-measured series in %s, which has no clock offsets to correct it", m.GetName(), where)
		}
	}
	return nil
}
