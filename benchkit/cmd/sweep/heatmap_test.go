package main

import (
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/relab/gorums/benchkit"
)

// csvHeader reads the first line of a CSV written to path.
func csvHeader(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return line
}

func TestNodeHealthRows(t *testing.T) {
	// One run, three nodes; bb10 at half the others' throughput. Two CDF rows
	// per node prove throughput is read once per node, not per row.
	var cdf []plotNodeCDFRecord
	for _, n := range []struct {
		node string
		thr  float64
	}{{"bb2:9000", 100}, {"bb3:9000", 100}, {"bb10:9000", 50}} {
		for range 2 {
			cdf = append(cdf, plotNodeCDFRecord{
				Dimensions: benchkit.Dimensions{Benchmark: "Q", StreamMode: "dual", Nodes: 3},
				base:       "run_Q_N3", label: "run", node: n.node, throughput: n.thr,
			})
		}
	}
	rows := nodeHealthRows(cdf)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	// Median of {100,100,50} is 100. Natural host order: bb2, bb3, bb10.
	if rows[0].host != "bb2" || rows[2].host != "bb10" {
		t.Errorf("host order = %s..%s, want bb2..bb10", rows[0].host, rows[2].host)
	}
	byHost := map[string]float64{}
	for _, r := range rows {
		byHost[r.host] = r.rel
	}
	if byHost["bb2"] != 1.0 || byHost["bb10"] != 0.5 {
		t.Errorf("rel bb2=%g bb10=%g, want 1.0 and 0.5", byHost["bb2"], byHost["bb10"])
	}

	path := filepath.Join(t.TempDir(), "node_health.csv")
	if err := writeNodeHealthCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	if h := csvHeader(t, path); !strings.Contains(h, "col") || !strings.Contains(h, "rel") {
		t.Errorf("node_health header = %q", h)
	}
}

// TestNodeHealthRowsZeroMedianTreatedAsUniform verifies that a run whose
// nodes all report zero throughput (so the run's median is zero) is treated
// as uniform (rel 1.0) instead of dividing by zero and propagating NaN.
func TestNodeHealthRowsZeroMedianTreatedAsUniform(t *testing.T) {
	var cdf []plotNodeCDFRecord
	for _, node := range []string{"bb2:9000", "bb3:9000"} {
		cdf = append(cdf, plotNodeCDFRecord{
			Dimensions: benchkit.Dimensions{Benchmark: "Q", StreamMode: "dual", Nodes: 2},
			base:       "run_Q_N2", label: "run", node: node, throughput: 0,
		})
	}
	rows := nodeHealthRows(cdf)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if math.IsNaN(r.rel) {
			t.Fatalf("host %s: rel is NaN, want 1.0 (zero-median run treated as uniform)", r.host)
		}
		if r.rel != 1.0 {
			t.Errorf("host %s: rel = %g, want 1.0", r.host, r.rel)
		}
	}
}

// TestNodeHealthRowsSkipsSingleNodeRuns verifies that a run with only one
// node contributing a CDF row is excluded entirely: a lone node has no peers
// to compute a relative-to-median health signal against.
func TestNodeHealthRowsSkipsSingleNodeRuns(t *testing.T) {
	cdf := []plotNodeCDFRecord{
		{
			Dimensions: benchkit.Dimensions{Benchmark: "Q", StreamMode: "dual", Nodes: 1},
			base:       "run_Q_N1", label: "run", node: "bb2:9000", throughput: 100,
		},
	}
	if rows := nodeHealthRows(cdf); len(rows) != 0 {
		t.Errorf("rows = %d, want 0 (single-node run must be skipped)", len(rows))
	}
}

func TestDegradedShareRows(t *testing.T) {
	runs := []plotRunRecord{
		{Dimensions: benchkit.Dimensions{Benchmark: "Q", Nodes: 3, Workers: 8, StreamMode: "dedup"}, status: runStatusSucceeded},
		{Dimensions: benchkit.Dimensions{Benchmark: "Q", Nodes: 3, Workers: 8, StreamMode: "dedup"}, status: runStatusSucceeded},
		{Dimensions: benchkit.Dimensions{Benchmark: "Q", Nodes: 3, Workers: 8, StreamMode: "dedup"}, status: runStatusDegraded},
		{Dimensions: benchkit.Dimensions{Benchmark: "Q", Nodes: 3, Workers: 8, StreamMode: "dual"}, status: runStatusSucceeded},
	}
	rows := degradedShareRows(runs)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	share := map[string]float64{}
	for _, r := range rows {
		share[r.StreamMode] = r.share
	}
	// dedup: 1 of 3 degraded; dual: 0 of 1.
	if share["dedup"] < 0.33 || share["dedup"] > 0.34 {
		t.Errorf("dedup share = %g, want ~0.333", share["dedup"])
	}
	if share["dual"] != 0 {
		t.Errorf("dual share = %g, want 0", share["dual"])
	}

	path := filepath.Join(t.TempDir(), "degraded_share.csv")
	if err := writeDegradedShareCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	if h := csvHeader(t, path); !strings.Contains(h, "share") || !strings.Contains(h, "col") {
		t.Errorf("degraded_share header = %q", h)
	}
}

// TestWriteDegradedShareCSVAxisLabels verifies the heatmap's two axis labels: the
// node count and mode go on the row axis and the remaining varying dimensions on
// the column axis, and neither repeats a dimension every configuration shares —
// one long label per configuration on a single axis overlapped the grid.
func TestWriteDegradedShareCSVAxisLabels(t *testing.T) {
	var runs []plotRunRecord
	for _, nodes := range []int{9, 15} {
		for _, rate := range []int{1000, 2000} {
			for _, mode := range []string{"dedup", "dual"} {
				runs = append(runs, plotRunRecord{
					Dimensions: benchkit.Dimensions{
						Benchmark: "Q", Nodes: nodes, Workers: 32, Payload: 16384,
						Rate: rate, SendBuffer: 4096, StreamMode: mode,
					},
					status: runStatusSucceeded,
				})
			}
		}
	}
	path := filepath.Join(t.TempDir(), "degraded_share.csv")
	if err := writeDegradedShareCSV(path, degradedShareRows(runs)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimSpace(string(data)), "\n")
	header := strings.Split(rows[0], ",")
	rowCol, colCol := slices.Index(header, "row"), slices.Index(header, "col")
	if rowCol < 0 || colCol < 0 {
		t.Fatalf("degraded_share header %q lacks row/col", rows[0])
	}
	labels := map[string]bool{}
	for _, line := range rows[1:] {
		fields := strings.Split(line, ",")
		labels[fields[rowCol]+" | "+fields[colCol]] = true
	}
	for _, want := range []string{"N9 dedup | R1000", "N15 dual | R2000"} {
		if !labels[want] {
			t.Errorf("missing axis labels %q; got %v", want, slices.Sorted(maps.Keys(labels)))
		}
	}
	for label := range labels {
		for _, fixed := range []string{"W32", "P16384", "SB4096", "Q"} {
			if strings.Contains(label, fixed) {
				t.Errorf("label %q repeats %q, which every configuration shares", label, fixed)
			}
		}
	}
}

func TestSplitTrailingNum(t *testing.T) {
	tests := []struct {
		in     string
		prefix string
		num    int
	}{
		{"bb10", "bb", 10}, {"bb2", "bb", 2}, {"rack1-node5", "rack1-node", 5}, {"host", "host", 0},
	}
	for _, tt := range tests {
		if p, n := splitTrailingNum(tt.in); p != tt.prefix || n != tt.num {
			t.Errorf("splitTrailingNum(%q) = (%q,%d), want (%q,%d)", tt.in, p, n, tt.prefix, tt.num)
		}
	}
}
