package benchkit

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// metricByName returns the metric of r with the given name, or nil.
func metricByName(r *Result, name string) *Metric {
	for _, m := range r.GetMetrics() {
		if m.GetName() == name {
			return m
		}
	}
	return nil
}

// TestMetricSeriesExact verifies that a series in exact mode records its raw
// samples in order, as a client-measured distribution in nanoseconds.
func TestMetricSeriesExact(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	encode := s.Series("encode")
	encode.Add(3 * time.Microsecond)
	encode.Add(time.Microsecond)

	m := metricByName(s.GetResult(), "encode")
	if m == nil {
		t.Fatal("GetResult has no metric \"encode\"")
	}
	if got, want := m.GetUnit(), "ns"; got != want {
		t.Errorf("unit = %q, want %q", got, want)
	}
	if got := m.GetMeasurementMode(); got != MeasurementMode_CLIENT_MEASURED {
		t.Errorf("measurement mode = %v, want CLIENT_MEASURED", got)
	}
	if got, want := m.GetDistribution().GetSamples(), []int64{3000, 1000}; !slices.Equal(got, want) {
		t.Errorf("samples = %v, want %v", got, want)
	}
	if m.GetDistribution().GetHistogram() != nil {
		t.Error("histogram in exact mode is set, want nil")
	}
}

// TestMetricSeriesHDR verifies that a series in hdr mode records a histogram
// instead of raw samples, as the primary series does.
func TestMetricSeriesHDR(t *testing.T) {
	s := NewStats(StatsMode_HDR)
	encode := s.Series("encode")
	for range 5 {
		encode.Add(time.Millisecond)
	}

	d := metricByName(s.GetResult(), "encode").GetDistribution()
	if got := d.GetSamples(); got != nil {
		t.Errorf("samples in hdr mode = %v, want nil", got)
	}
	if got := totalCount(d.GetHistogram()); got != 5 {
		t.Errorf("histogram count = %d, want 5", got)
	}
}

// TestMetricCounter verifies that a counter reports its value as sum and max
// for one node.
func TestMetricCounter(t *testing.T) {
	s := &Stats{}
	sent := s.Counter("bytes_sent", "B")
	sent.Add(5)
	sent.Add(7)

	m := metricByName(s.GetResult(), "bytes_sent")
	if m == nil {
		t.Fatal("GetResult has no metric \"bytes_sent\"")
	}
	if got, want := m.GetUnit(), "B"; got != want {
		t.Errorf("unit = %q, want %q", got, want)
	}
	c := m.GetCounter()
	if c.GetSum() != 12 || c.GetMax() != 12 || c.GetNodes() != 1 {
		t.Errorf("counter = (sum %d, max %d, nodes %d), want (12, 12, 1)", c.GetSum(), c.GetMax(), c.GetNodes())
	}
}

// TestMetricSeriesBySender verifies that AddBySender marks a series
// server-measured, and that GetResult returns its uncorrected samples in
// sender ID order. In hdr mode the samples of several senders keep their sign
// until the clock-offset correction.
func TestMetricSeriesBySender(t *testing.T) {
	t.Run("Exact", func(t *testing.T) {
		s := &Stats{}
		deliver := s.Series("deliver")
		deliver.AddBySender(2, 50)
		deliver.AddBySender(1, 10)
		deliver.AddBySender(1, 20)

		m := metricByName(s.GetResult(), "deliver")
		if got := m.GetMeasurementMode(); got != MeasurementMode_SERVER_MEASURED {
			t.Errorf("measurement mode = %v, want SERVER_MEASURED", got)
		}
		if got, want := m.GetDistribution().GetSamples(), []int64{10, 20, 50}; !slices.Equal(got, want) {
			t.Errorf("samples = %v, want %v", got, want)
		}
	})
	t.Run("HDR", func(t *testing.T) {
		s := NewStats(StatsMode_HDR)
		deliver := s.Series("deliver")
		deliver.AddBySender(1, -time.Millisecond)
		deliver.AddBySender(2, time.Millisecond)

		h := metricByName(s.GetResult(), "deliver").GetDistribution().GetHistogram()
		if got := totalCount(h); got != 2 {
			t.Fatalf("histogram count = %d, want 2", got)
		}
		if got := h.GetValue()[0]; got >= 0 {
			t.Errorf("smallest histogram value = %d, want a negative value", got)
		}
	})
}

// TestMetricRegistration verifies that registering a name again with the same
// kind and unit returns the same handle.
func TestMetricRegistration(t *testing.T) {
	s := &Stats{}
	first, second := s.Series("encode"), s.Series("encode")
	if first != second {
		t.Error("Series(\"encode\") returned two handles, want one")
	}
	c1, c2 := s.Counter("bytes", "B"), s.Counter("bytes", "B")
	if c1 != c2 {
		t.Error("Counter(\"bytes\", \"B\") returned two handles, want one")
	}
}

// TestMetricRegistrationConflictPanics verifies that an invalid name, a name
// registered as the other kind, a counter registered with a different unit,
// and a series used with both Add and AddBySender all panic.
func TestMetricRegistrationConflictPanics(t *testing.T) {
	tests := map[string]func(s *Stats){
		"EmptyName":       func(s *Stats) { s.Series("") },
		"UpperCase":       func(s *Stats) { s.Series("Encode") },
		"LeadingDigit":    func(s *Stats) { s.Counter("1bytes", "B") },
		"Dash":            func(s *Stats) { s.Counter("bytes-sent", "B") },
		"SeriesAsCounter": func(s *Stats) { s.Series("x"); s.Counter("x", "B") },
		"CounterAsSeries": func(s *Stats) { s.Counter("x", "B"); s.Series("x") },
		"CounterUnit":     func(s *Stats) { s.Counter("x", "B"); s.Counter("x", "KiB") },
		"AddThenBySender": func(s *Stats) { x := s.Series("x"); x.Add(1); x.AddBySender(1, 1) },
		"BySenderThenAdd": func(s *Stats) { x := s.Series("x"); x.AddBySender(1, 1); x.Add(1) },
	}
	for name, register := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			register(&Stats{})
		})
	}
}

// TestMetricTickCounts verifies that TickInterval records the cumulative
// sample count of each series in exact mode, and nothing in hdr mode.
func TestMetricTickCounts(t *testing.T) {
	for _, tt := range []struct {
		mode StatsMode
		want []uint64
	}{
		{StatsMode_EXACT, []uint64{2, 2, 5}},
		{StatsMode_HDR, nil},
	} {
		t.Run(tt.mode.String(), func(t *testing.T) {
			s := NewStats(tt.mode)
			encode := s.Series("encode")
			encode.Add(1)
			encode.Add(1)
			s.TickInterval()
			s.TickInterval()
			for range 3 {
				encode.Add(1)
			}
			s.TickInterval()

			got := metricByName(s.GetResult(), "encode").GetDistribution().GetTickCounts()
			if !slices.Equal(got, tt.want) {
				t.Errorf("tick counts = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMetricTickCountsLateSeries verifies that a series registered after some
// ticks counts zero samples at each earlier tick in exact mode, so that its
// tick counts align with the ticks of the run, and that Reset restarts the
// count of ticks.
func TestMetricTickCountsLateSeries(t *testing.T) {
	for _, tt := range []struct {
		mode      StatsMode
		want      []uint64
		wantReset []uint64
	}{
		{StatsMode_EXACT, []uint64{0, 0, 1, 3}, []uint64{0, 1}},
		{StatsMode_HDR, nil, nil},
	} {
		t.Run(tt.mode.String(), func(t *testing.T) {
			s := NewStats(tt.mode)
			s.TickInterval()
			s.TickInterval()
			encode := s.Series("encode")
			encode.Add(1)
			s.TickInterval()
			encode.Add(1)
			encode.Add(1)
			s.TickInterval()

			got := metricByName(s.GetResult(), "encode").GetDistribution().GetTickCounts()
			if !slices.Equal(got, tt.want) {
				t.Errorf("tick counts = %v, want %v", got, tt.want)
			}

			s.Reset(tt.mode)
			s.TickInterval()
			s.Series("decode").Add(1)
			s.TickInterval()
			got = metricByName(s.GetResult(), "decode").GetDistribution().GetTickCounts()
			if !slices.Equal(got, tt.wantReset) {
				t.Errorf("tick counts after Reset = %v, want %v", got, tt.wantReset)
			}
		})
	}
}

// TestMetricCounterResetOverlap verifies that an Add that overlaps a Reset
// counts wholly before the Reset or wholly after it: when every Add adds a
// positive value, a counter present after a Reset has a positive sum. The
// overlap is a matter of scheduling, so the test repeats it many times; run it
// with -race.
func TestMetricCounterResetOverlap(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	sent := s.Counter("sent", "B")
	stop := make(chan struct{})
	var wg sync.WaitGroup
	defer wg.Wait()
	defer close(stop)
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					sent.Add(1)
				}
			}
		})
	}
	for i := range 50000 {
		s.Reset(StatsMode_EXACT)
		if m := metricByName(s.GetResult(), "sent"); m != nil && m.GetCounter().GetSum() == 0 {
			t.Fatalf("reset %d: counter present with sum 0, want absent or a positive sum", i)
		}
	}
}

// TestMetricConcurrentAdd verifies that concurrent writers, ticks, and result
// snapshots lose no sample or count. Run it with -race.
func TestMetricConcurrentAdd(t *testing.T) {
	const writers, adds = 8, 500
	s := NewStats(StatsMode_EXACT)
	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			encode := s.Series("encode")
			sent := s.Counter("sent", "B")
			for i := range adds {
				encode.Add(time.Duration(i))
				s.Series(fmt.Sprintf("deliver_%d", w%2)).AddBySender(uint32(w), 1)
				sent.Add(2)
			}
		})
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			s.TickInterval()
			s.GetResult()
		}
	}()
	wg.Wait()
	<-done

	r := s.GetResult()
	if got := len(metricByName(r, "encode").GetDistribution().GetSamples()); got != writers*adds {
		t.Errorf("encode samples = %d, want %d", got, writers*adds)
	}
	for _, name := range []string{"deliver_0", "deliver_1"} {
		if got := len(metricByName(r, name).GetDistribution().GetSamples()); got != writers/2*adds {
			t.Errorf("%s samples = %d, want %d", name, got, writers/2*adds)
		}
	}
	if got := metricByName(r, "sent").GetCounter().GetSum(); got != 2*writers*adds {
		t.Errorf("sent = %d, want %d", got, 2*writers*adds)
	}
}
