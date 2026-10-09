package gengorums

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// wantReservedIdents is the set of reserved identifiers the dev package exports.
var wantReservedIdents = []string{"Config", "ConfigContext", "Node", "NodeContext"}

// TestBundleReservedIdentifiers pins the set of identifiers that the bundler
// will mark as reserved and inject as type aliases into every generated
// _gorums.pb.go file. If this test fails, update aliases.go intentionally and
// then update wantReservedIdents to match.
func TestBundleReservedIdentifiers(t *testing.T) {
	pkg, err := loadPackage(devDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, got := findIdentifiers(pkg)
	if !slices.Equal(got, wantReservedIdents) {
		t.Errorf("generated static surface changed:\ngot:  %v\nwant: %v\nIf intentional, update aliases.go and wantReservedIdents.", got, wantReservedIdents)
	}
}

// TestBundleStaticFileUpToDate checks that template_static.go matches
// the bundle generated from the static files in the dev package.
// If this test fails, run make dev to regenerate template_static.go.
func TestBundleStaticFileUpToDate(t *testing.T) {
	committed, err := os.ReadFile("template_static.go")
	if err != nil {
		t.Fatal(err)
	}
	// The bundle loads the dev package relative to the repository root.
	t.Chdir("../../..")
	generated, err := staticBundle()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(string(committed), string(generated)); diff != "" {
		t.Errorf("template_static.go is out of date; run make dev (-committed +generated):\n%s", diff)
	}
}

// TestStaticFiles checks that only static Go files are selected for the
// bundle: generated zorums files, tests, non-Go files, and directories are not.
func TestStaticFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"b.go", "a.go", "zorums.pb.go", "zorums_server_gorums.pb.go",
		"a_test.go", "zorums.proto", "notes.txt",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := staticFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.go"), filepath.Join(dir, "b.go")}
	if !slices.Equal(got, want) {
		t.Errorf("staticFiles() = %v, want %v", got, want)
	}
}

// TestStaticFilesEmpty checks that a directory without static files is an error.
func TestStaticFilesEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "zorums.pb.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := staticFiles(dir); err == nil {
		t.Error("staticFiles() succeeded on a directory without static Go files")
	}
}

// TestLoadPackageIgnoresBrokenGeneratedFile checks that a broken generated
// dev file, whether it replaces an existing one or is new, does not stop the
// bundler. make dev bundles the static files before it regenerates the
// generated files.
func TestLoadPackageIgnoresBrokenGeneratedFile(t *testing.T) {
	existing := filepath.Join(devDir(t), "zorums_server_gorums.pb.go")
	added := filepath.Join(devDir(t), "zorums_added_gorums.pb.go")
	for _, tc := range []struct {
		name string
		src  string
	}{
		{name: "syntax error", src: "package dev\n\nfunc broken( {\n"},
		{name: "type error", src: "package dev\n\nvar broken int = \"no\"\n"},
		{name: "missing package clause", src: "func broken( {\n"},
		{name: "wrong package clause", src: "package broken\n"},
		{name: "missing import", src: "package dev\n\nimport _ \"does.not.exist/removed\"\n"},
		{name: "invalid build constraint", src: "//go:build (\n\npackage dev\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg, err := loadPackage(devDir(t), map[string][]byte{
				existing: []byte(tc.src),
				added:    []byte(tc.src),
			})
			if err != nil {
				t.Fatalf("broken generated file blocked the bundle: %v", err)
			}
			if _, got := findIdentifiers(pkg); !slices.Equal(got, wantReservedIdents) {
				t.Fatalf("reserved identifiers = %v, want %v", got, wantReservedIdents)
			}
		})
	}
}

// TestLoadPackageReportsStaticFileError checks that an error in a static dev
// file still fails the bundle. Those files are the bundle inputs.
func TestLoadPackageReportsStaticFileError(t *testing.T) {
	aliases := filepath.Join(devDir(t), "aliases.go")
	for _, tc := range []struct {
		name string
		src  string
	}{
		{name: "syntax error", src: "package dev\n\nfunc broken( {\n"},
		{name: "type error", src: "package dev\n\nvar broken int = \"no\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadPackage(devDir(t), map[string][]byte{
				aliases: []byte(tc.src),
			})
			if err == nil || !strings.Contains(err.Error(), "aliases.go") {
				t.Fatalf("broken static file error = %v", err)
			}
		})
	}
}

// devDir returns the absolute path of the dev package directory.
func devDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../dev")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
