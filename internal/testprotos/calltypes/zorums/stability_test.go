package zorums_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/relab/gorums/internal/protoc"
)

// TestGorumsStability runs protoc twice on the same proto file that captures all
// variations of Gorums specific code generation. The test objective is to discover
// if the output changes between runs over the same proto file.
func TestGorumsStability(t *testing.T) {
	// TODO(meling); replace with Go 1.15 specific funcs
	// dir1 := t.TempDir()
	// dir2 := t.TempDir()
	dir1, err := ioutil.TempDir("", "gorums-stability")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1)

	_, err = protoc.Run("sourceRelative", "zorums.proto")
	if err != nil {
		t.Fatal(err)
	}
	moveFiles(t, "zorums*.pb.go", dir1)

	dir2, err := ioutil.TempDir("", "gorums-stability")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir2)

	_, err = protoc.Run("sourceRelative", "zorums.proto")
	if err != nil {
		t.Fatal(err)
	}
	moveFiles(t, "zorums*.pb.go", dir2)

	out, _ := exec.Command("diff", dir1, dir2).CombinedOutput()
	// checking only 'out' here; err would only show exit status 1 if output is different
	if string(out) != "" {
		t.Errorf("unexpected instability; observed changes between protoc runs:\n%s", string(out))
	}
}

// moveFiles moves files matching glob to toDir.
func moveFiles(t *testing.T, glob, toDir string) {
	t.Helper()
	matches, err := filepath.Glob(glob)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		err = os.Rename(m, filepath.Join(toDir, m))
		if err != nil {
			t.Fatal(err)
		}
	}
}
