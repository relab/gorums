package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/relab/gorums/benchkit"
)

func tlRec(payload, workers int, p50, p99 float64) aggRunRecord {
	return aggRunRecord{
		Dimensions: benchkit.Dimensions{
			Benchmark: "Q", Nodes: 3, Workers: workers, Payload: payload, StreamMode: "dual",
		},
		throughput: aggStat{mean: float64(workers) * 1000, n: 2},
		p50US:      aggStat{mean: p50, sd: 1, n: 2},
		p95US:      aggStat{mean: p50 * 1.5, n: 2},
		p99US:      aggStat{mean: p99, n: 2},
	}
}

func TestTLCurveGroups(t *testing.T) {
	t.Run("SplitWideRange", func(t *testing.T) {
		// peak p99 900 (1024) vs 12000 (16384) -> 13x > tlSplitRatio -> two bands.
		agg := []aggRunRecord{
			tlRec(1024, 2, 200, 500), tlRec(1024, 4, 300, 900),
			tlRec(16384, 2, 2000, 9000), tlRec(16384, 4, 3000, 12000),
		}
		gp := map[int]int{}
		for _, r := range tlCurveRows(agg, []string{"workers"}) {
			gp[r.Payload] = r.group
		}
		if gp[1024] != 1 || gp[16384] != 2 {
			t.Errorf("groups = %v, want 1024->1, 16384->2", gp)
		}
	})

	t.Run("SingleBandNarrowRange", func(t *testing.T) {
		// 900 vs 3000 -> 3.3x <= ratio -> one band.
		agg := []aggRunRecord{
			tlRec(1024, 2, 200, 500), tlRec(1024, 4, 300, 900),
			tlRec(16384, 2, 800, 2000), tlRec(16384, 4, 1200, 3000),
		}
		for _, r := range tlCurveRows(agg, []string{"workers"}) {
			if r.group != 1 {
				t.Errorf("payload %d group = %d, want 1", r.Payload, r.group)
			}
		}
	})

	t.Run("SplitByBufferCapacity", func(t *testing.T) {
		// Same payload and rate, two send-buffer arms with a wide p99 gap
		// (bufferbloat: a large buffer trades latency for throughput) ->
		// 16x > tlSplitRatio. loadScaleDimensions must include the buffer
		// capacities in the load-scale identity, or both arms share one peak
		// and never split regardless of the gap between them.
		agg := []aggRunRecord{
			{
				Dimensions: benchkit.Dimensions{
					Benchmark: "Q", Nodes: 3, Workers: 4, Payload: 1024, Rate: 5000,
					SendBuffer: 0, StreamMode: "dual",
				},
				throughput: aggStat{mean: 4000, n: 2},
				p50US:      aggStat{mean: 200, sd: 1, n: 2},
				p95US:      aggStat{mean: 300, n: 2},
				p99US:      aggStat{mean: 500, n: 2},
			},
			{
				Dimensions: benchkit.Dimensions{
					Benchmark: "Q", Nodes: 3, Workers: 4, Payload: 1024, Rate: 5000,
					SendBuffer: 65536, StreamMode: "dual",
				},
				throughput: aggStat{mean: 4000, n: 2},
				p50US:      aggStat{mean: 4000, sd: 1, n: 2},
				p95US:      aggStat{mean: 6000, n: 2},
				p99US:      aggStat{mean: 8000, n: 2},
			},
		}
		gp := map[int]int{}
		for _, r := range tlCurveRows(agg, []string{"workers"}) {
			gp[r.SendBuffer] = r.group
		}
		if gp[0] == gp[65536] {
			t.Errorf("groups = %v, want distinct bands for the two send-buffer arms", gp)
		}
	})

	t.Run("DropsLatencylessRows", func(t *testing.T) {
		agg := []aggRunRecord{
			tlRec(1024, 2, 200, 500),
			{Dimensions: benchkit.Dimensions{
				Benchmark: "Q", Nodes: 3, Workers: 4, Payload: 1024, StreamMode: "dual",
			},
				throughput: aggStat{mean: 4000, n: 2}}, // no latency
		}
		rows := tlCurveRows(agg, []string{"workers"})
		if len(rows) != 1 {
			t.Fatalf("rows = %d, want 1 (latencyless dropped)", len(rows))
		}
	})
}

func TestWriteTLCurveCSV(t *testing.T) {
	rows := tlCurveRows([]aggRunRecord{tlRec(1024, 2, 200, 500), tlRec(1024, 4, 300, 900)}, []string{"workers"})
	path := filepath.Join(t.TempDir(), "tl_curve.csv")
	if err := writeTLCurveCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("rows = %d incl header, want 3", len(recs))
	}
	// workers=2 -> throughput 2000 ops/s -> 2 kops/s.
	if recs[1][slices.Index(recs[0], "throughput_kops")] != "2" {
		t.Errorf("throughput_kops = %q, want 2", recs[1][slices.Index(recs[0], "throughput_kops")])
	}
}
