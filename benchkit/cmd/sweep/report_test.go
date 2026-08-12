package main

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/relab/gorums/benchkit"
)

func TestFacetFor(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		xcol   string
		want   string
	}{
		{"single-other-varies", map[string]int{"nodes": 1, "workers": 3, "payload": 1, "rate": 1}, "workers", ""},
		{"two-others-vary-fewest", map[string]int{"nodes": 2, "workers": 3, "payload": 5, "rate": 1}, "workers", "nodes"},
		{"x-excluded", map[string]int{"nodes": 4, "workers": 3, "payload": 2, "rate": 1}, "nodes", "payload"},
		{"none-vary", map[string]int{"nodes": 1, "workers": 3, "payload": 1, "rate": 1}, "payload", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := facetFor(tt.counts, tt.xcol); got != tt.want {
				t.Errorf("facetFor(%v, %q) = %q, want %q", tt.counts, tt.xcol, got, tt.want)
			}
		})
	}
}

// TestPlanFiguresBufferDimensions verifies that a swept buffer capacity is
// plotted like any other dimension, and that a buffer the sweep did not vary
// plans no figure. The unset marker is a constant, so it must not read as a
// varying dimension.
func TestPlanFiguresBufferDimensions(t *testing.T) {
	tests := []struct {
		name        string
		sendBuffers []int
		recvBuffers []int
		want        []string
		notWant     []string
	}{
		{
			name:        "SendBufferVaries",
			sendBuffers: []int{64, 256, 1024},
			recvBuffers: []int{0},
			want:        []string{"throughput_vs_send_buffer", "latency_vs_send_buffer"},
			notWant:     []string{"throughput_vs_recv_buffer"},
		},
		{
			name:        "RecvBufferVaries",
			sendBuffers: []int{0},
			recvBuffers: []int{0, 16},
			want:        []string{"throughput_vs_recv_buffer", "latency_vs_recv_buffer"},
			notWant:     []string{"throughput_vs_send_buffer"},
		},
		{
			name:        "NeitherVaries",
			sendBuffers: []int{0},
			recvBuffers: []int{0},
			notWant:     []string{"throughput_vs_send_buffer", "throughput_vs_recv_buffer"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var agg []aggRunRecord
			for _, sb := range tt.sendBuffers {
				for _, rb := range tt.recvBuffers {
					agg = append(agg, aggRunRecord{
						Dimensions: benchkit.Dimensions{
							Benchmark: "Q", Nodes: 3, Workers: 1,
							SendBuffer: sb, RecvBuffer: rb, StreamMode: "dual",
						},
						reps: 3, throughput: aggStat{mean: 1000, n: 3},
						p50US: aggStat{mean: 1000, n: 3},
					})
				}
			}
			slugs := map[string]bool{}
			for _, s := range planFigures(agg, reportInputs{}) {
				slugs[s.slug] = true
			}
			for _, want := range tt.want {
				if !slugs[want] {
					t.Errorf("missing planned figure %q; got %v", want, slices.Sorted(maps.Keys(slugs)))
				}
			}
			for _, bad := range tt.notWant {
				if slugs[bad] {
					t.Errorf("unexpected figure %q planned for a buffer that does not vary", bad)
				}
			}
		})
	}
}

// TestPlanFiguresOmitsLatencyWithoutData verifies that a throughput-only
// sweep (no run recorded a latency sample or histogram) plans no
// latency_vs_* figure, matching the goodput figure's existing Payload > 0
// data-presence guard, instead of rendering a heading over empty panels.
func TestPlanFiguresOmitsLatencyWithoutData(t *testing.T) {
	var agg []aggRunRecord
	for _, w := range []int{2, 4, 8} {
		agg = append(agg, aggRunRecord{
			Dimensions: benchkit.Dimensions{Benchmark: "M", Nodes: 3, Workers: w, StreamMode: "dual"},
			reps:       3, throughput: aggStat{mean: 1000, n: 3},
			// No p50US/p95US/p99US set: a server-measured throughput-only run.
		})
	}
	slugs := map[string]bool{}
	for _, s := range planFigures(agg, reportInputs{}) {
		slugs[s.slug] = true
	}
	if !slugs["throughput_vs_workers"] {
		t.Error("missing throughput_vs_workers")
	}
	if slugs["latency_vs_workers"] {
		t.Error("latency_vs_workers planned with no latency data in any run")
	}
}

func TestPlanFigures(t *testing.T) {
	// workers and payload vary; nodes and rate fixed.
	var agg []aggRunRecord
	for _, w := range []int{2, 4, 8} {
		for _, p := range []int{1024, 16384} {
			for _, m := range []string{"dual", "dedup"} {
				agg = append(agg, aggRunRecord{
					Dimensions: benchkit.Dimensions{
						Benchmark: "Q", Nodes: 3, Workers: w, Payload: p, StreamMode: m,
					},
					reps: 3, throughput: aggStat{mean: 1000, n: 3},
					p50US: aggStat{mean: 1000, n: 3},
				})
			}
		}
	}
	specs := planFigures(agg, reportInputs{})
	slugs := map[string]figureSpec{}
	for _, s := range specs {
		slugs[s.slug] = s
	}

	// throughput/latency vs workers and vs payload; goodput vs payload; no
	// nodes/rate figures (they don't vary); no goodput_vs_nodes (nodes fixed).
	for _, want := range []string{
		"throughput_vs_workers", "latency_vs_workers",
		"throughput_vs_payload", "latency_vs_payload", "goodput_vs_payload",
	} {
		if _, ok := slugs[want]; !ok {
			t.Errorf("missing planned figure %q; got %v", want, slices.Sorted(maps.Keys(slugs)))
		}
	}
	for _, bad := range []string{
		"throughput_vs_nodes", "throughput_vs_rate", "goodput_vs_nodes", "mem_per_op_vs_nodes",
	} {
		if _, ok := slugs[bad]; ok {
			t.Errorf("unexpected figure %q planned", bad)
		}
	}
	// With exactly two varying dims (workers, payload), each figure facets by
	// the fewest-valued other dim. For x=workers the only other varying dim is
	// payload -> single varying -> no facet.
	if f := slugs["throughput_vs_workers"].facet; f != "" {
		t.Errorf("throughput_vs_workers facet = %q, want none", f)
	}
	if !slugs["goodput_vs_payload"].payloadPositive {
		t.Error("goodput figure should restrict to payload>0")
	}
}

// TestPlanFiguresRatioRequiresVaryingWorkers verifies that a comparison ratio
// figure is planned per varying dimension, mirroring the metric-vs loop: a
// dimension gets a ratio figure only while it varies, since ratio-vs plots
// against that dimension on a fixed x-axis and without a varying x every
// series collapses to one point and nothing is drawn but the parity line. A
// dual-vs-dedup comparison swept over nodes at fixed workers must plan the
// nodes-vs figure (not a workers one), and vice versa.
func TestPlanFiguresRatioRequiresVaryingWorkers(t *testing.T) {
	rec := func(nodes, workers int, mode string) aggRunRecord {
		return aggRunRecord{
			Dimensions: benchkit.Dimensions{
				Benchmark: "Q", Nodes: nodes, Workers: workers, StreamMode: mode,
			},
			reps: 3, throughput: aggStat{mean: 1000, n: 3},
			p50US: aggStat{mean: 1000, n: 3},
		}
	}

	t.Run("WorkersFixedNodesVary", func(t *testing.T) {
		var agg []aggRunRecord
		for _, n := range []int{3, 9} {
			for _, m := range []string{"dual", "dedup"} {
				agg = append(agg, rec(n, 8, m))
			}
		}
		slugs := map[string]bool{}
		for _, s := range planFigures(agg, reportInputs{comparison: pivotComparison(agg, "dual")}) {
			slugs[s.slug] = true
		}
		for _, unwanted := range []string{"throughput_ratio_vs_workers", "latency_ratio_vs_workers"} {
			if slugs[unwanted] {
				t.Errorf("unexpected figure %q planned with workers fixed", unwanted)
			}
		}
		for _, want := range []string{"throughput_ratio_vs_nodes", "latency_ratio_vs_nodes"} {
			if !slugs[want] {
				t.Errorf("missing planned figure %q; got %v", want, slices.Sorted(maps.Keys(slugs)))
			}
		}
	})

	t.Run("WorkersVary", func(t *testing.T) {
		var agg []aggRunRecord
		for _, w := range []int{2, 8} {
			for _, m := range []string{"dual", "dedup"} {
				agg = append(agg, rec(3, w, m))
			}
		}
		slugs := map[string]bool{}
		for _, s := range planFigures(agg, reportInputs{comparison: pivotComparison(agg, "dual")}) {
			slugs[s.slug] = true
		}
		for _, want := range []string{"throughput_ratio_vs_workers", "latency_ratio_vs_workers"} {
			if !slugs[want] {
				t.Errorf("missing planned figure %q; got %v", want, slices.Sorted(maps.Keys(slugs)))
			}
		}
	})
}

// TestPlanFiguresRatioMetricAware verifies that ratio-figure planning consults
// the paired comparison rows per metric and per x-dimension, instead of one
// sweep-wide "some ratio is computable" boolean: a throughput-only comparison
// must plan no latency-ratio figure, and a dimension that varies across the
// sweep but not within any comparable series must plan no ratio figure at all,
// since ratio-vs would draw nothing but the parity line.
func TestPlanFiguresRatioMetricAware(t *testing.T) {
	rec := func(nodes, workers int, mode string, latency bool) aggRunRecord {
		r := aggRunRecord{
			Dimensions: benchkit.Dimensions{
				Benchmark: "Q", Nodes: nodes, Workers: workers, StreamMode: mode,
			},
			reps: 3, throughput: aggStat{mean: 1000, n: 3},
		}
		if latency {
			r.p50US = aggStat{mean: 1000, n: 3}
		}
		return r
	}

	tests := []struct {
		name     string
		agg      []aggRunRecord
		want     []string
		unwanted []string
	}{
		{
			name: "throughput only",
			agg: []aggRunRecord{
				rec(3, 2, "dual", false), rec(3, 2, "dedup", false),
				rec(3, 8, "dual", false), rec(3, 8, "dedup", false),
			},
			want:     []string{"throughput_ratio_vs_workers"},
			unwanted: []string{"latency_ratio_vs_workers"},
		},
		{
			// nodes varies across the sweep, but only N=3 has both modes, so a
			// nodes-axis series holds a single ratio point.
			name: "x varies only outside the paired rows",
			agg: []aggRunRecord{
				rec(3, 2, "dual", true), rec(3, 2, "dedup", true),
				rec(3, 8, "dual", true), rec(3, 8, "dedup", true),
				rec(9, 2, "dual", true), rec(9, 8, "dual", true),
			},
			want: []string{"throughput_ratio_vs_workers", "latency_ratio_vs_workers"},
			unwanted: []string{
				"throughput_ratio_vs_nodes", "latency_ratio_vs_nodes",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			slugs := map[string]bool{}
			for _, s := range planFigures(test.agg, reportInputs{comparison: pivotComparison(test.agg, "dual")}) {
				slugs[s.slug] = true
			}
			for _, want := range test.want {
				if !slugs[want] {
					t.Errorf("missing planned figure %q; got %v", want, slices.Sorted(maps.Keys(slugs)))
				}
			}
			for _, unwanted := range test.unwanted {
				if slugs[unwanted] {
					t.Errorf("unexpected figure %q planned", unwanted)
				}
			}
		})
	}
}

// TestMetricVsCallUsesHumanFacetLabel verifies that metricVsCall passes the
// facet dimension's human-readable dimLabel, not the raw CSV column name, so
// a facet panel titled "send_buffer = 4096" instead reads "Send queue
// capacity (requests) = 4096". Passing none for facet must also pass none
// for facet-label rather than an empty label.
func TestMetricVsCallUsesHumanFacetLabel(t *testing.T) {
	withFacet := metricVsCall(figureSpec{
		xcol: "workers", ycol: "throughput", bandCol: "throughput_ci95",
		facet: "send_buffer",
	})
	if !strings.Contains(withFacet, `facet-label: "Send queue capacity (requests)"`) {
		t.Errorf("metricVsCall missing human facet-label:\n%s", withFacet)
	}
	if strings.Contains(withFacet, `facet-label: "send_buffer"`) {
		t.Errorf("metricVsCall used the raw column name as facet-label:\n%s", withFacet)
	}

	withoutFacet := metricVsCall(figureSpec{xcol: "workers", ycol: "throughput", bandCol: "throughput_ci95"})
	if !strings.Contains(withoutFacet, "facet-label: none") {
		t.Errorf("metricVsCall without a facet should pass facet-label: none:\n%s", withoutFacet)
	}
}

// TestPlanFiguresTLCurveLoadDimensions verifies that a throughput-latency curve
// is planned for whichever load dimension the sweep varied: a rate sweep at a
// fixed worker count traces the curve along the rate, which used to plan no
// curve at all, and a sweep varying both gets one figure per dimension.
func TestPlanFiguresTLCurveLoadDimensions(t *testing.T) {
	rec := func(workers, rate int) aggRunRecord {
		return aggRunRecord{
			Dimensions: benchkit.Dimensions{
				Benchmark: "Q", Nodes: 3, Workers: workers, Rate: rate, StreamMode: "dual",
			},
			reps:       3,
			throughput: aggStat{mean: float64(rate), n: 3},
			p50US:      aggStat{mean: float64(rate) / 10, n: 3},
			p95US:      aggStat{mean: float64(rate) / 8, n: 3},
			p99US:      aggStat{mean: float64(rate) / 5, n: 3},
		}
	}
	tests := []struct {
		name string
		agg  []aggRunRecord
		want map[string]string // figure slug -> load dimension
	}{
		{
			name: "rate varies at fixed workers",
			agg:  []aggRunRecord{rec(32, 1000), rec(32, 2000), rec(32, 3000)},
			want: map[string]string{"tl_curve_rate": "rate"},
		},
		{
			name: "workers varies at fixed rate",
			agg:  []aggRunRecord{rec(2, 0), rec(4, 0), rec(8, 0)},
			want: map[string]string{"tl_curve_workers": "workers"},
		},
		{
			name: "both vary",
			agg: []aggRunRecord{
				rec(2, 1000), rec(4, 1000), rec(2, 2000), rec(4, 2000),
			},
			want: map[string]string{"tl_curve_workers": "workers", "tl_curve_rate": "rate"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loads := map[string]string{}
			for _, s := range planFigures(test.agg, reportInputs{}) {
				if s.kind == kindTLCurve {
					loads[s.slug] = s.load
				}
			}
			if !maps.Equal(loads, test.want) {
				t.Errorf("tl-curve figures = %v, want %v", loads, test.want)
			}
		})
	}
}

// TestFigureSubject verifies that the identity every figure shares — the single
// benchmark, and the stream mode when the sweep compared only one — is reported
// for the section headings to carry, and that a dimension the sweep varied is
// not, since the legends distinguish series by it.
func TestFigureSubject(t *testing.T) {
	rec := func(bench, mode string) aggRunRecord {
		return aggRunRecord{Dimensions: benchkit.Dimensions{Benchmark: bench, Nodes: 3, StreamMode: mode}}
	}
	tests := []struct {
		name string
		agg  []aggRunRecord
		want string
	}{
		{"one benchmark, one mode", []aggRunRecord{rec("Q", "dual")}, "Q, dual"},
		{"one benchmark, two modes", []aggRunRecord{rec("Q", "dual"), rec("Q", "dedup")}, "Q"},
		{"two benchmarks, one mode", []aggRunRecord{rec("Q", "dual"), rec("M", "dual")}, "dual"},
		{"both vary", []aggRunRecord{rec("Q", "dual"), rec("M", "dedup")}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := figureSubject(test.agg); got != test.want {
				t.Errorf("figureSubject = %q, want %q", got, test.want)
			}
		})
	}
}

// TestExperimentSummary verifies the line printed under the report title: it
// names the sweep, every dimension with the values it took (numeric ones in
// ascending order, unset ones left out), and the sweep-wide settings, so no
// figure heading has to repeat the fixed configuration.
func TestExperimentSummary(t *testing.T) {
	var agg []aggRunRecord
	for _, n := range []int{15, 9} {
		for _, mode := range []string{"dual", "dedup"} {
			agg = append(agg, aggRunRecord{Dimensions: benchkit.Dimensions{
				Benchmark: "Q", Nodes: n, Workers: 32, StreamMode: mode,
			}})
		}
	}
	got := experimentSummary(agg, sweepSettings{
		label: "eval-v1", duration: "20s", trim: "2s", runs: 40,
	})
	want := "eval-v1; Q; nodes 9, 15; workers 32; stream mode dedup, dual; " +
		"4 configurations, 40 runs; 20s per run, 2s trim"
	if got != want {
		t.Errorf("experimentSummary =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "rate") || strings.Contains(got, "buffer") {
		t.Errorf("experimentSummary names a dimension the sweep left unset: %q", got)
	}
}

func TestWriteReportTyp(t *testing.T) {
	specs := []figureSpec{
		{
			slug: "throughput_vs_workers", section: scalingSection, heading: "Aggregate #throughput [raw]",
			dataCSV: "agg.csv", xcol: "workers", ycol: "throughput", bandCol: "throughput_ci95",
			ylabel: "kops/s", yscale: 1.0 / 1e3, facet: "payload",
		},
		{
			kind: kindRunStatus, slug: "run_status", section: healthSection,
			heading: "Run outcomes", dataCSV: "run_status.csv",
		},
	}
	path := filepath.Join(t.TempDir(), "report.typ")
	if err := writeReportTyp(path, reportHeader{title: "Test #report [raw]", experiment: "Q; nodes 3"}, specs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		`#import "gorumsplot.typ": *`,
		`= #text("Test #report [raw]")`,
		`#align(center)[#emph[#text("Q; nodes 3")]]`,
		// Sections head the figures, which sit one level deeper.
		`== #text("Scaling")`,
		`=== #text("Aggregate #throughput [raw]")`,
		`== #text("Cluster health")`,
		`=== #text("Run outcomes")`,
		// The second section opens a page; the first stays with the title.
		"#pagebreak(weak: true)\n== #text(\"Cluster health\")",
		`csv("agg.csv"`,
		`metric-vs(agg, xcol: "workers", ycol: "throughput", band-col: "throughput_ci95"`,
		`facet: "payload"`,
		`#run-status-table(st)`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("report.typ missing %q\n---\n%s", want, src)
		}
	}
	if strings.Contains(src, "#pagebreak(weak: true)\n== #text(\"Scaling\")") {
		t.Errorf("the first section should not open a page of its own\n---\n%s", src)
	}
	if note := sectionNotes[scalingSection]; !strings.Contains(src, note) {
		t.Errorf("report.typ missing the %q section note\n---\n%s", scalingSection, src)
	}
	if strings.Contains(src, "#fitwidth(run-status-table") {
		t.Errorf("run-status table should keep its intrinsic width\n---\n%s", src)
	}
}

// TestGenerateReportCompiles writes a full agg.csv from synthetic records,
// generates report.typ + the helper lib, and compiles it with Typst. It is
// skipped when the typst binary is not on PATH, so it stays a no-op in CI
// environments without Typst while catching template/CSV breakage locally.
func TestGenerateReportCompiles(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; skipping report compile check")
	}
	var runs []plotRunRecord
	for _, n := range []int{3, 9} {
		for _, w := range []int{2, 4, 8, 16} {
			for _, p := range []int{0, 1024, 16384} {
				for _, sendBuffer := range []int{0, 256} {
					for _, m := range []string{"dual", "dedup"} {
						lat := 500.0 + float64(w*50) + float64(p)/100
						thr := float64(w) * 5000 * float64(n) / 3
						// Two reps so spread columns are populated.
						for _, jitter := range []float64{0.98, 1.02} {
							runs = append(runs, plotRunRecord{
								Dimensions: benchkit.Dimensions{
									Benchmark: "Q", Nodes: n, Workers: w, Payload: p,
									SendBuffer: sendBuffer, StreamMode: m,
								},
								status:      runStatusSucceeded,
								throughput:  thr * jitter,
								allocsPerOp: 12, memPerOp: 2048,
								meanUS: new(lat), p50US: new(lat), p95US: new(lat * 1.5), p99US: new(lat * 2),
							})
						}
					}
				}
			}
		}
	}
	agg := aggregateReps(runs, false)

	dir := t.TempDir()
	if err := writeAggRunsCSV(filepath.Join(dir, "agg.csv"), agg); err != nil {
		t.Fatal(err)
	}
	if err := writeTLCurveCSV(filepath.Join(dir, "tl_curve.csv"), tlCurveRows(agg, []string{"workers"})); err != nil {
		t.Fatal(err)
	}
	// A minimal per-node CDF for one run so the CDF figure is exercised too.
	base := "run_Q_N3_W8_P1024"
	var cdfRows []plotNodeCDFRecord
	for _, node := range []string{"bb1:9000", "bb2:9000"} {
		for i := 0; i <= 20; i++ {
			prob := float64(i) / 20
			cdfRows = append(cdfRows, plotNodeCDFRecord{
				Dimensions: benchkit.Dimensions{
					Benchmark: "Q", Nodes: 3, Workers: 8, Payload: 1024, StreamMode: "dual",
				},
				base: base, label: "run", status: runStatusSucceeded, rep: 1,
				node: node, throughput: 70000, prob: prob, cdfUS: 200 + 800*prob,
			})
		}
	}
	if err := writePlotNodeCDFCSV(filepath.Join(dir, "node_cdf.csv"), cdfRows); err != nil {
		t.Fatal(err)
	}
	if err := copyReportLib(dir); err != nil {
		t.Fatal(err)
	}
	specs := planFigures(agg, reportInputs{cdfRuns: []cdfRun{{base: base, title: "N3 W8 P1024"}}})
	if len(specs) == 0 {
		t.Fatal("no figures planned")
	}
	// The full figure set should include a tl-curve and the per-node CDF.
	kinds := map[figureKind]bool{}
	for _, s := range specs {
		kinds[s.kind] = true
	}
	if !kinds[kindTLCurve] || !kinds[kindPerNodeCDF] {
		t.Errorf("expected tl-curve and per-node-cdf figures; kinds=%v", kinds)
	}
	if err := writeReportTyp(filepath.Join(dir, "report.typ"), reportHeader{title: "Compile test"}, specs); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("typst", "compile", "report.typ", "report.pdf")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.pdf")); err != nil {
		t.Fatalf("report.pdf not produced: %v", err)
	}
}

// TestTypstFiguresCompileWithEmptyData verifies the helper library's empty-data
// guards: every figure function called with no rows must render nothing instead
// of failing the compilation. The hazard is grid(columns: 0), which Typst
// rejects with "number must be positive", reachable through an empty legend or
// an empty panel list. It is skipped when the typst binary is not on PATH, like
// TestGenerateReportCompiles.
func TestTypstFiguresCompileWithEmptyData(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; skipping report compile check")
	}
	dir := t.TempDir()
	if err := copyReportLib(dir); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		`#import "gorumsplot.typ": *`,
		`#hlegend(())`,
		// An odd entry count over several rows leaves the last column ragged,
		// so the grid is handed an empty cell.
		`#hlegend(((red, "a"), (blue, "b"), (green, "c")), cols: 2)`,
		`#time-series((), (), sat: ())`,
		`#per-node-cdf((), ())`,
		`#per-node-cdf((), ((base: "no-such-base", title: "N3"),))`,
		`#heatmap(())`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "empty.typ"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("typst", "compile", "empty.typ", "empty.pdf")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile of empty figures failed: %v\n%s", err, out)
	}
}

// TestGenerateReportRateTLCurveCompiles verifies that a rate sweep at a fixed
// worker count — the shape of the paced dedup evaluations, which used to get no
// throughput-latency figure at all — plans a curve traced along the rate and
// renders it, with a nine-panel per-node CDF grid beside it. It is skipped when
// the typst binary is not on PATH, like TestGenerateReportCompiles.
func TestGenerateReportRateTLCurveCompiles(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; skipping report compile check")
	}
	var runs []plotRunRecord
	var cdfRows []plotNodeCDFRecord
	var cdfSet []cdfRun
	for _, n := range []int{3, 9, 27} {
		for _, rate := range []int{1000, 2000, 3000} {
			for _, mode := range []string{"dual", "dedup"} {
				// Latency rises with the offered rate, so the curve has shape.
				lat := 400.0 + float64(rate)/4
				thr := float64(rate) * float64(n)
				for _, jitter := range []float64{0.98, 1.02} {
					runs = append(runs, plotRunRecord{
						Dimensions: benchkit.Dimensions{
							Benchmark: "Q", Nodes: n, Workers: 32, Payload: 1024,
							Rate: rate, StreamMode: mode,
						},
						status:      runStatusSucceeded,
						throughput:  thr * jitter,
						allocsPerOp: 12, memPerOp: 2048,
						meanUS: new(lat), p50US: new(lat), p95US: new(lat * 1.5), p99US: new(lat * 2),
					})
				}
				base := fmt.Sprintf("run_Q_N%d_R%d_S%s", n, rate, mode)
				cdfSet = append(cdfSet, cdfRun{base: base, title: fmt.Sprintf("N%d R%d %s", n, rate, mode)})
				for _, node := range []string{"bb1:9000", "bb2:9000"} {
					for i := 0; i <= 20; i++ {
						prob := float64(i) / 20
						cdfRows = append(cdfRows, plotNodeCDFRecord{
							Dimensions: benchkit.Dimensions{
								Benchmark: "Q", Nodes: n, Workers: 32, Payload: 1024,
								Rate: rate, StreamMode: mode,
							},
							base: base, label: "run", status: runStatusSucceeded, rep: 1,
							node: node, throughput: thr, prob: prob, cdfUS: lat + 800*prob,
						})
					}
				}
			}
		}
	}
	agg := aggregateReps(runs, false)

	dir := t.TempDir()
	if err := writeAggRunsCSV(filepath.Join(dir, "agg.csv"), agg); err != nil {
		t.Fatal(err)
	}
	if err := writeTLCurveCSV(filepath.Join(dir, "tl_curve.csv"), tlCurveRows(agg, []string{"rate"})); err != nil {
		t.Fatal(err)
	}
	if err := writePlotNodeCDFCSV(filepath.Join(dir, "node_cdf.csv"), cdfRows); err != nil {
		t.Fatal(err)
	}
	if err := copyReportLib(dir); err != nil {
		t.Fatal(err)
	}

	specs := planFigures(agg, reportInputs{cdfRuns: cdfSet[:maxCDFRuns]})
	var tl figureSpec
	for _, s := range specs {
		if s.kind == kindTLCurve {
			tl = s
		}
		if s.slug == "tl_curve_workers" {
			t.Errorf("planned a workers-traced curve with the worker count fixed")
		}
	}
	if tl.load != "rate" {
		t.Fatalf("tl-curve load = %q, want %q", tl.load, "rate")
	}
	if err := writeReportTyp(filepath.Join(dir, "report.typ"),
		reportHeader{title: "Rate TL-curve compile test", experiment: experimentSummary(agg, sweepSettings{})},
		specs); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("typst", "compile", "report.typ", "report.pdf")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\n%s", err, out)
	}
}

// TestGenerateReportRatioVsNodesCompiles verifies the exact failure scenario
// from the ratio-figures follow-up: a dual-vs-dedup comparison swept over
// nodes at a fixed worker count must plan and render a ratio figure against
// nodes, not workers, and the render must show real (non-parity) points, not
// just the dashed 1.0 line. It is skipped when the typst binary is not on
// PATH, like TestGenerateReportCompiles.
func TestGenerateReportRatioVsNodesCompiles(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; skipping report compile check")
	}
	var runs []plotRunRecord
	for _, n := range []int{3, 9} {
		for _, m := range []string{"dual", "dedup"} {
			// dedup measurably faster than dual so the ratio isn't 1.0.
			thr := 5000.0 * float64(n)
			lat := 1000.0
			if m == "dedup" {
				thr *= 1.2
				lat *= 0.8
			}
			for _, jitter := range []float64{0.98, 1.02} {
				runs = append(runs, plotRunRecord{
					Dimensions: benchkit.Dimensions{
						Benchmark: "Q", Nodes: n, Workers: 8, StreamMode: m,
					},
					status:      runStatusSucceeded,
					throughput:  thr * jitter,
					allocsPerOp: 12, memPerOp: 2048,
					meanUS: new(lat), p50US: new(lat), p95US: new(lat * 1.5), p99US: new(lat * 2),
				})
			}
		}
	}
	agg := aggregateReps(runs, false)

	dir := t.TempDir()
	if err := writeAggRunsCSV(filepath.Join(dir, "agg.csv"), agg); err != nil {
		t.Fatal(err)
	}
	cmpRows := pivotComparison(agg, "dual")
	if err := writeComparisonCSV(filepath.Join(dir, "comparison.csv"), cmpRows); err != nil {
		t.Fatal(err)
	}
	if err := copyReportLib(dir); err != nil {
		t.Fatal(err)
	}

	specs := planFigures(agg, reportInputs{comparison: cmpRows})
	var ratioSpec figureSpec
	found := false
	for _, s := range specs {
		if s.slug == "throughput_ratio_vs_workers" || s.slug == "latency_ratio_vs_workers" {
			t.Errorf("unexpected workers-vs ratio figure %q with workers fixed", s.slug)
		}
		if s.slug == "throughput_ratio_vs_nodes" {
			ratioSpec, found = s, true
		}
	}
	if !found {
		t.Fatal("throughput_ratio_vs_nodes not planned")
	}
	if ratioSpec.xcol != "nodes" {
		t.Errorf("ratio figure xcol = %q, want %q", ratioSpec.xcol, "nodes")
	}

	if err := writeReportTyp(filepath.Join(dir, "report.typ"), reportHeader{title: "Ratio-vs-nodes compile test"}, specs); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("typst", "compile", "report.typ", "report.pdf")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.pdf")); err != nil {
		t.Fatalf("report.pdf not produced: %v", err)
	}
}
