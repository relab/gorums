package main

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

const prOpenGreen = `{"number":999,"url":"https://github.com/relab/gorums/pull/999","state":"OPEN","statusCheckRollup":[
{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS"},
{"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"SKIPPED"},
{"__typename":"StatusContext","context":"ci","state":"SUCCESS"}]}`

func prJSON(state, checks string) string {
	return `{"number":999,"url":"u","state":"` + state + `","statusCheckRollup":[` + checks + `]}`
}

func publishRunner(pr string) *fakeRunner {
	f := newFakeRunner()
	f.answers["gh pr view"] = pr
	f.answers["git branch --show-current"] = "master\n"
	f.answers["gorelease -version"] = "report text\n"
	return f
}

func publishTool(t *testing.T, f *fakeRunner, version string) *tool {
	t.Helper()
	tl, _ := newTestTool(t, f)
	setTestVersion(t, tl, version)
	return tl
}

var publishDefaults = publishOptions{yes: true}

func TestPublishOpenPR(t *testing.T) {
	f := publishRunner(prOpenGreen)
	tl := publishTool(t, f, "v0.12.0")
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	assertOrder(t, f.calls,
		"query gh pr view release/v0.12.0 --json",
		"exec gh pr merge release/v0.12.0 --squash --delete-branch",
		"exec git switch master",
		"exec git pull --ff-only origin master",
		"query git tag -l v0.12.0 benchkit/v0.12.0",
		"query git ls-remote --tags origin refs/tags/v0.12.0 refs/tags/benchkit/v0.12.0",
		"exec git tag -a v0.12.0 -m Gorums v0.12.0",
		"exec git tag -a benchkit/v0.12.0 -m Gorums benchkit v0.12.0",
		"exec git push --atomic origin refs/tags/v0.12.0 refs/tags/benchkit/v0.12.0",
		"exec gh release create v0.12.0 --title Gorums v0.12.0 --generate-notes --notes",
	)
	assertCalled(t, f.calls, "GOWORK=off GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org GOPRIVATE= GONOPROXY= GONOSUMDB= GOFLAGS=-modcacherw GOMODCACHE=")
	assertCalled(t, f.calls, "go get github.com/relab/gorums@v0.12.0 github.com/relab/gorums/benchkit@v0.12.0")
	assertCalled(t, f.calls, "go build ./...")
	if strings.Contains(strings.Join(f.calls, "\n"), "--prerelease") {
		t.Error("final release marked as a pre-release")
	}
}

func TestPublishMergedPR(t *testing.T) {
	f := publishRunner(prJSON("MERGED", ""))
	tl := publishTool(t, f, "v0.12.0")
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, f.calls, "exec gh pr merge")
	assertOrder(t, f.calls, "exec git pull --ff-only origin master", "exec git push --atomic")
}

func TestPublishNoPR(t *testing.T) {
	f := publishRunner("")
	f.fail["gh pr view"] = errors.New(`no pull requests found for branch "release/v0.12.0"`)
	tl := publishTool(t, f, "v0.12.0")
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	assertAbsent(t, f.calls, "exec gh pr merge")
	assertOrder(t, f.calls, "exec git push --atomic")
}

func TestPublishPreRelease(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{"final", "v0.12.0", false},
		{"v0 is not a pre-release", "v0.9.1", false},
		{"suffix", "v0.12.0-rc.1", true},
		{"suffix without dot", "v1.0.0-rc1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := publishRunner(prJSON("MERGED", ""))
			tl := publishTool(t, f, tt.version)
			o := publishDefaults
			o.draft = true
			if err := tl.publish(o); err != nil {
				t.Fatal(err)
			}
			calls := strings.Join(f.calls, "\n")
			if got := strings.Contains(calls, "--prerelease"); got != tt.want {
				t.Errorf("--prerelease present = %v, want %v", got, tt.want)
			}
			if !strings.Contains(calls, "--draft") {
				t.Error("--draft missing")
			}
		})
	}
}

func TestPublishRefusals(t *testing.T) {
	tests := []struct {
		name    string
		pr      string
		prep    func(*fakeRunner)
		opts    func(*publishOptions)
		wantErr string
	}{
		{name: "failing check", pr: prJSON("OPEN", `{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE"}`), wantErr: "test"},
		{name: "pending check", pr: prJSON("OPEN", `{"__typename":"CheckRun","name":"slow","status":"IN_PROGRESS","conclusion":""}`), wantErr: "slow"},
		{name: "no checks", pr: prJSON("OPEN", ""), wantErr: "no CI checks"},
		{name: "PR lookup fails", pr: "", prep: func(f *fakeRunner) { f.fail["gh pr view"] = errBoom }, wantErr: "cannot look up"},
		{name: "closed PR", pr: prJSON("CLOSED", ""), wantErr: "closed"},
		{name: "not confirmed", pr: prOpenGreen, opts: func(o *publishOptions) { o.yes = false }, wantErr: "--yes"},
		{name: "local tag exists", pr: prJSON("MERGED", ""),
			prep: func(f *fakeRunner) { f.answers["git tag -l"] = "v0.12.0\n" }, wantErr: "already exists"},
		{name: "remote tag exists", pr: prJSON("MERGED", ""),
			prep: func(f *fakeRunner) { f.answers["git ls-remote"] = "abc\trefs/tags/v0.12.0\n" }, wantErr: "already exists"},
		{name: "pull fails", pr: prJSON("MERGED", ""),
			prep: func(f *fakeRunner) { f.fail["git pull"] = errBoom }, wantErr: "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := publishRunner(tt.pr)
			if tt.prep != nil {
				tt.prep(f)
			}
			tl := publishTool(t, f, "v0.12.0")
			o := publishDefaults
			if tt.opts != nil {
				tt.opts(&o)
			}
			err := tl.publish(o)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Fatalf("publish() error = %v, want it to contain %q", err, tt.wantErr)
			}
			assertAbsent(t, f.calls, "exec gh pr merge")
			assertAbsent(t, f.calls, "exec git tag")
			assertAbsent(t, f.calls, "exec git push")
		})
	}
}

func TestPublishVersionMismatchAfterMerge(t *testing.T) {
	// The merged master must carry the version being published.
	f := publishRunner(prJSON("MERGED", ""))
	tl := publishTool(t, f, "v0.12.0")
	f.onExec = func(line string) {
		if strings.HasPrefix(line, "git pull") {
			setTestVersion(t, tl, "v0.11.0")
		}
	}
	err := tl.publish(publishDefaults)
	if err == nil || !strings.Contains(err.Error(), "v0.12.0") {
		t.Fatalf("publish() error = %v, want a version mismatch", err)
	}
	assertAbsent(t, f.calls, "exec git tag")
}

func TestPublishVerifyRetries(t *testing.T) {
	f := publishRunner(prJSON("MERGED", ""))
	f.failFirst["go get github.com/relab/gorums@"] = 2
	tl := publishTool(t, f, "v0.12.0")
	var slept []time.Duration
	tl.sleep = func(d time.Duration) { slept = append(slept, d) }
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 2 {
		t.Errorf("slept %d times, want 2 (%v)", len(slept), slept)
	}
}

func TestPublishVerifyTimesOut(t *testing.T) {
	f := publishRunner(prJSON("MERGED", ""))
	f.fail["go get github.com/relab/gorums@"] = errBoom
	tl := publishTool(t, f, "v0.12.0")
	slept := 0
	tl.sleep = func(time.Duration) { slept++ }
	tl.verifyTimeout = 30 * time.Second
	err := tl.publish(publishDefaults)
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("publish() error = %v, want a verification error", err)
	}
	if slept != 2 {
		t.Errorf("slept %d times, want 2", slept)
	}
	assertCalled(t, f.calls, "exec gh release create")
}

func TestPublishWithoutGoreleaseReport(t *testing.T) {
	f := publishRunner(prJSON("MERGED", ""))
	f.fail["gorelease -version"] = errBoom
	tl := publishTool(t, f, "v0.12.0")
	var out strings.Builder
	tl.out = &out
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	assertCalled(t, f.calls, "exec gh release create v0.12.0 --title Gorums v0.12.0 --generate-notes --verify-tag")
	if !strings.Contains(out.String(), "no gorelease report") {
		t.Errorf("no warning about the missing report:\n%s", out.String())
	}
}

func TestPublishDryRun(t *testing.T) {
	f := publishRunner(prOpenGreen)
	tl := publishTool(t, f, "v0.12.0")
	tl.dryRun = true
	var out strings.Builder
	tl.out = &out
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	if got := f.execs(); len(got) != 0 {
		t.Errorf("dry run executed %v", got)
	}
	for _, want := range []string{"+ gh pr merge", "+ git push --atomic", "+ gh release create"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestChecksState(t *testing.T) {
	tests := []struct {
		name        string
		checks      []checkRun
		wantPending []string
		wantFailed  []string
	}{
		{"all green", []checkRun{
			{Typename: "CheckRun", Name: "a", Status: "COMPLETED", Conclusion: "SUCCESS"},
			{Typename: "CheckRun", Name: "b", Status: "COMPLETED", Conclusion: "NEUTRAL"},
			{Typename: "StatusContext", Context: "c", State: "SUCCESS"},
		}, nil, nil},
		{"mixed", []checkRun{
			{Typename: "CheckRun", Name: "a", Status: "QUEUED"},
			{Typename: "CheckRun", Name: "b", Status: "COMPLETED", Conclusion: "CANCELLED"},
			{Typename: "StatusContext", Context: "c", State: "PENDING"},
			{Typename: "StatusContext", Context: "d", State: "FAILURE"},
		}, []string{"a", "c"}, []string{"b", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pending, failed := checksState(tt.checks)
			if strings.Join(pending, ",") != strings.Join(tt.wantPending, ",") ||
				strings.Join(failed, ",") != strings.Join(tt.wantFailed, ",") {
				t.Errorf("checksState() = %v, %v; want %v, %v", pending, failed, tt.wantPending, tt.wantFailed)
			}
		})
	}
}

// The module cache of the verification must not lie inside the module, or
// ./... would match packages in it.
func TestPublishVerifyCacheOutsideModule(t *testing.T) {
	f := publishRunner(prJSON("MERGED", ""))
	tl := publishTool(t, f, "v0.12.0")
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\((\S+)\) .*GOMODCACHE=(\S+)`)
	checked := false
	for _, c := range f.calls {
		if !strings.Contains(c, "go mod init") {
			continue
		}
		m := re.FindStringSubmatch(c)
		if m == nil {
			t.Fatalf("cannot read module and cache directories from %q", c)
		}
		if strings.HasPrefix(m[2]+"/", m[1]+"/") {
			t.Errorf("module cache %s is inside the module %s", m[2], m[1])
		}
		checked = true
	}
	if !checked {
		t.Error("no verification command found")
	}
}
