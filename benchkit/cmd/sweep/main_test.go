package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidateStreamModes(t *testing.T) {
	tests := []struct {
		name    string
		modes   []string
		binary  string
		wantErr bool
	}{
		{name: "Empty", modes: nil},
		{name: "Dual", modes: []string{"dual"}},
		{name: "DualAndDedup", modes: []string{"dual", "dedup"}},
		{name: "Invalid", modes: []string{"bogus"}, wantErr: true},
		{name: "BaselineWithBinary", modes: []string{"baseline"}, binary: "/tmp/bench"},
		{name: "BaselineWithoutBinary", modes: []string{"baseline"}, wantErr: true},
		{name: "BaselineMixed", modes: []string{"baseline", "dual"}, binary: "/tmp/bench", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStreamModes(tt.modes, tt.binary)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStreamModes(%v, %q) = %v, wantErr %v", tt.modes, tt.binary, err, tt.wantErr)
			}
		})
	}
}

// TestMainParseFlags exercises the actual CLI parser in a child process because
// invalid flags and validation failures exit the process.
func TestMainParseFlags(t *testing.T) {
	type parsed struct {
		Driver, Collect, CollectNow, OutDir, SSHConfig, Transfer string
		Hosts                                                    []string
		Check, List, ExplainCheck                                bool
	}
	if os.Getenv("SWEEP_TEST_PARSE_FLAGS") == "1" {
		i := slices.Index(os.Args, "--")
		os.Args = append([]string{"sweep"}, os.Args[i+1:]...)
		flag.CommandLine = flag.NewFlagSet("sweep", flag.ExitOnError)
		cfg, hosts := parseFlags()
		if err := json.NewEncoder(os.Stdout).Encode(parsed{
			Driver: cfg.driver, Collect: cfg.collect, CollectNow: cfg.collectNow,
			OutDir: cfg.outDir, SSHConfig: cfg.sshConfig, Transfer: cfg.transferMode,
			Hosts: hosts, Check: cfg.check, List: cfg.list != "", ExplainCheck: cfg.explainCheck,
		}); err != nil {
			t.Fatal(err)
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bb12 := []string{"bb1", "bb2"}
	tests := []struct {
		name                                 string
		args                                 []string
		saved                                bool
		driver, collect, collectNow, wantErr string
		hosts                                []string
		check, list, explainCheck            bool
	}{
		{name: "Local", args: []string{"-hosts", "bb1,bb2"}, hosts: bb12},
		// parseFlags keeps the driver in hosts; runDriver removes it from the pool.
		{name: "BareDetach", args: []string{"-hosts", "bb1,bb2", "-detach"}, driver: "bb1", hosts: bb12},
		{name: "ExplicitDetach", args: []string{"-detach", "head", "-hosts", "bb1,bb2"}, driver: "head", hosts: bb12},
		{name: "EqualsDetach", args: []string{"-hosts", "bb1,bb2", "-detach=head"}, driver: "head"},
		{name: "DoubleDashDetach", args: []string{"--detach", "head", "-hosts", "bb1,bb2"}, driver: "head"},
		{name: "FirstDetach", args: []string{"-detach=first", "-hosts", "bb1,bb2"}, driver: "bb1"},
		{name: "CollectExplicit", args: []string{"-collect", "head:/remote/run"}, driver: "head", collect: "/remote/run"},
		{name: "SnapshotExplicit", args: []string{"-collect-now=head:/remote/run"}, driver: "head", collectNow: "/remote/run"},
		{name: "CollectLatest", args: []string{"-collect"}, saved: true, driver: "head", collect: "/remote/saved"},
		{name: "SnapshotLatest", args: []string{"--collect-now"}, saved: true, driver: "head", collectNow: "/remote/saved"},
		{name: "ListLatest", args: []string{"-list"}, saved: true, driver: "head", list: true},
		{name: "ListExplicit", args: []string{"-list", "other"}, driver: "other", list: true},
		{name: "ListExplicitIgnoresSaved", args: []string{"-list=other"}, saved: true, driver: "other", list: true},
		{name: "CheckFirst", args: []string{"-detach", "-hosts", "bb1,bb2", "-check"}, driver: "bb1", check: true},
		{name: "CheckExplicit", args: []string{"-detach=head", "-hosts", "bb1,bb2", "-check"}, driver: "head", check: true},
		{name: "ExplainCheck", args: []string{"-detach=head", "-explain-check", "-explain-model", "test"}, driver: "head", explainCheck: true},
		{name: "MissingDefaultHost", args: []string{"-detach", "-explain-check", "-explain-model", "test"}, wantErr: "-detach without a hostname requires -hosts"},
		{name: "CollectWithoutHost", args: []string{"-collect=/remote/run"}, wantErr: "want <host>:<remote-work-dir>"},
		{name: "CollectRelativePath", args: []string{"-collect", "head:remote/run"}, wantErr: "want <host>:<remote-work-dir>"},
		{name: "DetachCollect", args: []string{"-detach", "head", "-collect", "head:/remote/run"}, wantErr: "-detach launches a run"},
		{name: "DetachList", args: []string{"-detach=head", "-list"}, wantErr: "-detach launches a run"},
		{name: "RemovedDriver", args: []string{"-driver=head", "-hosts", "bb1,bb2"}, wantErr: "-driver was removed"},
		{name: "EmptyDetach", args: []string{"-detach=", "-hosts", "bb1,bb2"}, wantErr: "invalid boolean value"},
		{name: "BooleanFalse", args: []string{"-detach=false", "-hosts", "bb1,bb2"}, wantErr: "invalid boolean value"},
		{name: "DrivenDetach", args: []string{"-driven", "-detach=head", "-hosts", "bb1,bb2"}, wantErr: "-detach cannot be combined with -driven"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			state := lastRunState{Driver: "head", RemoteWorkDir: "/remote/saved", LocalRunDir: filepath.Join(root, "saved"), SSHConfig: filepath.Join(root, "ssh.config"), TransferMode: "sftp"}
			if tt.saved {
				if err := writeLastRunState(root, state); err != nil {
					t.Fatal(err)
				}
			}
			args := append([]string{"-test.run=^TestMainParseFlags$", "--", "-outdir", root}, tt.args...)
			cmd := exec.Command(exe, args...)
			cmd.Env = append(os.Environ(), "SWEEP_TEST_PARSE_FLAGS=1", "OLLAMA_API_KEY=test")
			// Keep stderr apart, so a log line can never precede the JSON on stdout.
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(stderr.String(), tt.wantErr) {
					t.Fatalf("got %v, %s; want %q", err, stderr.String(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse flags: %v\n%s", err, stderr.String())
			}
			var got parsed
			if err := json.NewDecoder(bytes.NewReader(out)).Decode(&got); err != nil {
				t.Fatalf("decode: %v\n%s", err, out)
			}
			if got.Driver != tt.driver || got.Collect != tt.collect || got.CollectNow != tt.collectNow || got.Check != tt.check || got.List != tt.list || got.ExplainCheck != tt.explainCheck {
				t.Fatalf("got %+v; want driver=%q collect=%q snapshot=%q check=%v list=%v explain-check=%v", got, tt.driver, tt.collect, tt.collectNow, tt.check, tt.list, tt.explainCheck)
			}
			if tt.hosts != nil && !slices.Equal(got.Hosts, tt.hosts) {
				t.Fatalf("hosts = %v, want %v", got.Hosts, tt.hosts)
			}
			if tt.saved && tt.driver == state.Driver && (got.OutDir != state.LocalRunDir || got.SSHConfig != state.SSHConfig || got.Transfer != state.TransferMode) {
				t.Fatalf("saved recovery settings not restored: %+v", got)
			}
		})
	}
}
