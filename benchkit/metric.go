package benchkit

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sync"
	"time"
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
	case s.mode == StatsMode_HDR:
		combined := newHDRStore()
		for _, st := range s.bySender {
			if hs, ok := st.(*hdrStore); ok {
				for v, c := range hs.buckets() {
					combined.addN(v, c)
				}
			}
		}
		hist = combined.snapshot()
	default:
		samples, _ = correctedDist(s.mode, s.bySender, nil)
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
