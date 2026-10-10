package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const gorumsModule = "github.com/relab/gorums"

// prepareOptions are the flags of the prepare subcommand.
type prepareOptions struct {
	version     string // overrides the version suggested by gorelease; may have a pre-release suffix
	allowMajor  bool   // confirms a major version bump
	bumpGen     bool   // sets GenVersion to the new minor version
	bumpMin     bool   // sets MinVersion, and GenVersion, to the new minor version
	skipUpgrade bool
	skipTests   bool
}

// generatorOutputFiles are the generated files that show what the code
// generator emits, apart from the header that names the tool versions.
const generatorOutputFiles = "cmd/protoc-gen-gorums/dev/zorums*_gorums.pb.go"

// prepare upgrades dependencies, picks the next version, and updates every
// file that depends on it. It writes files and makes no commits.
func (t *tool) prepare(o prepareOptions) error {
	if o.version != "" {
		if _, err := parseSemver(o.version); err != nil {
			return err
		}
	}
	if err := t.require(t.checkPrerequisites()); err != nil {
		return err
	}
	if err := t.require(t.checkWorkTree()); err != nil {
		return err
	}
	if err := t.exec("go", "install", "golang.org/x/exp/cmd/gorelease@latest"); err != nil {
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
	t.announceVersion(v, o)
	cur, err := t.currentVersion()
	if err != nil {
		return err
	}
	if err := t.require(checkVersion(v, cur, o.allowMajor)); err != nil {
		return err
	}
	if err := t.require(t.checkMaxVersion(v)); err != nil {
		return err
	}
	t.generatorHint(parseBase(report), o)
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
	// The generated code contains GenVersion, so the runtime versions are
	// set before the code is regenerated.
	if err := t.bumpRuntime(v, o); err != nil {
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
	// Each module replaces gorums with the local checkout, so the unreleased
	// version resolves.
	for _, m := range modules {
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

// checkPrerequisites checks the programs and settings that the release needs.
func (t *tool) checkPrerequisites() error {
	for _, name := range []string{"git", "make", "go", "protoc", "gh"} {
		if _, err := t.lookPath(name); err != nil {
			return fmt.Errorf("%s is required but is not on PATH", name)
		}
	}
	if err := t.checkGoBinOnPath(); err != nil {
		return err
	}
	if _, err := t.query("gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is not logged in: run gh auth login: %w", err)
	}
	return nil
}

// checkGoBinOnPath requires that the directory where go install puts programs
// is on PATH, because protoc finds the code generators there.
func (t *tool) checkGoBinOnPath() error {
	out, err := t.query("go", "env", "GOBIN", "GOPATH")
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		return fmt.Errorf("unexpected output from go env: %q", out)
	}
	bin := strings.TrimSpace(lines[0])
	if bin == "" {
		gopath := filepath.SplitList(strings.TrimSpace(lines[1]))
		if len(gopath) == 0 {
			return errors.New("go env reports neither GOBIN nor GOPATH")
		}
		bin = filepath.Join(gopath[0], "bin")
	}
	for _, dir := range filepath.SplitList(t.path) {
		if filepath.Clean(dir) == filepath.Clean(bin) {
			return nil
		}
	}
	return fmt.Errorf("%s is not on PATH: go install puts gorelease and the code generators there", bin)
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
	if err := t.exec("git", "fetch", "origin", "master"); err != nil {
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
	return v, strings.TrimSpace(report), nil
}

// announceVersion tells where the version comes from and how to choose another.
func (t *tool) announceVersion(v semver, o prepareOptions) {
	if o.version != "" {
		t.logf("Version: %s, from -version.", v)
		return
	}
	t.logf("Version: %s, suggested by gorelease.", v)
	t.logf("To release another version, for example a release candidate, run:")
	t.logf("  %s prepare -version %s-rc.1", progName, v)
}

// checkVersion requires that v does not go back and that a major version bump
// is confirmed.
func checkVersion(v, cur semver, allowMajor bool) error {
	if v.compare(cur) < 0 {
		return fmt.Errorf("version %s is older than the current %s", v, cur)
	}
	if v.major == cur.major {
		return nil
	}
	if v.major >= 2 {
		return fmt.Errorf("version %s needs the module path %s/v%d, which %s cannot create", v, gorumsModule, v.major, progName)
	}
	if !allowMajor {
		return fmt.Errorf("version %s is a major version bump from %s: pass -allow-major to confirm", v, cur)
	}
	return nil
}

// checkMaxVersion requires that MaxVersion, which is the minor version, does
// not drop below GenVersion. This happens when a major bump resets the minor.
func (t *tool) checkMaxVersion(v semver) error {
	src, err := t.readFile(runtimeFile)
	if err != nil {
		return err
	}
	gen, _, err := parseRuntimeVersions(src)
	if err != nil {
		return err
	}
	if v.minor < gen {
		return fmt.Errorf("version %s sets MaxVersion to %d, below GenVersion %d: "+
			"generated code would not compile, so the runtime version scheme must change before this release", v, v.minor, gen)
	}
	return nil
}

// generatorHint tells the maintainer to consider the runtime flags when the
// code generator's output changed since the base version.
func (t *tool) generatorHint(base string, o prepareOptions) {
	if base == "" || o.bumpGen || o.bumpMin {
		return
	}
	out, err := t.query("git", "diff", "--numstat", "-I^//[[:space:]][[:space:]]*protoc", base, "--", generatorOutputFiles)
	if err != nil || strings.TrimSpace(out) == "" {
		return
	}
	n := len(strings.Split(strings.TrimSpace(out), "\n"))
	t.logf("The generator output changed in %d files since %s.", n, base)
	t.logf("If the new output needs runtime features that %s lacks, add -bump-gen.", base)
	t.logf("If the runtime no longer supports code generated by %s, add -bump-min.", base)
	t.logf("Decide now: the flags cannot be added to a prepared tree.")
	t.logf("")
}

// bumpRuntime sets GenVersion and MinVersion in runtime/gorumsimpl/version.go
// to the new minor version, as the flags ask.
func (t *tool) bumpRuntime(v semver, o prepareOptions) error {
	if !o.bumpGen && !o.bumpMin {
		return nil
	}
	src, err := t.readFile(runtimeFile)
	if err != nil {
		return err
	}
	gen, minV, err := parseRuntimeVersions(src)
	if err != nil {
		return err
	}
	newGen, newMin := v.minor, minV
	if o.bumpMin {
		newMin = v.minor
	}
	if newGen == gen && newMin == minV {
		t.logf("GenVersion is %d and MinVersion is %d: already at the new minor version.", gen, minV)
		return nil
	}
	out, err := rewriteRuntimeFile(src, newGen, newMin)
	if err != nil {
		return err
	}
	t.logf("GenVersion %d -> %d, MinVersion %d -> %d", gen, newGen, minV, newMin)
	return t.writeFile(runtimeFile, out)
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
			t.logf("Runtime versions: GenVersion = %d, MinVersion = %d, MaxVersion = %d.", gen, minV, v.minor)
		}
	}
	t.logf("")
	t.logf("Review the diff, then run: %s pr", progName)
	return nil
}
