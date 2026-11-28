package main

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/relab/gorums/benchkit"
	"google.golang.org/protobuf/proto"
)

func TestPlotDataExactSamples(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N2_W1_P0"
	n1 := nodeAssignment{host: "bb1", port: 9000}
	n2 := nodeAssignment{host: "bb2", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{
		resultFilename(base, n1, resultExt),
		resultFilename(base, n2, resultExt),
	})
	writePlotReport(t, dir, base, n1, "bb1:9000", benchkit.Result_builder{
		Config:      plotRunConfig("Q", 2, 1, 0, 0),
		Throughput:  10,
		TotalOps:    3,
		AllocsPerOp: 1,
		MemPerOp:    100,
		Latencies:   []int64{1000, 2000},
	}.Build())
	writePlotReport(t, dir, base, n2, "bb2:9000", benchkit.Result_builder{
		Config:      plotRunConfig("Q", 2, 1, 0, 0),
		Throughput:  20,
		TotalOps:    3,
		AllocsPerOp: 3,
		MemPerOp:    300,
		Latencies:   []int64{3000, 4000},
	}.Build())

	runs, cdf, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	row := runs[0]
	if row.StreamMode != "dual" {
		t.Errorf("streamMode = %q, want dual", row.StreamMode)
	}
	if row.throughput != 30 {
		t.Errorf("throughput = %v, want 30", row.throughput)
	}
	if row.totalOps != 6 {
		t.Errorf("totalOps = %d, want 6", row.totalOps)
	}
	if row.allocsPerOp != 2 || row.memPerOp != 200 {
		t.Errorf("cost = allocs %v mem %v, want 2 and 200", row.allocsPerOp, row.memPerOp)
	}
	assertFloatPtr(t, "mean_us", row.meanUS, 2.5)
	assertFloatPtr(t, "p50_us", row.p50US, 2.5)
	assertFloatPtr(t, "p95_us", row.p95US, 3.85)
	if row.samples == nil || *row.samples != 4 {
		t.Fatalf("samples = %v, want 4", row.samples)
	}
	if len(cdf) != 2*cdfPoints {
		t.Fatalf("cdf rows = %d, want %d", len(cdf), 2*cdfPoints)
	}
	if cdf[0].prob != 0 || cdf[0].cdfUS != 1 {
		t.Errorf("first cdf row = prob %v value %v, want 0 and 1", cdf[0].prob, cdf[0].cdfUS)
	}
	if cdf[0].StreamMode != "dual" {
		t.Errorf("cdf streamMode = %q, want dual", cdf[0].StreamMode)
	}
	if last := cdf[cdfPoints-1]; last.prob != 1 || last.cdfUS != 2 {
		t.Errorf("last bb1 cdf row = prob %v value %v, want 1 and 2", last.prob, last.cdfUS)
	}
}

// TestAggregatePlotRunUsesMeasurementMode verifies that aggregatePlotRun
// sums throughput from client-measured nodes only when any exist, matching
// [mergeResults] exactly: a PBFT-style primary-client run reports
// the primary's client ops/s, not primary+Σbackup execute rates, even though
// the server-measured backups here also carry latency samples (server
// measured EXACT results do) — the case that defeats a "has latency data"
// proxy for "client-measured".
func TestAggregatePlotRunUsesMeasurementMode(t *testing.T) {
	entries := []plotNodeEntry{
		{
			node: "primary", throughput: 5000,
			measurementMode: benchkit.MeasurementMode_CLIENT_MEASURED,
			latency:         benchkit.Summary{Latencies: []int64{100, 200}, LatencyValid: true}.Dist(),
		},
		{
			node: "backup1", throughput: 50000,
			measurementMode: benchkit.MeasurementMode_SERVER_MEASURED,
			latency:         benchkit.Summary{Latencies: []int64{80, 90}, LatencyValid: true}.Dist(),
		},
		{
			node: "backup2", throughput: 50000,
			measurementMode: benchkit.MeasurementMode_SERVER_MEASURED,
			latency:         benchkit.Summary{Latencies: []int64{80, 90}, LatencyValid: true}.Dist(),
		},
	}
	row := aggregatePlotRun("base", runManifest{}, "PBFT", entries)
	if row.throughput != 5000 {
		t.Errorf("throughput = %v, want 5000 (primary client ops/s only, not primary+backups)", row.throughput)
	}
}

// TestAggregatePlotRunSumsAllWhenNoneClientMeasured verifies the fallback:
// when no node is client-measured (a symmetric multi-node client), every
// node's throughput is summed.
func TestAggregatePlotRunSumsAllWhenNoneClientMeasured(t *testing.T) {
	entries := []plotNodeEntry{
		{node: "n1", throughput: 5000, measurementMode: benchkit.MeasurementMode_SERVER_MEASURED},
		{node: "n2", throughput: 6000, measurementMode: benchkit.MeasurementMode_SERVER_MEASURED},
	}
	row := aggregatePlotRun("base", runManifest{}, "Multicast", entries)
	if row.throughput != 11000 {
		t.Errorf("throughput = %v, want 11000 (sum across symmetric multi-node clients)", row.throughput)
	}
}

// TestPlotDataStreamMode verifies that the stream mode recorded in the run
// manifest propagates to both the run rows and the per-node CDF rows, so
// dedup and dual runs remain distinguishable plot dimensions.
func TestPlotDataStreamMode(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N2_W1_P0_Sdedup_r1"
	n1 := nodeAssignment{host: "bb1", port: 9000}
	n2 := nodeAssignment{host: "bb2", port: 9000}
	writePlotManifestWithStreamMode(t, dir, base, runStatusSucceeded, 1, "", "dedup", []string{
		resultFilename(base, n1, resultExt),
		resultFilename(base, n2, resultExt),
	})
	writePlotReport(t, dir, base, n1, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfigWithStreamMode("Q", 2, 1, 0, 0, "dedup"),
		Throughput: 10,
		Latencies:  []int64{1000},
	}.Build())
	writePlotReport(t, dir, base, n2, "bb2:9000", benchkit.Result_builder{
		Config:     plotRunConfigWithStreamMode("Q", 2, 1, 0, 0, "dedup"),
		Throughput: 20,
		Latencies:  []int64{2000},
	}.Build())

	runs, cdf, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if got := runs[0].StreamMode; got != "dedup" {
		t.Fatalf("streamMode = %q, want dedup", got)
	}
	if len(cdf) == 0 {
		t.Fatal("cdf rows = 0, want rows")
	}
	if cdf[0].StreamMode != "dedup" {
		t.Fatalf("cdf streamMode = %q, want dedup", cdf[0].StreamMode)
	}
}

func TestPlotDataHDRHistograms(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N2_W1_P0"
	n1 := nodeAssignment{host: "bb1", port: 9000}
	n2 := nodeAssignment{host: "bb2", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{
		resultFilename(base, n1, resultExt),
		resultFilename(base, n2, resultExt),
	})
	writePlotReport(t, dir, base, n1, "bb1", benchkit.Result_builder{
		Config:    plotRunConfigWithStats("Q", 2, 1, 0, 0, benchkit.StatsMode_HDR),
		Histogram: benchkit.LatencyHistogram_builder{Value: []int64{100, 200}, Count: []uint64{5, 10}}.Build(),
	}.Build())
	writePlotReport(t, dir, base, n2, "bb2", benchkit.Result_builder{
		Config:    plotRunConfigWithStats("Q", 2, 1, 0, 0, benchkit.StatsMode_HDR),
		Histogram: benchkit.LatencyHistogram_builder{Value: []int64{200, 400}, Count: []uint64{10, 15}}.Build(),
	}.Build())

	runs, cdf, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	row := runs[0]
	assertFloatPtr(t, "mean_us", row.meanUS, 0.2625)
	assertFloatPtr(t, "p50_us", row.p50US, 0.2)
	assertFloatPtr(t, "p95_us", row.p95US, 0.4)
	if row.samples == nil || *row.samples != 40 {
		t.Fatalf("samples = %v, want 40", row.samples)
	}
	if len(cdf) != 2*cdfPoints {
		t.Fatalf("cdf rows = %d, want %d", len(cdf), 2*cdfPoints)
	}
}

func TestPlotDataTrim(t *testing.T) {
	const s = int64(1_000_000_000)
	dir := t.TempDir()
	base := "e1_Q_N1_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "1s", []string{resultFilename(base, n, resultExt)})
	writePlotReport(t, dir, base, n, "bb1", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 999,
		Latencies:  make([]int64, 65),
		Events: []*benchkit.Event{
			tputEvent(0, 5),
			tputEvent(1*s, 10),
			tputEvent(2*s, 20),
			tputEvent(3*s, 30),
		},
	}.Build())

	runs, _, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	row := runs[0]
	if row.throughput != 20 {
		t.Errorf("throughput = %v, want 20", row.throughput)
	}
	if row.samples == nil || *row.samples != 60 {
		t.Fatalf("samples = %v, want 60", row.samples)
	}
}

func TestPlotDataRepetitionsRemainSeparate(t *testing.T) {
	dir := t.TempDir()
	for rep := 1; rep <= 2; rep++ {
		base := "e1_Q_N1_W1_P0"
		if rep == 2 {
			base += "_r2"
		}
		n := nodeAssignment{host: "bb1", port: 9000}
		writePlotManifest(t, dir, base, runStatusSucceeded, rep, "", []string{resultFilename(base, n, resultExt)})
		writePlotReport(t, dir, base, n, "bb1", benchkit.Result_builder{
			Config:     plotRunConfig("Q", 1, 1, 0, 0),
			Throughput: float64(rep),
			Latencies:  []int64{1000},
		}.Build())
	}

	runs, _, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
	if runs[0].rep != 1 || runs[1].rep != 2 {
		t.Fatalf("reps = %d, %d; want 1, 2", runs[0].rep, runs[1].rep)
	}
}

// TestPlotDataIncludesDegradedRuns verifies that degraded runs (completed but
// with a pathologically slow node) flow into the plot data tagged with their
// status, so consumers can exclude them from aggregates while still diagnosing
// the slow node, and that failed runs remain excluded.
func TestPlotDataIncludesDegradedRuns(t *testing.T) {
	dir := t.TempDir()
	node := nodeAssignment{host: "bb1", port: 9000}
	bases := []struct {
		base   string
		status string
	}{
		{"e1_Q_N1_W1_P0", runStatusSucceeded},
		{"e1_Q_N1_W1_P0_r2", runStatusDegraded},
		{"e1_Q_N1_W1_P0_r3", runStatusFailed},
	}
	for i, b := range bases {
		writePlotManifest(t, dir, b.base, b.status, i+1, "", []string{resultFilename(b.base, node, resultExt)})
		writePlotReport(t, dir, b.base, node, "bb1", benchkit.Result_builder{
			Config:     plotRunConfig("Q", 1, 1, 0, 0),
			Throughput: 10,
			Latencies:  []int64{1000},
		}.Build())
	}

	runs, cdf, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2 (succeeded + degraded, failed excluded)", len(runs))
	}
	if runs[0].status != runStatusSucceeded || runs[1].status != runStatusDegraded {
		t.Errorf("statuses = %q, %q; want %q, %q",
			runs[0].status, runs[1].status, runStatusSucceeded, runStatusDegraded)
	}
	if len(cdf) != 2*cdfPoints {
		t.Fatalf("cdf rows = %d, want %d", len(cdf), 2*cdfPoints)
	}
	if cdf[0].status != runStatusSucceeded || cdf[len(cdf)-1].status != runStatusDegraded {
		t.Errorf("cdf statuses = %q, %q; want %q, %q",
			cdf[0].status, cdf[len(cdf)-1].status, runStatusSucceeded, runStatusDegraded)
	}
}

// TestCollectPlotDataEventsCoverEveryStatus verifies that the event streams are
// collected for every run regardless of outcome, including a failed run that
// contributes no plot row: a failed run's throughput-over-time trace is what
// shows whether its nodes were producing work before the failure, and the raw
// file it would otherwise have to come from is not retained for a successful
// run at all.
func TestCollectPlotDataEventsCoverEveryStatus(t *testing.T) {
	const s = int64(1_000_000_000)
	dir := t.TempDir()
	node := nodeAssignment{host: "bb1", port: 9000}
	statuses := map[string]string{
		"e1_Q_N1_W1_P0":    runStatusSucceeded,
		"e1_Q_N1_W1_P0_r2": runStatusDegraded,
		"e1_Q_N1_W1_P0_r3": runStatusFailed,
	}
	for base, status := range statuses {
		writePlotManifest(t, dir, base, status, 1, "", []string{resultFilename(base, node, resultExt)})
		writePlotReport(t, dir, base, node, "bb1:9000", benchkit.Result_builder{
			Config:     plotRunConfig("Q", 1, 1, 0, 0),
			Throughput: 10,
			Latencies:  []int64{1000},
			Events:     []*benchkit.Event{tputEvent(0, 5), tputEvent(1*s, 10)},
		}.Build())
	}

	runs, _, events, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	if len(runs) != 2 {
		t.Errorf("plot rows = %d, want 2 (failed run excluded)", len(runs))
	}
	got := map[string]int{}
	for _, run := range events.GetRuns() {
		benches := run.GetBenchmarks()
		if len(benches) != 1 || benches[0].GetBenchmark() != "Q" {
			t.Fatalf("%s benchmarks = %+v, want one entry for Q", run.GetBase(), benches)
		}
		nodes := benches[0].GetNodes()
		if len(nodes) != 1 || nodes[0].GetNode() != "bb1:9000" {
			t.Fatalf("%s nodes = %+v, want one entry for bb1:9000", run.GetBase(), nodes)
		}
		got[run.GetBase()] = len(nodes[0].GetEvents())
	}
	for base := range statuses {
		if got[base] != 2 {
			t.Errorf("%s (%s) events = %d, want 2", base, statuses[base], got[base])
		}
	}
}

// TestWriteCompactPlotDataEventsFile verifies the events.binpb round trip: it is
// written beside plotdata.binpb when any run recorded events, is absent when no
// run did (interval reporting off), and a stale one is removed by a re-export.
func TestWriteCompactPlotDataEventsFile(t *testing.T) {
	const s = int64(1_000_000_000)
	dir := t.TempDir()
	base := "e1_Q_N1_W1_P0"
	node := nodeAssignment{host: "bb1", port: 9000}
	writeRun := func(events ...*benchkit.Event) {
		writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, node, resultExt)})
		writePlotReport(t, dir, base, node, "bb1:9000", benchkit.Result_builder{
			Config:     plotRunConfig("Q", 1, 1, 0, 0),
			Throughput: 10,
			Latencies:  []int64{1000},
			Events:     events,
		}.Build())
	}

	writeRun(tputEvent(0, 5), tputEvent(1*s, 10))
	if err := writeCompactPlotData(dir); err != nil {
		t.Fatalf("writeCompactPlotData: %v", err)
	}
	events, err := readPlotEvents(dir)
	if err != nil {
		t.Fatalf("readPlotEvents: %v", err)
	}
	if len(events.GetRuns()) != 1 || events.GetRuns()[0].GetBase() != base {
		t.Fatalf("event runs = %+v, want one entry for %s", events.GetRuns(), base)
	}

	// Re-exporting a run with no events at all must not leave the stale file.
	writeRun()
	if err := writeCompactPlotData(dir); err != nil {
		t.Fatalf("writeCompactPlotData: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, plotDataDir, plotEventsFile)); !os.IsNotExist(err) {
		t.Errorf("events.binpb present for a sweep with no events: %v", err)
	}
	events, err = readPlotEvents(dir)
	if err != nil {
		t.Fatalf("readPlotEvents with no events file: %v", err)
	}
	if events != nil {
		t.Errorf("readPlotEvents = %+v, want nil", events)
	}
}

// TestPrepareCompactTransferRebuildsFromRawResults verifies the -export-compact
// rescue path: re-running the preparation over a work directory whose earlier
// compact transfer predates the event streams replaces that directory, so the
// small download carries the events without shipping the raw archive.
func TestPrepareCompactTransferRebuildsFromRawResults(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N1_W1_P0"
	node := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, node, resultExt)})
	writePlotReport(t, dir, base, node, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 10,
		Latencies:  []int64{1000},
		Events:     []*benchkit.Event{tputEvent(0, 5), tputEvent(1_000_000_000, 10)},
	}.Build())

	// An earlier transfer directory, holding plot data but no event streams.
	stale := filepath.Join(dir, compactTransferDir, plotDataDir)
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, plotDataFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	summary, err := prepareCompactTransfer(dir, false)
	if err != nil {
		t.Fatalf("prepareCompactTransfer: %v", err)
	}
	if summary.eventBytes <= 0 {
		t.Errorf("eventBytes = %d, want > 0", summary.eventBytes)
	}
	events := filepath.Join(dir, compactTransferDir, plotDataDir, plotEventsFile)
	info, err := os.Stat(events)
	if err != nil {
		t.Fatalf("events.binpb missing from the rebuilt transfer: %v", err)
	}
	if info.Size() != summary.eventBytes {
		t.Errorf("events.binpb is %d bytes, want the reported %d", info.Size(), summary.eventBytes)
	}
}

func TestPrepareCompactTransferExcludesSuccessfulRawResults(t *testing.T) {
	dir := t.TempDir()
	successBase := "e1_Q_N1_W1_P0"
	failedBase := "e1_Q_N2_W1_P0"
	successNode := nodeAssignment{host: "bb1", port: 9000}
	failedNode := nodeAssignment{host: "bb2", port: 9000}
	writePlotManifest(t, dir, successBase, runStatusSucceeded, 1, "", []string{resultFilename(successBase, successNode, resultExt)})
	writePlotManifest(t, dir, failedBase, runStatusFailed, 1, "", []string{resultFilename(failedBase, failedNode, resultExt)})
	writePlotReport(t, dir, successBase, successNode, "bb1", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 1,
		Latencies:  []int64{1000},
	}.Build())
	writePlotReport(t, dir, failedBase, failedNode, "bb2", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 2, 1, 0, 0),
		Throughput: 2,
		Latencies:  []int64{2000},
	}.Build())
	if err := os.WriteFile(filepath.Join(dir, "sweep.log"), []byte("log\n"), 0o644); err != nil {
		t.Fatalf("write sweep.log: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, logSubdir), 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, logSubdir, successBase+".log"), []byte("node log\n"), 0o644); err != nil {
		t.Fatalf("write node log: %v", err)
	}

	summary, err := prepareCompactTransfer(dir, false)
	if err != nil {
		t.Fatalf("prepareCompactTransfer: %v", err)
	}
	if summary.failedResults != 1 {
		t.Fatalf("failedResults = %d, want 1", summary.failedResults)
	}
	transfer := filepath.Join(dir, compactTransferDir)
	if _, err := os.Stat(filepath.Join(transfer, resultFilename(failedBase, failedNode, resultExt))); err != nil {
		t.Fatalf("failed result missing from compact transfer: %v", err)
	}
	if _, err := os.Stat(filepath.Join(transfer, resultFilename(successBase, successNode, resultExt))); !os.IsNotExist(err) {
		t.Fatalf("successful result should not be in compact transfer: %v", err)
	}
	if _, err := os.Stat(filepath.Join(transfer, plotDataDir, plotDataFile)); err != nil {
		t.Fatalf("plotdata.binpb missing: %v", err)
	}
}

func writePlotManifest(t *testing.T, dir, base, status string, rep int, trim string, files []string) {
	writePlotManifestWithStreamMode(t, dir, base, status, rep, trim, "", files)
}

func writePlotManifestWithStreamMode(t *testing.T, dir, base, status string, rep int, trim, streamMode string, files []string) {
	t.Helper()
	writePlotManifestDims(t, dir, base, status, rep, trim, benchkit.Dimensions{
		Benchmark: "Q", Nodes: 1, Workers: 1, StreamMode: streamMode,
	}, nil, files)
}

// writePlotManifestDims writes a run manifest for the given configuration and
// node hosts, for tests of the selection logic that groups runs by either.
func writePlotManifestDims(t *testing.T, dir, base, status string, rep int, trim string, dims benchkit.Dimensions, hosts, files []string) {
	t.Helper()
	m := runManifest{
		Dimensions: dims, Rep: rep,
		Label:  "e1",
		Trim:   trim,
		Status: status,
		Hosts:  hosts,
		Files:  files,
	}
	data, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+manifestSuffix), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func writePlotReport(t *testing.T, dir, base string, node nodeAssignment, label string, results ...*benchkit.Result) {
	t.Helper()
	path := filepath.Join(dir, resultFilename(base, node, resultExt))
	if err := benchkit.WriteLabeledReport(results, label, path); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

// writePlotEventsFile writes events as the sweep directory's
// plotdata/events.binpb, the form a compact transfer carries.
func writePlotEventsFile(t *testing.T, dir string, events *benchkit.PlotEvents) {
	t.Helper()
	plotdataDir := filepath.Join(dir, plotDataDir)
	if err := os.MkdirAll(plotdataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := proto.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plotdataDir, plotEventsFile), data, 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}
}

// tputEvent is one throughput-interval event: ops completed over the one second
// ending at offsetNs.
func tputEvent(offsetNs int64, ops uint64) *benchkit.Event {
	return benchkit.Event_builder{
		Offset:     offsetNs,
		Throughput: benchkit.ThroughputInterval_builder{Ops: ops, Duration: 1_000_000_000}.Build(),
	}.Build()
}

func plotRunConfig(name string, nodes, workers, payload, rate int32) *benchkit.RunConfig {
	return plotRunConfigWithStats(name, nodes, workers, payload, rate, benchkit.StatsMode_EXACT)
}

func plotRunConfigWithStats(name string, nodes, workers, payload, rate int32, statsMode benchkit.StatsMode) *benchkit.RunConfig {
	return benchkit.RunConfig_builder{
		Name:      name,
		NumNodes:  nodes,
		Workers:   workers,
		Payload:   payload,
		Rate:      int64(rate),
		StatsMode: statsMode,
	}.Build()
}

func plotRunConfigWithStreamMode(name string, nodes, workers, payload, rate int32, streamMode string) *benchkit.RunConfig {
	return benchkit.RunConfig_builder{
		Name:       name,
		NumNodes:   nodes,
		Workers:    workers,
		Payload:    payload,
		Rate:       int64(rate),
		StreamMode: streamMode,
	}.Build()
}

// TestBuildPlotDataNormalizesIdentity verifies that buildPlotData groups flat
// run and per-node CDF records into the nested message with run and node
// identity stored once, and that plotRecordsFromMessage flattens it back to
// equivalent records.
func TestBuildPlotDataNormalizesIdentity(t *testing.T) {
	meanUS, p50US, p95US, p99US := 100.0, 90.0, 150.0, 200.0
	samples := uint64(42)
	runs := []plotRunRecord{{
		Benchmark: "Q", Nodes: 2, Workers: 4, Payload: 128, StreamMode: "dual",
		base: "run1", label: "e1", status: runStatusSucceeded, rep: 1,
		throughput: 30, totalOps: 6, failedOps: 0, allocsPerOp: 2, memPerOp: 200, nodesSeen: 2,
		meanUS: &meanUS, p50US: &p50US, p95US: &p95US, p99US: &p99US, samples: &samples,
	}}
	cdf := []plotNodeCDFRecord{
		{
			Benchmark: "Q", Nodes: 2, Workers: 4, Payload: 128, StreamMode: "dual",
			base: "run1", label: "e1", status: runStatusSucceeded, rep: 1,
			node: "bb1:9000", throughput: 10, meanUS: 100, p50US: 90, p95US: 150, p99US: 200, samples: 2,
			prob: 0, cdfUS: 1,
		},
		{
			Benchmark: "Q", Nodes: 2, Workers: 4, Payload: 128, StreamMode: "dual",
			base: "run1", label: "e1", status: runStatusSucceeded, rep: 1,
			node: "bb1:9000", throughput: 10, meanUS: 100, p50US: 90, p95US: 150, p99US: 200, samples: 2,
			prob: 1, cdfUS: 2,
		},
	}

	pd := buildPlotData(runs, cdf)
	if len(pd.GetRuns()) != 1 {
		t.Fatalf("runs = %d, want 1", len(pd.GetRuns()))
	}
	run := pd.GetRuns()[0]
	if run.GetBase() != "run1" || run.GetLabel() != "e1" || run.GetRep() != 1 {
		t.Errorf("run identity = %q %q %d, want run1 e1 1", run.GetBase(), run.GetLabel(), run.GetRep())
	}
	if len(run.GetBenchmarks()) != 1 {
		t.Fatalf("benchmarks = %d, want 1", len(run.GetBenchmarks()))
	}
	bench := run.GetBenchmarks()[0]
	// The benchmark name and sweep dimensions ride in a RunConfig, the same
	// type the raw result files carry them in.
	cfg := bench.GetConfig()
	if cfg.GetName() != "Q" || cfg.GetNumNodes() != 2 || cfg.GetWorkers() != 4 || cfg.GetStreamMode() != "dual" {
		t.Errorf("config = %+v, want name=Q nodes=2 workers=4 streamMode=dual", cfg)
	}
	if bench.GetThroughput() != 30 {
		t.Errorf("throughput = %v, want 30", bench.GetThroughput())
	}
	if len(bench.GetNodes()) != 1 {
		t.Fatalf("nodes = %d, want 1", len(bench.GetNodes()))
	}
	node := bench.GetNodes()[0]
	if node.GetNode() != "bb1:9000" {
		t.Errorf("node = %q, want bb1:9000", node.GetNode())
	}
	if got := node.GetCdfUs(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("cdf_us = %v, want [1 2]", got)
	}

	gotRuns, gotCDF := plotRecordsFromMessage(pd)
	if len(gotRuns) != 1 || gotRuns[0].base != "run1" || gotRuns[0].throughput != 30 {
		t.Errorf("round-tripped runs = %+v", gotRuns)
	}
	if gotRuns[0].samples == nil || *gotRuns[0].samples != 42 {
		t.Errorf("round-tripped samples = %v, want 42", gotRuns[0].samples)
	}
	if len(gotCDF) != 2 {
		t.Fatalf("round-tripped cdf rows = %d, want 2", len(gotCDF))
	}
	if gotCDF[0].prob != 0 || gotCDF[0].cdfUS != 1 || gotCDF[0].node != "bb1:9000" {
		t.Errorf("round-tripped cdf[0] = %+v", gotCDF[0])
	}
	if gotCDF[1].prob != 1 || gotCDF[1].cdfUS != 2 {
		t.Errorf("round-tripped cdf[1] = %+v", gotCDF[1])
	}
}

// TestBuildPlotDataOmitsSummaryWithoutSamples verifies that a benchmark with
// no latency data round-trips as a nil summary, not a spurious all-zero one,
// preserving the flat record's nil-pointer distinction.
func TestBuildPlotDataOmitsSummaryWithoutSamples(t *testing.T) {
	runs := []plotRunRecord{{base: "run1", Benchmark: "Q", throughput: 5}}
	pd := buildPlotData(runs, nil)
	if s := pd.GetRuns()[0].GetBenchmarks()[0].GetSummary(); s != nil {
		t.Errorf("summary = %+v, want nil", s)
	}
	gotRuns, _ := plotRecordsFromMessage(pd)
	if gotRuns[0].meanUS != nil || gotRuns[0].samples != nil {
		t.Errorf("meanUS = %v samples = %v, want nil", gotRuns[0].meanUS, gotRuns[0].samples)
	}
}

// TestPlotDataMarshalRoundTrip verifies that the message buildPlotData
// produces survives a protobuf marshal/unmarshal cycle unchanged.
func TestPlotDataMarshalRoundTrip(t *testing.T) {
	runs := []plotRunRecord{{
		base: "run1", Benchmark: "Q", StreamMode: "dual", throughput: 5,
	}}
	want := buildPlotData(runs, nil)
	data, err := proto.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := &benchkit.PlotData{}
	if err := proto.Unmarshal(data, got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !proto.Equal(want, got) {
		t.Errorf("round-tripped message differs:\nwant %v\ngot  %v", want, got)
	}
}

// TestWriteCompactPlotDataWritesBinpb verifies that writeCompactPlotData
// writes the normalized plotdata.binpb instead of the old runs.csv and
// node_cdf.csv pair, and that readPlotData decodes it back into the same
// records collectPlotData produced.
func TestWriteCompactPlotDataWritesBinpb(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N1_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, n, resultExt)})
	writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 10,
		Latencies:  []int64{1000, 2000},
	}.Build())

	wantRuns, wantCDF, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}

	if err := writeCompactPlotData(dir); err != nil {
		t.Fatalf("writeCompactPlotData: %v", err)
	}
	plotdataDir := filepath.Join(dir, plotDataDir)
	if _, err := os.Stat(filepath.Join(plotdataDir, plotDataFile)); err != nil {
		t.Fatalf("plotdata.binpb missing: %v", err)
	}
	for _, legacy := range []string{"runs.csv", "node_cdf.csv"} {
		if _, err := os.Stat(filepath.Join(plotdataDir, legacy)); !os.IsNotExist(err) {
			t.Errorf("legacy %s should not be written, stat err = %v", legacy, err)
		}
	}

	pd, err := readPlotData(dir)
	if err != nil {
		t.Fatalf("readPlotData: %v", err)
	}
	gotRuns, gotCDF := plotRecordsFromMessage(pd)
	if len(gotRuns) != len(wantRuns) || len(gotCDF) != len(wantCDF) {
		t.Fatalf("round-tripped %d runs / %d cdf rows, want %d / %d",
			len(gotRuns), len(gotCDF), len(wantRuns), len(wantCDF))
	}
	if gotRuns[0].base != wantRuns[0].base || gotRuns[0].throughput != wantRuns[0].throughput {
		t.Errorf("run mismatch: got %+v, want %+v", gotRuns[0], wantRuns[0])
	}
	if gotCDF[0].cdfUS != wantCDF[0].cdfUS || gotCDF[len(gotCDF)-1].cdfUS != wantCDF[len(wantCDF)-1].cdfUS {
		t.Errorf("cdf mismatch: got first/last %v/%v, want %v/%v",
			gotCDF[0].cdfUS, gotCDF[len(gotCDF)-1].cdfUS, wantCDF[0].cdfUS, wantCDF[len(wantCDF)-1].cdfUS)
	}
}

// TestWritePlotNodesCSV verifies that the exported per-node CSV collapses
// each node's CDF into a single row with a space-joined vector column,
// instead of one row per CDF point.
func TestWritePlotNodesCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nodes.csv")
	rows := []plotNodeCDFRecord{
		{
			base: "run1", Benchmark: "Q", node: "bb1:9000", throughput: 10,
			meanUS: 100, p50US: 90, p95US: 150, p99US: 200, samples: 2, prob: 0, cdfUS: 1,
		},
		{
			base: "run1", Benchmark: "Q", node: "bb1:9000", throughput: 10,
			meanUS: 100, p50US: 90, p95US: 150, p99US: 200, samples: 2, prob: 0.5, cdfUS: 1.5,
		},
		{
			base: "run1", Benchmark: "Q", node: "bb1:9000", throughput: 10,
			meanUS: 100, p50US: 90, p95US: 150, p99US: 200, samples: 2, prob: 1, cdfUS: 2,
		},
		{
			base: "run1", Benchmark: "Q", node: "bb2:9000", throughput: 20,
			meanUS: 110, p50US: 95, p95US: 160, p99US: 210, samples: 3, prob: 0, cdfUS: 3,
		},
	}
	if err := writePlotNodesCSV(path, plotNodeRows(buildPlotData([]plotRunRecord{{base: "run1", Benchmark: "Q"}}, rows))); err != nil {
		t.Fatalf("writePlotNodesCSV: %v", err)
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
		t.Fatalf("rows = %d incl header, want 3 (header + one per node)", len(recs))
	}
	header := recs[0]
	nodeCol, cdfCol := slices.Index(header, "node"), slices.Index(header, "cdf_us")
	if nodeCol < 0 || cdfCol < 0 {
		t.Fatalf("header %v missing node or cdf_us column", header)
	}
	if recs[1][nodeCol] != "bb1:9000" {
		t.Fatalf("row 1 node = %q, want bb1:9000", recs[1][nodeCol])
	}
	if want := "1 1.5 2"; recs[1][cdfCol] != want {
		t.Errorf("row 1 cdf_us = %q, want %q", recs[1][cdfCol], want)
	}
	if recs[2][nodeCol] != "bb2:9000" {
		t.Fatalf("row 2 node = %q, want bb2:9000", recs[2][nodeCol])
	}
	if want := "3"; recs[2][cdfCol] != want {
		t.Errorf("row 2 cdf_us = %q, want %q", recs[2][cdfCol], want)
	}
}

// TestExportPlotCSV verifies that exportPlotCSV regenerates plotdata/runs.csv
// and plotdata/nodes.csv from a collected directory's plotdata.binpb.
func TestExportPlotCSV(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N1_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, n, resultExt)})
	writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 1, 1, 0, 0),
		Throughput: 10,
		Latencies:  []int64{1000, 2000},
	}.Build())
	if err := writeCompactPlotData(dir); err != nil {
		t.Fatalf("writeCompactPlotData: %v", err)
	}

	if err := exportPlotCSV(dir); err != nil {
		t.Fatalf("exportPlotCSV: %v", err)
	}
	plotdataDir := filepath.Join(dir, plotDataDir)
	runs, err := readPlotRunsCSV(filepath.Join(plotdataDir, "runs.csv"))
	if err != nil {
		t.Fatalf("readPlotRunsCSV: %v", err)
	}
	if len(runs) != 1 || runs[0].base != base || runs[0].throughput != 10 {
		t.Errorf("exported runs = %+v", runs)
	}
	f, err := os.Open(filepath.Join(plotdataDir, "nodes.csv"))
	if err != nil {
		t.Fatalf("open nodes.csv: %v", err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("nodes.csv rows = %d incl header, want 2 (header + one node)", len(recs))
	}
	nodeCol, cdfCol := slices.Index(recs[0], "node"), slices.Index(recs[0], "cdf_us")
	if recs[1][nodeCol] != "bb1:9000" {
		t.Errorf("node = %q, want bb1:9000", recs[1][nodeCol])
	}
	if got := len(strings.Fields(recs[1][cdfCol])); got != cdfPoints {
		t.Errorf("cdf_us has %d points, want %d", got, cdfPoints)
	}
}

func assertFloatPtr(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %v", name, want)
	}
	if math.Abs(*got-want) > 1e-9 {
		t.Fatalf("%s = %v, want %v", name, *got, want)
	}
}

// TestPlotDataRoundTripPreservesBufferSizes verifies that the buffer capacities
// survive the reduction into plot data and the read back out. They are what
// separates the arms of a buffer sweep, so losing them here collapses every arm
// into one aggregate row without any error.
func TestPlotDataRoundTripPreservesBufferSizes(t *testing.T) {
	runs := []plotRunRecord{
		{base: "s_Q_N3_W1_P0_RB0_Sdual_r1", Benchmark: "Q", Nodes: 3, Workers: 1, StreamMode: "dual", throughput: 100},
		{base: "s_Q_N3_W1_P0_RB16_Sdual_r1", Benchmark: "Q", Nodes: 3, Workers: 1, StreamMode: "dual", RecvBuffer: 16, throughput: 200},
		{base: "s_Q_N3_W1_P0_SB64_Sdual_r1", Benchmark: "Q", Nodes: 3, Workers: 1, StreamMode: "dual", SendBuffer: 64, throughput: 300},
	}
	pd := buildPlotData(runs, nil)
	got := map[[2]int]bool{}
	for _, r := range pd.GetRuns() {
		for _, b := range r.GetBenchmarks() {
			c := b.GetConfig()
			got[[2]int{int(c.GetSendBuffer()), int(c.GetRecvBuffer())}] = true
		}
	}
	for _, want := range [][2]int{{0, 0}, {0, 16}, {64, 0}} {
		if !got[want] {
			t.Errorf("plot data lost buffer capacities send=%d recv=%d; got %v", want[0], want[1], got)
		}
	}

	// The arms must stay distinct all the way into the aggregate rows.
	back, _ := plotRecordsFromMessage(pd)
	if n := len(aggregateReps(back, false)); n != len(runs) {
		t.Errorf("aggregated to %d rows, want %d: buffer arms were folded together", n, len(runs))
	}
}

// TestCollectPlotDataPreservesBufferSizes verifies that the buffer capacities
// survive the reduction from raw result files, the hop where the report pipeline
// actually starts. A round trip that begins at a plotRunRecord cannot see a loss
// here, which is how a collapsed buffer sweep reached a report unnoticed.
func TestCollectPlotDataPreservesBufferSizes(t *testing.T) {
	dir := t.TempDir()
	arms := []struct {
		base       string
		recvBuffer int32
		throughput float64
	}{
		{"s_QuorumCall_N3_W1_P0_RB0_Sdual_r1", 0, 100},
		{"s_QuorumCall_N3_W1_P0_RB16_Sdual_r1", 16, 200},
		{"s_QuorumCall_N3_W1_P0_RB256_Sdual_r1", 256, 300},
	}
	for _, a := range arms {
		results := []*benchkit.Result{
			benchkit.Result_builder{
				Config: benchkit.RunConfig_builder{
					Name: "QuorumCall", NumNodes: 3, Workers: 1,
					RecvBuffer: a.recvBuffer, StreamMode: "dual",
				}.Build(),
				Throughput: a.throughput,
				TotalOps:   1000,
			}.Build(),
		}
		path := filepath.Join(dir, a.base+"_n1_9000"+resultExt)
		if err := benchkit.WriteLabeledReport(results, "n1", path); err != nil {
			t.Fatalf("write result: %v", err)
		}
		m := runManifest{
			Benchmark: "QuorumCall", Nodes: 3, Workers: 1, StreamMode: "dual",
			Rep:   1,
			Label: "s", Status: runStatusSucceeded,
			Files: []string{a.base + "_n1_9000" + resultExt},
		}
		blob, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatalf("marshal manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, a.base+manifestSuffix), blob, 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}

	runs, _, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	got := map[int]bool{}
	for _, r := range runs {
		got[r.RecvBuffer] = true
	}
	for _, want := range []int{0, 16, 256} {
		if !got[want] {
			t.Errorf("reduction lost recv buffer %d; got %v", want, got)
		}
	}
	if n := len(aggregateReps(runs, false)); n != len(arms) {
		t.Errorf("aggregated to %d rows, want %d: the arms were folded together", n, len(arms))
	}
}

// TestCollectPlotDataStampsManifestCustom verifies that the reduction stamps the
// custom dimension values from the run manifest into each result, replacing
// whatever the binary recorded, so a binary may leave them out and the values
// survive the trip through plotdata.binpb.
func TestCollectPlotDataStampsManifestCustom(t *testing.T) {
	dir := t.TempDir()
	customDims := benchkit.NewCustomDimensions(map[string]string{"d": "20", "ready": "tree"})
	spec := runSpec{
		Benchmark: "Idea0", Nodes: 3, Workers: 1, StreamMode: "dual", Custom: customDims, Rep: 1,
	}
	base := runBase("s", spec)
	file := base + "_n1_9000" + resultExt
	results := []*benchkit.Result{benchkit.Result_builder{
		Config: benchkit.RunConfig_builder{
			Name: "Idea0", NumNodes: 3, Workers: 1, StreamMode: "dual",
			CustomDimensions: map[string]string{"d": "stale"},
		}.Build(),
		Throughput: 100, TotalOps: 1000,
	}.Build()}
	if err := benchkit.WriteLabeledReport(results, "n1", filepath.Join(dir, file)); err != nil {
		t.Fatal(err)
	}
	m := runManifest{runSpec: spec, Label: "s", Status: runStatusSucceeded, Files: []string{file}}
	blob, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+manifestSuffix), blob, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeCompactPlotData(dir); err != nil {
		t.Fatal(err)
	}
	pd, err := readPlotData(dir)
	if err != nil {
		t.Fatal(err)
	}
	runs, _ := plotRecordsFromMessage(pd)
	if len(runs) != 1 || runs[0].Custom != customDims {
		t.Fatalf("runs = %+v, want one run with custom dimensions %q", runs, customDims)
	}
}

// plotSeriesMetric returns a client-measured series Metric with the given
// samples in nanoseconds.
func plotSeriesMetric(name string, samples ...int64) *benchkit.Metric {
	return benchkit.Metric_builder{
		Name: name, Unit: "ns",
		Distribution: benchkit.Distribution_builder{Samples: samples}.Build(),
	}.Build()
}

// plotHDRSeriesMetric returns a client-measured series Metric recorded as an
// hdr histogram with count samples of value v nanoseconds.
func plotHDRSeriesMetric(name string, v int64, count uint64) *benchkit.Metric {
	return benchkit.Metric_builder{
		Name: name, Unit: "ns",
		Distribution: benchkit.Distribution_builder{
			Histogram: benchkit.LatencyHistogram_builder{Value: []int64{v}, Count: []uint64{count}}.Build(),
		}.Build(),
	}.Build()
}

// plotCounterMetric returns a counter Metric of one node with the given value.
func plotCounterMetric(name, unit string, v uint64) *benchkit.Metric {
	return benchkit.Metric_builder{
		Name: name, Unit: unit,
		Counter: benchkit.CounterValue_builder{Sum: v, Max: v, Nodes: 1}.Build(),
	}.Build()
}

// metricSummaryByName returns the metric summary with the given name, or nil.
func metricSummaryByName(metrics []*benchkit.MetricSummary, name string) *benchkit.MetricSummary {
	for _, m := range metrics {
		if m.GetName() == name {
			return m
		}
	}
	return nil
}

// TestPlotDataMetrics verifies that collectPlotData reduces each node's named
// metrics and merges them across the nodes of a benchmark: series merge their
// distributions, and counters add their sums and keep the largest value. A node
// that recorded metrics but no primary latency keeps its metrics, gets a
// PlotNode of its own, and adds no CDF rows. The metrics survive buildPlotData,
// a marshal round trip, and plotRecordsFromMessage.
func TestPlotDataMetrics(t *testing.T) {
	dir := t.TempDir()
	base := "e1_Q_N2_W1_P0"
	n1 := nodeAssignment{host: "bb1", port: 9000}
	n2 := nodeAssignment{host: "bb2", port: 9000}
	writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{
		resultFilename(base, n1, resultExt),
		resultFilename(base, n2, resultExt),
	})
	writePlotReport(t, dir, base, n1, "bb1:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 2, 1, 0, 0),
		Throughput: 10,
		Latencies:  []int64{1000, 2000},
		Metrics: []*benchkit.Metric{
			plotSeriesMetric("encode", 1000, 3000),
			plotCounterMetric("uploaded", "B", 70),
		},
	}.Build())
	writePlotReport(t, dir, base, n2, "bb2:9000", benchkit.Result_builder{
		Config:     plotRunConfig("Q", 2, 1, 0, 0),
		Throughput: 20,
		Metrics: []*benchkit.Metric{
			plotSeriesMetric("encode", 5000),
			plotCounterMetric("uploaded", "B", 30),
		},
	}.Build())

	runs, cdf, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if len(cdf) != cdfPoints {
		t.Errorf("cdf rows = %d, want %d (only bb1 recorded latency)", len(cdf), cdfPoints)
	}
	checkMetrics := func(t *testing.T, row plotRunRecord) {
		t.Helper()
		encode := metricSummaryByName(row.metrics, "encode")
		if got := encode.GetDistribution(); got.GetSamples() != 3 || got.GetP50Us() != 3 || got.GetMeanUs() != 3 || encode.GetUnit() != "ns" {
			t.Errorf("encode = %v, want 3 samples with mean and p50 3 µs in \"ns\"", encode)
		}
		uploaded := metricSummaryByName(row.metrics, "uploaded").GetCounter()
		if uploaded.GetSum() != 100 || uploaded.GetMax() != 70 || uploaded.GetNodes() != 2 {
			t.Errorf("uploaded = (sum %d, max %d, nodes %d), want (100, 70, 2)", uploaded.GetSum(), uploaded.GetMax(), uploaded.GetNodes())
		}
		if len(row.nodeMetrics) != 2 {
			t.Fatalf("node metrics = %d nodes, want 2", len(row.nodeMetrics))
		}
		bb2 := row.nodeMetrics[1]
		if bb2.node != "bb2:9000" || bb2.throughput != 20 {
			t.Errorf("node metrics[1] = (%q, %v), want (bb2:9000, 20)", bb2.node, bb2.throughput)
		}
		if got := metricSummaryByName(bb2.metrics, "uploaded").GetCounter().GetSum(); got != 30 {
			t.Errorf("bb2 uploaded = %d, want 30", got)
		}
		if got := metricSummaryByName(bb2.metrics, "encode").GetDistribution().GetP50Us(); got != 5 {
			t.Errorf("bb2 encode p50 = %v µs, want 5", got)
		}
	}
	checkMetrics(t, runs[0])

	pd := buildPlotData(runs, cdf)
	bench := pd.GetRuns()[0].GetBenchmarks()[0]
	if got := len(bench.GetMetrics()); got != 2 {
		t.Errorf("benchmark metrics = %d, want 2", got)
	}
	nodes := bench.GetNodes()
	if len(nodes) != 2 {
		t.Fatalf("plot nodes = %d, want 2", len(nodes))
	}
	if nodes[1].GetNode() != "bb2:9000" || nodes[1].GetSummary() != nil || len(nodes[1].GetCdfUs()) != 0 {
		t.Errorf("metric-only node = %v, want bb2:9000 without summary or CDF", nodes[1])
	}
	if nodes[1].GetThroughput() != 20 || len(nodes[1].GetMetrics()) != 2 {
		t.Errorf("metric-only node = %v, want throughput 20 and 2 metrics", nodes[1])
	}

	data, err := proto.Marshal(pd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	back := &benchkit.PlotData{}
	if err := proto.Unmarshal(data, back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !proto.Equal(pd, back) {
		t.Errorf("round-tripped message differs:\nwant %v\ngot  %v", pd, back)
	}
	gotRuns, gotCDF := plotRecordsFromMessage(back)
	if len(gotCDF) != cdfPoints {
		t.Errorf("round-tripped cdf rows = %d, want %d", len(gotCDF), cdfPoints)
	}
	checkMetrics(t, gotRuns[0])
}

// TestPlotDataMetricStatsModeAcrossRuns verifies that a series recorded as
// exact samples in one repetition and as an hdr histogram in another passes
// the collection, with each run's own summary in the metric's column.
func TestPlotDataMetricStatsModeAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	n1 := nodeAssignment{host: "bb1", port: 9000}
	for _, run := range []struct {
		base string
		rep  int
		m    *benchkit.Metric
	}{
		{"e1_r1", 1, plotSeriesMetric("encode", 1000)},
		{"e1_r2", 2, plotHDRSeriesMetric("encode", 2000, 2)},
	} {
		writePlotManifest(t, dir, run.base, runStatusSucceeded, run.rep, "", []string{resultFilename(run.base, n1, resultExt)})
		writePlotReport(t, dir, run.base, n1, "bb1:9000", benchkit.Result_builder{
			Config: plotRunConfig("Q", 1, 1, 0, 0), Throughput: 1, Metrics: []*benchkit.Metric{run.m},
		}.Build())
	}
	runs, _, _, err := collectPlotData(dir)
	if err != nil {
		t.Fatalf("collectPlotData: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
	want := map[string]uint64{"e1_r1": 1, "e1_r2": 2}
	for _, row := range runs {
		if got := metricSummaryByName(row.metrics, "encode").GetDistribution().GetSamples(); got != want[row.base] {
			t.Errorf("%s encode samples = %d, want %d", row.base, got, want[row.base])
		}
	}
}

// TestPlotDataMetricMismatch verifies that a named metric whose unit or kind
// differs between the nodes of a run, or between the repetitions of a
// configuration, fails the collection with an error naming the run, the
// benchmark, the metric, and both units. A series whose stats mode differs
// between the nodes of a run also fails it.
func TestPlotDataMetricMismatch(t *testing.T) {
	n1 := nodeAssignment{host: "bb1", port: 9000}
	n2 := nodeAssignment{host: "bb2", port: 9000}
	tests := []struct {
		name string
		// metrics of node 1 and node 2 in run r1, and of node 1 in run r2
		r1n1, r1n2, r2n1 *benchkit.Metric
		want             []string
	}{
		{
			name: "AcrossNodes",
			r1n1: plotCounterMetric("uploaded", "B", 1), r1n2: plotCounterMetric("uploaded", "KiB", 1),
			want: []string{"e1_r1", "benchmark Q", `"uploaded"`, `"B"`, `"KiB"`},
		},
		{
			name: "AcrossReps",
			r1n1: plotCounterMetric("uploaded", "B", 1), r2n1: plotSeriesMetric("uploaded", 1),
			want: []string{"e1_r2", "benchmark Q", `"uploaded"`, `"B"`, `"ns"`},
		},
		{
			name: "StatsModeAcrossNodes",
			r1n1: plotSeriesMetric("encode", 1000), r1n2: plotHDRSeriesMetric("encode", 2000, 2),
			want: []string{"e1_r1", "benchmark Q", `"encode"`, `series in "ns" (exact)`, `series in "ns" (hdr)`},
		},
		{
			name: "StatsModeAcrossNodesHDRFirst",
			r1n1: plotHDRSeriesMetric("encode", 2000, 2), r1n2: plotSeriesMetric("encode", 1000),
			want: []string{"e1_r1", "benchmark Q", `"encode"`, `series in "ns" (hdr)`, `series in "ns" (exact)`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, run := range []struct {
				base       string
				rep        int
				m1, m2     *benchkit.Metric
				nodesFiles []nodeAssignment
			}{
				{"e1_r1", 1, tt.r1n1, tt.r1n2, []nodeAssignment{n1, n2}},
				{"e1_r2", 2, tt.r2n1, nil, []nodeAssignment{n1, n2}},
			} {
				var files []string
				for _, n := range run.nodesFiles {
					files = append(files, resultFilename(run.base, n, resultExt))
				}
				writePlotManifest(t, dir, run.base, runStatusSucceeded, run.rep, "", files)
				for i, m := range []*benchkit.Metric{run.m1, run.m2} {
					r := benchkit.Result_builder{Config: plotRunConfig("Q", 2, 1, 0, 0), Throughput: 1}.Build()
					if m != nil {
						r.SetMetrics([]*benchkit.Metric{m})
					}
					writePlotReport(t, dir, run.base, run.nodesFiles[i], run.nodesFiles[i].host, r)
				}
			}
			_, _, _, err := collectPlotData(dir)
			if err == nil {
				t.Fatal("collectPlotData: got nil error, want a metric mismatch error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %s", err, w)
				}
			}
		})
	}
}

// TestWritePlotNodesCSVMetrics verifies that nodes.csv appends one column
// group per named metric, sorted by name: a series gets its mean and
// percentiles in microseconds and a counter its sum and max. A node that
// recorded metrics but no latency gets a row with empty latency fields, and a
// node without a metric gets empty metric fields.
func TestWritePlotNodesCSVMetrics(t *testing.T) {
	meanUS, samples := 2.0, uint64(2)
	runs := []plotRunRecord{{
		base: "run1", Benchmark: "Q", throughput: 30,
		meanUS: &meanUS, p50US: &meanUS, p95US: &meanUS, p99US: &meanUS, samples: &samples,
		nodeMetrics: []plotNodeMetrics{
			{node: "bb2:9000", throughput: 20, metrics: []*benchkit.MetricSummary{
				seriesSummary("encode", 5), counterSummary("uploaded", "B", 30, 30),
			}},
		},
	}}
	cdf := []plotNodeCDFRecord{
		{base: "run1", Benchmark: "Q", node: "bb1:9000", throughput: 10,
			meanUS: 2, p50US: 2, p95US: 2, p99US: 2, samples: 2, prob: 0, cdfUS: 1},
		{base: "run1", Benchmark: "Q", node: "bb1:9000", throughput: 10,
			meanUS: 2, p50US: 2, p95US: 2, p99US: 2, samples: 2, prob: 1, cdfUS: 3},
	}
	path := filepath.Join(t.TempDir(), "nodes.csv")
	if err := writePlotNodesCSV(path, plotNodeRows(buildPlotData(runs, cdf))); err != nil {
		t.Fatalf("writePlotNodesCSV: %v", err)
	}
	recs := readCSVRecords(t, path)
	header := recs[0]
	first := slices.Index(header, "cdf_us") + 1
	wantCols := []string{"encode.mean_us", "encode.p50_us", "encode.p95_us", "encode.p99_us", "uploaded.sum", "uploaded.max"}
	if got := header[first:]; !slices.Equal(got, wantCols) {
		t.Fatalf("metric columns = %v, want %v", got, wantCols)
	}
	if len(recs) != 3 {
		t.Fatalf("rows = %d incl header, want 3", len(recs))
	}
	col := func(rec []string, name string) string { return rec[slices.Index(header, name)] }
	bb1, bb2 := recs[1], recs[2]
	if col(bb1, "node") != "bb1:9000" || col(bb1, "cdf_us") != "1 3" || col(bb1, "encode.p50_us") != "" {
		t.Errorf("bb1 row = %v, want its CDF and empty metric fields", bb1)
	}
	for name, want := range map[string]string{
		"node": "bb2:9000", "throughput": "20", "mean_us": "", "samples": "", "cdf_us": "",
		"encode.p50_us": "5", "uploaded.sum": "30", "uploaded.max": "30",
	} {
		if got := col(bb2, name); got != want {
			t.Errorf("bb2 %s = %q, want %q", name, got, want)
		}
	}
}

// TestWritePlotNodesCSVLargeCounter verifies that nodes.csv writes the sum and
// max of a counter as exact integers, also above 2^53, where a float64 rounds.
func TestWritePlotNodesCSVLargeCounter(t *testing.T) {
	const large = 9007199254740993 // 2^53 + 1
	runs := []plotRunRecord{{
		base: "run1", Benchmark: "Q", throughput: 1,
		nodeMetrics: []plotNodeMetrics{
			{node: "bb1:9000", throughput: 1, metrics: []*benchkit.MetricSummary{
				counterSummary("uploaded", "B", large, large-1),
			}},
		},
	}}
	path := filepath.Join(t.TempDir(), "nodes.csv")
	if err := writePlotNodesCSV(path, plotNodeRows(buildPlotData(runs, nil))); err != nil {
		t.Fatalf("writePlotNodesCSV: %v", err)
	}
	recs := readCSVRecords(t, path)
	if len(recs) != 2 {
		t.Fatalf("rows = %d incl header, want 2", len(recs))
	}
	header := recs[0]
	for name, want := range map[string]string{"uploaded.sum": "9007199254740993", "uploaded.max": "9007199254740992"} {
		if got := recs[1][slices.Index(header, name)]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}
