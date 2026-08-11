package main

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/relab/gorums/benchkit"
)

// TestFindDegradedNodes verifies the median-relative degradation checks: a node
// is flagged when its throughput falls below the given fraction of the run
// median, when it exceeds the given multiple of it (a symmetric benchmark cannot
// produce that, so the node recorded work it never did), or when its median
// latency is a fraction of its peers' (too fast for the round trip the benchmark
// measures). Healthy skew (±30% around the median is normal on real clusters) is
// never flagged at the default bounds, and a non-positive bound disables its
// check.
func TestFindDegradedNodes(t *testing.T) {
	defaults := degradationBounds{below: 0.5, above: 2, latencyBelow: 0.2}
	tputs := func(values map[string]float64) map[string]nodeMeasurement {
		nodes := make(map[string]nodeMeasurement, len(values))
		for host, tput := range values {
			nodes[host] = nodeMeasurement{throughput: tput}
		}
		return nodes
	}
	tests := []struct {
		name       string
		nodes      map[string]nodeMeasurement
		bounds     degradationBounds
		want       []string  // flagged hosts, most extreme first
		wantRel    []float64 // relative_to_median per flagged host
		wantReason []string  // reason per flagged host
	}{
		{
			name: "healthy skew not flagged",
			nodes: tputs(map[string]float64{
				"bb2:9000": 4014, "bb3:9000": 5020, "bb4:9000": 5968,
				"bb5:9000": 6512, "bb6:9000": 7248,
			}),
			bounds: defaults,
		},
		{
			name: "pathological node flagged",
			nodes: tputs(map[string]float64{
				"bb2:9000": 5000, "bb3:9000": 5500, "bb4:9000": 6000,
				"bb5:9000": 5200, "bb16:9000": 233,
			}),
			bounds:     defaults,
			want:       []string{"bb16:9000"},
			wantRel:    []float64{233.0 / 5200.0},
			wantReason: []string{degradedSlow},
		},
		{
			name: "two degraded nodes sorted most extreme first",
			nodes: tputs(map[string]float64{
				"bb2:9000": 5000, "bb3:9000": 5000, "bb4:9000": 5000,
				"bb16:9000": 250, "bb24:9000": 1000,
			}),
			bounds:     defaults,
			want:       []string{"bb16:9000", "bb24:9000"},
			wantRel:    []float64{0.05, 0.2},
			wantReason: []string{degradedSlow, degradedSlow},
		},
		{
			// The run-over case: a node that kept recording operations after its
			// peers finished reported 116x the run median.
			name: "node far above the median flagged",
			nodes: tputs(map[string]float64{
				"bb2:9000": 5000, "bb3:9000": 5500, "bb4:9000": 4500,
				"bb9:9000": 517786,
			}),
			bounds:     defaults,
			want:       []string{"bb9:9000"},
			wantRel:    []float64{517786.0 / 5250.0}, // median of 4500, 5000, 5500, 517786
			wantReason: []string{degradedExcessThroughput},
		},
		{
			name: "implausibly fast node flagged on latency alone",
			nodes: map[string]nodeMeasurement{
				"bb2:9000": {throughput: 5000, p50US: 1400},
				"bb3:9000": {throughput: 5200, p50US: 1450},
				"bb4:9000": {throughput: 5100, p50US: 1400},
				"bb9:9000": {throughput: 7400, p50US: 7.4},
			},
			bounds:     defaults,
			want:       []string{"bb9:9000"},
			wantRel:    []float64{7.4 / 1400.0},
			wantReason: []string{degradedFastLatency},
		},
		{
			name: "healthy latency skew not flagged",
			nodes: map[string]nodeMeasurement{
				"bb2:9000": {throughput: 5000, p50US: 1400},
				"bb3:9000": {throughput: 5200, p50US: 900},
				"bb4:9000": {throughput: 5100, p50US: 1900},
			},
			bounds: defaults,
		},
		{
			name: "nodes without latency data judged on throughput only",
			nodes: map[string]nodeMeasurement{
				"bb2:9000": {throughput: 5000},
				"bb3:9000": {throughput: 5200},
				"bb4:9000": {throughput: 5100},
			},
			bounds: defaults,
		},
		{
			name:   "exactly at threshold not flagged",
			nodes:  tputs(map[string]float64{"bb2:9000": 1000, "bb3:9000": 1000, "bb4:9000": 500}),
			bounds: defaults,
		},
		{
			name:   "exactly at upper threshold not flagged",
			nodes:  tputs(map[string]float64{"bb2:9000": 1000, "bb3:9000": 1000, "bb4:9000": 2000}),
			bounds: defaults,
		},
		{
			name:   "disabled bounds",
			nodes:  tputs(map[string]float64{"bb2:9000": 5000, "bb16:9000": 1}),
			bounds: degradationBounds{},
		},
		{
			name:   "upper bound alone",
			nodes:  tputs(map[string]float64{"bb2:9000": 5000, "bb3:9000": 5000, "bb16:9000": 1}),
			bounds: degradationBounds{above: 2},
		},
		{
			name:   "single node never flagged",
			nodes:  tputs(map[string]float64{"bb2:9000": 5000}),
			bounds: defaults,
		},
		{
			name:   "zero median disables check",
			nodes:  tputs(map[string]float64{"bb2:9000": 0, "bb3:9000": 0, "bb4:9000": 0}),
			bounds: defaults,
		},
		{
			name:   "empty",
			nodes:  map[string]nodeMeasurement{},
			bounds: defaults,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findDegradedNodes(tt.nodes, tt.bounds)
			if len(got) != len(tt.want) {
				t.Fatalf("flagged %d node(s) %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i, d := range got {
				if d.Host != tt.want[i] {
					t.Errorf("flagged[%d].Host = %q, want %q", i, d.Host, tt.want[i])
				}
				if math.Abs(d.Relative-tt.wantRel[i]) > 1e-9 {
					t.Errorf("flagged[%d].Relative = %v, want %v", i, d.Relative, tt.wantRel[i])
				}
				if d.Reason != tt.wantReason[i] {
					t.Errorf("flagged[%d].Reason = %q, want %q", i, d.Reason, tt.wantReason[i])
				}
				if want := tt.nodes[d.Host].throughput; d.Throughput != want {
					t.Errorf("flagged[%d].Throughput = %v, want %v", i, d.Throughput, want)
				}
			}
		})
	}
}

// TestCollectNodeMeasurements verifies that each node's throughput and median
// latency are read from the collected result files keyed by host label, and that
// missing files are skipped (coverage is countResultFiles's job, not this one's).
func TestCollectNodeMeasurements(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N3_W1_P0"
	nodes := []nodeAssignment{
		{host: "bb2", port: 9000},
		{host: "bb3", port: 9000},
		{host: "bb4", port: 9000}, // no result file written
	}
	writePlotReport(t, dir, base, nodes[0], "bb2:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 3, 1, 0, 0),
		Throughput: 5000,
		Latencies:  []int64{1_000_000, 2_000_000, 3_000_000},
	}.Build())
	writePlotReport(t, dir, base, nodes[1], "bb3:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 3, 1, 0, 0),
		Throughput: 233,
		// No latency samples: p50 stays 0, which the latency bound skips.
	}.Build())

	got := collectNodeMeasurements(dir, base, nodes, 0)
	want := map[string]nodeMeasurement{
		"bb2:9000": {throughput: 5000, p50US: 2000},
		"bb3:9000": {throughput: 233},
	}
	if len(got) != len(want) {
		t.Fatalf("measurements = %v, want %v", got, want)
	}
	for host, m := range want {
		if got[host] != m {
			t.Errorf("measurement[%q] = %+v, want %+v", host, got[host], m)
		}
	}
}

// TestUpdateManifestOutcomeDegraded verifies that a degraded outcome records
// the degraded status and the flagged nodes with their relative throughput in
// the manifest.
func TestUpdateManifestOutcomeDegraded(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N5_W1_P0"
	nodes := []nodeAssignment{{host: "bb2", port: 9000}, {host: "bb16", port: 9000}}
	cfg := &config{sweepLabel: "e1", duration: time.Second}
	writeManifest(dir, base, runSpec{
		Dimensions: benchkit.Dimensions{Benchmark: "Q", Nodes: 5, Workers: 1},
		Rep:        1,
	}, nodes, cfg, "", "")

	deg := []degradedNode{{Host: "bb16:9000", Throughput: 233, Relative: 0.045}}
	tcp := map[string]map[string]uint64{"bb16": {"TcpExt.TCPTimeouts": 4900}}
	err := updateManifestOutcome(dir, base, runOutcome{
		status: runStatusDegraded, collectedFiles: 2, degraded: deg, tcpStats: tcp,
	})
	if err != nil {
		t.Fatalf("updateManifestOutcome: %v", err)
	}

	data, err := os.ReadFile(manifestPath(dir, base))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m runManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if m.Status != runStatusDegraded {
		t.Errorf("Status = %q, want %q", m.Status, runStatusDegraded)
	}
	if len(m.DegradedNodes) != 1 || m.DegradedNodes[0].Host != "bb16:9000" {
		t.Fatalf("DegradedNodes = %+v, want bb16:9000", m.DegradedNodes)
	}
	if m.DegradedNodes[0].Relative != 0.045 {
		t.Errorf("Relative = %v, want 0.045", m.DegradedNodes[0].Relative)
	}
	if m.TCPStats["bb16"]["TcpExt.TCPTimeouts"] != 4900 {
		t.Errorf("TCPStats = %v, want bb16 TCPTimeouts=4900", m.TCPStats)
	}
}
