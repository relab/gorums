package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayScript(t *testing.T) {
	args := []string{
		"./cmd/sweep/sweep",
		"-hosts", "bb[1-25]",
		"-sweep", "e3-tlcurve",
		"-n", "5,9,17,25",
		"-workers", "1,2,4,8,16,32",
		"-duration", "13s",
		"-trim", "3s",
		"-verbose",
		"-benchmarks", "SymmetricQuorumCall,QuorumCall",
		"-extra-args", "-label='canary run'",
	}
	want := "#!/bin/sh\n" +
		"set -eu\n\n" +
		"# Rebuild the sweep driver before replaying the experiment.\n" +
		rebuildSweepCommand + "\n\n" +
		"exec ./cmd/sweep/sweep '-hosts' 'bb[1-25]' '-sweep' 'e3-tlcurve'" +
		" '-n' '5,9,17,25' '-workers' '1,2,4,8,16,32' '-duration' '13s' '-trim' '3s'" +
		" '-verbose' '-benchmarks' 'SymmetricQuorumCall,QuorumCall'" +
		" '-extra-args' '-label='\\''canary run'\\'''\n"
	if got := replayScript(args); got != want {
		t.Errorf("replayScript =\n%s\nwant\n%s", got, want)
	}
}

func TestReplayScriptShellQuoteEmptyArg(t *testing.T) {
	got := replayScript([]string{"sweep", "-extra-args", ""})
	if !strings.Contains(got, "'-extra-args' ''\n") {
		t.Errorf("replayScript did not quote empty arg:\n%s", got)
	}
}

func TestWriteReplayScript(t *testing.T) {
	dir := t.TempDir()
	path, err := writeReplayScript(dir, []string{"sweep", "-hosts", "bb1"})
	if err != nil {
		t.Fatalf("writeReplayScript: %v", err)
	}
	if path != filepath.Join(dir, replayScriptName) {
		t.Errorf("path = %q, want %q", path, filepath.Join(dir, replayScriptName))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat replay script: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("mode = %v, want 0755", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replay script: %v", err)
	}
	if !strings.Contains(string(data), rebuildSweepCommand+"\n\nexec ./cmd/sweep/sweep '-hosts' 'bb1'\n") {
		t.Errorf("replay script content:\n%s", data)
	}
}
