package main

import (
	"strings"
	"testing"
)

const versionSrc = `package version

const (
	Major      = 0
	Minor      = 11
	Patch      = 0
	PreRelease = "devel"
)
`

const implSrc = `package gorumsimpl

const (
	MaxVersion = version.Minor
	GenVersion = 11
	MinVersion = 10
)
`

func TestParseVersionFile(t *testing.T) {
	got, err := parseVersionFile([]byte(versionSrc))
	if err != nil {
		t.Fatal(err)
	}
	if want := (semver{0, 11, 0, "devel"}); got != want {
		t.Errorf("parseVersionFile() = %+v, want %+v", got, want)
	}
}

func TestRewriteVersionFile(t *testing.T) {
	tests := []struct {
		name string
		v    semver
		want []string
	}{
		{"release", semver{0, 12, 0, ""}, []string{"Major      = 0", "Minor      = 12", "Patch      = 0", `PreRelease = ""`}},
		{"patch", semver{0, 11, 3, ""}, []string{"Minor      = 11", "Patch      = 3"}},
		{"candidate", semver{1, 0, 0, "rc.1"}, []string{"Major      = 1", "Minor      = 0", `PreRelease = "rc.1"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := rewriteVersionFile([]byte(versionSrc), tt.v)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(string(out), w) {
					t.Errorf("rewritten file lacks %q:\n%s", w, out)
				}
			}
			back, err := parseVersionFile(out)
			if err != nil || back != tt.v {
				t.Errorf("round trip = %+v, %v; want %+v", back, err, tt.v)
			}
		})
	}
}

func TestRewriteVersionFileErrors(t *testing.T) {
	tests := map[string]string{
		"missing constant":   strings.ReplaceAll(versionSrc, "Patch      = 0\n", ""),
		"duplicate constant": versionSrc + "\nconst (\n\tMajor = 2\n)\n",
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := rewriteVersionFile([]byte(src), semver{0, 12, 0, ""}); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestParseRuntimeVersions(t *testing.T) {
	gen, minV, err := parseRuntimeVersions([]byte(implSrc))
	if err != nil {
		t.Fatal(err)
	}
	if gen != 11 || minV != 10 {
		t.Errorf("parseRuntimeVersions() = %d, %d; want 11, 10", gen, minV)
	}
}
