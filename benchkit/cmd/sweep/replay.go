package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/relab/iago"
)

const replayScriptName = "sweep.sh"
const rebuildSweepCommand = "just sweep"

func writeReplayScript(outDir string, args []string) (string, error) {
	path := filepath.Join(outDir, replayScriptName)
	run, rebuild := sweepInvocation(".")
	data := []byte(replayScript(run, rebuild, args))
	if err := os.WriteFile(path, data, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func replayScript(run, rebuild string, args []string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("set -eu\n\n")
	if rebuild != "" {
		b.WriteString("# Rebuild the sweep driver before replaying the experiment.\n")
		b.WriteString(rebuild)
		b.WriteString("\n\n")
	}
	b.WriteString("exec ")
	b.WriteString(run)
	for _, arg := range args[1:] {
		b.WriteByte(' ')
		b.WriteString(iago.Quote(arg))
	}
	b.WriteByte('\n')
	return b.String()
}
