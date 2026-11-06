package main

import (
	"strings"
	"testing"
)

func TestCheckBranch(t *testing.T) {
	tests := []struct {
		name    string
		version string
		branch  string // the -branch flag; empty means the checked-out branch
		current string // the checked-out branch
		wantErr string
	}{
		{name: "release", version: "v0.12.0", branch: "release/v0.12.0"},
		{name: "pre-release", version: "v0.12.0-rc.1", branch: "release/v0.12.0-rc.1"},
		{name: "checked-out branch", version: "v0.12.0", current: "release/v0.12.0"},
		{name: "other version", version: "v0.12.0", branch: "release/v0.11.0", wantErr: "not the release branch for v0.12.0"},
		{name: "pre-release suffix missing", version: "v0.12.0-rc.1", branch: "release/v0.12.0", wantErr: "not the release branch for v0.12.0-rc.1"},
		{name: "not a release branch", version: "v0.12.0", branch: "release/v0-test-388-smoke", wantErr: "not the release branch"},
		{name: "master", version: "v0.12.0", current: "master", wantErr: "master is not the release branch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRunner()
			f.answers["git branch --show-current"] = tt.current + "\n"
			tl, out := newTestTool(t, f)
			setTestVersion(t, tl, tt.version)
			err := tl.checkBranch(checkBranchOptions{branch: tt.branch})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("checkBranch() = %v, want nil", err)
				}
				if !strings.Contains(out.String(), "matches "+tt.version) {
					t.Errorf("output = %q, want it to name the version %s", out.String(), tt.version)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("checkBranch() = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}
