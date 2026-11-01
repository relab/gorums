package main

import (
	"flag"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeOptionalArgs(t *testing.T) {
	fs := flag.NewFlagSet("sweep", flag.ContinueOnError)
	var detach, collect, outDir string
	fs.Var(optionalValueFlag{&detach, "first"}, "detach", "")
	fs.Var(optionalValueFlag{&collect, latestRunSentinel}, "collect", "")
	fs.StringVar(&outDir, "outdir", "", "")

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"Value", []string{"sweep", "-collect", "bb1:/remote/a run", "-outdir", "out"}, []string{"sweep", "-collect=bb1:/remote/a run", "-outdir", "out"}},
		{"Bare", []string{"sweep", "-collect", "-detach", "bb1"}, []string{"sweep", "-collect", "-detach=bb1"}},
		// The flag package treats "--name" like "-name"; without the same
		// rewrite, "--collect <ref>" would parse <ref> as a stray positional.
		{"DoubleDash", []string{"sweep", "--collect", "bb1:/remote/a run"}, []string{"sweep", "--collect=bb1:/remote/a run"}},
		{"AlreadyJoined", []string{"sweep", "-detach=bb1", "x"}, []string{"sweep", "-detach=bb1", "x"}},
		{"OtherFlag", []string{"sweep", "-outdir", "out"}, []string{"sweep", "-outdir", "out"}},
		{"Positional", []string{"sweep", "detach", "bb1"}, []string{"sweep", "detach", "bb1"}},
		{"Terminator", []string{"sweep", "--", "-detach", "bb1"}, []string{"sweep", "--", "-detach", "bb1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOptionalArgs(fs, tt.args); !slices.Equal(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestOptionalValueFlagSet(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "true", want: "first"},
		{in: "bb1", want: "bb1"},
		{in: "", wantErr: true},
		{in: "false", wantErr: true},
	}
	for _, tt := range tests {
		var got string
		err := optionalValueFlag{&got, "first"}.Set(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("Set(%q) = %q, %v; want %q, error %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestRemovedFlagError(t *testing.T) {
	for _, args := range [][]string{
		{"sweep", "-driver", "bb1"},
		{"sweep", "--driver=bb1", "-detach"},
		{"sweep", "-hosts", "bb[1-3]", "-driver=first"},
	} {
		if err := removedFlagError(args); err == nil || !strings.Contains(err.Error(), "-detach <host>") {
			t.Errorf("removedFlagError(%q) = %v, want a -detach hint", args, err)
		}
	}
	for _, args := range [][]string{
		{"sweep", "-detach", "bb1"},
		{"sweep", "-driven"},
		{"sweep", "--", "-driver"},
	} {
		if err := removedFlagError(args); err != nil {
			t.Errorf("removedFlagError(%q) = %v, want nil", args, err)
		}
	}
}

func TestSplitRemoteRun(t *testing.T) {
	host, wd, err := splitRemoteRun("bb1:/tmp/sweep-me/sweep-driver-e1-x")
	if err != nil || host != "bb1" || wd != "/tmp/sweep-me/sweep-driver-e1-x" {
		t.Errorf("got %q, %q, %v", host, wd, err)
	}
	for _, bad := range []string{"/tmp/run", "bb1:", ":/tmp/run", "bb1:tmp/run"} {
		if _, _, err := splitRemoteRun(bad); err == nil {
			t.Errorf("splitRemoteRun(%q) succeeded, want error", bad)
		}
	}
}
