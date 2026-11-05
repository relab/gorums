package main

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

// releaseSHA is the merge commit of the release pull request in the tests.
const releaseSHA = "abc123"

const prOpenGreen = `{"number":999,"url":"https://github.com/relab/gorums/pull/999","state":"OPEN","mergeCommit":null,"statusCheckRollup":[
{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS"},
{"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"SKIPPED"},
{"__typename":"StatusContext","context":"ci","state":"SUCCESS"},
` + smokeChecksGreen + `]}`

// smokeChecksGreen are the release-smoke check runs of a release pull request
// whose smoke checks all passed.
const smokeChecksGreen = `{"__typename":"CheckRun","name":"local-checks","workflowName":"release-smoke","status":"COMPLETED","conclusion":"SUCCESS"},
{"__typename":"CheckRun","name":"docker-smoke","workflowName":"release-smoke","status":"COMPLETED","conclusion":"SUCCESS"},
{"__typename":"CheckRun","name":"cluster-smoke","workflowName":"release-smoke","status":"COMPLETED","conclusion":"SUCCESS"}`

// otherChecksGreen are passing checks from the pull request CI.
const otherChecksGreen = `{"__typename":"CheckRun","name":"test","workflowName":"Go Test","status":"COMPLETED","conclusion":"SUCCESS"},
{"__typename":"CheckRun","name":"lint","workflowName":"golangci-lint","status":"COMPLETED","conclusion":"SUCCESS"}`

// prJSON returns the pull request JSON that gh prints. A merged pull request
// has a merge commit.
func prJSON(state, checks string) string {
	merge := "null"
	if state == "MERGED" {
		merge = `{"oid":"` + releaseSHA + `"}`
	}
	return `{"number":999,"url":"u","state":"` + state + `","mergeCommit":` + merge + `,"statusCheckRollup":[` + checks + `]}`
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
	f.answers["git show "+releaseSHA+":"+versionFile] = versionFileAt(t, version)
	return tl
}

// versionFileAt returns a version file with version v.
func versionFileAt(t *testing.T, v string) string {
	t.Helper()
	sv, err := parseSemver(v)
	if err != nil {
		t.Fatal(err)
	}
	out, err := rewriteVersionFile([]byte(versionSrc), sv)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

var publishDefaults = publishOptions{yes: true}

func TestPublishOpenPR(t *testing.T) {
	f := publishRunner(prOpenGreen)
	tl := publishTool(t, f, "v0.12.0")
	f.onExec = func(line string) {
		if strings.HasPrefix(line, "gh pr merge") {
			f.answers["gh pr view"] = prJSON("MERGED", "") // the merge commit exists now
		}
	}
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	assertOrder(t, f.calls,
		"query gh pr view release/v0.12.0 --json",
		"exec gh pr merge release/v0.12.0 --squash --delete-branch",
		"query gh pr view release/v0.12.0 --json",
		"exec git fetch origin master",
		"query git cat-file -e abc123^{commit}",
		"query git merge-base --is-ancestor abc123 origin/master",
		"query git show abc123:internal/version/version.go",
		"query git tag -l v0.12.0 benchkit/v0.12.0",
		"query git ls-remote --tags origin refs/tags/v0.12.0 refs/tags/benchkit/v0.12.0",
		"exec git tag -a v0.12.0 -m Gorums v0.12.0 abc123",
		"exec git tag -a benchkit/v0.12.0 -m Gorums benchkit v0.12.0 abc123",
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
	assertOrder(t, f.calls, "exec git fetch origin master", "exec git tag -a v0.12.0", "exec git push --atomic")
}

// The tags go on the merge commit of the release pull request. The local
// master may hold other commits, and origin/master may have moved on, and
// neither may end up under a permanent module version.
func TestPublishTagsMergeCommit(t *testing.T) {
	f := publishRunner(prJSON("MERGED", ""))
	f.answers["git rev-parse"] = "unpublished999\n"
	tl := publishTool(t, f, "v0.12.0")
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatal(err)
	}
	tagged := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "exec git tag -a") {
			tagged++
			if !strings.HasSuffix(c, " "+releaseSHA) {
				t.Errorf("tag command does not name the merge commit: %q", c)
			}
		}
		if strings.Contains(c, "HEAD") || strings.Contains(c, "unpublished999") {
			t.Errorf("publish depends on HEAD: %q", c)
		}
	}
	if tagged != 2 {
		t.Errorf("created %d tags, want 2", tagged)
	}
	assertAbsent(t, f.calls, "exec git pull")
	assertAbsent(t, f.calls, "exec git switch")
}

func TestPublishNoPR(t *testing.T) {
	f := publishRunner("")
	f.fail["gh pr view"] = errors.New(`no pull requests found for branch "release/v0.12.0"`)
	tl := publishTool(t, f, "v0.12.0")
	err := tl.publish(publishDefaults)
	if err == nil || !strings.Contains(err.Error(), "no pull request found for release/v0.12.0") {
		t.Fatalf("publish() error = %v, want a missing-PR error", err)
	}
	assertAbsent(t, f.calls, "exec")
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
		{name: "fetch fails", pr: prJSON("MERGED", ""),
			prep: func(f *fakeRunner) { f.fail["git fetch"] = errBoom }, wantErr: "boom"},
		{name: "merged without a merge commit", pr: `{"number":999,"url":"u","state":"MERGED","mergeCommit":null,"statusCheckRollup":[]}`,
			wantErr: "no merge commit"},
		{name: "merge commit missing locally", pr: prJSON("MERGED", ""),
			prep: func(f *fakeRunner) { f.fail["git cat-file"] = errBoom }, wantErr: "not in this repository"},
		{name: "merge commit not on origin/master", pr: prJSON("MERGED", ""),
			prep: func(f *fakeRunner) { f.fail["git merge-base --is-ancestor"] = errBoom }, wantErr: "not on origin/master"},
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

func TestPublishMergeCommitHasOtherVersion(t *testing.T) {
	f := publishRunner(prJSON("MERGED", ""))
	tl := publishTool(t, f, "v0.12.0")
	f.answers["git show "+releaseSHA+":"+versionFile] = versionFileAt(t, "v0.11.0")
	err := tl.publish(publishDefaults)
	if err == nil || !strings.Contains(err.Error(), "v0.11.0") || !strings.Contains(err.Error(), "v0.12.0") {
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
	for _, want := range []string{
		"+ gh pr merge",
		"+ git tag -a v0.12.0 -m Gorums v0.12.0 <merge-commit>",
		"+ git push --atomic",
		"+ gh release create",
	} {
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

// TestPublishSmokeChecks checks that publish merges an open release pull
// request only when every release-smoke job succeeded on it: a missing,
// skipped, or neutral smoke check stops the merge.
func TestPublishSmokeChecks(t *testing.T) {
	smoke := func(job, conclusion string) string {
		return `{"__typename":"CheckRun","name":"` + job + `","workflowName":"release-smoke","status":"COMPLETED","conclusion":"` + conclusion + `"}`
	}
	tests := []struct {
		name    string
		checks  string
		wantErr []string // "" entries: no error
	}{
		{name: "AllGreen", checks: otherChecksGreen + "," + smokeChecksGreen},
		{
			name:    "NoSmokeChecks",
			checks:  otherChecksGreen,
			wantErr: []string{"release-smoke / local-checks", "release-smoke / docker-smoke", "release-smoke / cluster-smoke"},
		},
		{
			name:    "ClusterSmokeSkipped",
			checks:  otherChecksGreen + "," + smoke("local-checks", "SUCCESS") + "," + smoke("docker-smoke", "SUCCESS") + "," + smoke("cluster-smoke", "SKIPPED"),
			wantErr: []string{"release-smoke / cluster-smoke"},
		},
		{
			name:    "DockerSmokeNeutral",
			checks:  otherChecksGreen + "," + smoke("local-checks", "SUCCESS") + "," + smoke("docker-smoke", "NEUTRAL") + "," + smoke("cluster-smoke", "SUCCESS"),
			wantErr: []string{"release-smoke / docker-smoke"},
		},
		{
			// A job of the same name in another workflow does not count.
			name: "SameJobNameOtherWorkflow",
			checks: otherChecksGreen + "," + smoke("local-checks", "SUCCESS") + "," + smoke("docker-smoke", "SUCCESS") +
				`,{"__typename":"CheckRun","name":"cluster-smoke","workflowName":"other","status":"COMPLETED","conclusion":"SUCCESS"}`,
			wantErr: []string{"release-smoke / cluster-smoke"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := publishRunner(prJSON("OPEN", tt.checks))
			tl := publishTool(t, f, "v0.12.0")
			f.onExec = func(line string) {
				if strings.HasPrefix(line, "gh pr merge") {
					f.answers["gh pr view"] = prJSON("MERGED", "")
				}
			}
			err := tl.publish(publishDefaults)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("publish() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("publish() = nil, want an error naming %v", tt.wantErr)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("publish() error %q does not name %q", err, want)
				}
			}
			assertAbsent(t, f.calls, "exec gh pr merge")
		})
	}
}

// TestPublishMergedPRWarnsOnMissingSmoke checks the escape hatch: a release
// pull request merged by hand, for example during a cluster outage, still
// publishes, but publish warns about each smoke check that did not pass.
func TestPublishMergedPRWarnsOnMissingSmoke(t *testing.T) {
	f := publishRunner(prJSON("MERGED", otherChecksGreen))
	tl, out := newTestTool(t, f)
	setTestVersion(t, tl, "v0.12.0")
	f.answers["git show "+releaseSHA+":"+versionFile] = versionFileAt(t, "v0.12.0")
	if err := tl.publish(publishDefaults); err != nil {
		t.Fatalf("publish() = %v, want nil", err)
	}
	for _, want := range []string{"warning:", "release-smoke / cluster-smoke", "release-smoke / docker-smoke", "release-smoke / local-checks"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
	}
	assertCalled(t, f.calls, "exec git push --atomic")
}
