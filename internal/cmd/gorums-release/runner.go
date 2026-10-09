package main

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmd describes one external command.
type cmd struct {
	dir  string   // relative to the repository root, or absolute; empty for the root
	env  []string // extra KEY=VALUE entries
	name string
	args []string
}

// String formats the command for display, in the form "(dir) name args".
func (c cmd) String() string {
	s := strings.Join(append([]string{c.name}, c.args...), " ")
	if len(c.env) > 0 {
		s = strings.Join(c.env, " ") + " " + s
	}
	if c.dir != "" {
		s = "(" + c.dir + ") " + s
	}
	return s
}

// runner executes commands. Query is for read-only commands whose output the
// caller needs; Exec is for commands that change state.
type runner interface {
	Query(c cmd) (string, error)
	Exec(c cmd) error
}

// execRunner runs commands with os/exec, relative to the repository root.
type execRunner struct {
	root           string
	stdout, stderr io.Writer
}

func (r execRunner) command(c cmd) *exec.Cmd {
	e := exec.Command(c.name, c.args...)
	e.Dir = r.root
	if c.dir != "" {
		e.Dir = c.dir
		if !filepath.IsAbs(c.dir) {
			e.Dir = filepath.Join(r.root, c.dir)
		}
	}
	if len(c.env) > 0 {
		e.Env = append(e.Environ(), c.env...)
	}
	return e
}

// Query returns the standard output of c. A failure carries the standard error.
func (r execRunner) Query(c cmd) (string, error) {
	out, err := r.command(c).Output()
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && len(ee.Stderr) > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	if err != nil {
		err = fmt.Errorf("%s: %w", c, err)
	}
	return string(out), err
}

// Exec runs c and streams its output.
func (r execRunner) Exec(c cmd) error {
	e := r.command(c)
	e.Stdout, e.Stderr = r.stdout, r.stderr
	if err := e.Run(); err != nil {
		return fmt.Errorf("%s: %w", c, err)
	}
	return nil
}
