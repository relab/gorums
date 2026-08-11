package benchkit

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestResultPercentilesAndLatencies(t *testing.T) {
	s := &Stats{}
	for i := 1; i <= 100; i++ {
		s.AddLatency(time.Duration(i) * time.Nanosecond)
	}
	r := s.GetResult()

	// Hyndman-Fan R7: p50 of 1..100 is 50.5; p95 is 95.05; p99 is 99.01.
	// time.Duration truncates to integer nanoseconds, so the expectations
	// below are the floor of those values.
	got := r.Percentiles(0.5, 0.95, 0.99)
	want := []time.Duration{50, 95, 99}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("Percentiles[%d] = %v, want %v", i, g, want[i])
		}
	}

	latencies := r.GetLatencies()
	if len(latencies) != 100 {
		t.Errorf("Latencies length = %d, want 100", len(latencies))
	}
	if latencies[0] != 1 || latencies[99] != 100 {
		t.Errorf("Latencies[0]=%v, Latencies[99]=%v; want 1 and 100", latencies[0], latencies[99])
	}
}

// TestStatsOps verifies that Ops counts every recorded operation regardless of
// how it was recorded, and that Clear resets the counter. ServerMeasured uses
// it to derive client-side per-op memory stats from the client's own send
// count rather than the aggregated server op count.
func TestStatsOps(t *testing.T) {
	s := &Stats{}
	s.AddLatency(time.Nanosecond)
	s.AddOp()
	s.AddOp()
	s.AddLatencyBySender(1, time.Nanosecond)
	if got := s.Ops(); got != 4 {
		t.Errorf("Ops() = %d, want 4", got)
	}
	s.Clear()
	if got := s.Ops(); got != 0 {
		t.Errorf("Ops() after Clear = %d, want 0", got)
	}
}

func TestStatsClearResetsSamples(t *testing.T) {
	s := &Stats{}
	s.AddLatency(5 * time.Nanosecond)
	s.AddLatency(7 * time.Nanosecond)
	s.Clear()
	r := s.GetResult()
	if got := r.GetLatencies(); len(got) != 0 {
		t.Errorf("Latencies after Clear = %v, want empty", got)
	}
	if got := r.Percentiles(0.5); got != nil {
		t.Errorf("Percentiles after Clear = %v, want nil", got)
	}
}

func TestStatsGetResultMeanAndStdDev(t *testing.T) {
	s := &Stats{}
	// Latencies: 10, 20, 30 ns
	// Sample mean = 20 ns; sample variance = 200/2 = 100; sample stddev = 10 ns.
	s.Start()
	for _, v := range []int{10, 20, 30} {
		s.AddLatency(time.Duration(v) * time.Nanosecond)
	}
	s.End()

	r := s.GetResult()
	if got := r.GetTotalOps(); got != 3 {
		t.Errorf("TotalOps = %d, want 3", got)
	}
	if gotMean, gotSD := r.LatencyMeanAndStdDev(); gotMean != 20*time.Nanosecond || gotSD != 10*time.Nanosecond {
		t.Errorf("LatencyMeanAndStdDev = (%v, %v), want (20ns, 10ns)", gotMean, gotSD)
	}
	if got := r.GetLatencies(); len(got) != 3 {
		t.Errorf("Latencies length = %d, want 3", len(got))
	}
}

func TestResultFormat(t *testing.T) {
	r := Result_builder{
		Config:      RunConfig_builder{Name: "TestBench"}.Build(),
		Throughput:  1234.56,
		Latencies:   []int64{time.Millisecond.Nanoseconds(), 2 * time.Millisecond.Nanoseconds()},
		MemPerOp:    42,
		AllocsPerOp: 7,
	}.Build()
	got := r.Format()

	// Format must contain the benchmark name and all stat columns.
	for _, want := range []string{"TestBench", "ops/sec", "ms", "B/op", "allocs/op"} {
		if !strings.Contains(got, want) {
			t.Errorf("Format() missing %q in output: %s", want, got)
		}
	}
	for _, want := range []string{"1234.6 ops/sec", "1.5 ms"} {
		if !strings.Contains(got, want) {
			t.Errorf("Format() missing one-decimal value %q in output: %s", want, got)
		}
	}
}

// TestResultRow verifies that [Result.Row] returns exactly the nine
// documented columns in order, and that [Result.Format] is derived from Row
// (tab-joined with a trailing tab) rather than an independently formatted
// string.
func TestResultRow(t *testing.T) {
	r := Result_builder{
		Config:      RunConfig_builder{Name: "TestBench"}.Build(),
		Throughput:  1234.56,
		Latencies:   []int64{time.Millisecond.Nanoseconds(), 2 * time.Millisecond.Nanoseconds()},
		MemPerOp:    42,
		AllocsPerOp: 7,
	}.Build()

	row := r.Row()
	if len(row) != 9 {
		t.Fatalf("len(Row()) = %d, want 9", len(row))
	}
	if row[0] != "TestBench" {
		t.Errorf("Row()[0] = %q, want %q", row[0], "TestBench")
	}
	if row[1] != "1234.6 ops/sec" {
		t.Errorf("Row()[1] = %q, want %q", row[1], "1234.6 ops/sec")
	}
	if row[7] != "42 B/op" || row[8] != "7 allocs/op" {
		t.Errorf("Row()[7:9] = %v, want [42 B/op, 7 allocs/op]", row[7:9])
	}

	if want := strings.Join(row, "\t") + "\t"; r.Format() != want {
		t.Errorf("Format() = %q, want %q (Row tab-joined with a trailing tab)", r.Format(), want)
	}
}

// TestResultRowNoLatencySamples verifies that Row falls back to "n/a" for the
// percentile columns when no latency samples are recorded, matching Format's
// prior behavior.
func TestResultRowNoLatencySamples(t *testing.T) {
	r := Result_builder{Config: RunConfig_builder{Name: "Empty"}.Build()}.Build()
	row := r.Row()
	for i, want := range []string{"n/a", "n/a", "n/a"} {
		if got := row[4+i]; got != want {
			t.Errorf("Row()[%d] = %q, want %q", 4+i, got, want)
		}
	}
}

func TestResultLatencyMethodsEdgeCases(t *testing.T) {
	empty := &Result{}
	if gotMean, gotSD := empty.LatencyMeanAndStdDev(); gotMean != 0 || gotSD != 0 {
		t.Errorf("LatencyMeanAndStdDev on empty = (%v, %v), want (0, 0)", gotMean, gotSD)
	}

	single := Result_builder{Latencies: []int64{42}}.Build()
	if gotMean, gotSD := single.LatencyMeanAndStdDev(); gotMean != 42*time.Nanosecond || gotSD != 0 {
		t.Errorf("LatencyMeanAndStdDev on single sample = (%v, %v), want (42ns, 0)", gotMean, gotSD)
	}
}

// TestResultPercentilesMalformedHistogram verifies that Percentiles weights
// only the aligned (value, count) pairs when a LatencyHistogram carries more
// counts than values (e.g. from a truncated or corrupt result file), so every
// requested quantile resolves to a recorded value instead of a fabricated
// zero-nanosecond reading.
func TestResultPercentilesMalformedHistogram(t *testing.T) {
	r := Result_builder{
		Histogram: LatencyHistogram_builder{
			Value: []int64{10, 20},
			Count: []uint64{1, 2, 3}, // one more count than values
		}.Build(),
	}.Build()
	// The unmatched third count is dropped, leaving total=3 over the pairs
	// (10, 1) and (20, 2); p50's rank (2) and p99's rank (3) both land in the
	// second pair.
	got := r.Percentiles(0.5, 0.99)
	want := []time.Duration{20, 20}
	if !slices.Equal(got, want) {
		t.Errorf("Percentiles(malformed histogram) = %v, want %v", got, want)
	}
}

func TestSymmetricMulticastCorrectedResult(t *testing.T) {
	// Two senders: node 1 with offset +100 (its clock is 100ns ahead of ours,
	// so its raw samples read 100ns low and need +100), node 2 with offset -50.
	// Loopback (node 3) has offset 0 and is left unchanged.
	s := &Stats{}
	s.Start()
	s.AddLatencyBySender(1, 200*time.Nanosecond)
	s.AddLatencyBySender(1, 300*time.Nanosecond)
	s.AddLatencyBySender(2, 500*time.Nanosecond)
	s.AddLatencyBySender(3, 40*time.Nanosecond)
	s.End()

	offsets := map[uint32]int64{1: 100, 2: -50, 3: 0}
	r := s.GetResultCorrected(offsets)

	// Buckets are visited in sorted node-ID order: 1, 1, 2, 3.
	want := []int64{300, 400, 450, 40}
	if got := r.GetLatencies(); !slices.Equal(got, want) {
		t.Errorf("corrected latencies = %v, want %v", got, want)
	}
	if got := r.GetTotalOps(); got != 4 {
		t.Errorf("TotalOps = %d, want 4", got)
	}
}

func TestSymmetricMulticastCorrectedMissingOffset(t *testing.T) {
	// A sender absent from the offsets map is treated as zero offset.
	s := &Stats{}
	s.Start()
	s.AddLatencyBySender(7, 123*time.Nanosecond)
	s.End()

	r := s.GetResultCorrected(map[uint32]int64{})
	if got, want := r.GetLatencies(), []int64{123}; !slices.Equal(got, want) {
		t.Errorf("corrected latencies = %v, want %v", got, want)
	}
}

// TestSymmetricMulticastCorrectedResultHDR verifies that in StatsMode_HDR the
// per-sender correction shifts each sender's histogram by that sender's clock
// offset and re-quantizes the shifted per-sender histograms onto one bounded
// histogram: Latencies is nil, the count and distribution match the exact path.
func TestSymmetricMulticastCorrectedResultHDR(t *testing.T) {
	// Same senders and offsets as the exact test; corrected samples are
	// {300, 400} (sender 1), {450} (sender 2), {40} (sender 3, loopback).
	s := NewStats(StatsMode_HDR)
	s.Start()
	s.AddLatencyBySender(1, 200*time.Nanosecond)
	s.AddLatencyBySender(1, 300*time.Nanosecond)
	s.AddLatencyBySender(2, 500*time.Nanosecond)
	s.AddLatencyBySender(3, 40*time.Nanosecond)
	s.End()

	r := s.GetResultCorrected(map[uint32]int64{1: 100, 2: -50, 3: 0})
	if got := r.GetLatencies(); got != nil {
		t.Errorf("Latencies in HDR mode = %v, want nil", got)
	}
	if got := r.GetTotalOps(); got != 4 {
		t.Errorf("TotalOps = %d, want 4", got)
	}
	h := r.GetHistogram()
	if h == nil {
		t.Fatal("Histogram in HDR mode = nil, want non-nil")
	}
	var total uint64
	for _, c := range h.GetCount() {
		total += c
	}
	if total != 4 {
		t.Errorf("histogram counts sum = %d, want 4", total)
	}
	// The corrected distribution is {40, 300, 400, 450}ns; its mean is 297.5ns,
	// reproduced within HDR precision (3 sigfigs resolves these values finely).
	if mean, _ := r.LatencyMeanAndStdDev(); math.Abs(float64(mean)-297.5) > 5 {
		t.Errorf("LatencyMeanAndStdDev mean = %v, want ≈297.5ns", mean)
	}
}

// TestStatsResetSwitchesMode verifies that Reset reconfigures the aggregate and
// per-sender stores to the requested mode, so one Stats can back consecutive
// runs with different StatsMode values.
func TestStatsResetSwitchesMode(t *testing.T) {
	s := NewStats(StatsMode_EXACT)

	s.Reset(StatsMode_HDR)
	s.Start()
	s.AddLatency(3 * time.Microsecond)
	s.AddLatencyBySender(1, 3*time.Microsecond)
	s.End()
	if got := s.GetResult().GetLatencies(); got != nil {
		t.Errorf("aggregate Latencies after Reset(HDR) = %v, want nil", got)
	}
	if got := s.GetResultCorrected(nil).GetLatencies(); got != nil {
		t.Errorf("per-sender Latencies after Reset(HDR) = %v, want nil", got)
	}
	if s.GetResult().GetHistogram() == nil {
		t.Error("aggregate Histogram after Reset(HDR) = nil, want non-nil")
	}

	s.Reset(StatsMode_EXACT)
	s.Start()
	s.AddLatency(7 * time.Microsecond)
	s.End()
	if got := s.GetResult().GetLatencies(); len(got) != 1 {
		t.Errorf("aggregate Latencies after Reset(EXACT) = %v, want one sample", got)
	}
	if s.GetResult().GetHistogram() != nil {
		t.Error("aggregate Histogram after Reset(EXACT) != nil, want nil")
	}
}

func TestStatsClearResetsBySender(t *testing.T) {
	s := &Stats{}
	s.AddLatencyBySender(1, 5*time.Nanosecond)
	s.AddLatencyBySender(2, 7*time.Nanosecond)
	s.Clear()
	if got := s.GetResultCorrected(map[uint32]int64{1: 1, 2: 1}).GetLatencies(); len(got) != 0 {
		t.Errorf("bySender latencies after Clear = %v, want empty", got)
	}
}

func TestStatsHDRModeResult(t *testing.T) {
	// HDR mode counts ops and exposes them via TotalOps; raw samples are not
	// retained (Latencies is nil) and the distribution is carried by the
	// Histogram field instead, from which percentiles and mean/stddev are
	// derived within the histogram's precision.
	s := NewStats(StatsMode_HDR)
	s.Start()
	for i := range 10 {
		s.AddLatency(time.Duration(i+1) * time.Microsecond)
	}
	s.End()
	r := s.GetResult()
	if got := r.GetTotalOps(); got != 10 {
		t.Errorf("TotalOps = %d, want 10", got)
	}
	if got := r.GetLatencies(); got != nil {
		t.Errorf("Latencies in HDR mode = %v, want nil", got)
	}
	if got := r.GetThroughput(); got == 0 {
		t.Errorf("Throughput in HDR mode = 0, want non-zero")
	}
	h := r.GetHistogram()
	if h == nil {
		t.Fatal("Histogram in HDR mode = nil, want non-nil")
	}
	var total uint64
	for _, c := range h.GetCount() {
		total += c
	}
	if total != 10 {
		t.Errorf("histogram counts sum = %d, want 10", total)
	}
	// p50 of 1µs..10µs is 5µs; histogram precision is 3 sigfigs.
	if pcts := r.Percentiles(0.5); pcts == nil || math.Abs(float64(pcts[0]-5*time.Microsecond)) > 50 {
		t.Errorf("Percentiles(0.5) = %v, want ≈5µs", pcts)
	}
	// Mean of 1µs..10µs is 5.5µs.
	if mean, _ := r.LatencyMeanAndStdDev(); math.Abs(float64(mean-5500*time.Nanosecond)) > 50 {
		t.Errorf("LatencyMeanAndStdDev mean = %v, want ≈5.5µs", mean)
	}
}

func TestStatsTickInterval(t *testing.T) {
	// TickInterval returns Welford stats and op delta for samples added since
	// the last tick, then resets for the next interval.
	s := &Stats{}
	s.AddLatency(100 * time.Nanosecond)
	s.AddLatency(200 * time.Nanosecond)
	mean, stddev, count, opDelta := s.TickInterval()

	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if opDelta != 2 {
		t.Errorf("opDelta = %d, want 2", opDelta)
	}
	wantMean := 150.0
	if math.Abs(mean-wantMean) > 0.001 {
		t.Errorf("mean = %v, want %v", mean, wantMean)
	}
	// Sample stddev of [100, 200]: sqrt(((100-150)² + (200-150)²) / 1) = 70.71...
	wantSD := math.Sqrt(5000.0)
	if math.Abs(stddev-wantSD) > 0.001 {
		t.Errorf("stddev = %v, want %v", stddev, wantSD)
	}

	// After TickInterval, adding more samples starts a fresh interval.
	s.AddLatency(50 * time.Nanosecond)
	mean2, _, count2, opDelta2 := s.TickInterval()
	if count2 != 1 {
		t.Errorf("second tick count = %d, want 1", count2)
	}
	if opDelta2 != 1 {
		t.Errorf("second tick opDelta = %d, want 1", opDelta2)
	}
	if math.Abs(mean2-50.0) > 0.001 {
		t.Errorf("second tick mean = %v, want 50", mean2)
	}
}

func TestStatsTickIntervalEmpty(t *testing.T) {
	// TickInterval with no samples in the interval returns all zeros.
	s := &Stats{}
	mean, stddev, count, opDelta := s.TickInterval()
	if mean != 0 || stddev != 0 || count != 0 || opDelta != 0 {
		t.Errorf("TickInterval on empty = (%v, %v, %v, %v), want all zeros",
			mean, stddev, count, opDelta)
	}
}

func TestStatsClearResetsIntervalState(t *testing.T) {
	// After Clear, TickInterval should see no ops from before the clear.
	s := &Stats{}
	s.AddLatency(100 * time.Nanosecond)
	s.AddLatency(200 * time.Nanosecond)
	s.Clear()
	_, _, count, opDelta := s.TickInterval()
	if count != 0 || opDelta != 0 {
		t.Errorf("TickInterval after Clear = (count=%d, opDelta=%d), want (0, 0)",
			count, opDelta)
	}
}
