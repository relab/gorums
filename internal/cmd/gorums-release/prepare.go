package main

import (
	"errors"
	"fmt"
	"strings"
)

const gorumsModule = "github.com/relab/gorums"

// prepareOptions are the flags of the prepare subcommand.
type prepareOptions struct {
	version     string // overrides the version suggested by gorelease; may have a pre-release suffix
	skipUpgrade bool
	skipTests   bool
}

// prepare upgrades dependencies, picks the next version, and updates every
// file that depends on it. It writes files and makes no commits.
func (t *tool) prepare(o prepareOptions) error {
	if o.version != "" {
		if _, err := parseSemver(o.version); err != nil {
			return err
		}
	}
	if err := t.require(t.checkWorkTree()); err != nil {
		return err
	}
	if err := t.checkTools(); err != nil {
		return err
	}
	// gorelease refuses a repository with uncommitted or untracked files, so
	// it runs before anything is modified.
	v, report, err := t.chooseVersion(o)
	if err != nil {
		return err
	}
	t.logf("%s", reportSummary(report))
	t.logf("")
	if o.version == "" {
		t.logf("Version: %s, suggested by gorelease.", v)
		t.logf("To release another version, for example a release candidate, run:")
		t.logf("  %s prepare --version %s-rc.1", progName, v)
	} else {
		t.logf("Version: %s, from --version.", v)
	}
	t.logf("")
	if !o.skipUpgrade {
		if err := t.upgrade(); err != nil {
			return err
		}
	}
	// Install the code generators pinned by the upgraded go.mod.
	if err := t.exec("make", "tools"); err != nil {
		return err
	}
	if err := t.setVersion(v); err != nil {
		return err
	}
	for _, m := range []string{"examples", "benchkit"} {
		if err := t.execIn(m, "go", "mod", "edit", "-require="+gorumsModule+"@"+v.String()); err != nil {
			return err
		}
	}
	if err := t.exec("make", "genproto"); err != nil {
		return err
	}
	// The benchkit module is not tidied again: it requires the new gorums
	// version, which does not exist until the tag is pushed.
	for _, m := range []string{"", "examples"} {
		if err := t.execIn(m, "go", "mod", "tidy"); err != nil {
			return err
		}
	}
	if !o.skipTests {
		if err := t.exec("make", "test"); err != nil {
			return err
		}
		if err := t.exec("make", "testrace"); err != nil {
			return err
		}
	}
	return t.prepareSummary(v)
}

// checkWorkTree requires a clean, up-to-date master.
func (t *tool) checkWorkTree() error {
	branch, err := t.currentBranch()
	if err != nil {
		return err
	}
	if branch != "master" {
		return fmt.Errorf("on branch %q: switch to master", branch)
	}
	out, err := t.query("git", "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "" {
		return errors.New("uncommitted or untracked files: commit or remove them first")
	}
	if _, err := t.query("git", "fetch", "origin", "master"); err != nil {
		return err
	}
	head, err := t.query("git", "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	remote, err := t.query("git", "rev-parse", "origin/master")
	if err != nil {
		return err
	}
	if strings.TrimSpace(head) != strings.TrimSpace(remote) {
		return errors.New("master differs from origin/master: pull or push first")
	}
	return nil
}

// checkTools installs gorelease and checks the other tools the release needs.
func (t *tool) checkTools() error {
	if _, err := t.query("gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is not installed or not logged in: %w", err)
	}
	if _, err := t.query("protoc", "--version"); err != nil {
		return fmt.Errorf("protoc is required: %w", err)
	}
	return t.exec("go", "install", "golang.org/x/exp/cmd/gorelease@latest")
}

// upgrade updates and tidies the dependencies of every module.
func (t *tool) upgrade() error {
	for _, m := range modules {
		if err := t.execIn(m, "go", "get", "-u", "./..."); err != nil {
			return err
		}
		if err := t.execIn(m, "go", "mod", "tidy"); err != nil {
			return err
		}
	}
	return nil
}

// chooseVersion returns the release version and the gorelease report. Without
// an override it adopts the version suggested by gorelease.
func (t *tool) chooseVersion(o prepareOptions) (semver, string, error) {
	args := []string(nil)
	if o.version != "" {
		args = []string{"-version", o.version}
	}
	report, err := t.query("gorelease", args...)
	if err != nil {
		return semver{}, "", fmt.Errorf("gorelease failed: %w\n%s", err, report)
	}
	name := o.version
	if name == "" {
		if name, err = parseSuggested(report); err != nil {
			return semver{}, "", err
		}
	}
	v, err := parseSemver(name)
	if err != nil {
		return semver{}, "", err
	}
	if o.version == "" && v.major >= 1 {
		return semver{}, "", fmt.Errorf("gorelease suggests %s; pass --version %s to confirm leaving v0", v, v)
	}
	cur, err := t.currentVersion()
	if err != nil {
		return semver{}, "", err
	}
	if v.compareCore(cur) < 0 {
		return semver{}, "", fmt.Errorf("version %s is older than the current %s", v, cur)
	}
	return v, strings.TrimSpace(report), nil
}

// setVersion rewrites internal/version/version.go.
func (t *tool) setVersion(v semver) error {
	src, err := t.readFile(versionFile)
	if err != nil {
		return err
	}
	out, err := rewriteVersionFile(src, v)
	if err != nil {
		return err
	}
	return t.writeFile(versionFile, out)
}

// prepareSummary tells the maintainer what to review and what comes next.
func (t *tool) prepareSummary(v semver) error {
	t.logf("")
	t.logf("Prepared %s.", v)
	if diff, err := t.query("git", "diff", "--stat"); err == nil {
		t.logf("%s", strings.TrimSpace(diff))
	}
	if src, err := t.readFile(runtimeFile); err == nil {
		if gen, minV, err := parseRuntimeVersions(src); err == nil {
			t.logf("")
			t.logf("Not changed by this tool: GenVersion = %d, MinVersion = %d (%s).", gen, minV, runtimeFile)
			t.logf("Raise them by hand only if generated code or runtime compatibility changed.")
		}
	}
	t.logf("")
	t.logf("Review the diff, then run: %s pr", progName)
	return nil
}
