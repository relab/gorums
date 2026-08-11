package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeOptionalPathArgs(t *testing.T) {
	got := normalizeOptionalPathArgs([]string{"sweep", "-collect", "/local/a run", "-outdir", "out"})
	want := []string{"sweep", "-collect=/local/a run", "-outdir", "out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	got = normalizeOptionalPathArgs([]string{"sweep", "-collect-now", "-driver", "bb1"})
	want = []string{"sweep", "-collect-now", "-driver", "bb1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	// The double-dash forms, which Go's flag package treats identically to
	// the single-dash forms, must be normalized the same way; otherwise
	// "--collect <path>" parses <path> as a bare boolean followed by a stray
	// positional argument, silently collecting the latest run instead of the
	// requested one (see main.go's flag.NArg() check for the other half of
	// this fix).
	got = normalizeOptionalPathArgs([]string{"sweep", "--collect", "/local/a run", "-outdir", "out"})
	want = []string{"sweep", "--collect=/local/a run", "-outdir", "out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	got = normalizeOptionalPathArgs([]string{"sweep", "--collect-now", "-driver", "bb1"})
	want = []string{"sweep", "--collect-now", "-driver", "bb1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

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
