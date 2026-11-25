package main

import (
	"os"
	"path/filepath"
	"strings"
)

const benchkitModulePath = "github.com/relab/gorums/benchkit"

// benchmarkPackage and sweepPackage are import paths that resolve to the
// benchkit version that the current module requires.
const (
	benchmarkPackage = benchkitModulePath + "/cmd/benchmark"
	sweepPackage     = benchkitModulePath + "/cmd/sweep"
)

// sweepInvocation returns the command that starts sweep in dir, and the
// command that rebuilds it first; rebuild is empty when run builds sweep.
func sweepInvocation(dir string) (run, rebuild string) {
	if isBenchkitModuleRoot(dir) {
		return "./cmd/sweep/sweep", rebuildSweepCommand
	}
	return "go tool sweep", ""
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
