package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/relab/gorums/benchkit"
	"google.golang.org/protobuf/encoding/protojson"
)

// TestConvertBinaryResults verifies that convertBinaryResults decodes a collected
// .binpb file and writes a protojson .json sibling carrying the key result fields
// (name, throughput, latencies). The binary fixture is built via the shared
// buildBinaryResultFile helper, exercising the generated decode + protojson
// marshal path.
func TestConvertBinaryResults(t *testing.T) {
	wantName := "SymmetricQuorumCall"
	wantThroughput := 12345.6
	wantLatencies := []int64{100, 200, 300}

	dir := t.TempDir()
	base := "rate-test_SymmetricQuorumCall_N1_W1_P0"
	node := nodeAssignment{host: "bb1", port: 9000}

	binPath := filepath.Join(dir, resultFilename(base, node, ".binpb"))
	if err := os.WriteFile(binPath, buildBinaryResultFile(t, wantName, wantThroughput, wantLatencies), 0o644); err != nil {
		t.Fatalf("write binary fixture: %v", err)
	}

	convertBinaryResults(dir, base, []nodeAssignment{node})

	jsonPath := filepath.Join(dir, resultFilename(base, node, ".json"))
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read converted json: %v", err)
	}

	var res benchkit.Report
	if err := protojson.Unmarshal(data, &res); err != nil {
		t.Fatalf("unmarshal converted json: %v", err)
	}
	if len(res.GetResults()) != 1 {
		t.Fatalf("results count = %d, want 1", len(res.GetResults()))
	}
	r := res.GetResults()[0]
	if r.GetConfig().GetName() != wantName {
		t.Errorf("name = %q, want %q", r.GetConfig().GetName(), wantName)
	}
	if r.GetThroughput() != wantThroughput {
		t.Errorf("throughput = %v, want %v", r.GetThroughput(), wantThroughput)
	}
	if got := r.GetLatencies(); len(got) != len(wantLatencies) {
		t.Fatalf("latencies count = %d, want %d", len(got), len(wantLatencies))
	} else {
		for i, lat := range wantLatencies {
			if got[i] != lat {
				t.Errorf("latencies[%d] = %d, want %d", i, got[i], lat)
			}
		}
	}
}

// TestConvertDirBinaryResults verifies that convertDirBinaryResults converts
// every .binpb in a directory to its .json sibling, returns the count of files
// converted, and skips undecodable files without failing the whole batch (so a
// partially downloaded driver run still converts whatever it has).
func TestConvertDirBinaryResults(t *testing.T) {
	dir := t.TempDir()
	base := "e1_SymmetricQuorumCall_N2_W1_P0"
	good := []nodeAssignment{{host: "bb1", port: 9000}, {host: "bb2", port: 9000}}
	for _, node := range good {
		binPath := filepath.Join(dir, resultFilename(base, node, resultExt))
		if err := os.WriteFile(binPath, buildBinaryResultFile(t, "SymmetricQuorumCall", 1, []int64{1, 2}), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	// A .binpb that is not a benchkit file must be skipped, not counted.
	badPath := filepath.Join(dir, resultFilename(base, nodeAssignment{host: "bb3", port: 9000}, resultExt))
	if err := os.WriteFile(badPath, []byte("not a benchkit file"), 0o644); err != nil {
		t.Fatalf("write bad fixture: %v", err)
	}

	n, err := convertDirBinaryResults(dir)
	if err != nil {
		t.Fatalf("convertDirBinaryResults: %v", err)
	}
	if n != len(good) {
		t.Errorf("converted count = %d, want %d", n, len(good))
	}
	for _, node := range good {
		jsonPath := filepath.Join(dir, resultFilename(base, node, ".json"))
		if _, err := os.Stat(jsonPath); err != nil {
			t.Errorf("missing converted json for %s: %v", node.host, err)
		}
	}
	badJSON := filepath.Join(dir, resultFilename(base, nodeAssignment{host: "bb3", port: 9000}, ".json"))
	if _, err := os.Stat(badJSON); err == nil {
		t.Errorf("undecodable file should not produce json: %s", badJSON)
	}
}
