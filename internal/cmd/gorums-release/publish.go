package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Verification of the new tags polls the Go module proxy at this interval,
// for at most defaultVerifyTimeout.
const (
	verifyInterval       = 10 * time.Second
	defaultVerifyTimeout = 5 * time.Minute
)

// publishOptions are the flags of the publish subcommand.
type publishOptions struct {
	yes   bool
	draft bool
}

// prInfo is the part of gh pr view --json output that publish uses.
type prInfo struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	State       string `json:"state"`
	MergeCommit struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
	StatusCheckRollup []checkRun `json:"statusCheckRollup"`
}

// checkRun is one entry of a pull request's status check rollup: either a
// check run or a commit status.
type checkRun struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Context    string `json:"context"`
	State      string `json:"state"`
}

// publish merges the release PR if needed, then tags the merge commit, creates
// the GitHub release, and checks that the Go module proxy serves the new tags.
func (t *tool) publish(o publishOptions) error {
	v, err := t.currentVersion()
	if err != nil {
		return err
	}
	tag := v.String()
	sha, err := t.mergeRelease("release/"+tag, o.yes)
	if err != nil {
		return err
	}
	if err := t.exec("git", "fetch", "origin", "master"); err != nil {
		return err
	}
	// The tags name the merge commit of the release pull request. HEAD is not
	// proof of the release revision: the local master can hold other commits,
	// and origin/master can have moved on.
	if sha == "" {
		sha = "<merge-commit>" // a dry run has not merged the pull request
	} else if err := t.require(t.checkReleaseCommit(sha, v)); err != nil {
		return err
	}
	if err := t.checkTagsFree(tag); err != nil {
		return err
	}
	benchkitTag := "benchkit/" + tag
	if err := t.exec("git", "tag", "-a", tag, "-m", "Gorums "+tag, sha); err != nil {
		return err
	}
	if err := t.exec("git", "tag", "-a", benchkitTag, "-m", "Gorums benchkit "+tag, sha); err != nil {
		return err
	}
	if err := t.exec("git", "push", "--atomic", "origin", "refs/tags/"+tag, "refs/tags/"+benchkitTag); err != nil {
		return err
	}
	if err := t.createRelease(tag, v, o); err != nil {
		return err
	}
	return t.verify(tag)
}

// lookupPR reads the release pull request for a branch.
func (t *tool) lookupPR(branch string) (prInfo, error) {
	out, err := t.query("gh", "pr", "view", branch, "--json", "number,url,state,mergeCommit,statusCheckRollup")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no pull requests found") {
			return prInfo{}, fmt.Errorf("no pull request found for %s: run %s pr first", branch, progName)
		}
		return prInfo{}, fmt.Errorf("cannot look up the release pull request: %w", err)
	}
	var pr prInfo
	if err := json.Unmarshal([]byte(out), &pr); err != nil {
		return prInfo{}, fmt.Errorf("cannot read pull request: %w", err)
	}
	return pr, nil
}

// mergeRelease squash-merges the release PR when it is still open, and returns
// the merge commit. A dry run returns "" for an open pull request.
func (t *tool) mergeRelease(branch string, yes bool) (string, error) {
	pr, err := t.lookupPR(branch)
	if err != nil {
		return "", err
	}
	switch pr.State {
	case "MERGED":
		t.logf("Pull request %s is already merged.", pr.URL)
	case "OPEN":
		if len(pr.StatusCheckRollup) == 0 {
			return "", fmt.Errorf("no CI checks reported for %s", pr.URL)
		}
		pending, failed := checksState(pr.StatusCheckRollup)
		if len(failed) > 0 {
			return "", fmt.Errorf("CI checks failed for %s: %s", pr.URL, strings.Join(failed, ", "))
		}
		if len(pending) > 0 {
			return "", fmt.Errorf("CI checks still running for %s: %s", pr.URL, strings.Join(pending, ", "))
		}
		if err := t.confirm(fmt.Sprintf("Squash-merge %s?", pr.URL), yes); err != nil {
			return "", err
		}
		if err := t.exec("gh", "pr", "merge", branch, "--squash", "--delete-branch"); err != nil {
			return "", err
		}
		if t.dryRun {
			return "", nil
		}
		if pr, err = t.lookupPR(branch); err != nil {
			return "", err
		}
		if pr.State != "MERGED" {
			return "", fmt.Errorf("pull request %s is %s after the merge", pr.URL, strings.ToLower(pr.State))
		}
	default:
		return "", fmt.Errorf("pull request %s is %s, not merged", pr.URL, strings.ToLower(pr.State))
	}
	if pr.MergeCommit.OID == "" {
		return "", fmt.Errorf("pull request %s has no merge commit", pr.URL)
	}
	return pr.MergeCommit.OID, nil
}

// checkReleaseCommit requires that sha is in this repository, is on
// origin/master, and carries the version v.
func (t *tool) checkReleaseCommit(sha string, v semver) error {
	if _, err := t.query("git", "cat-file", "-e", sha+"^{commit}"); err != nil {
		return fmt.Errorf("the merge commit %s is not in this repository: %w", sha, err)
	}
	if _, err := t.query("git", "merge-base", "--is-ancestor", sha, "origin/master"); err != nil {
		return fmt.Errorf("the merge commit %s is not on origin/master: %w", sha, err)
	}
	src, err := t.query("git", "show", sha+":"+versionFile)
	if err != nil {
		return err
	}
	at, err := parseVersionFile([]byte(src))
	if err != nil {
		return err
	}
	if at != v {
		return fmt.Errorf("the merge commit %s has version %s, not %s", sha, at, v)
	}
	return nil
}

// checksState lists the names of unfinished and unsuccessful checks.
func checksState(rollup []checkRun) (pending, failed []string) {
	for _, c := range rollup {
		if c.Typename == "StatusContext" {
			switch c.State {
			case "SUCCESS":
			case "PENDING", "EXPECTED":
				pending = append(pending, c.Context)
			default:
				failed = append(failed, c.Context)
			}
			continue
		}
		switch {
		case c.Status != "COMPLETED":
			pending = append(pending, c.Name)
		case c.Conclusion != "SUCCESS" && c.Conclusion != "SKIPPED" && c.Conclusion != "NEUTRAL":
			failed = append(failed, c.Name)
		}
	}
	return pending, failed
}

// checkTagsFree requires that neither release tag exists locally or on origin.
func (t *tool) checkTagsFree(tag string) error {
	benchkitTag := "benchkit/" + tag
	local, err := t.query("git", "tag", "-l", tag, benchkitTag)
	if err != nil {
		return err
	}
	if strings.TrimSpace(local) != "" {
		return fmt.Errorf("tag already exists locally: %s", strings.Join(strings.Fields(local), ", "))
	}
	remote, err := t.query("git", "ls-remote", "--tags", "origin", "refs/tags/"+tag, "refs/tags/"+benchkitTag)
	if err != nil {
		return err
	}
	if strings.TrimSpace(remote) != "" {
		return fmt.Errorf("tag already exists on origin:\n%s", strings.TrimSpace(remote))
	}
	return nil
}

// createRelease creates the GitHub release. GitHub generates the change list;
// the gorelease report is placed before it.
func (t *tool) createRelease(tag string, v semver, o publishOptions) error {
	args := []string{"release", "create", tag, "--title", "Gorums " + tag, "--generate-notes"}
	if report, err := t.report(tag); err != nil {
		t.logf("warning: the release notes have no gorelease report: %v", err)
	} else if report != "" {
		args = append(args, "--notes", "API changes reported by gorelease:\n\n```\n"+report+"\n```\n")
	}
	if v.pre != "" {
		args = append(args, "--prerelease")
	}
	if o.draft {
		args = append(args, "--draft")
	}
	return t.exec("gh", append(args, "--verify-tag")...)
}

// verify builds a throwaway module that requires both new tags, as a user of
// Gorums would, and retries until the Go proxy serves them or the timeout ends.
func (t *tool) verify(tag string) error {
	if t.dryRun {
		t.logf("+ verify that the Go proxy serves %s and benchkit/%s", tag, tag)
		return nil
	}
	dir, err := os.MkdirTemp("", "gorums-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	// The module cache must not be inside the module: ./... would match it.
	modDir := filepath.Join(dir, "module")
	if err := os.Mkdir(modDir, 0o755); err != nil {
		return err
	}
	src := "package main\n\nimport (\n\t_ \"github.com/relab/gorums\"\n\t_ \"github.com/relab/gorums/benchkit\"\n)\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(modDir, "main.go"), []byte(src), 0o644); err != nil {
		return err
	}
	// Use the public proxy and a fresh module cache, so that neither the
	// maintainer's settings nor a cached copy can hide a missing tag.
	env := []string{
		"GOWORK=off",
		"GOPROXY=https://proxy.golang.org",
		"GOSUMDB=sum.golang.org",
		"GOPRIVATE=",
		"GONOPROXY=",
		"GONOSUMDB=",
		"GOFLAGS=-modcacherw",
		"GOMODCACHE=" + filepath.Join(dir, "modcache"),
	}
	step := func(args ...string) error {
		return t.execCmd(cmd{dir: modDir, env: env, name: "go", args: args})
	}
	if err := step("mod", "init", "example.com/verify"); err != nil {
		return err
	}
	attempts := max(1, int(t.verifyTimeout/verifyInterval))
	for i := 1; ; i++ {
		err = step("get", gorumsModule+"@"+tag, gorumsModule+"/benchkit@"+tag)
		if err == nil {
			if err = step("mod", "tidy"); err == nil {
				err = step("build", "./...")
			}
		}
		if err == nil {
			t.logf("Verified: %s and benchkit/%s are available.", tag, tag)
			return nil
		}
		if i >= attempts {
			return fmt.Errorf("release %s is published, but not available through the Go proxy after %s: %w", tag, t.verifyTimeout, err)
		}
		t.logf("Not available yet (attempt %d of %d); waiting %s.", i, attempts, verifyInterval)
		t.sleep(verifyInterval)
	}
}
