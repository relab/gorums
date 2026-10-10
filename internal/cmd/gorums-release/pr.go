package main

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
)

// prOptions are the flags of the pr subcommand.
type prOptions struct {
	web bool
}

// pr commits the prepared files on a release branch and opens the pull request.
// The version commit comes first and the regenerated code last, in separate
// commits.
func (t *tool) pr(o prOptions) error {
	v, err := t.currentVersion()
	if err != nil {
		return err
	}
	tag := v.String()
	branch := "release/" + tag
	if err := t.require(t.checkOnMaster()); err != nil {
		return err
	}
	out, err := t.query("git", "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	changes := parseStatus(out)
	var other, generated []string
	var unexpected []string
	for _, p := range changes {
		switch releaseGroup(p) {
		case groupVersion:
			other = append(other, p)
		case groupGenerated:
			generated = append(generated, p)
		default:
			unexpected = append(unexpected, p)
		}
	}
	if len(unexpected) > 0 {
		return fmt.Errorf("unexpected changes, not part of a release: %s", strings.Join(unexpected, ", "))
	}
	if !slices.Contains(other, versionFile) {
		return fmt.Errorf("%s is unchanged: run prepare first", versionFile)
	}
	slices.Sort(other)
	slices.Sort(generated)

	if err := t.exec("git", "switch", "-c", branch); err != nil {
		return err
	}
	if err := t.commit(other, "gorums: release "+tag); err != nil {
		return err
	}
	if len(generated) > 0 {
		if err := t.commit(generated, "all: regenerate code for "+tag); err != nil {
			return err
		}
	}
	// The version was chosen before dependencies were upgraded, so check it
	// again on the committed tree before anything is pushed.
	report, err := t.report(tag)
	if err := t.require(err); err != nil {
		return fmt.Errorf("%w\nThe release branch %s has local commits and is not pushed", err, branch)
	}
	if err := t.exec("git", "push", "-u", "origin", "HEAD"); err != nil {
		return err
	}
	if err := t.exec("gh", "pr", "create", "--base", "master", "--head", branch,
		"--title", "gorums: release "+tag, "--body", prBody(tag, report)); err != nil {
		return err
	}
	if !t.dryRun {
		if url, err := t.query("gh", "pr", "view", branch, "--json", "url", "--jq", ".url"); err == nil {
			t.logf("Release PR: %s", strings.TrimSpace(url))
		}
	}
	if o.web {
		if err := t.exec("gh", "pr", "view", "--web", branch); err != nil {
			return err
		}
	}
	t.logf("After CI passes, run: %s publish", progName)
	return nil
}

// checkOnMaster requires the master branch as the starting point.
func (t *tool) checkOnMaster() error {
	branch, err := t.currentBranch()
	if err != nil {
		return err
	}
	if branch != "master" {
		return fmt.Errorf("on branch %q: switch to master", branch)
	}
	return nil
}

// commit stages exactly the named paths and commits them.
func (t *tool) commit(paths []string, subject string) error {
	if err := t.exec("git", append([]string{"add", "--"}, paths...)...); err != nil {
		return err
	}
	return t.exec("git", "commit", "-m", subject)
}

// prBody is the description of the release pull request.
func prBody(tag, report string) string {
	body := "Release " + tag + "."
	if report != "" {
		body += "\n\nReport from gorelease:\n\n```\n" + report + "\n```"
	}
	return body
}

// The groups of files that a release changes. Each group is one commit.
const (
	groupVersion   = "version"   // version constants and dependency files
	groupGenerated = "generated" // output of make genproto
)

// releaseGroup returns the group of a file that prepare changes, or "" for a
// file that a release does not change.
func releaseGroup(p string) string {
	switch {
	case isGenerated(p):
		return groupGenerated
	case p == versionFile || p == runtimeFile || p == "go.work" || isModuleFile(p):
		return groupVersion
	}
	return ""
}

// isGenerated reports whether p is produced by make genproto.
func isGenerated(p string) bool {
	return strings.HasSuffix(p, ".pb.go") ||
		p == "cmd/protoc-gen-gorums/gengorums/template_static.go"
}

// isModuleFile reports whether p is the go.mod or go.sum of a module.
func isModuleFile(p string) bool {
	base := path.Base(p)
	if base != "go.mod" && base != "go.sum" {
		return false
	}
	dir := path.Dir(p)
	if dir == "." {
		dir = ""
	}
	return slices.Contains(modules, dir)
}

// parseStatus returns the paths listed by git status --porcelain.
func parseStatus(out string) []string {
	var paths []string
	for line := range strings.Lines(out) {
		line = strings.TrimRight(line, "\n")
		if len(line) < 4 {
			continue
		}
		p := line[3:]
		if _, to, ok := strings.Cut(p, " -> "); ok {
			p = to
		}
		if u, err := strconv.Unquote(p); err == nil {
			p = u
		}
		paths = append(paths, p)
	}
	return paths
}
