package testprotos_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/protoc"
)

var skipDirs = map[string]struct{}{
	"failing": {},
	"zorums":  {},
}

// TestGenerateProtoFiles generates single RPC calls for different call types.
func TestGenerateProtoFiles(t *testing.T) {
	taskFn := func(suffix string, task func(path string) error) func(string, os.FileInfo, error) error {
		return func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if _, ok := skipDirs[info.Name()]; ok && info.IsDir() {
				return filepath.SkipDir
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), suffix) {
				return task(path)
			}
			return nil
		}
	}
	err := filepath.Walk(".",
		taskFn(".pb.go", func(path string) error {
			return os.Remove(path)
		}))
	if err != nil {
		t.Errorf("error walking the path %q: %v", ".", err)
	}

	err = filepath.Walk(".",
		taskFn(".proto", func(path string) error {
			_, err := protoc.Run("sourceRelative", path)
			return err
		}))
	if err != nil {
		t.Errorf("error walking the path %q: %v", ".", err)
	}

	// build the packages with .pb.go files to check they compile
	alreadyBuilt := make(map[string]struct{})
	err = filepath.Walk(".",
		taskFn(".pb.go", func(path string) error {
			dir := filepath.Dir(path)
			if _, ok := alreadyBuilt[dir]; ok {
				return nil
			}
			alreadyBuilt[dir] = struct{}{}
			return build(t, dir)
		}))
	if err != nil {
		t.Errorf("error walking the path %q: %v", ".", err)
	}

	err = filepath.Walk(".",
		taskFn(".pb.go", func(path string) error {
			return os.Remove(path)
		}))
	if err != nil {
		t.Errorf("error walking the path %q: %v", ".", err)
	}
}

func build(t *testing.T, path string) error {
	t.Helper()
	cmd := exec.Command("go", "build")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if string(out) != "" {
		t.Log(strings.TrimSpace(string(out)))
	}
	if err != nil {
		return err
	}
	return nil
}
