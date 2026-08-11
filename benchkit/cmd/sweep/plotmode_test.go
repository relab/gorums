package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/relab/gorums/benchkit"
	"google.golang.org/protobuf/proto"
)

// reportFixtureData builds the run and per-node CDF records shared by every
// report input test: two stream modes, a varying worker count, a degraded
// rep, and one run's per-node CDF for the health + CDF figures.
func reportFixtureData() ([]plotRunRecord, []plotNodeCDFRecord) {
	var runs []plotRunRecord
	var cdf []plotNodeCDFRecord
	for _, mode := range []string{"dual", "dedup"} {
		for _, w := range []int{2, 4, 8} {
			for rep := 1; rep <= 2; rep++ {
				lat := 400 + float64(w*40)
				base := "run_Q_N3_W" + strconv.Itoa(w) + "_S" + mode + "_r" + strconv.Itoa(rep)
				status := runStatusSucceeded
				if mode == "dual" && w == 8 && rep == 2 {
					status = runStatusDegraded // one degraded rep to populate the share/status figures
				}
				runs = append(runs, plotRunRecord{
					Dimensions: benchkit.Dimensions{
						Benchmark: "Q", Nodes: 3, Workers: w, Payload: 1024, StreamMode: mode,
					},
					base: base, label: "run", status: status, rep: rep,
					throughput: float64(w) * 5000, allocsPerOp: 12, memPerOp: 2048,
					meanUS: new(lat), p50US: new(lat), p95US: new(lat * 1.5), p99US: new(lat * 2),
					samples: new(uint64(1000)),
				})
				// One run's per-node CDF, for the health + CDF figures.
				if w == 8 && rep == 1 {
					for _, node := range []string{"bb1:9000", "bb2:9000"} {
						for i := 0; i <= 10; i++ {
							prob := float64(i) / 10
							cdf = append(cdf, plotNodeCDFRecord{
								Dimensions: benchkit.Dimensions{
									Benchmark: "Q", Nodes: 3, Workers: w, Payload: 1024, StreamMode: mode,
								},
								base: base, label: "run", status: status, rep: rep,
								node: node, throughput: float64(w) * 5000, prob: prob, cdfUS: lat + 800*prob,
							})
						}
					}
				}
			}
		}
	}
	return runs, cdf
}

// reportFixtureManifestsAndLogs writes the manifests and run log shared by
// every report input test: a manifest per run status so runStatusRows sees a
// degraded outcome, a failed one that carries no plotdata row, and a run log
// with a cross-machine offset line for the offset CDF.
func reportFixtureManifestsAndLogs(t *testing.T, dir string) {
	t.Helper()
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, "m1", runStatusSucceeded, 1, "", []string{resultFilename("m1", n, resultExt)})
	writePlotManifest(t, dir, "m2", runStatusDegraded, 2, "", []string{resultFilename("m2", n, resultExt)})
	writePlotManifest(t, dir, "m3", runStatusFailed, 3, "", []string{resultFilename("m3", n, resultExt)})

	logs := filepath.Join(dir, logSubdir)
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	logLine := "[offsets node 3 (10.0.0.1:9000)] peer 1: before=-120µs after=-118µs drift=-2µs\n"
	if err := os.WriteFile(filepath.Join(logs, "run_Q_N3.log"), []byte(logLine), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildReportDir writes a compact plotdata directory using the legacy CSV
// pair (runs.csv, node_cdf.csv), exercising the fallback report input path
// for sweep output directories collected before plotdata.binpb existed.
func buildReportDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	plotdataDir := filepath.Join(dir, plotDataDir)
	if err := os.MkdirAll(plotdataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runs, cdf := reportFixtureData()
	if err := writePlotRunsCSV(filepath.Join(plotdataDir, "runs.csv"), runs); err != nil {
		t.Fatal(err)
	}
	if err := writePlotNodeCDFCSV(filepath.Join(plotdataDir, "node_cdf.csv"), cdf); err != nil {
		t.Fatal(err)
	}
	reportFixtureManifestsAndLogs(t, dir)
	return dir
}

// buildReportDirBinpb writes the same fixture data as buildReportDir, but as
// the normalized plotdata.binpb a collected sweep directory now contains,
// exercising the primary (non-legacy) report input path.
func buildReportDirBinpb(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	plotdataDir := filepath.Join(dir, plotDataDir)
	if err := os.MkdirAll(plotdataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runs, cdf := reportFixtureData()
	data, err := proto.Marshal(buildPlotData(runs, cdf))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plotdataDir, plotDataFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	reportFixtureManifestsAndLogs(t, dir)
	return dir
}

// reportManifests loads a fixture directory's run manifests, for tests that
// drive the report pipeline's manifest-fed steps directly instead of through
// generateReport.
func reportManifests(t *testing.T, dir string) []loadedRunManifest {
	t.Helper()
	manifests, err := loadRunManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	return manifests
}

func TestGenerateReport(t *testing.T) {
	dir := buildReportDir(t)
	if err := generateReport(dir, reportOptions{title: "Test"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, reportSubdir)
	for _, name := range []string{
		"agg.csv", "comparison.csv", "tl_curve.csv", "node_cdf.csv",
		"node_health.csv", "degraded_share.csv", "run_status.csv", "offsets.csv",
		"report.typ", reportLibName,
	} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}

	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; skipping compile check")
	}
	cmd := exec.Command("typst", "compile", "report.typ", "report.pdf")
	cmd.Dir = out
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\n%s", err, b)
	}
}

// TestGenerateReportFromBinpb verifies that a report generated from the
// normalized plotdata.binpb produces the same figures and derived CSVs as one
// generated from the legacy runs.csv/node_cdf.csv pair.
func TestGenerateReportFromBinpb(t *testing.T) {
	dir := buildReportDirBinpb(t)
	if err := generateReport(dir, reportOptions{title: "Test"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, reportSubdir)
	for _, name := range []string{
		"agg.csv", "comparison.csv", "tl_curve.csv", "node_cdf.csv",
		"node_health.csv", "degraded_share.csv", "run_status.csv", "offsets.csv",
		"report.typ", reportLibName,
	} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func TestGenerateReportExcludeRun(t *testing.T) {
	dir := buildReportDir(t)
	runs, _, _, err := loadReportData(dir, reportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Exclude every dual run by base; only dedup rows should remain.
	opts := reportOptions{excludeRuns: map[string]bool{}}
	for _, r := range runs {
		if r.StreamMode == "dual" {
			opts.excludeRuns[r.base] = true
		}
	}
	kept := filterRuns(runs, opts)
	for _, r := range kept {
		if r.StreamMode == "dual" {
			t.Fatalf("dual run %s survived exclude-run", r.base)
		}
	}
	if len(kept) == 0 {
		t.Fatal("all runs excluded")
	}
}

func TestFilterRunsExcludeDim(t *testing.T) {
	runs := []plotRunRecord{
		{base: "a", Dimensions: benchkit.Dimensions{Benchmark: "Q", StreamMode: "dual", Nodes: 3, Workers: 8, Payload: 1024}},
		{base: "b", Dimensions: benchkit.Dimensions{Benchmark: "Q", StreamMode: "dual", Nodes: 3, Workers: 2, Payload: 1024}},
	}
	opts := reportOptions{excludes: map[string]map[string]bool{"workers": {"8": true}}}
	kept := filterRuns(runs, opts)
	if len(kept) != 1 || kept[0].Workers != 2 {
		t.Errorf("kept = %+v, want only workers=2", kept)
	}
}

func TestReadPlotRunsCSVRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.csv")
	want := []plotRunRecord{{
		Dimensions: benchkit.Dimensions{
			Benchmark: "Q", Nodes: 3, Workers: 8, Payload: 1024, StreamMode: "dedup",
		},
		base: "run_Q", label: "run", status: runStatusSucceeded, rep: 1,
		throughput: 40000, allocsPerOp: 12, memPerOp: 2048, nodesSeen: 3,
		meanUS: new(500.0), p50US: new(450.0), p95US: new(900.0), p99US: new(1200.0),
	}}
	if err := writePlotRunsCSV(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readPlotRunsCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
	g := got[0]
	if g.base != "run_Q" || g.Workers != 8 || g.StreamMode != "dedup" || g.throughput != 40000 {
		t.Errorf("round-trip mismatch: %+v", g)
	}
	if g.p50US == nil || *g.p50US != 450 {
		t.Errorf("p50US = %v, want 450", g.p50US)
	}
}

func TestReadReportNodeCDFCSVSelectsRunsAndReducesHealth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node_cdf.csv")
	var rows []plotNodeCDFRecord
	for base := range 8 {
		for _, node := range []string{"bb1:9000", "bb2:9000"} {
			for point := range 3 {
				rows = append(rows, plotNodeCDFRecord{
					Dimensions: benchkit.Dimensions{
						Benchmark: "Q", Nodes: 2, Workers: base/2 + 1, StreamMode: "dual",
					},
					base: "run-" + strconv.Itoa(base), label: "run",
					status: runStatusSucceeded, rep: 1,
					node: node, throughput: float64(100 + base),
					prob: float64(point) / 2, cdfUS: float64(100 + point),
				})
			}
		}
	}
	if err := writePlotNodeCDFCSV(path, rows); err != nil {
		t.Fatal(err)
	}

	opts := reportOptions{excludeRuns: map[string]bool{"run-0": true}}
	cdf, health, err := readReportNodeCDFCSV(path, opts, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cdfBases(cdfRuns(cdf, nil)), []string{"run-1", "run-2", "run-4"}; !slices.Equal(got, want) {
		t.Errorf("CDF bases = %v, want %v", got, want)
	}
	if len(cdf) != 3*2*3 {
		t.Errorf("CDF rows = %d, want %d", len(cdf), 3*2*3)
	}
	// Health retains one row per node for every eligible run, including runs
	// whose full CDF was not selected for the report.
	if len(health) != 7*2 {
		t.Errorf("health rows = %d, want %d", len(health), 7*2)
	}
}

// TestReduceReportCDFDistinguishesBufferArms verifies that two runs differing
// only by a buffer capacity count as two configurations, so each arm of a
// buffer sweep contributes its own per-node CDF instead of the arms merging
// into a single sample.
func TestReduceReportCDFDistinguishesBufferArms(t *testing.T) {
	var rows []plotNodeCDFRecord
	for _, sendBuffer := range []int{64, 256} {
		for point := range 3 {
			rows = append(rows, plotNodeCDFRecord{
				Dimensions: benchkit.Dimensions{
					Benchmark: "Q", Nodes: 3, Workers: 8, SendBuffer: sendBuffer, StreamMode: "dual",
				},
				base: "run-S" + strconv.Itoa(sendBuffer), label: "run",
				status: runStatusSucceeded, rep: 1,
				node: "bb1:9000", throughput: 5000,
				prob: float64(point) / 2, cdfUS: float64(100 + point),
			})
		}
	}
	cdf, _ := reduceReportCDF(rows, reportOptions{}, 2)
	if got, want := cdfBases(cdfRuns(cdf, nil)), []string{"run-S64", "run-S256"}; !slices.Equal(got, want) {
		t.Errorf("CDF bases = %v, want %v", got, want)
	}
}

// TestReduceReportCDFSelectsPanelRuns verifies the CDF panel selection on a 3x3
// node-count/payload grid whose base order walks the payloads of N=3 first, so a
// first-seen rule would spend a small budget entirely on N=3. A budget smaller
// than the grid must still cover every value each dimension took, and a budget
// larger than the runs that requires must be filled rather than left short.
func TestReduceReportCDFSelectsPanelRuns(t *testing.T) {
	var rows []plotNodeCDFRecord
	for _, nodes := range []int{3, 9, 27} {
		for _, payload := range []int{0, 1024, 16384} {
			base := fmt.Sprintf("run_N%d_P%d", nodes, payload)
			for point := range 3 {
				rows = append(rows, plotNodeCDFRecord{
					Dimensions: benchkit.Dimensions{
						Benchmark: "Q", Nodes: nodes, Workers: 8, Payload: payload, StreamMode: "dual",
					},
					base: base, label: "run", status: runStatusSucceeded, rep: 1,
					node: "bb1:9000", throughput: 5000,
					prob: float64(point) / 2, cdfUS: float64(100 + point),
				})
			}
		}
	}
	varying := varyingDimensions([]benchkit.Dimensions{
		{Nodes: 3, Payload: 0}, {Nodes: 9, Payload: 1024},
	})

	t.Run("SpreadsUnderABudget", func(t *testing.T) {
		cdf, _ := reduceReportCDF(rows, reportOptions{}, 5)
		runs := cdfRuns(cdf, varying)
		nodesSeen, payloadsSeen := map[int]bool{}, map[int]bool{}
		for _, r := range cdf {
			nodesSeen[r.Nodes] = true
			payloadsSeen[r.Payload] = true
		}
		if len(nodesSeen) != 3 || len(payloadsSeen) != 3 {
			t.Errorf("5 panels cover %d node count(s) and %d payload(s), want 3 and 3; panels = %v",
				len(nodesSeen), len(payloadsSeen), cdfRunTitles(runs))
		}
		// Each panel is titled by its configuration, not by the run base.
		for _, run := range runs {
			if run.title == run.base {
				t.Errorf("run %s has no configuration label", run.base)
			}
		}
	})

	t.Run("FillsTheBudget", func(t *testing.T) {
		// Five configurations introduce a new dimension value; the other four
		// must fill the remaining panels rather than leaving the page short.
		cdf, _ := reduceReportCDF(rows, reportOptions{}, 8)
		if got := len(cdfRuns(cdf, varying)); got != 8 {
			t.Errorf("panels = %d, want 8 of the 9 configurations", got)
		}
		if got := len(cdfRuns(mustReduce(rows, 9), varying)); got != 9 {
			t.Errorf("panels = %d, want all 9 configurations", got)
		}
	})
}

func mustReduce(rows []plotNodeCDFRecord, limit int) []plotNodeCDFRecord {
	cdf, _ := reduceReportCDF(rows, reportOptions{}, limit)
	return cdf
}

func cdfRunTitles(runs []cdfRun) []string {
	titles := make([]string, len(runs))
	for i, run := range runs {
		titles[i] = run.title
	}
	return titles
}

// TestWriteTimeSeriesFiguresAlternatesStreamModes verifies that the over-time
// figures cover both arms of a comparison: every natural order groups the modes
// together, so a cap smaller than the candidate list used to spend itself on one
// mode. It also verifies that a run over the same nodes as the previous figure
// shares its legend instead of repeating it.
func TestWriteTimeSeriesFiguresAlternatesStreamModes(t *testing.T) {
	dir := t.TempDir()
	n := nodeAssignment{host: "bb1", port: 9000}
	var preferred []string
	for _, mode := range []string{"dedup", "dual"} {
		for _, workers := range []int32{1, 2, 4} {
			base := fmt.Sprintf("e1_Q_N1_W%d_P0_S%s_r1", workers, mode)
			preferred = append(preferred, base)
			writePlotManifestDims(t, dir, base, runStatusSucceeded, 1, "", benchkit.Dimensions{
				Benchmark: "Q", Nodes: 1, Workers: int(workers), StreamMode: mode,
			}, []string{n.hostAddr()}, []string{resultFilename(base, n, resultExt)})
			writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
				Config: plotRunConfigWithStreamMode("Q", 1, workers, 0, 0, mode),
				Events: []*benchkit.Event{tputEvent(0, 100), tputEvent(1_000_000_000, 100)},
			}.Build())
		}
	}

	out := filepath.Join(dir, reportSubdir)
	figures := writeTimeSeriesFigures(dir, out, reportManifests(t, dir), preferred, nil, 4)
	var modes []string
	for _, f := range figures {
		if strings.Contains(f.base, "Sdedup") {
			modes = append(modes, "dedup")
		} else {
			modes = append(modes, "dual")
		}
	}
	if want := []string{"dedup", "dual", "dedup", "dual"}; !slices.Equal(modes, want) {
		t.Errorf("figure modes = %v, want %v (bases %v)", modes, want, figureBases(figures))
	}
	// Every run here has the same single node, so only the first figure draws a
	// legend and the rest read the colors off it.
	if figures[0].sharesNodes {
		t.Error("the first figure must draw its own legend")
	}
	for _, f := range figures[1:] {
		if !f.sharesNodes {
			t.Errorf("%s repeats the legend of an identical node set", f.base)
		}
	}
}

func figureBases(figures []timeSeriesRunFigures) []string {
	bases := make([]string, len(figures))
	for i, f := range figures {
		bases[i] = f.base
	}
	return bases
}

// TestGenerateReportIncludesTimeSeries verifies the end-to-end wiring: a
// local sweep directory (raw per-node result files still present, matching
// what autoReport sees right after a local run) whose result carries an
// Events stream produces timeseries/<base>/<bench>_{throughput,latency,
// saturation}.csv alongside the other report CSVs, time-series and
// a time-series figure reading all three in report.typ, and a report that still
// compiles with Typst. This result's Events carry no PhaseMarker, so its
// saturation CSV has zero rows — the case the figure must guard against, since
// an empty node list would otherwise error in hlegend's grid(columns: 0).
func TestGenerateReportIncludesTimeSeries(t *testing.T) {
	dir := t.TempDir()
	const base = "run_Q_N1_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, n, resultExt)})
	writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 100,
		Latencies:  []int64{1000, 2000}, // gives this run per-node CDF data, so its base is selected
		Events: []*benchkit.Event{
			benchkit.Event_builder{
				Offset:     0,
				Throughput: benchkit.ThroughputInterval_builder{Ops: 100, Duration: 1_000_000_000}.Build(),
			}.Build(),
			benchkit.Event_builder{
				Offset:  0,
				Latency: benchkit.LatencyInterval_builder{Mean: 1500, Stddev: 500, Count: 2}.Build(),
			}.Build(),
		},
	}.Build())

	if err := generateReport(dir, reportOptions{title: "Test"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, reportSubdir)

	for _, name := range []string{
		filepath.Join("timeseries", base, "Q_throughput.csv"),
		filepath.Join("timeseries", base, "Q_latency.csv"),
		filepath.Join("timeseries", base, "Q_saturation.csv"),
	} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}

	typData, err := os.ReadFile(filepath.Join(out, "report.typ"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"time-series(csv(", "Q_saturation.csv"} {
		if !strings.Contains(string(typData), want) {
			t.Errorf("report.typ missing %q in its time-series figure call:\n%s", want, typData)
		}
	}
	// The saturation curve is a panel of the time-series figure, not a section
	// of its own: a run measured at one offered rate has a single point per node
	// there, which the sweep's throughput-vs-rate figure already shows.
	if strings.Contains(string(typData), "== #text(\"Saturation curve") {
		t.Errorf("report.typ still gives the saturation curve its own section:\n%s", typData)
	}

	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; skipping compile check")
	}
	cmd := exec.Command("typst", "compile", "report.typ", "report.pdf")
	cmd.Dir = out
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\n%s", err, b)
	}
}

// timeSeriesFixture writes a run directory holding every base in bases as a
// manifest of one node, with a decodable raw result file written only for the
// bases in withRaw. It returns the directory and the per-node health records
// covering all of them, in the order the report's CDF reducer produces: one
// row per base, first-seen base per configuration first.
func timeSeriesFixture(t *testing.T, bases []string, withRaw []string, streamMode string) string {
	t.Helper()
	dir := t.TempDir()
	n := nodeAssignment{host: "bb1", port: 9000}
	for _, base := range bases {
		writePlotManifestWithStreamMode(t, dir, base, runStatusSucceeded, 1, "", streamMode,
			[]string{resultFilename(base, n, resultExt)})
		if slices.Contains(withRaw, base) {
			writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
				Config: plotRunConfig("Q", 1, 1, 0, 0),
				Events: []*benchkit.Event{tputEvent(0, 100)},
			}.Build())
		}
	}
	return dir
}

// TestWriteTimeSeriesFiguresSelectsRepWithRawData verifies which repetition of
// a configuration gets time-series figures. The base named by the
// configuration's per-node CDF figure is preferred so both figures describe
// the same run, but a directory whose raw result files were only partly
// archived must fall back to a repetition that still has them instead of
// producing no figures at all.
func TestWriteTimeSeriesFiguresSelectsRepWithRawData(t *testing.T) {
	const r1, r2 = "e1_Q_N1_W1_P0_r1", "e1_Q_N1_W1_P0_r2"
	tests := []struct {
		name    string
		withRaw []string
		want    []string
	}{
		{"preferred rep has raw data", []string{r1}, []string{r1}},
		{"only another rep has raw data", []string{r2}, []string{r2}},
		{"both reps have raw data", []string{r1, r2}, []string{r1}},
		{"no rep has raw data", nil, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := timeSeriesFixture(t, []string{r1, r2}, test.withRaw, "")
			out := filepath.Join(dir, reportSubdir)

			figures := writeTimeSeriesFigures(dir, out, reportManifests(t, dir), []string{r1}, nil, maxTimeSeriesRuns)

			var got []string
			for _, f := range figures {
				got = append(got, f.base)
				if !slices.Equal(f.benches, []string{"Q"}) {
					t.Errorf("%s benches = %v, want [Q]", f.base, f.benches)
				}
				if _, err := os.Stat(filepath.Join(out, "timeseries", f.base, "Q_throughput.csv")); err != nil {
					t.Errorf("%s throughput CSV not written: %v", f.base, err)
				}
			}
			if !slices.Equal(got, test.want) {
				t.Errorf("time-series bases = %v, want %v", got, test.want)
			}
		})
	}
}

// TestWriteTimeSeriesFiguresFromExportedEvents verifies that a compact-transfer
// directory renders time series from the exported plotdata/events.binpb, with no
// raw result file present at all: a compact transfer retains none for a
// successful run, which used to leave the report with no time-series figure.
func TestWriteTimeSeriesFiguresFromExportedEvents(t *testing.T) {
	dir := t.TempDir()
	const base = "e1_Q_N1_W1_P0_r1"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, n, resultExt)})
	writePlotEventsFile(t, dir, benchkit.PlotEvents_builder{
		Runs: []*benchkit.PlotRunEvents{benchkit.PlotRunEvents_builder{
			Base: base,
			Benchmarks: []*benchkit.PlotBenchmarkEvents{benchkit.PlotBenchmarkEvents_builder{
				Benchmark: "Q",
				Nodes: []*benchkit.PlotNodeEvents{benchkit.PlotNodeEvents_builder{
					Node:   "bb1:9000",
					Events: []*benchkit.Event{tputEvent(0, 100), tputEvent(1_000_000_000, 200)},
				}.Build()},
			}.Build()},
		}.Build()},
	}.Build())

	out := filepath.Join(dir, reportSubdir)
	figures := writeTimeSeriesFigures(dir, out, reportManifests(t, dir), nil, nil, maxTimeSeriesRuns)
	if len(figures) != 1 || figures[0].base != base || !slices.Equal(figures[0].benches, []string{"Q"}) {
		t.Fatalf("figures = %+v, want one set for %s with benchmark Q", figures, base)
	}
	data, err := os.ReadFile(filepath.Join(out, "timeseries", base, "Q_throughput.csv"))
	if err != nil {
		t.Fatalf("read throughput CSV: %v", err)
	}
	if got := strings.Count(strings.TrimSpace(string(data)), "\n"); got != 2 {
		t.Errorf("throughput CSV has %d data row(s), want 2\n%s", got, data)
	}
}

// TestGenerateReportTimeSeriesWithoutLatencyData verifies that a throughput-only
// benchmark gets its time-series figure. Its runs record no latency sample, so
// there is no per-node CDF data, which the selection used to be gated on even
// though the event stream carries valid throughput intervals.
func TestGenerateReportTimeSeriesWithoutLatencyData(t *testing.T) {
	dir := t.TempDir()
	const base = "e1_Q_N1_W1_P0_r1"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, n, resultExt)})
	writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 300,
		// No Latencies and no Histogram: a server-measured throughput-only run.
		Events: []*benchkit.Event{tputEvent(0, 100), tputEvent(1_000_000_000, 200)},
	}.Build())

	if err := generateReport(dir, reportOptions{title: "Throughput only"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, reportSubdir)
	if _, err := os.Stat(filepath.Join(out, "node_cdf.csv")); !os.IsNotExist(err) {
		t.Errorf("node_cdf.csv written for a run with no latency samples: %v", err)
	}
	typ, err := os.ReadFile(filepath.Join(out, "report.typ"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(typ), "time-series(") {
		t.Errorf("report.typ plans no time-series figure:\n%s", typ)
	}
}

// TestWriteFailedTimeSeriesFigures verifies the failed-runs budget and its
// caveat text: failed runs sharing a configuration and an error signature (which
// folds away the exit status) yield one representative figure, the cap drops the
// rest, and the note states how the run failed and how many of its nodes wrote
// no data at all.
func TestWriteFailedTimeSeriesFigures(t *testing.T) {
	dir := t.TempDir()
	n1 := nodeAssignment{host: "bb1", port: 9000}
	n2 := nodeAssignment{host: "bb2", port: 9000}
	// Three failures of one configuration, differing only in exit status, plus
	// one of another configuration.
	for rep, exit := range []int{1, 2, 3} {
		base := "e1_Q_N1_W1_P0_r" + strconv.Itoa(rep+1)
		writeFailedRun(t, dir, base, "", rep+1, exit, n1, n2)
	}
	writeFailedRun(t, dir, "e1_Q_N1_W1_P0_Sdedup_r1", "dedup", 1, 9, n1, n2)

	out := filepath.Join(dir, reportSubdir)
	figures := writeFailedTimeSeriesFigures(dir, out, reportManifests(t, dir), nil, maxFailedRuns)
	var got []string
	for _, f := range figures {
		got = append(got, f.base)
	}
	want := []string{"e1_Q_N1_W1_P0_Sdedup_r1", "e1_Q_N1_W1_P0_r1"}
	if !slices.Equal(got, want) {
		t.Fatalf("failed-run bases = %v, want %v (one per configuration and error signature)", got, want)
	}
	note := figures[1].note
	for _, want := range []string{"failed during measurement", "1 of 2 nodes wrote no result file", "Representative of 3 failed runs"} {
		if !strings.Contains(note, want) {
			t.Errorf("note %q missing %q", note, want)
		}
	}

	// The cap drops whole groups, not runs within a group.
	if capped := writeFailedTimeSeriesFigures(dir, out, reportManifests(t, dir), nil, 1); len(capped) != 1 {
		t.Errorf("limit 1 produced %d figure set(s), want 1", len(capped))
	}
}

// TestGenerateReportFailedRunSectionCompiles verifies the end-to-end failed-runs
// section: a sweep directory holding one successful and one failed run puts the
// failed run's trace under its own heading, with its caveat note, and the report
// still compiles with Typst.
func TestGenerateReportFailedRunSectionCompiles(t *testing.T) {
	dir := t.TempDir()
	n1 := nodeAssignment{host: "bb1", port: 9000}
	n2 := nodeAssignment{host: "bb2", port: 9000}
	const okBase = "e1_Q_N1_W1_P0_r1"
	writePlotManifest(t, dir, okBase, runStatusSucceeded, 1, "", []string{resultFilename(okBase, n1, resultExt)})
	writePlotReport(t, dir, okBase, n1, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 100,
		Latencies:  []int64{1000, 2000},
		Events:     []*benchkit.Event{tputEvent(0, 100), tputEvent(1_000_000_000, 100)},
	}.Build())
	writeFailedRun(t, dir, "e1_Q_N1_W1_P0_r2", "", 2, 1, n1, n2)

	if err := generateReport(dir, reportOptions{title: "Failed runs"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, reportSubdir)
	typ, err := os.ReadFile(filepath.Join(out, "report.typ"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(typ)
	for _, want := range []string{
		`== #text("Failed runs")`,
		`=== #text("Throughput and latency over time — Q, dual")`,
		"e1_Q_N1_W1_P0_r2/Q_{throughput,latency,saturation}.csv",
		"Run failed during measurement",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("report.typ missing %q\n---\n%s", want, src)
		}
	}
	// The run base identifies the figure in its data note, not in its heading.
	if strings.Contains(src, `#text("Throughput and latency over time — Q (e1_Q`) {
		t.Errorf("report.typ still names the run base in a heading\n---\n%s", src)
	}

	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; skipping compile check")
	}
	cmd := exec.Command("typst", "compile", "report.typ", "report.pdf")
	cmd.Dir = out
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\n%s", err, b)
	}
}

// writeFailedRun writes the manifest and the surviving node's result file of a
// failed run: node bb1 recorded events before the run failed, and the second
// node wrote nothing at all.
func writeFailedRun(t *testing.T, dir, base, streamMode string, rep, exitStatus int, present, missing nodeAssignment) {
	t.Helper()
	files := []string{resultFilename(base, present, resultExt), resultFilename(base, missing, resultExt)}
	writePlotManifestWithStreamMode(t, dir, base, runStatusFailed, rep, "", streamMode, files)
	if err := updateManifest(dir, base, func(m *runManifest) {
		m.FailurePhase = failurePhaseMeasurement
		m.Error = fmt.Sprintf("Process exited with status %d", exitStatus)
		m.CollectedFiles = 1
		m.MissingFiles = files[1:]
	}); err != nil {
		t.Fatalf("update manifest: %v", err)
	}
	writePlotReport(t, dir, base, present, present.hostAddr(), benchkit.Result_builder{
		Config: plotRunConfigWithStreamMode("Q", 1, 1, 0, 0, streamMode),
		Events: []*benchkit.Event{tputEvent(0, 100), tputEvent(1_000_000_000, 50)},
	}.Build())
}

// TestWriteTimeSeriesFiguresOnePerConfigurationUpToLimit verifies the figure
// budget: distinct configurations each contribute at most one run, and no more
// than limit runs are rendered in total, so a large sweep cannot produce a
// time-series figure per run.
func TestWriteTimeSeriesFiguresOnePerConfigurationUpToLimit(t *testing.T) {
	var bases []string
	dir := t.TempDir()
	n := nodeAssignment{host: "bb1", port: 9000}
	for _, mode := range []string{"dual", "dedup"} {
		for rep := 1; rep <= 2; rep++ {
			base := "e1_Q_N1_W1_P0_S" + mode + "_r" + strconv.Itoa(rep)
			bases = append(bases, base)
			writePlotManifestWithStreamMode(t, dir, base, runStatusSucceeded, rep, "", mode,
				[]string{resultFilename(base, n, resultExt)})
			writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
				Config: plotRunConfigWithStreamMode("Q", 1, 1, 0, 0, mode),
				Events: []*benchkit.Event{tputEvent(0, 100)},
			}.Build())
		}
	}
	out := filepath.Join(dir, reportSubdir)

	all := writeTimeSeriesFigures(dir, out, reportManifests(t, dir), []string{bases[0], bases[2]}, nil, maxTimeSeriesRuns)
	var got []string
	for _, f := range all {
		got = append(got, f.base)
	}
	// Configurations follow the preferred order, so the figure budget is spent
	// on the same runs the per-node CDF grid selected: the dual arm is named
	// first in preferred, so it precedes the dedup one despite sorting after it.
	if want := []string{bases[0], bases[2]}; !slices.Equal(got, want) {
		t.Errorf("bases = %v, want %v (one per stream mode, preferred first)", got, want)
	}

	capped := writeTimeSeriesFigures(dir, out, reportManifests(t, dir), []string{bases[0], bases[2]}, nil, 1)
	if len(capped) != 1 {
		t.Errorf("limit 1 produced %d figure set(s), want 1", len(capped))
	}
}

// TestGenerateReportSaturationCurveNonRampedCompiles verifies the other edge
// case named in the saturation-curve follow-up: a non-ramped run (a single
// PhaseMarker_START and no RATE_STEP events) writes exactly one saturation
// row per node rather than zero, and the figure still compiles — a lone
// point is drawn as a marker (as metric-vs does), not silently dropped like
// time-series/per-node-cdf do for a single point.
func TestGenerateReportSaturationCurveNonRampedCompiles(t *testing.T) {
	dir := t.TempDir()
	const base = "run_Q_N1_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, n, resultExt)})
	writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 100,
		Latencies:  []int64{1000, 2000},
		Events: []*benchkit.Event{
			benchkit.Event_builder{
				Offset: 0,
				Phase:  benchkit.PhaseMarker_builder{Phase: benchkit.PhaseMarker_START, Rate: 100}.Build(),
			}.Build(),
			benchkit.Event_builder{
				Offset:     0,
				Throughput: benchkit.ThroughputInterval_builder{Ops: 100, Duration: 1_000_000_000}.Build(),
			}.Build(),
			benchkit.Event_builder{
				Offset:  0,
				Latency: benchkit.LatencyInterval_builder{Mean: 1500, Stddev: 500, Count: 2}.Build(),
			}.Build(),
		},
	}.Build())

	if err := generateReport(dir, reportOptions{title: "Test"}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, reportSubdir)

	satPath := filepath.Join(out, "timeseries", base, "Q_saturation.csv")
	data, err := os.ReadFile(satPath)
	if err != nil {
		t.Fatalf("read %s: %v", satPath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("Q_saturation.csv has %d lines, want 2 (header + one row per node)", len(lines))
	}

	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not on PATH; skipping compile check")
	}
	cmd := exec.Command("typst", "compile", "report.typ", "report.pdf")
	cmd.Dir = out
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile failed: %v\n%s", err, b)
	}
}
