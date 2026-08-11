package main

import (
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/relab/gorums/benchkit"
)

// buildBinaryResultFile returns the bytes of a result file as
// [benchkit.WriteReport] writes them, so the test exercises the same decode
// path sweep uses in production rather than a hand-built framing.
func buildBinaryResultFile(t *testing.T, name string, throughput float64, latencies []int64) []byte {
	t.Helper()
	results := []*benchkit.Result{benchkit.Result_builder{
		Config:     benchkit.RunConfig_builder{Name: name}.Build(),
		Throughput: throughput,
		Latencies:  latencies,
	}.Build()}
	path := filepath.Join(t.TempDir(), "results"+resultExt)
	if err := benchkit.WriteLabeledReport(results, "test", path); err != nil {
		t.Fatalf("WriteLabeledReport: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestBinaryResultsDecode verifies that parseBinaryResultFile extracts name,
// throughput, and latencies from a binary result file into the run summary.
func TestBinaryResultsDecode(t *testing.T) {
	wantName := "QuorumCall"
	wantThroughput := 12345.6
	wantLatencies := []int64{100, 200, 300}

	file := buildBinaryResultFile(t, wantName, wantThroughput, wantLatencies)

	byBench := make(map[string]*benchSummary)
	if err := parseBinaryResultFile(file, byBench, 0); err != nil {
		t.Fatalf("parseBinaryResultFile: %v", err)
	}

	s, ok := byBench[wantName]
	if !ok {
		t.Fatalf("no entry for %q; got keys: %v", wantName, slices.Collect(maps.Keys(byBench)))
	}
	if s.throughput != wantThroughput {
		t.Errorf("throughput = %v, want %v", s.throughput, wantThroughput)
	}
	if got := s.latency.Count(); got != uint64(len(wantLatencies)) {
		t.Errorf("latency samples = %d, want %d", got, len(wantLatencies))
	}
	// Mean over 100, 200, 300.
	if mean, _ := s.latency.MeanAndStdDev(); mean != 200 {
		t.Errorf("latency mean = %v, want 200", mean)
	}
	if s.nodes != 1 {
		t.Errorf("nodes = %d, want 1", s.nodes)
	}
}

// TestBinaryResultsRejectsNonBinary verifies parseBinaryResultFile rejects a
// file that does not carry the binary magic header.
func TestBinaryResultsRejectsNonBinary(t *testing.T) {
	byBench := make(map[string]*benchSummary)
	if err := parseBinaryResultFile([]byte(`{"label":"x","results":[]}`), byBench, 0); err == nil {
		t.Error("parseBinaryResultFile(non-binary) = nil error, want error")
	}
}

// TestMergeResultsMeasurementMode verifies that throughput is summed from
// client-measured results when any exist in the report, so a PBFT-style
// primary-client run reports the primary's client ops/s rather than
// primary+Σbackup execute rates, and that a report with no client-measured
// result sums every node (symmetric multi-node clients).
func TestMergeResultsMeasurementMode(t *testing.T) {
	result := func(mode benchkit.MeasurementMode, throughput float64) *benchkit.Result {
		return benchkit.Result_builder{
			Config: benchkit.RunConfig_builder{
				Name: "Q", MeasurementMode: mode,
			}.Build(),
			Throughput: throughput,
		}.Build()
	}
	tests := []struct {
		name    string
		results []*benchkit.Result
		want    float64
	}{
		{
			name: "MixedSumsClientOnly",
			results: []*benchkit.Result{
				result(benchkit.MeasurementMode_CLIENT_MEASURED, 5000),
				result(benchkit.MeasurementMode_SERVER_MEASURED, 50000),
				result(benchkit.MeasurementMode_SERVER_MEASURED, 50000),
			},
			want: 5000,
		},
		{
			name: "ServerOnlySumsAll",
			results: []*benchkit.Result{
				result(benchkit.MeasurementMode_SERVER_MEASURED, 5000),
				result(benchkit.MeasurementMode_SERVER_MEASURED, 6000),
			},
			want: 11000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byBench := make(map[string]*benchSummary)
			mergeResults(benchkit.Report_builder{Results: tt.results}.Build(), byBench, 0)
			s := byBench["Q"]
			if s == nil {
				t.Fatal("no summary for Q")
			}
			if s.throughput != tt.want {
				t.Errorf("throughput = %v, want %v", s.throughput, tt.want)
			}
			if s.nodes != len(tt.results) {
				t.Errorf("nodes = %d, want %d", s.nodes, len(tt.results))
			}
		})
	}
}

// TestMergeResultsHDRHistograms verifies that the histograms of nodes without
// raw samples (HDR mode) merge by value across the reports of a run, and that
// the merged distribution yields the cluster-wide mean, stddev, and
// percentiles.
func TestMergeResultsHDRHistograms(t *testing.T) {
	node := func(values []int64, counts []uint64) *benchkit.Report {
		return benchkit.Report_builder{
			Results: []*benchkit.Result{
				benchkit.Result_builder{
					Config: benchkit.RunConfig_builder{
						Name:      "Q",
						StatsMode: benchkit.StatsMode_HDR,
					}.Build(),
					Histogram: benchkit.LatencyHistogram_builder{Value: values, Count: counts}.Build(),
				}.Build(),
			},
		}.Build()
	}
	byBench := make(map[string]*benchSummary)
	mergeResults(node([]int64{100, 200}, []uint64{5, 10}), byBench, 0)
	mergeResults(node([]int64{200, 400}, []uint64{10, 15}), byBench, 0)

	s := byBench["Q"]
	if s == nil {
		t.Fatal("no summary for Q")
	}
	// Merged weights: 100×5, 200×20, 400×15.
	if got := s.latency.Count(); got != 40 {
		t.Errorf("merged samples = %d, want 40", got)
	}
	// Weighted mean: (100·5 + 200·20 + 400·15) / 40 = 262.5.
	mean, stddev := s.latency.MeanAndStdDev()
	if mean != 262.5 {
		t.Errorf("mean = %v, want 262.5", mean)
	}
	// Population variance: (5·162.5² + 20·62.5² + 15·137.5²) / 40.
	wantSD := math.Sqrt((5*162.5*162.5 + 20*62.5*62.5 + 15*137.5*137.5) / 40)
	if math.Abs(stddev-wantSD) > 1e-9 {
		t.Errorf("stddev = %v, want %v", stddev, wantSD)
	}
	// p50: the 20th of 40 samples is 200; p95: the 38th is 400.
	qs := s.latency.Quantiles(0.50, 0.95)
	if qs[0] != 200 || qs[1] != 400 {
		t.Errorf("quantiles = %v, want [200 400]", qs)
	}
}
