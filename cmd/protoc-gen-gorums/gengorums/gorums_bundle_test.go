package gengorums

import (
	"os"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestReservedIdentifiers pins the set of identifiers that the bundler will
// mark as reserved and inject as type aliases into every generated _gorums.pb.go
// file. If this test fails, update aliases.go intentionally and then update
// the want slice below to match.
func TestReservedIdentifiers(t *testing.T) {
	pkg := loadPackage("github.com/relab/gorums/cmd/protoc-gen-gorums/dev")
	_, got := findIdentifiers(pkg)
	want := []string{"Config", "ConfigContext", "Node", "NodeContext"}
	if !slices.Equal(got, want) {
		t.Errorf("generated static surface changed:\ngot:  %v\nwant: %v\nIf intentional, update aliases.go and this want slice.", got, want)
	}
}

// TestGorumsBundleStaticFileUpToDate checks that template_static.go matches
// the bundle generated from the static files in the dev package.
// If this test fails, run make dev to regenerate template_static.go.
func TestGorumsBundleStaticFileUpToDate(t *testing.T) {
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
