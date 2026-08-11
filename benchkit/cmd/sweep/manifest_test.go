package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums/benchkit"
)

// TestWriteManifest verifies that writeManifest records the run configuration
// and the expected per-node result files under <base>.manifest.json.
func TestWriteManifest(t *testing.T) {
	dir := t.TempDir()
	cfg := &config{sweepLabel: "nscale", duration: 10 * time.Second, trim: time.Second}
	p := runSpec{
		Dimensions: benchkit.Dimensions{
			Benchmark: "Symmetric", Nodes: 2, Workers: 4, Payload: 16,
			Rate: 1000, StreamMode: "dedup",
		},
		Rep: 2,
	}
	nodes := []nodeAssignment{
		{host: "bb1", peerHost: "152.94.162.21", port: 9000},
		{host: "bb2", peerHost: "152.94.162.11", port: 9000},
	}
	const base = "nscale_Symmetric_N2_W4_P16_R1000_r2"

	writeManifest(dir, base, p, nodes, cfg, "abc123", "/tmp/bench")

	data, err := os.ReadFile(filepath.Join(dir, base+manifestSuffix))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m runManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	want := runManifest{
		runSpec: runSpec{
			Dimensions: benchkit.Dimensions{
				Benchmark: "Symmetric", Nodes: 2, Workers: 4, Payload: 16,
				Rate: 1000, StreamMode: "dedup",
			},
			Rep: 2,
		},
		Label:     "nscale",
		Duration:  "10s",
		Trim:      "1s",
		Timestamp: m.Timestamp, // checked separately below
		Status:    runStatusStarted,
		GitSHA:    "abc123",
		Binary:    "/tmp/bench",
		Hosts:     []string{"bb1:9000", "bb2:9000"},
		Files: []string{
			base + "_bb1_9000.binpb",
			base + "_bb2_9000.binpb",
		},
		NodeMap: []nodeMapEntry{
			{
				ID:          2,
				Host:        "bb1:9000",
				PeerAddress: "152.94.162.21:9000",
				File:        base + "_bb1_9000.binpb",
			},
			{
				ID:          1,
				Host:        "bb2:9000",
				PeerAddress: "152.94.162.11:9000",
				File:        base + "_bb2_9000.binpb",
			},
		},
	}
	if m.Label != want.Label || m.Benchmark != want.Benchmark ||
		m.Nodes != want.Nodes || m.Workers != want.Workers ||
		m.Payload != want.Payload || m.Rate != want.Rate || m.Rep != want.Rep ||
		m.StreamMode != want.StreamMode ||
		m.Duration != want.Duration || m.Trim != want.Trim ||
		m.Status != want.Status || m.GitSHA != want.GitSHA || m.Binary != want.Binary {
		t.Errorf("manifest = %+v, want %+v", m, want)
	}
	if !slices.Equal(m.Hosts, want.Hosts) {
		t.Errorf("hosts = %v, want %v", m.Hosts, want.Hosts)
	}
	if !slices.Equal(m.Files, want.Files) {
		t.Errorf("files = %v, want %v", m.Files, want.Files)
	}
	if !slices.Equal(m.NodeMap, want.NodeMap) {
		t.Errorf("node_map = %+v, want %+v", m.NodeMap, want.NodeMap)
	}
	if _, err := time.Parse(time.RFC3339, m.Timestamp); err != nil {
		t.Errorf("timestamp %q is not RFC 3339: %v", m.Timestamp, err)
	}

	var flat map[string]json.RawMessage
	if err := json.Unmarshal(data, &flat); err != nil {
		t.Fatalf("unmarshal flat manifest: %v", err)
	}
	for _, key := range []string{
		"benchmark", "nodes", "workers", "payload", "rate",
		"send_buffer", "recv_buffer", "stream_mode", "rep",
	} {
		if _, ok := flat[key]; !ok {
			t.Errorf("flat manifest missing %q: %s", key, data)
		}
	}
	if _, nested := flat["dimensions"]; nested {
		t.Errorf("manifest unexpectedly nested dimensions: %s", data)
	}
}

func TestOldManifestDefaultsMissingBuffersToZero(t *testing.T) {
	const old = `{
		"benchmark":"Q","nodes":3,"workers":1,"payload":0,
		"rate":0,"stream_mode":"dual","rep":1
	}`
	var m runManifest
	if err := json.Unmarshal([]byte(old), &m); err != nil {
		t.Fatal(err)
	}
	if m.SendBuffer != 0 || m.RecvBuffer != 0 {
		t.Fatalf("missing buffers decoded as send=%d recv=%d, want zero", m.SendBuffer, m.RecvBuffer)
	}
}

// readManifest reads and unmarshals the manifest for base, failing the test on
// any error.
func readManifest(t *testing.T, dir, base string) runManifest {
	t.Helper()
	data, err := os.ReadFile(manifestPath(dir, base))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m runManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	return m
}

// writeResultFile creates an empty result file for one node so countResultFiles
// counts it as collected.
func writeResultFile(t *testing.T, dir, base string, n nodeAssignment) {
	t.Helper()
	path := filepath.Join(dir, resultFilename(base, n, resultExt))
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write result file: %v", err)
	}
}

// TestUpdateManifestOutcome verifies that updateManifestOutcome records the
// final status, error, failure phase, and result-file coverage for each of the
// failure phases and for a successful run.
func TestUpdateManifestOutcome(t *testing.T) {
	cfg := &config{sweepLabel: "nscale", duration: 10 * time.Second}
	p := runSpec{Dimensions: benchkit.Dimensions{
		Nodes: 3, Workers: 4, Benchmark: "Symmetric",
	}}
	nodes := []nodeAssignment{
		{host: "bb1", peerHost: "152.94.162.11", port: 9000},
		{host: "bb2", peerHost: "152.94.162.12", port: 9000},
		{host: "bb3", peerHost: "152.94.162.13", port: 9000},
	}
	const base = "nscale_Symmetric_N3_W4_P0"

	t.Run("SetupFailure", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(dir, base, p, nodes, cfg, "", "")
		runErr := os.ErrDeadlineExceeded
		collected, missing := countResultFiles(dir, base, nodes)
		if collected != 0 {
			t.Fatalf("collected = %d, want 0 (no result files written)", collected)
		}
		o := runOutcome{
			status: runStatusFailed, err: runErr, failurePhase: failurePhaseSetup,
			collectedFiles: collected, missingFiles: missing,
		}
		if err := updateManifestOutcome(dir, base, o); err != nil {
			t.Fatalf("updateManifestOutcome: %v", err)
		}
		m := readManifest(t, dir, base)
		if m.Status != runStatusFailed {
			t.Errorf("status = %q, want %q", m.Status, runStatusFailed)
		}
		if m.FailurePhase != failurePhaseSetup {
			t.Errorf("failure_phase = %q, want %q", m.FailurePhase, failurePhaseSetup)
		}
		if m.CollectedFiles != 0 {
			t.Errorf("collected_files = %d, want 0", m.CollectedFiles)
		}
		if len(m.MissingFiles) != 3 {
			t.Errorf("missing_files = %v, want 3 entries", m.MissingFiles)
		}
		if !strings.Contains(m.Error, runErr.Error()) {
			t.Errorf("error = %q, want containing %q", m.Error, runErr.Error())
		}
		if _, err := time.Parse(time.RFC3339, m.Completed); err != nil {
			t.Errorf("completed %q is not RFC 3339: %v", m.Completed, err)
		}
	})

	t.Run("MeasurementFailure", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(dir, base, p, nodes, cfg, "", "")
		// Two of three nodes wrote a result file; the third failed mid-run.
		writeResultFile(t, dir, base, nodes[0])
		writeResultFile(t, dir, base, nodes[1])
		collected, missing := countResultFiles(dir, base, nodes)
		if collected != 2 {
			t.Fatalf("collected = %d, want 2", collected)
		}
		o := runOutcome{
			status: runStatusFailed, err: os.ErrDeadlineExceeded,
			failurePhase: failurePhaseMeasurement, collectedFiles: collected, missingFiles: missing,
		}
		if err := updateManifestOutcome(dir, base, o); err != nil {
			t.Fatalf("updateManifestOutcome: %v", err)
		}
		m := readManifest(t, dir, base)
		if m.FailurePhase != failurePhaseMeasurement {
			t.Errorf("failure_phase = %q, want %q", m.FailurePhase, failurePhaseMeasurement)
		}
		if m.CollectedFiles != 2 {
			t.Errorf("collected_files = %d, want 2", m.CollectedFiles)
		}
		want := []string{resultFilename(base, nodes[2], resultExt)}
		if !slices.Equal(m.MissingFiles, want) {
			t.Errorf("missing_files = %v, want %v", m.MissingFiles, want)
		}
	})

	t.Run("Success", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(dir, base, p, nodes, cfg, "", "")
		for _, n := range nodes {
			writeResultFile(t, dir, base, n)
		}
		collected, missing := countResultFiles(dir, base, nodes)
		o := runOutcome{status: runStatusSucceeded, collectedFiles: collected, missingFiles: missing}
		if err := updateManifestOutcome(dir, base, o); err != nil {
			t.Fatalf("updateManifestOutcome: %v", err)
		}
		m := readManifest(t, dir, base)
		if m.Status != runStatusSucceeded {
			t.Errorf("status = %q, want %q", m.Status, runStatusSucceeded)
		}
		if m.Error != "" {
			t.Errorf("error after success = %q, want empty", m.Error)
		}
		if m.FailurePhase != "" {
			t.Errorf("failure_phase after success = %q, want empty", m.FailurePhase)
		}
		if m.CollectedFiles != 3 {
			t.Errorf("collected_files = %d, want 3", m.CollectedFiles)
		}
		if len(m.MissingFiles) != 0 {
			t.Errorf("missing_files after success = %v, want none", m.MissingFiles)
		}
	})
}

// TestStaleBinaryWarning verifies that a warning is produced exactly when the
// binary's embedded VCS revision and the repository HEAD are both known and
// differ, and that the message names both commits and the rebuild command.
func TestStaleBinaryWarning(t *testing.T) {
	const (
		head  = "127919b96af22a9be8c39627fd2909a3388d6aa1"
		stale = "07508fbaf77e68be6da451cce5c299360525b881"
	)
	tests := []struct {
		name      string
		binaryRev string
		headSHA   string
		wantWarn  bool
	}{
		{"Match", head, head, false},
		{"Stale", stale, head, true},
		{"UnknownBinaryRevision", "", head, false},
		{"UnknownHead", stale, "", false},
		{"BothUnknown", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := staleBinaryWarning(tt.binaryRev, tt.headSHA)
			if got := msg != ""; got != tt.wantWarn {
				t.Fatalf("staleBinaryWarning(%q, %q) = %q, want warning: %v", tt.binaryRev, tt.headSHA, msg, tt.wantWarn)
			}
			if !tt.wantWarn {
				return
			}
			for _, want := range []string{stale[:12], head[:12], rebuildSweepCommand} {
				if !strings.Contains(msg, want) {
					t.Errorf("warning %q does not contain %q", msg, want)
				}
			}
		})
	}
}
