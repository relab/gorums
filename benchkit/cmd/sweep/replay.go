package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/relab/iago"
)

const replayScriptName = "sweep.sh"
const rebuildSweepCommand = "make -C .. sweep"

func writeReplayScript(outDir string, args []string) (string, error) {
	path := filepath.Join(outDir, replayScriptName)
	data := []byte(replayScript(args))
	if err := os.WriteFile(path, data, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func replayScript(args []string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("set -eu\n\n")
	b.WriteString("# Rebuild the sweep driver before replaying the experiment.\n")
	b.WriteString(rebuildSweepCommand + "\n\n")
	b.WriteString("exec ./cmd/sweep/sweep")
	for _, arg := range args[1:] {
		b.WriteByte(' ')
		b.WriteString(iago.Quote(arg))
	}
	b.WriteByte('\n')
	return b.String()
}
