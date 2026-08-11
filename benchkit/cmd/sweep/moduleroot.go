package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const benchkitModulePath = "github.com/relab/gorums/benchkit"

// requireBenchkitModuleRoot verifies that sweep is running from the benchkit
// module root, where its relative build and binary paths resolve.
func requireBenchkitModuleRoot() error {
	if isBenchkitModuleRoot(".") {
		return nil
	}
	return errors.New("run sweep from the benchkit module root")
}

// isBenchkitModuleRoot reports whether dir has benchkit's module directive and
// command layout.
func isBenchkitModuleRoot(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err == nil && modulePath(data) == benchkitModulePath {
		if info, statErr := os.Stat(filepath.Join(dir, "cmd", "benchmark")); statErr == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// modulePath returns the module directive from a go.mod file.
func modulePath(data []byte) string {
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}
