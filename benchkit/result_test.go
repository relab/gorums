package benchkit

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestResultSetClientTotals(t *testing.T) {
	tests := []struct {
		name           string
		sent           uint64
		elapsed        time.Duration
		wantThroughput float64
	}{
		{"Sends", 100, 2 * time.Second, 50},
		{"NoSends", 0, 2 * time.Second, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pre-existing server-side totals must be overridden.
			r := Result_builder{TotalOps: 999, TotalTime: 1}.Build()
			r.SetClientTotals(tt.sent, tt.elapsed)
			if got := r.GetTotalOps(); got != tt.sent {
				t.Errorf("TotalOps = %d, want %d", got, tt.sent)
			}
			if got := time.Duration(r.GetTotalTime()); got != tt.elapsed {
				t.Errorf("TotalTime = %v, want %v", got, tt.elapsed)
			}
			if got := r.GetThroughput(); got != tt.wantThroughput {
				t.Errorf("Throughput = %v, want %v", got, tt.wantThroughput)
			}
		})
	}
}

// TestResultSetPerOpMemoryFromServerStats verifies that per-op memory derives
// from the largest server's counters divided by the client send count, since
// summing the servers would count the shared process once per local server.
func TestResultSetPerOpMemoryFromServerStats(t *testing.T) {
	stat := func(allocs, mem uint64) *MemoryStat {
		return MemoryStat_builder{Allocs: allocs, Memory: mem}.Build()
	}
	tests := []struct {
		name       string
		stats      []*MemoryStat
		sent       uint64
		wantAllocs uint64
		wantMem    uint64
	}{
		{"OneServer", []*MemoryStat{stat(300, 6000)}, 10, 30, 600},
		{"LocalServersShareProcess", []*MemoryStat{stat(290, 5800), stat(310, 6200), stat(300, 6000)}, 10, 31, 620},
		{"NoSends", []*MemoryStat{stat(300, 6000)}, 0, 0, 0},
		{"NoServerStats", nil, 10, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Result_builder{ServerStats: tt.stats}.Build()
			r.SetPerOpMemoryFromServerStats(tt.sent)
			if got := r.GetAllocsPerOp(); got != tt.wantAllocs {
				t.Errorf("AllocsPerOp = %d, want %d", got, tt.wantAllocs)
			}
			if got := r.GetMemPerOp(); got != tt.wantMem {
				t.Errorf("MemPerOp = %d, want %d", got, tt.wantMem)
			}
		})
	}
}

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
