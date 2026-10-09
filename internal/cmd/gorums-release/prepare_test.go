package main

import (
	"strings"
	"testing"
)

const goreleaseReport = `# summary
Inferred base version: v0.11.0
Suggested version: v0.12.0
`

// readyRunner answers the precondition queries of a clean, current master.
func readyRunner() *fakeRunner {
	f := newFakeRunner()
	f.answers["git branch --show-current"] = "master\n"
	f.answers["git status"] = ""
	f.answers["git rev-parse"] = "abc123\n"
	f.answers["gorelease"] = goreleaseReport
	return f
}

func TestPrepare(t *testing.T) {
	f := readyRunner()
	tl, _ := newTestTool(t, f)
	if err := tl.prepare(prepareOptions{}); err != nil {
		t.Fatal(err)
	}
	if got, want := readVersion(t, tl), (semver{0, 12, 0, ""}); got != want {
		t.Errorf("version = %+v, want %+v", got, want)
	}
	assertOrder(t, f.calls,
		"query git fetch origin master",
		"query gh auth status",
		"exec go install golang.org/x/exp/cmd/gorelease@latest",
		"query gorelease",
		"exec go get -u ./...",
		"exec (examples) go get -u ./...",
		"exec (benchkit) go get -u ./...",
		"exec (benchkit) go mod tidy",
		"exec make tools",
		"exec (examples) go mod edit -require=github.com/relab/gorums@v0.12.0",
		"exec (benchkit) go mod edit -require=github.com/relab/gorums@v0.12.0",
		"exec make genproto",
		"exec go mod tidy",
		"exec (examples) go mod tidy",
		"exec make test",
		"exec make testrace",
	)
	// The benchkit module cannot be tidied once it requires the unreleased tag.
	edit := -1
	for i, c := range f.execs() {
		if strings.HasPrefix(c, "(benchkit) go mod edit") {
			edit = i
		}
		if edit >= 0 && c == "(benchkit) go mod tidy" {
			t.Error("benchkit tidied after requiring the unreleased version")
		}
	}
	assertAbsent(t, f.calls, "exec git commit")
}

func TestPrepareVersionChoice(t *testing.T) {
	tests := []struct {
		name       string
		opts       prepareOptions
		suggested  string
		want       semver
		wantQuery  string
		wantErrSub string
	}{
		{name: "suggestion", suggested: "v0.12.0", want: semver{0, 12, 0, ""}, wantQuery: "query gorelease"},
		{name: "patch suggestion", suggested: "v0.11.1", want: semver{0, 11, 1, ""}},
		{name: "override", opts: prepareOptions{version: "v0.12.3"}, suggested: "v0.12.0",
			want: semver{0, 12, 3, ""}, wantQuery: "query gorelease -version v0.12.3"},
		{name: "pre-release", opts: prepareOptions{version: "v0.12.0-rc.1"}, suggested: "v0.12.0",
			want: semver{0, 12, 0, "rc.1"}, wantQuery: "query gorelease -version v0.12.0-rc.1"},
		{name: "pre-release without dot", opts: prepareOptions{version: "v0.12.0-rc1"}, suggested: "v0.12.0",
			want: semver{0, 12, 0, "rc1"}},
		{name: "v1 pre-release explicit", opts: prepareOptions{version: "v1.0.0-rc.1"}, suggested: "v0.12.0",
			want: semver{1, 0, 0, "rc.1"}},
		{name: "v1 suggestion refused", suggested: "v1.0.0", wantErrSub: "--version"},
		{name: "v1 explicit", opts: prepareOptions{version: "v1.0.0"}, suggested: "v0.12.0",
			want: semver{1, 0, 0, ""}},
		{name: "downgrade refused", opts: prepareOptions{version: "v0.10.0"}, suggested: "v0.12.0",
			wantErrSub: "older"},
		{name: "invalid version", opts: prepareOptions{version: "0.12"}, suggested: "v0.12.0",
			wantErrSub: "invalid version"},
		{name: "empty suffix", opts: prepareOptions{version: "v0.12.0-"}, suggested: "v0.12.0",
			wantErrSub: "invalid version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readyRunner()
			f.answers["gorelease"] = "Suggested version: " + tt.suggested + "\n"
			tl, _ := newTestTool(t, f)
			tt.opts.skipTests = true
			err := tl.prepare(tt.opts)
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("prepare() error = %v, want it to contain %q", err, tt.wantErrSub)
				}
				if got := readVersion(t, tl); got != (semver{0, 11, 0, "devel"}) {
					t.Errorf("version file changed to %+v on error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := readVersion(t, tl); got != tt.want {
				t.Errorf("version = %+v, want %+v", got, tt.want)
			}
			if tt.wantQuery != "" {
				assertOrder(t, f.calls, tt.wantQuery)
			}
		})
	}
}

func TestPrepareSkips(t *testing.T) {
	f := readyRunner()
	tl, _ := newTestTool(t, f)
	if err := tl.prepare(prepareOptions{skipUpgrade: true, skipTests: true}); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, f.calls, "exec go get")
	assertAbsent(t, f.calls, "exec make test")
	assertOrder(t, f.calls, "exec make genproto")
}

func TestPreparePreconditions(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		answer  string
		wantErr string
	}{
		{"wrong branch", "git branch --show-current", "feature/x\n", "master"},
		{"dirty tree", "git status", " M go.mod\n", "uncommitted"},
		{"untracked file", "git status", "?? x.go\n", "untracked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readyRunner()
			f.answers[tt.prefix] = tt.answer
			tl, _ := newTestTool(t, f)
			err := tl.prepare(prepareOptions{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("prepare() error = %v, want it to contain %q", err, tt.wantErr)
			}
			assertAbsent(t, f.calls, "exec")
		})
	}
}

func TestPrepareStaleMaster(t *testing.T) {
	f := readyRunner()
	f.answers["git rev-parse HEAD"] = "abc\n"
	f.answers["git rev-parse origin/master"] = "def\n"
	tl, _ := newTestTool(t, f)
	if err := tl.prepare(prepareOptions{}); err == nil || !strings.Contains(err.Error(), "origin/master") {
		t.Fatalf("prepare() error = %v, want a stale-master error", err)
	}
}

func TestPrepareDryRun(t *testing.T) {
	f := readyRunner()
	f.answers["git branch --show-current"] = "feature/x\n"
	tl, out := newTestTool(t, f)
	tl.dryRun = true
	if err := tl.prepare(prepareOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := readVersion(t, tl); got != (semver{0, 11, 0, "devel"}) {
		t.Errorf("dry run changed the version file to %+v", got)
	}
	if got := f.execs(); len(got) != 0 {
		t.Errorf("dry run executed %v", got)
	}
	for _, want := range []string{"warning (dry run)", "+ make genproto", "+ write internal/version/version.go"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestPrepareFailureStops(t *testing.T) {
	f := readyRunner()
	f.fail["make genproto"] = errBoom
	tl, _ := newTestTool(t, f)
	if err := tl.prepare(prepareOptions{}); err == nil {
		t.Fatal("expected an error")
	}
	assertAbsent(t, f.calls, "exec make test")
}

func TestPrepareVersionHint(t *testing.T) {
	tests := []struct {
		name    string
		opts    prepareOptions
		want    []string
		wantNot string
	}{
		{"suggested", prepareOptions{}, []string{"Version: v0.12.0, suggested by gorelease", "gorums-release prepare --version v0.12.0-rc.1"}, ""},
		{"override", prepareOptions{version: "v0.12.3"}, []string{"Version: v0.12.3, from --version"}, "--version v"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readyRunner()
			tl, out := newTestTool(t, f)
			tl.dryRun = true
			tt.opts.skipTests = true
			if err := tl.prepare(tt.opts); err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("output lacks %q:\n%s", w, out.String())
				}
			}
			if tt.wantNot != "" && strings.Contains(out.String(), tt.wantNot+"0.12.0-rc") {
				t.Errorf("output has an unexpected hint:\n%s", out.String())
			}
		})
	}
}
