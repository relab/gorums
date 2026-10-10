package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// modules lists the Go modules of the repository, relative to the root.
var modules = []string{"", "examples", "benchkit"}

// tool holds what every subcommand needs.
type tool struct {
	root       string
	dryRun     bool
	run        runner
	out        io.Writer
	in         io.Reader
	isTerminal bool
	sleep      func(time.Duration)
	lookPath   func(string) (string, error) // finds a program on PATH
	path       string                       // the value of PATH

	// verifyTimeout bounds the wait for the Go module proxy after publishing.
	verifyTimeout time.Duration
}

// query runs a read-only command. A dry run still runs it, to plan the later
// steps, and prints it with a "?" so that it can be told from a skipped command.
func (t *tool) query(name string, args ...string) (string, error) {
	c := cmd{name: name, args: args}
	if t.dryRun {
		fmt.Fprintf(t.out, "? %s\n", c)
	}
	return t.run.Query(c)
}

// exec runs a state-changing command in the repository root.
func (t *tool) exec(name string, args ...string) error {
	return t.execCmd(cmd{name: name, args: args})
}

// execIn runs a state-changing command in dir, relative to the root.
func (t *tool) execIn(dir, name string, args ...string) error {
	return t.execCmd(cmd{dir: dir, name: name, args: args})
}

// execCmd prints c and, unless this is a dry run, runs it.
func (t *tool) execCmd(c cmd) error {
	fmt.Fprintf(t.out, "+ %s\n", c)
	if t.dryRun {
		return nil
	}
	return t.run.Exec(c)
}

// logf prints a progress line.
func (t *tool) logf(format string, args ...any) {
	fmt.Fprintf(t.out, format+"\n", args...)
}

// require reports a failed precondition. A dry run only warns, so that it can
// be tried from any branch.
func (t *tool) require(err error) error {
	if err != nil && t.dryRun {
		t.logf("warning (dry run): %v", err)
		return nil
	}
	return err
}

// readFile reads a file relative to the repository root.
func (t *tool) readFile(rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(t.root, rel))
}

// writeFile writes a file relative to the repository root, unless this is a
// dry run.
func (t *tool) writeFile(rel string, data []byte) error {
	t.logf("+ write %s", rel)
	if t.dryRun {
		return nil
	}
	return os.WriteFile(filepath.Join(t.root, rel), data, 0o644)
}

// currentVersion reads the version from internal/version/version.go.
func (t *tool) currentVersion() (semver, error) {
	src, err := t.readFile(versionFile)
	if err != nil {
		return semver{}, err
	}
	return parseVersionFile(src)
}

// currentBranch returns the checked-out branch.
func (t *tool) currentBranch() (string, error) {
	out, err := t.query("git", "branch", "--show-current")
	return strings.TrimSpace(out), err
}

// confirm asks a yes/no question. It fails without a terminal, unless yes is set.
func (t *tool) confirm(question string, yes bool) error {
	if yes {
		return nil
	}
	if !t.isTerminal {
		return errors.New("not a terminal: pass --yes to confirm")
	}
	fmt.Fprintf(t.out, "%s [y/N] ", question)
	line, _ := bufio.NewReader(t.in).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
		return errors.New("aborted")
	}
	return nil
}

// report returns the gorelease report for version v. It fails if gorelease
// rejects the version for the committed tree.
func (t *tool) report(v string) (string, error) {
	out, err := t.query("gorelease", "-version", v)
	if err != nil {
		return "", fmt.Errorf("gorelease rejects %s: %w\n%s", v, err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}
