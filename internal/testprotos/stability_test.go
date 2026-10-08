package testprotos_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/gorums/internal/protoc"
)

// zorumsProto is the dev proto file that captures all variations of
// Gorums-specific code generation.
const zorumsProto = "../../cmd/protoc-gen-gorums/dev/zorums.proto"

// TestStabilityZorums runs protoc twice on the dev zorums.proto file and
// checks that both runs produce identical output.
func TestStabilityZorums(t *testing.T) {
	proto, err := filepath.Abs(zorumsProto)
	if err != nil {
		t.Fatal(err)
	}
	first := generate(t, proto)
	second := generate(t, proto)
	if len(first) == 0 {
		t.Fatal("protoc generated no files")
	}
	if diff := cmp.Diff(first, second); diff != "" {
		t.Errorf("unstable output between protoc runs (-first +second):\n%s", diff)
	}
}

// generate runs protoc on proto into a new temporary directory and returns
// the generated files, keyed by their path relative to that directory.
func generate(t *testing.T, proto string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	if out, err := protoc.Run(dir, proto); err != nil {
		t.Fatalf("protoc failed: %v\n%s", err, out)
	}
	files := make(map[string]string)
	err := fs.WalkDir(os.DirFS(dir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			return err
		}
		files[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
