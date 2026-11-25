package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestModulePath(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"module first", "module github.com/relab/gorums/benchkit\n\ngo 1.26.2\n", benchkitModulePath},
		{"leading comment", "// generated fixture\nmodule example.com/test\n", "example.com/test"},
		{"missing", "go 1.26.2\n", ""},
		{"malformed", "module\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modulePath([]byte(tt.data)); got != tt.want {
				t.Errorf("modulePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsBenchkitModuleRoot(t *testing.T) {
	tests := []struct {
		name      string
		module    string
		benchmark bool
		want      bool
	}{
		{"benchkit", benchkitModulePath, true, true},
		{"gorums root", "github.com/relab/gorums", true, false},
		{"missing command", benchkitModulePath, false, false},
		{"missing go.mod", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.module != "" {
				if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+tt.module+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.benchmark {
				if err := os.MkdirAll(filepath.Join(dir, "cmd", "benchmark"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if got := isBenchkitModuleRoot(dir); got != tt.want {
				t.Errorf("isBenchkitModuleRoot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModuleRootSweepInvocation(t *testing.T) {
	root := t.TempDir()
	writeBenchkitRoot(t, root)
	tests := []struct {
		name        string
		dir         string
		wantRun     string
		wantRebuild string
	}{
		{"benchkit root", root, "./cmd/sweep/sweep", rebuildSweepCommand},
		{"other module", t.TempDir(), "go tool sweep", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, rebuild := sweepInvocation(tt.dir)
			if run != tt.wantRun || rebuild != tt.wantRebuild {
				t.Errorf("sweepInvocation() = (%q, %q), want (%q, %q)", run, rebuild, tt.wantRun, tt.wantRebuild)
			}
		})
	}
}

// writeBenchkitRoot makes dir look like the benchkit module root.
func writeBenchkitRoot(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+benchkitModulePath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "benchmark"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// chdirOutsideBenchkit changes to an empty directory outside the benchkit
// module, where benchkit packages resolve through the gorums go.work. It skips
// in -short mode, since its callers cross-build binaries.
func chdirOutsideBenchkit(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("cross-builds a binary")
	}
	out, err := exec.Command("go", "env", "GOWORK").Output()
	if err != nil {
		t.Fatalf("go env GOWORK: %v", err)
	}
	work := strings.TrimSpace(string(out))
	if work == "" || work == "off" {
		t.Skip("needs the gorums go.work to resolve benchkit")
	}
	t.Setenv("GOWORK", work)
	t.Chdir(t.TempDir())
}
