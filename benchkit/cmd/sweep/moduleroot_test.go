package main

import (
	"os"
	"path/filepath"
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

func TestModuleRootCheck(t *testing.T) {
	tests := []struct {
		name    string
		root    bool
		binary  string
		build   string
		env     string
		wantErr bool
	}{
		{"default build at root", true, "", "", "", false},
		{"default build elsewhere", false, "", "", "", true},
		{"binary elsewhere", false, "./bench", "", "", false},
		{"build flag elsewhere", false, "", "go build ./cmd/x", "", false},
		{"build env elsewhere", false, "", "", "go build ./cmd/x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.root {
				writeBenchkitRoot(t, dir)
			}
			t.Chdir(dir)
			t.Setenv("BENCHKIT_BUILD", tt.env)
			cfg := &config{binaryPath: tt.binary, buildCmd: tt.build}
			if err := checkModuleRoot(cfg); (err != nil) != tt.wantErr {
				t.Errorf("checkModuleRoot() = %v, want error: %v", err, tt.wantErr)
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
