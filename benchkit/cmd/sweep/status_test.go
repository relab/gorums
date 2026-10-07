package main

import (
	"os"
	"path/filepath"
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
