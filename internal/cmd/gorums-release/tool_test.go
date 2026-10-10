package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner records commands and answers queries from canned output.
type fakeRunner struct {
	calls   []string          // "query <cmd>" or "exec <cmd>"
	answers map[string]string // text in the command line -> output
	fail    map[string]error  // text in the command line -> error
	// failFirst makes the first n runs of a command containing the key fail with errBoom.
	failFirst map[string]int
	// onExec, if set, runs when a state-changing command starts.
	onExec func(line string)
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{answers: map[string]string{}, fail: map[string]error{}, failFirst: map[string]int{}}
}

// longest returns the value for the longest key contained in line.
func longest[V any](m map[string]V, line string) (V, bool) {
	var best string
	var val V
	var found bool
	for k, v := range m {
		if strings.Contains(line, k) && (!found || len(k) > len(best)) {
			best, val, found = k, v, true
		}
	}
	return val, found
}

// result returns the configured error for c.
func (f *fakeRunner) result(c cmd) error {
	for k, n := range f.failFirst {
		if n > 0 && strings.Contains(c.String(), k) {
			f.failFirst[k]--
			return errBoom
		}
	}
	err, _ := longest(f.fail, c.String())
	return err
}

func (f *fakeRunner) Query(c cmd) (string, error) {
	f.calls = append(f.calls, "query "+c.String())
	err := f.result(c)
	out, _ := longest(f.answers, c.String())
	return out, err
}

func (f *fakeRunner) Exec(c cmd) error {
	f.calls = append(f.calls, "exec "+c.String())
	if f.onExec != nil {
		f.onExec(c.String())
	}
	return f.result(c)
}

// execs returns the state-changing commands, in order.
func (f *fakeRunner) execs() []string {
	var out []string
	for _, c := range f.calls {
		if s, ok := strings.CutPrefix(c, "exec "); ok {
			out = append(out, s)
		}
	}
	return out
}

// assertOrder checks that each want is a prefix of some call, in order.
func assertOrder(t *testing.T, calls []string, wants ...string) {
	t.Helper()
	i := 0
	for _, w := range wants {
		found := false
		for ; i < len(calls); i++ {
			if strings.HasPrefix(calls[i], w) {
				found = true
				i++
				break
			}
		}
		if !found {
			t.Fatalf("missing %q, in order, in calls:\n%s", w, strings.Join(calls, "\n"))
		}
	}
}

// assertCalled checks that some call contains sub.
func assertCalled(t *testing.T, calls []string, sub string) {
	t.Helper()
	for _, c := range calls {
		if strings.Contains(c, sub) {
			return
		}
	}
	t.Fatalf("no call contains %q in calls:\n%s", sub, strings.Join(calls, "\n"))
}

// setTestVersion writes version v to the test repository's version file.
func setTestVersion(t *testing.T, tl *tool, v string) {
	t.Helper()
	sv, err := parseSemver(v)
	if err != nil {
		t.Fatal(err)
	}
	out, err := rewriteVersionFile([]byte(versionSrc), sv)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tl.root, versionFile), out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertAbsent checks that no call has the given prefix.
func assertAbsent(t *testing.T, calls []string, prefix string) {
	t.Helper()
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			t.Errorf("unexpected call %q", c)
		}
	}
}

// newTestTool builds a tool on a temporary repository root that holds the
// two version files.
func newTestTool(t *testing.T, f *fakeRunner) (*tool, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	for rel, src := range map[string]string{versionFile: versionSrc, runtimeFile: implSrc} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	return &tool{
		root:     root,
		run:      f,
		out:      &out,
		in:       strings.NewReader(""),
		sleep:    func(time.Duration) {},
		lookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		path:     "/home/u/go/bin:/usr/bin",

		verifyTimeout: time.Minute,
	}, &out
}

func readVersion(t *testing.T, tl *tool) semver {
	t.Helper()
	v, err := tl.currentVersion()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		name     string
		terminal bool
		input    string
		yes      bool
		wantErr  bool
	}{
		{"yes flag", false, "", true, false},
		{"no terminal", false, "y\n", false, true},
		{"answer y", true, "y\n", false, false},
		{"answer yes", true, "YES\n", false, false},
		{"answer n", true, "n\n", false, true},
		{"empty answer", true, "\n", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl, _ := newTestTool(t, newFakeRunner())
			tl.isTerminal, tl.in = tt.terminal, strings.NewReader(tt.input)
			if err := tl.confirm("go?", tt.yes); (err != nil) != tt.wantErr {
				t.Errorf("confirm() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequire(t *testing.T) {
	err := errors.New("boom")
	tl, out := newTestTool(t, newFakeRunner())
	if got := tl.require(err); got != err {
		t.Errorf("require() = %v, want %v", got, err)
	}
	tl.dryRun = true
	if got := tl.require(err); got != nil {
		t.Errorf("require() in dry run = %v, want nil", got)
	}
	if !strings.Contains(out.String(), "warning (dry run): boom") {
		t.Errorf("no warning in output: %q", out.String())
	}
}

var errBoom = errors.New("boom")
