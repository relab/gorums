package benchkit

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintResultsAlignsMetricColumns(t *testing.T) {
	results := []*Result{
		Result_builder{
			Config:      RunConfig_builder{Name: "SymmetricMulticast"}.Build(),
			Throughput:  68972.16,
			Latencies:   []int64{584300, 584300},
			MemPerOp:    0,
			AllocsPerOp: 0,
		}.Build(),
		Result_builder{
			Config:      RunConfig_builder{Name: "SymmetricQuorumCall"}.Build(),
			Throughput:  7910.27,
			Latencies:   []int64{2600000, 2600000},
			MemPerOp:    7353,
			AllocsPerOp: 155,
		}.Build(),
	}

	var buf bytes.Buffer
	PrintResults(&buf, results, Options{}, false, "bb3:9000")
	got := buf.String()

	for _, want := range []string{"68972.2 ops/sec", "7910.3 ops/sec", "584.3 µs", "2.6 ms"} {
		if !strings.Contains(got, want) {
			t.Errorf("PrintResults() missing %q in output:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("PrintResults() should leave a blank line after node results, got:\n%q", got)
	}

	lines := strings.Split(strings.TrimSuffix(got, "\n\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d output lines, want 3:\n%s", len(lines), got)
	}
	latencyStart := strings.Index(lines[0], "Latency")
	if latencyStart < 0 {
		t.Fatalf("missing Latency header in:\n%s", got)
	}
	firstDot := strings.Index(lines[1][latencyStart:], ".")
	secondDot := strings.Index(lines[2][latencyStart:], ".")
	if firstDot < 0 || secondDot < 0 || firstDot != secondDot {
		t.Fatalf("latency decimal points not aligned:\n%s\n%s", lines[1], lines[2])
	}
}

// TestPrintResultsDoesNotMutateResult guards against the rendering bug where
// folding per-server memory into the combined B/op and allocs/op columns
// mutated the *Result that the caller later serializes.
func TestPrintResultsDoesNotMutateResult(t *testing.T) {
	r := Result_builder{
		Config:      RunConfig_builder{Name: "SymmetricMulticast"}.Build(),
		Throughput:  68972.16,
		Latencies:   []int64{584300, 584300},
		TotalOps:    100,
		MemPerOp:    1000,
		AllocsPerOp: 10,
		ServerStats: []*MemoryStat{
			MemoryStat_builder{Memory: 200_000, Allocs: 2_000}.Build(),
			MemoryStat_builder{Memory: 300_000, Allocs: 3_000}.Build(),
		},
	}.Build()

	var buf bytes.Buffer
	// Remote run without -server-stats triggers the per-server folding path.
	PrintResults(&buf, []*Result{r}, Options{Remote: true}, false, "")
	got := buf.String()

	// The rendered row must show the combined per-op memory:
	// 1000 + 200000/100 + 300000/100 = 6000 B/op; 10 + 2000/100 + 3000/100 = 60 allocs/op.
	for _, want := range []string{"6000 B/op", "60 allocs/op"} {
		if !strings.Contains(got, want) {
			t.Errorf("PrintResults() missing folded value %q in output:\n%s", want, got)
		}
	}
	// The persisted Result must be untouched.
	if got, want := r.GetMemPerOp(), uint64(1000); got != want {
		t.Errorf("MemPerOp mutated: got %d, want %d", got, want)
	}
	if got, want := r.GetAllocsPerOp(), uint64(10); got != want {
		t.Errorf("AllocsPerOp mutated: got %d, want %d", got, want)
	}
}
