package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLastRunStateRoundTripAndCollectScript(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "run one")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := lastRunState{
		Driver: "bb1", RemoteWorkDir: "/local/sweep-me/run one",
		RemoteNamespace: "/local/sweep-me", Label: "run one",
		LaunchedAt:  time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
		LocalRunDir: runDir, SSHConfig: "/tmp/ssh config", TransferMode: "rsync",
		Collection: "pending",
	}
	if err := writeLastRunState(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := readLastRunState(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	path, err := writeCollectScript(runDir, want)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || !containsAll(string(data), "'-collect=/local/sweep-me/run one'", "'-driver'", "'bb1'") {
		t.Fatalf("unexpected collect script:\n%s", data)
	}
}

func containsAll(s string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(s, value) {
			return false
		}
	}
	return true
}
