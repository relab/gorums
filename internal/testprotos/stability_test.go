package testprotos_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/gorums/internal/gentest"
)

// devDir is the directory of the dev zorums.proto file, which captures all
// variations of Gorums-specific code generation.
const devDir = "cmd/protoc-gen-gorums/dev"

// generateZorums generates code for zorums.proto with the import paths that
// make dev uses.
func generateZorums(t *testing.T, dev bool) map[string]string {
	t.Helper()
	root := gentest.RootDir(t)
	opts := gentest.Options{
		ImportPaths: []string{filepath.Join(root, devDir), filepath.Join(root, "third_party")},
		Dev:         dev,
	}
	files, err := gentest.Generate(t, opts, "zorums.proto")
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestStabilityZorums generates code for the dev zorums.proto file twice and
// checks that both runs produce identical output.
func TestStabilityZorums(t *testing.T) {
	first := generateZorums(t, false)
	second := generateZorums(t, false)
	if len(first) == 0 {
		t.Fatal("generated no files")
	}
	if diff := cmp.Diff(first, second); diff != "" {
		t.Errorf("unstable output between runs (-first +second):\n%s", diff)
	}
}

// TestStabilityDevFilesUpToDate checks that the committed dev zorums*.pb.go
// files match what the generator in this tree produces. Run make dev to
// regenerate them.
func TestStabilityDevFilesUpToDate(t *testing.T) {
	root := gentest.RootDir(t)
	files := generateZorums(t, true)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		t.Run(filepath.Base(name), func(t *testing.T) {
			committed, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
			if err != nil {
				t.Fatalf("%v; run make dev", err)
			}
			want := strings.Split(gentest.Normalize(string(committed)), "\n")
			got := strings.Split(gentest.Normalize(files[name]), "\n")
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("%s is out of date (-committed +generated); run make dev:\n%s", name, diff)
			}
		})
	}
	// A committed file that the generator no longer produces is stale.
	committed, err := filepath.Glob(filepath.Join(root, devDir, "zorums*.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range committed {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := files[filepath.ToSlash(rel)]; !ok {
			t.Errorf("%s is not generated any more; remove it or run make dev", rel)
		}
	}
}
