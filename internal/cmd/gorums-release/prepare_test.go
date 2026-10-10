package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runtimeAtZero is a runtime version file for a module that is at minor 0.
const runtimeAtZero = `package gorumsimpl

const (
	MaxVersion = version.Minor
	GenVersion = 0
	MinVersion = 0
)
`

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
	f.answers["go env GOBIN GOPATH"] = "\n/home/u/go\n"
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
		"query gh auth status",
		"exec git fetch origin master",
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
		"exec (benchkit) go mod tidy",
		"exec make test",
		"exec make testrace",
	)
	assertAbsent(t, f.calls, "exec git commit")
}

func TestPrepareVersionChoice(t *testing.T) {
	tests := []struct {
		name       string
		current    string // version in the repository; default v0.11.0-devel
		runtime    string // content of the runtime version file; default implSrc
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
		{name: "major pre-release confirmed", runtime: runtimeAtZero,
			opts: prepareOptions{version: "v1.0.0-rc.1", allowMajor: true}, suggested: "v0.12.0",
			want: semver{1, 0, 0, "rc.1"}},
		{name: "major pre-release refused", runtime: runtimeAtZero,
			opts: prepareOptions{version: "v1.0.0-rc.1"}, suggested: "v0.12.0", wantErrSub: "-allow-major"},
		{name: "major suggestion refused", suggested: "v1.0.0", wantErrSub: "-allow-major"},
		{name: "major version refused", opts: prepareOptions{version: "v1.0.0"}, suggested: "v0.12.0",
			wantErrSub: "-allow-major"},
		{name: "major confirmed", runtime: runtimeAtZero, opts: prepareOptions{version: "v1.0.0", allowMajor: true},
			suggested: "v0.12.0", want: semver{1, 0, 0, ""}},
		{name: "major suggestion confirmed", runtime: runtimeAtZero, opts: prepareOptions{allowMajor: true},
			suggested: "v1.0.0", want: semver{1, 0, 0, ""}},
		{name: "major resets the minor below GenVersion", opts: prepareOptions{version: "v1.0.0", allowMajor: true},
			suggested: "v0.12.0", wantErrSub: "MaxVersion"},
		{name: "v2 refused", current: "v1.4.0", opts: prepareOptions{version: "v2.0.0", allowMajor: true},
			suggested: "v1.5.0", wantErrSub: "module path"},
		{name: "same major needs no flag", current: "v1.4.0", runtime: runtimeAtZero,
			opts: prepareOptions{version: "v1.5.0"}, suggested: "v1.5.0", want: semver{1, 5, 0, ""}},
		{name: "downgrade refused", opts: prepareOptions{version: "v0.10.0"}, suggested: "v0.12.0",
			wantErrSub: "older"},
		{name: "older candidate refused", current: "v0.12.0", opts: prepareOptions{version: "v0.12.0-rc.1"},
			suggested: "v0.12.1", wantErrSub: "older"},
		{name: "older candidate number refused", current: "v0.12.0-rc.2", opts: prepareOptions{version: "v0.12.0-rc.1"},
			suggested: "v0.12.1", wantErrSub: "older"},
		{name: "final after candidate", current: "v0.12.0-rc.2", opts: prepareOptions{version: "v0.12.0"},
			suggested: "v0.12.1", want: semver{0, 12, 0, ""}},
		{name: "next candidate", current: "v0.12.0-rc.1", opts: prepareOptions{version: "v0.12.0-rc.2"},
			suggested: "v0.12.1", want: semver{0, 12, 0, "rc.2"}},
		{name: "same version again", current: "v0.12.0", opts: prepareOptions{version: "v0.12.0"},
			suggested: "v0.12.1", want: semver{0, 12, 0, ""}},
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
			if tt.runtime != "" {
				if err := os.WriteFile(filepath.Join(tl.root, runtimeFile), []byte(tt.runtime), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before := semver{0, 11, 0, "devel"}
			if tt.current != "" {
				setTestVersion(t, tl, tt.current)
				before, _ = parseSemver(tt.current)
			}
			tt.opts.skipTests = true
			err := tl.prepare(tt.opts)
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("prepare() error = %v, want it to contain %q", err, tt.wantErrSub)
				}
				if got := readVersion(t, tl); got != before {
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
	for _, want := range []string{
		"? git status --porcelain",
		"? gorelease",
		"+ git fetch origin master",
		"+ make genproto",
		"+ write internal/version/version.go",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestPrepareDryRunWarns(t *testing.T) {
	f := readyRunner()
	f.answers["git branch --show-current"] = "feature/x\n"
	tl, out := newTestTool(t, f)
	tl.dryRun = true
	if err := tl.prepare(prepareOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `warning (dry run): on branch "feature/x"`) {
		t.Errorf("no warning about the branch:\n%s", out.String())
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
		{"suggested", prepareOptions{}, []string{"Version: v0.12.0, suggested by gorelease", "gorums-release prepare -version v0.12.0-rc.1"}, ""},
		{"override", prepareOptions{version: "v0.12.3"}, []string{"Version: v0.12.3, from -version"}, "-version v"},
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

func TestPreparePrerequisites(t *testing.T) {
	for _, missing := range []string{"git", "make", "go", "protoc", "gh"} {
		t.Run("missing "+missing, func(t *testing.T) {
			f := readyRunner()
			tl, _ := newTestTool(t, f)
			tl.lookPath = func(name string) (string, error) {
				if name == missing {
					return "", errBoom
				}
				return "/usr/bin/" + name, nil
			}
			err := tl.prepare(prepareOptions{})
			if err == nil || !strings.Contains(err.Error(), missing+" is required") {
				t.Fatalf("prepare() error = %v, want a missing-%s error", err, missing)
			}
			assertAbsent(t, f.calls, "exec")
		})
	}
	tests := []struct {
		name    string
		prep    func(*fakeRunner, *tool)
		wantErr string
	}{
		{"gh not logged in", func(f *fakeRunner, _ *tool) { f.fail["gh auth status"] = errBoom }, "gh auth login"},
		{"go bin not on PATH", func(_ *fakeRunner, tl *tool) { tl.path = "/usr/bin" }, "/home/u/go/bin is not on PATH"},
		{"GOBIN not on PATH", func(f *fakeRunner, _ *tool) { f.answers["go env GOBIN GOPATH"] = "/opt/gobin\n/home/u/go\n" }, "/opt/gobin is not on PATH"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readyRunner()
			tl, _ := newTestTool(t, f)
			tt.prep(f, tl)
			err := tl.prepare(prepareOptions{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("prepare() error = %v, want it to contain %q", err, tt.wantErr)
			}
			assertAbsent(t, f.calls, "exec")
		})
	}
	t.Run("GOBIN on PATH", func(t *testing.T) {
		f := readyRunner()
		f.answers["go env GOBIN GOPATH"] = "/opt/gobin\n/home/u/go\n"
		tl, _ := newTestTool(t, f)
		tl.path = "/usr/bin:/opt/gobin/"
		if err := tl.prepare(prepareOptions{skipTests: true}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPrepareBumpRuntime(t *testing.T) {
	tests := []struct {
		name         string
		opts         prepareOptions
		suggested    string
		wantGen      int
		wantMin      int
		wantFileSame bool
	}{
		{name: "no flag", opts: prepareOptions{}, suggested: "v0.12.0", wantGen: 11, wantMin: 10, wantFileSame: true},
		{name: "bump gen", opts: prepareOptions{bumpGen: true}, suggested: "v0.12.0", wantGen: 12, wantMin: 10},
		{name: "bump min", opts: prepareOptions{bumpMin: true}, suggested: "v0.12.0", wantGen: 12, wantMin: 12},
		{name: "both flags", opts: prepareOptions{bumpGen: true, bumpMin: true}, suggested: "v0.12.0", wantGen: 12, wantMin: 12},
		{name: "patch release bump gen is a no-op", opts: prepareOptions{bumpGen: true}, suggested: "v0.11.1", wantGen: 11, wantMin: 10, wantFileSame: true},
		{name: "patch release bump min", opts: prepareOptions{bumpMin: true}, suggested: "v0.11.1", wantGen: 11, wantMin: 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readyRunner()
			f.answers["gorelease"] = "Suggested version: " + tt.suggested + "\n"
			tl, out := newTestTool(t, f)
			tt.opts.skipTests = true
			// The generated code contains GenVersion, so the file must be final
			// when code generation starts.
			var atGenproto string
			f.onExec = func(line string) {
				if line == "make genproto" {
					b, _ := tl.readFile(runtimeFile)
					atGenproto = string(b)
				}
			}
			if err := tl.prepare(tt.opts); err != nil {
				t.Fatal(err)
			}
			b, _ := tl.readFile(runtimeFile)
			gen, minV, err := parseRuntimeVersions(b)
			if err != nil || gen != tt.wantGen || minV != tt.wantMin {
				t.Errorf("runtime versions = %d, %d, %v; want %d, %d", gen, minV, err, tt.wantGen, tt.wantMin)
			}
			if atGenproto != string(b) {
				t.Errorf("the runtime file changed after make genproto started")
			}
			if tt.wantFileSame && string(b) != implSrc {
				t.Errorf("runtime file changed unexpectedly:\n%s", b)
			}
			if !strings.Contains(out.String(), "Runtime versions: GenVersion = ") {
				t.Errorf("summary lacks the runtime versions:\n%s", out.String())
			}
		})
	}
}

func TestPrepareBumpRuntimeDryRun(t *testing.T) {
	f := readyRunner()
	tl, out := newTestTool(t, f)
	tl.dryRun = true
	if err := tl.prepare(prepareOptions{bumpMin: true}); err != nil {
		t.Fatal(err)
	}
	b, _ := tl.readFile(runtimeFile)
	if string(b) != implSrc {
		t.Errorf("dry run changed the runtime file:\n%s", b)
	}
	for _, want := range []string{"GenVersion 11 -> 12, MinVersion 10 -> 12", "+ write runtime/gorumsimpl/version.go"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestPrepareGeneratorHint(t *testing.T) {
	changed := "10\t3\tcmd/protoc-gen-gorums/dev/zorums_server_gorums.pb.go\n1\t1\tcmd/protoc-gen-gorums/dev/zorums_types_gorums.pb.go\n"
	tests := []struct {
		name     string
		numstat  string
		opts     prepareOptions
		wantHint bool
	}{
		{"changed, no flag", changed, prepareOptions{}, true},
		{"unchanged", "", prepareOptions{}, false},
		{"changed, bump-gen given", changed, prepareOptions{bumpGen: true}, false},
		{"changed, bump-min given", changed, prepareOptions{bumpMin: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := readyRunner()
			f.answers["git diff --numstat"] = tt.numstat
			tl, out := newTestTool(t, f)
			tl.dryRun = true
			if err := tl.prepare(tt.opts); err != nil {
				t.Fatal(err)
			}
			got := strings.Contains(out.String(), "The generator output changed in 2 files since v0.11.0.")
			if got != tt.wantHint {
				t.Errorf("hint present = %v, want %v:\n%s", got, tt.wantHint, out.String())
			}
			if tt.wantHint && !strings.Contains(out.String(), "-bump-gen") {
				t.Errorf("hint does not name the flags:\n%s", out.String())
			}
		})
	}
}
