package zorums_test

import (
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
	_, err := protoc.Run("sourceRelative", "zorums.proto")
	if err != nil {
		t.Fatal(err)
	}
	dir1 := t.TempDir()
	moveFiles(t, "zorums*.pb.go", dir1)

	_, err = protoc.Run("sourceRelative", "zorums.proto")
	if err != nil {
		t.Fatal(err)
	}
	dir2 := t.TempDir()
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
		moveFile(t, m, filepath.Join(toDir, m))
	}
}

func moveFile(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err == nil {
		return
	}
	// Rename fails across devices, so copy and remove instead.
	s, err := os.Stat(from)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, s.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(from); err != nil {
		t.Fatal(err)
	}
}
