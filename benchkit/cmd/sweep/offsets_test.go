package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectOffsets(t *testing.T) {
	dir := t.TempDir()
	log := "" +
		"noise line\n" +
		"[offsets node 15 (10.0.0.1:9000)] peer 2: before=-297µs after=-301µs drift=-3µs\n" +
		"[offsets node 15 (10.0.0.1:9000)] peer 15: before=0µs after=0µs drift=0µs\n" + // self, skipped
		"[offsets node 15 (10.0.0.1:9000)] peer 3: before=1ms after=900µs drift=100µs\n"
	if err := os.WriteFile(filepath.Join(dir, "run_Q_N15_W8.log"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	samples, err := collectOffsets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2 (self peer skipped)", len(samples))
	}
	// before=-297µs -> |offset| 297; before=1ms -> 1000µs; drift 100µs.
	if samples[0].offsetUS != 297 || samples[0].nodeCount != 15 {
		t.Errorf("sample0 = %+v, want offset 297 nodes 15", samples[0])
	}
	if samples[1].offsetUS != 1000 || samples[1].driftUS != 100 {
		t.Errorf("sample1 = %+v, want offset 1000 drift 100", samples[1])
	}
}

func TestOffsetCDFRows(t *testing.T) {
	samples := []offsetSample{
		{nodeCount: 9, offsetUS: 10, driftUS: 1},
		{nodeCount: 9, offsetUS: 20, driftUS: 2},
		{nodeCount: 15, offsetUS: 100, driftUS: 5},
	}
	rows := offsetCDFRows(samples, 10)
	// Groups: all, N9, N15 for each of offset+drift, each with 11 points.
	if len(rows) != 2*3*11 {
		t.Fatalf("rows = %d, want %d", len(rows), 2*3*11)
	}
	// Every CDF ends at 1.0 and starts non-empty; check the "all" offset tail.
	var last offsetCDFRecord
	for _, r := range rows {
		if r.metric == "offset" && r.group == "all" {
			last = r
		}
	}
	if last.cdf != 1.0 || last.valueUS != 100 {
		t.Errorf("all offset tail = %+v, want cdf 1.0 value 100", last)
	}

	path := filepath.Join(t.TempDir(), "offsets.csv")
	if err := writeOffsetsCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestRunStatusRows(t *testing.T) {
	dir := t.TempDir()
	n1 := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, "r_Q_N3_r1", runStatusSucceeded, 1, "", []string{resultFilename("r_Q_N3_r1", n1, resultExt)})
	writePlotManifest(t, dir, "r_Q_N3_r2", runStatusDegraded, 2, "", []string{resultFilename("r_Q_N3_r2", n1, resultExt)})
	writePlotManifest(t, dir, "r_Q_N3_r3", runStatusFailed, 3, "", []string{resultFilename("r_Q_N3_r3", n1, resultExt)})

	rows, err := runStatusRows(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 node count", len(rows))
	}
	r := rows[0]
	if r.total != 3 || r.succeeded != 1 || r.degraded != 1 || r.failed != 1 || r.completed != 2 {
		t.Errorf("row = %+v, want total3 succ1 deg1 fail1 completed2", r)
	}
	if !anyDegradedOrFailed(rows) {
		t.Error("anyDegradedOrFailed = false, want true")
	}

	path := filepath.Join(dir, "run_status.csv")
	if err := writeRunStatusCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
