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
