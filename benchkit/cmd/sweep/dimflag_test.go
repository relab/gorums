package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums/benchkit"
)

// TestDimFlagSet verifies that the -dim flag accepts a lowercase name with
// distinct values, keeps the dimensions sorted by name, and rejects every
// malformed or ambiguous declaration with an error that names the cause.
func TestDimFlagSet(t *testing.T) {
	var dims []customDimension
	fs := flag.NewFlagSet("sweep", flag.ContinueOnError)
	fs.Var(dimFlag{&dims}, "dim", "")
	if err := fs.Parse([]string{"-dim", "t=1,2,3", "-dim", "ready=all,tree", "-dim", "d=20,22.5,-1,1e5"}); err != nil {
		t.Fatal(err)
	}
	want := []customDimension{
		{name: "d", values: []string{"20", "22.5", "-1", "1e5"}},
		{name: "ready", values: []string{"all", "tree"}},
		{name: "t", values: []string{"1", "2", "3"}},
	}
	if !slices.EqualFunc(dims, want, func(a, b customDimension) bool {
		return a.name == b.name && slices.Equal(a.values, b.values)
	}) {
		t.Errorf("dims = %v, want %v", dims, want)
	}

	tests := []struct {
		value   string
		wantErr string
	}{
		{"d", "want name=v1,v2"},
		{"=1,2", "invalid name"},
		{"D=1", "invalid name"},
		{"1d=1", "invalid name"},
		{"max_async=1", "invalid name"},
		{"workers=1", "reserved"},
		{"n=1", "reserved"},
		{"stream-mode=dual", "reserved"},
		{"time=1s", "reserved"},
		{"help=1", "reserved"},
		{"throughput=1", "reserved"},
		{"x=", "invalid value"},
		{"x=a,,b", "invalid value"},
		{"x=a b", "invalid value"},
		{"x=a/b", "invalid value"},
		{"x=a=b", "invalid value"},
		{"x=tree,Tree", "repeated value"},
		{"t=4", "repeated -dim t"},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			dims := []customDimension{{name: "t", values: []string{"1"}}}
			err := dimFlag{&dims}.Set(tt.value)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Set(%q) = %v, want error containing %q", tt.value, err, tt.wantErr)
			}
		})
	}
}

// TestDimFlagReservedNamesCoverCSVColumns verifies that no custom dimension can
// take the name of a column that sweep writes to a CSV file, so every column
// name in agg.csv and the other tables is unique.
func TestDimFlagReservedNamesCoverCSVColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")
	writers := map[string]func() error{
		"agg":        func() error { return writeAggRunsCSV(path, nil) },
		"comparison": func() error { return writeComparisonCSV(path, nil) },
		"runs":       func() error { return writePlotRunsCSV(path, nil) },
		"node_cdf":   func() error { return writePlotNodeCDFCSV(path, nil) },
		"nodes":      func() error { return writePlotNodesCSV(path, nil) },
		"tl_curve":   func() error { return writeTLCurveCSV(path, nil) },
		"health":     func() error { return writeNodeHealthCSV(path, nil) },
		"degraded":   func() error { return writeDegradedShareCSV(path, nil) },
		"run_status": func() error { return writeRunStatusCSV(path, nil) },
	}
	for name, write := range writers {
		if err := write(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		header, _, _ := strings.Cut(string(data), "\n")
		for column := range strings.SplitSeq(strings.TrimSpace(header), ",") {
			if !dimensionNamePattern.MatchString(column) {
				continue // no custom dimension can take this name
			}
			if !slices.Contains(reservedDimensionNames, column) {
				t.Errorf("%s.csv column %q is not a reserved dimension name", name, column)
			}
		}
	}
}

// TestDimFlagExtraArgsConflict verifies that a custom dimension also given in
// -extra-args is rejected: the node would receive the flag twice.
func TestDimFlagExtraArgsConflict(t *testing.T) {
	dims := []customDimension{{name: "d", values: []string{"20"}}, {name: "ready", values: []string{"all"}}}
	for _, extra := range []string{
		"-d=4", "--ready tree", "-x=1 -d 2", "'-d=9'", `"-d"=9`, `-\d=9`, "-x=1\t-d=9",
	} {
		if err := validateDimensionArgs(dims, extra); err == nil || !strings.Contains(err.Error(), "also given in -extra-args") {
			t.Errorf("validateDimensionArgs(%q) = %v, want an -extra-args error", extra, err)
		}
	}
	for _, extra := range []string{
		"", "-dd=4 -fault-kill-after=5s", "d=4", `-note="contains -d=9 here"`,
		"-note='-d=9'", "-x=1 -- -d=9", "- -x",
	} {
		if err := validateDimensionArgs(dims, extra); err != nil {
			t.Errorf("validateDimensionArgs(%q) = %v, want nil", extra, err)
		}
	}
	for _, extra := range []string{"'-d=9", `-note="-d=9`} {
		if err := validateDimensionArgs(dims, extra); err == nil || !strings.Contains(err.Error(), "unterminated") {
			t.Errorf("validateDimensionArgs(%q) = %v, want an unterminated quote error", extra, err)
		}
	}
}

// TestDimFlagExtraArgsNodeCommand verifies the -extra-args check against the
// arguments a node receives: the shell splits the node command into words, and
// the node's flag parser takes the last value of a repeated flag. The check
// rejects exactly the -extra-args that override the -dim value of the run.
func TestDimFlagExtraArgsNodeCommand(t *testing.T) {
	dims := []customDimension{{name: "d", values: []string{"20", "22"}}}
	tests := []struct {
		name      string
		extraArgs string
		wantD     string // the value of -d that the node's flag parser takes
		wantNote  string // the value of -note that the node's flag parser takes
		wantErr   bool
	}{
		{name: "QuotedFlagOverridesDim", extraArgs: "'-d=9'", wantD: "9", wantErr: true},
		{name: "FlagInsideQuotedValue", extraArgs: `-note="contains -d=9 here"`, wantD: "20", wantNote: "contains -d=9 here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config{duration: time.Second, prog: newRemoteProgram(""), extraArgs: tt.extraArgs}
			spec := runSpec{Benchmark: "Q", Params: benchkit.NewParams(map[string]string{"d": "20"})}
			cmd := buildNodeCmd(nodeAssignment{host: "bb1", port: 9000}, "bb1:9000", spec, "run", cfg)
			words, err := shellWords(cmd)
			if err != nil {
				t.Fatalf("shellWords(%q) = %v", cmd, err)
			}
			fs := flag.NewFlagSet("node", flag.ContinueOnError)
			for _, name := range []string{"self", "remotes", "benchmarks", "workers", "payload", "time", "output", "d", "note"} {
				fs.String(name, "", "")
			}
			if err := fs.Parse(words[1:]); err != nil {
				t.Fatalf("node flags %q: %v", words[1:], err)
			}
			if got := fs.Lookup("d").Value.String(); got != tt.wantD {
				t.Errorf("node -d = %q, want %q", got, tt.wantD)
			}
			if got := fs.Lookup("note").Value.String(); got != tt.wantNote {
				t.Errorf("node -note = %q, want %q", got, tt.wantNote)
			}
			if err := validateDimensionArgs(dims, tt.extraArgs); (err != nil) != tt.wantErr {
				t.Errorf("validateDimensionArgs(%q) = %v, want error %t", tt.extraArgs, err, tt.wantErr)
			}
		})
	}
}

// TestDimFlagShellWords verifies that shellWords splits a string into the
// words a POSIX shell passes to a command. Where /bin/sh is present, the test
// also compares each result with the words the shell itself produces.
func TestDimFlagShellWords(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{name: "Empty", in: "", want: nil},
		{name: "Blanks", in: " \t ", want: nil},
		{name: "Plain", in: "  -a  -b=1 ", want: []string{"-a", "-b=1"}},
		{name: "Tab", in: "a\tb", want: []string{"a", "b"}},
		{name: "SingleQuoted", in: "'-d=9'", want: []string{"-d=9"}},
		{name: "SingleQuotedBlanks", in: "'a b'  c", want: []string{"a b", "c"}},
		{name: "SingleQuotedBackslash", in: `'a\b\'`, want: []string{`a\b\`}},
		{name: "SingleQuotedDoubleQuote", in: `'say "hi"'`, want: []string{`say "hi"`}},
		{name: "DoubleQuoted", in: `-note="contains -d=9 here"`, want: []string{"-note=contains -d=9 here"}},
		{name: "DoubleQuotedSingleQuote", in: `-x "it's"`, want: []string{"-x", "it's"}},
		{name: "DoubleQuotedEscapes", in: `"a\"b\\c\$d\` + "`" + `e"`, want: []string{`a"b\c$d` + "`" + "e"}},
		{name: "DoubleQuotedLiteralBackslash", in: `"a\b"`, want: []string{`a\b`}},
		{name: "DoubleQuotedLineContinuation", in: "\"a\\\nb\"", want: []string{"ab"}},
		{name: "EscapedBlank", in: `a\ b`, want: []string{"a b"}},
		{name: "EscapedQuote", in: `\'a\"`, want: []string{`'a"`}},
		{name: "EscapedLetter", in: `-\d=9`, want: []string{"-d=9"}},
		{name: "LineContinuation", in: "a\\\nb", want: []string{"ab"}},
		{name: "LineContinuationAlone", in: "a \\\n b", want: []string{"a", "b"}},
		{name: "EmptyQuotes", in: `'' ""`, want: []string{"", ""}},
		{name: "AdjacentParts", in: `a''b"c"'d'`, want: []string{"abcd"}},
		{name: "Parameter", in: `$X "${Y}"`, want: []string{"$X", "${Y}"}},
		{name: "UnterminatedSingle", in: "'-d=9", wantErr: true},
		{name: "UnterminatedDouble", in: `-note="-d=9`, wantErr: true},
		{name: "UnterminatedEscapedDouble", in: `"a\"`, wantErr: true},
	}
	sh, shErr := exec.LookPath("sh")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shellWords(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("shellWords(%q) error = %v, want error %t", tt.in, err, tt.wantErr)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("shellWords(%q) = %q, want %q", tt.in, got, tt.want)
			}
			// The shell expands parameters, which shellWords keeps literally.
			if tt.wantErr || shErr != nil || strings.Contains(tt.in, "$") {
				return
			}
			// The leading x keeps printf from printing one empty word when
			// tt.in has no words.
			out, err := exec.Command(sh, "-c", `printf '%s\0' x `+tt.in).Output()
			if err != nil {
				t.Fatalf("sh: %v", err)
			}
			shWords := strings.Split(string(bytes.TrimSuffix(out, []byte{0})), "\x00")[1:]
			if !slices.Equal(got, shWords) {
				t.Errorf("shellWords(%q) = %q, sh gives %q", tt.in, got, shWords)
			}
		})
	}
}
