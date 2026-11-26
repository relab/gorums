package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if !anyUnsuccessful(rows) {
		t.Error("anyUnsuccessful = false, want true")
	}

	path := filepath.Join(dir, "run_status.csv")
	if err := writeRunStatusCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

// TestRunStatusRowsSkipped verifies that skipped runs are counted apart from
// failed ones and that they make the run-status table worth showing.
func TestRunStatusRowsSkipped(t *testing.T) {
	dir := t.TempDir()
	n1 := nodeAssignment{host: "bb1", port: 9000}
	writePlotManifest(t, dir, "r_Q_N3_r1", runStatusSucceeded, 1, "", []string{resultFilename("r_Q_N3_r1", n1, resultExt)})
	writePlotManifest(t, dir, "r_Q_N3_r2", runStatusSkipped, 2, "", []string{resultFilename("r_Q_N3_r2", n1, resultExt)})
	rows, err := runStatusRows(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r := rows[0]; r.total != 2 || r.skipped != 1 || r.failed != 0 || r.completed != 1 {
		t.Errorf("row = %+v, want total2 skipped1 fail0 completed1", r)
	}
	if !anyUnsuccessful(rows) {
		t.Error("anyUnsuccessful = false, want true for a skipped run")
	}
	path := filepath.Join(dir, "run_status.csv")
	if err := writeRunStatusCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if header, _, _ := strings.Cut(string(data), "\n"); !strings.Contains(header, "skipped") {
		t.Errorf("run_status.csv header = %q, want a skipped column", header)
	}
}
