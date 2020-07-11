package testprotos_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relab/gorums/internal/protoc"
)

var (
	skipDirs = map[string]struct{}{
		"failing": {},
		"zorums":  {},
	}
)

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
		t.Errorf("error walking the path %q: %w", ".", err)
	}

	err = filepath.Walk(".",
		taskFn(".proto", func(path string) error {
			return protoc.Run("sourceRelative", path)
		}))
	if err != nil {
		t.Errorf("error walking the path %q: %w", ".", err)
	}
	//TODO(meling) check that the generated code compiles

	err = filepath.Walk(".",
		taskFn(".pb.go", func(path string) error {
			return os.Remove(path)
		}))

	if err != nil {
		t.Errorf("error walking the path %q: %w", ".", err)
	}
}
