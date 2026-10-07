package benchkit

import (
	"math"
	"slices"
	"testing"
	"time"
)

// TestStatsOps verifies that Ops counts every recorded operation regardless of
// how it was recorded, and that Reset clears the counter. ServerMeasured uses
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
	s.Reset(StatsMode_EXACT)
	if got := s.Ops(); got != 0 {
		t.Errorf("Ops() after Reset = %d, want 0", got)
	}
}

func TestStatsResetClearsSamples(t *testing.T) {
	s := &Stats{}
	s.AddLatency(5 * time.Nanosecond)
	s.AddLatency(7 * time.Nanosecond)
	s.Reset(StatsMode_EXACT)
	r := s.GetResult()
	if got := r.GetLatencies(); len(got) != 0 {
		t.Errorf("Latencies after Reset = %v, want empty", got)
	}
	if got := r.Percentiles(0.5); got != nil {
		t.Errorf("Percentiles after Reset = %v, want nil", got)
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

func TestStatsGetResultCorrected(t *testing.T) {
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

func TestStatsGetResultCorrectedMissingOffset(t *testing.T) {
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

// TestStatsGetResultCorrectedHDR verifies that in StatsMode_HDR the
// per-sender correction shifts each sender's histogram by that sender's clock
// offset and re-quantizes the shifted per-sender histograms onto one bounded
// histogram: Latencies is nil, the count and distribution match the exact path.
func TestStatsGetResultCorrectedHDR(t *testing.T) {
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

// TestStatsCorrectedNegativeSamples verifies that a raw sample below zero
// keeps its value until the per-sender clock correction. A sender whose clock
// runs ahead of the receiver by more than the transit time yields negative raw
// samples, and both modes must correct them to the true transit time.
func TestStatsCorrectedNegativeSamples(t *testing.T) {
	const (
		transit = 100 * time.Microsecond
		ahead   = 10 * time.Millisecond
	)
	for _, mode := range []StatsMode{StatsMode_EXACT, StatsMode_HDR} {
		t.Run(mode.String(), func(t *testing.T) {
			s := NewStats(mode)
			s.Start()
			s.AddLatencyBySender(1, transit-ahead)
			s.End()

			r := s.GetResultCorrected(map[uint32]int64{1: int64(ahead)})
			if got := r.GetTotalOps(); got != 1 {
				t.Errorf("TotalOps = %d, want 1", got)
			}
			// 3 sigfigs at a raw magnitude near 10ms resolves to about 8µs.
			if got := r.Percentiles(0.5)[0]; got < transit-10*time.Microsecond || got > transit+10*time.Microsecond {
				t.Errorf("corrected p50 = %v, want ≈%v", got, transit)
			}
		})
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

func TestStatsResetClearsBySender(t *testing.T) {
	s := &Stats{}
	s.AddLatencyBySender(1, 5*time.Nanosecond)
	s.AddLatencyBySender(2, 7*time.Nanosecond)
	s.Reset(StatsMode_EXACT)
	if got := s.GetResultCorrected(map[uint32]int64{1: 1, 2: 1}).GetLatencies(); len(got) != 0 {
		t.Errorf("bySender latencies after Reset = %v, want empty", got)
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
	if pcts := r.Percentiles(0.5); len(pcts) == 0 || math.Abs(float64(pcts[0]-5*time.Microsecond)) > 50 {
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

func TestStatsResetClearsIntervalState(t *testing.T) {
	// After Reset, TickInterval should see no ops from before the reset.
	s := &Stats{}
	s.AddLatency(100 * time.Nanosecond)
	s.AddLatency(200 * time.Nanosecond)
	s.Reset(StatsMode_EXACT)
	_, _, count, opDelta := s.TickInterval()
	if count != 0 || opDelta != 0 {
		t.Errorf("TickInterval after Reset = (count=%d, opDelta=%d), want (0, 0)",
			count, opDelta)
	}
}
