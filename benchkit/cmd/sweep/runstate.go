package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	lastRunStateName  = ".sweep-last.json"
	collectScriptName = "collect.sh"
	latestRunSentinel = "__latest__"
)

// lastRunState is the laptop-side pointer needed to recover the most recently
// launched driver run without remembering either its driver or remote path.
type lastRunState struct {
	Driver          string    `json:"driver"`
	RemoteWorkDir   string    `json:"remote_work_dir"`
	RemoteNamespace string    `json:"remote_namespace"`
	Label           string    `json:"label"`
	LaunchedAt      time.Time `json:"launched_at"`
	LocalRunDir     string    `json:"local_run_dir"`
	SSHConfig       string    `json:"ssh_config,omitempty"`
	TransferMode    string    `json:"transfer_mode"`
	Collection      string    `json:"collection"`
}

func lastRunStatePath(rootDir string) string {
	return filepath.Join(rootDir, lastRunStateName)
}

func writeLastRunState(rootDir string, state lastRunState) error {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(rootDir, lastRunStateName+".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, lastRunStatePath(rootDir))
}

func readLastRunState(rootDir string) (lastRunState, error) {
	data, err := os.ReadFile(lastRunStatePath(rootDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lastRunState{}, fmt.Errorf("no previous driver run recorded in %s", lastRunStatePath(rootDir))
		}
		return lastRunState{}, err
	}
	var state lastRunState
	if err := json.Unmarshal(data, &state); err != nil {
		return lastRunState{}, fmt.Errorf("read %s: %w", lastRunStatePath(rootDir), err)
	}
	if state.Driver == "" || state.RemoteWorkDir == "" || state.LocalRunDir == "" {
		return lastRunState{}, fmt.Errorf("%s is missing driver, remote_work_dir, or local_run_dir", lastRunStatePath(rootDir))
	}
	return state, nil
}

func updateLastRunCollection(rootDir, remoteWorkDir, collection string) {
	state, err := readLastRunState(rootDir)
	if err != nil || state.RemoteWorkDir != remoteWorkDir {
		return
	}
	state.Collection = collection
	if err := writeLastRunState(rootDir, state); err != nil {
		// Collection already succeeded; failure to refresh a convenience pointer
		// must not turn that success into a failed collection.
		fmt.Fprintf(os.Stderr, "warning: update %s: %v\n", lastRunStatePath(rootDir), err)
	}
}

func writeCollectScript(outDir string, state lastRunState) (string, error) {
	path := filepath.Join(outDir, collectScriptName)
	var args []string
	args = append(args, "-driver", state.Driver, "-collect="+state.RemoteWorkDir, "-outdir", filepath.Dir(state.LocalRunDir))
	if state.SSHConfig != "" {
		args = append(args, "-config", state.SSHConfig)
	}
	if state.TransferMode != "" {
		args = append(args, "-transfer", state.TransferMode)
	}
	var b strings.Builder
	b.WriteString("#!/bin/sh\nset -eu\n\n")
	b.WriteString("# Safe collection waits for the remote run to finish.\n")
	b.WriteString("exec ./cmd/sweep/sweep")
	for _, arg := range args {
		b.WriteByte(' ')
		b.WriteString(shellQuote(arg))
	}
	b.WriteByte('\n')
	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
