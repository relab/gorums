package main

import (
	"strings"
	"testing"
)

const releaseStatus = ` M benchkit/go.mod
 M cmd/protoc-gen-gorums/gengorums/template_static.go
 M examples/go.mod
 M go.mod
 M go.sum
 M internal/tests/ordering/order_gorums.pb.go
 M internal/version/version.go
?? benchkit/new.pb.go
`

func prRunner(status string) *fakeRunner {
	f := newFakeRunner()
	f.answers["git branch --show-current"] = "master\n"
	f.answers["git status"] = status
	f.answers["gorelease -version v0.12.0"] = "v0.12.0 is a valid semantic version for this release.\n"
	f.answers["gh pr view"] = "https://github.com/relab/gorums/pull/999\n"
	return f
}

func TestPR(t *testing.T) {
	f := prRunner(releaseStatus)
	tl, out := newTestTool(t, f)
	setTestVersion(t, tl, "v0.12.0")
	if err := tl.pr(prOptions{}); err != nil {
		t.Fatal(err)
	}
	assertOrder(t, f.calls,
		"exec git switch -c release/v0.12.0",
		"exec git add -- benchkit/go.mod examples/go.mod go.mod go.sum internal/version/version.go",
		"exec git commit -m gorums: release v0.12.0",
		"exec git add -- benchkit/new.pb.go cmd/protoc-gen-gorums/gengorums/template_static.go internal/tests/ordering/order_gorums.pb.go",
		"exec git commit -m all: regenerate code for v0.12.0",
		"exec git push -u origin HEAD",
		"exec gh pr create --base master --head release/v0.12.0 --title gorums: release v0.12.0",
	)
	assertAbsent(t, f.calls, "exec git add -A")
	assertAbsent(t, f.calls, "exec git add .")
	assertAbsent(t, f.calls, "exec gh pr view --web")
	assertCalled(t, f.calls, "valid semantic version")
	if !strings.Contains(out.String(), "https://github.com/relab/gorums/pull/999") {
		t.Errorf("output lacks the PR URL:\n%s", out.String())
	}
}

func TestPRWithoutGeneratedFiles(t *testing.T) {
	f := prRunner(" M go.mod\n M internal/version/version.go\n")
	tl, _ := newTestTool(t, f)
	setTestVersion(t, tl, "v0.12.0")
	if err := tl.pr(prOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.Join(f.calls, "\n"), "exec git commit"); n != 1 {
		t.Errorf("made %d commits, want 1", n)
	}
}

func TestPRFlags(t *testing.T) {
	f := prRunner(releaseStatus)
	tl, _ := newTestTool(t, f)
	setTestVersion(t, tl, "v0.12.0-rc.1")
	f.answers["gorelease -version v0.12.0-rc.1"] = "report\n"
	if err := tl.pr(prOptions{web: true}); err != nil {
		t.Fatal(err)
	}
	assertCalled(t, f.calls, "exec git switch -c release/v0.12.0-rc.1")
	assertCalled(t, f.calls, "exec gh pr view --web release/v0.12.0-rc.1")
}

func TestPRRefusals(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		branch  string
		wantErr string
	}{
		{"unexpected file", releaseStatus + " M README.md\n", "master", "README.md"},
		{"unexpected untracked file", releaseStatus + "?? notes.txt\n", "master", "notes.txt"},
		{"prepare did not run", " M go.mod\n", "master", "prepare"},
		{"nothing changed", "", "master", "prepare"},
		{"wrong branch", releaseStatus, "feature/x", "master"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := prRunner(tt.status)
			f.answers["git branch --show-current"] = tt.branch + "\n"
			tl, _ := newTestTool(t, f)
			setTestVersion(t, tl, "v0.12.0")
			err := tl.pr(prOptions{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("pr() error = %v, want it to contain %q", err, tt.wantErr)
			}
			assertAbsent(t, f.calls, "exec")
		})
	}
}

func TestPRGoreleaseRejects(t *testing.T) {
	f := prRunner(releaseStatus)
	f.fail["gorelease -version v0.12.0"] = errBoom
	tl, _ := newTestTool(t, f)
	setTestVersion(t, tl, "v0.12.0")
	err := tl.pr(prOptions{})
	if err == nil || !strings.Contains(err.Error(), "gorelease rejects v0.12.0") {
		t.Fatalf("pr() error = %v, want a gorelease rejection", err)
	}
	assertOrder(t, f.calls, "exec git commit -m gorums: release v0.12.0")
	assertAbsent(t, f.calls, "exec git push")
	assertAbsent(t, f.calls, "exec gh pr create")
}

func TestPRDryRun(t *testing.T) {
	f := prRunner(releaseStatus)
	tl, out := newTestTool(t, f)
	tl.dryRun = true
	setTestVersion(t, tl, "v0.12.0")
	if err := tl.pr(prOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := f.execs(); len(got) != 0 {
		t.Errorf("dry run executed %v", got)
	}
	if !strings.Contains(out.String(), "+ git push -u origin HEAD") {
		t.Errorf("output lacks the push command:\n%s", out.String())
	}
}

func TestParseStatus(t *testing.T) {
	got := parseStatus(" M a.go\nM  b.go\n?? c.go\nR  old.go -> new.go\n M \"sp ace.go\"\n")
	want := []string{"a.go", "b.go", "c.go", "new.go", "sp ace.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("parseStatus() = %v, want %v", got, want)
	}
}
