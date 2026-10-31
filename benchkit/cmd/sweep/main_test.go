package main

import (
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
			Hosts: hosts, Check: cfg.check, List: cfg.list, ExplainCheck: cfg.explainCheck,
		}); err != nil {
			t.Fatal(err)
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name                                 string
		args                                 []string
		saved                                bool
		driver, collect, collectNow, wantErr string
		check, list, explainCheck            bool
	}{
		{name: "Local", args: []string{"-hosts", "bb1,bb2"}},
		{name: "BareDetach", args: []string{"-hosts", "bb1,bb2", "-detach"}, driver: "bb1"},
		{name: "ExplicitDetach", args: []string{"-detach", "head", "-hosts", "bb1,bb2"}, driver: "head"},
		{name: "EqualsDetach", args: []string{"-hosts", "bb1,bb2", "-detach=head"}, driver: "head"},
		{name: "DoubleDashDetach", args: []string{"--detach", "head", "-hosts", "bb1,bb2"}, driver: "head"},
		{name: "FirstDetach", args: []string{"-detach=first", "-hosts", "bb1,bb2"}, driver: "bb1"},
		{name: "CollectExplicit", args: []string{"-detach", "head", "-collect", "/local/run"}, driver: "head", collect: "/local/run"},
		{name: "SnapshotExplicit", args: []string{"-detach=head", "-collect-now=/local/run"}, driver: "head", collectNow: "/local/run"},
		{name: "CollectFirst", args: []string{"-detach", "-hosts", "bb1,bb2", "-collect=/local/run"}, driver: "bb1", collect: "/local/run"},
		{name: "CollectLatest", args: []string{"-collect"}, saved: true, driver: "head", collect: "/local/saved"},
		{name: "SnapshotLatest", args: []string{"--collect-now"}, saved: true, driver: "head", collectNow: "/local/saved"},
		{name: "CollectLatestExplicitHost", args: []string{"-detach=head", "-collect"}, saved: true, driver: "head", collect: "/local/saved"},
		{name: "LatestHostMismatch", args: []string{"-detach=other", "-collect"}, saved: true, wantErr: "latest run belongs to"},
		{name: "ListLatest", args: []string{"-list"}, saved: true, driver: "head", list: true},
		{name: "ListExplicit", args: []string{"-detach=head", "-list"}, driver: "head", list: true},
		{name: "ListFirst", args: []string{"-detach", "-hosts", "bb1,bb2", "-list"}, driver: "bb1", list: true},
		{name: "CheckFirst", args: []string{"-detach", "-hosts", "bb1,bb2", "-check"}, driver: "bb1", check: true},
		{name: "CheckExplicit", args: []string{"-detach=head", "-hosts", "bb1,bb2", "-check"}, driver: "head", check: true},
		{name: "ExplainCheck", args: []string{"-detach=head", "-explain-check", "-explain-model", "test"}, driver: "head", explainCheck: true},
		{name: "MissingDefaultHost", args: []string{"-detach", "-list"}, wantErr: "-detach requires -hosts"},
		{name: "MissingCollectionHost", args: []string{"-collect=/local/run"}, wantErr: "requires -detach <host>"},
		{name: "RemovedDriver", args: []string{"-driver=head", "-hosts", "bb1,bb2"}, wantErr: "flag provided but not defined: -driver"},
		{name: "EmptyDetach", args: []string{"-detach=", "-hosts", "bb1,bb2"}, wantErr: "use -detach [hostname]"},
		{name: "BooleanFalse", args: []string{"-detach=false", "-hosts", "bb1,bb2"}, wantErr: "use -detach [hostname]"},
		{name: "DrivenDetach", args: []string{"-driven", "-detach=head", "-hosts", "bb1,bb2"}, wantErr: "-detach cannot be combined with -driven"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			state := lastRunState{Driver: "head", RemoteWorkDir: "/local/saved", LocalRunDir: filepath.Join(root, "saved"), SSHConfig: filepath.Join(root, "ssh.config"), TransferMode: "sftp"}
			if tt.saved {
				if err := writeLastRunState(root, state); err != nil {
					t.Fatal(err)
				}
			}
			args := append([]string{"-test.run=^TestMainParseFlags$", "--", "-outdir", root}, tt.args...)
			cmd := exec.Command(exe, args...)
			cmd.Env = append(os.Environ(), "SWEEP_TEST_PARSE_FLAGS=1", "OLLAMA_API_KEY=test")
			out, err := cmd.CombinedOutput()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(string(out), tt.wantErr) {
					t.Fatalf("got %v, %s; want %q", err, out, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse flags: %v\n%s", err, out)
			}
			var got parsed
			if err := json.Unmarshal([]byte(strings.SplitN(string(out), "\n", 2)[0]), &got); err != nil {
				t.Fatalf("decode: %v\n%s", err, out)
			}
			if got.Driver != tt.driver || got.Collect != tt.collect || got.CollectNow != tt.collectNow || got.Check != tt.check || got.List != tt.list || got.ExplainCheck != tt.explainCheck {
				t.Fatalf("got %+v; want driver=%q collect=%q snapshot=%q check=%v list=%v explain-check=%v", got, tt.driver, tt.collect, tt.collectNow, tt.check, tt.list, tt.explainCheck)
			}
			if tt.saved && (got.OutDir != state.LocalRunDir || got.SSHConfig != state.SSHConfig || got.Transfer != state.TransferMode) {
				t.Fatalf("saved recovery settings not restored: %+v", got)
			}
		})
	}
}
