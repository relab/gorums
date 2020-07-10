package zorums_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGorumsStability(t *testing.T) {
	t.Log("Running protoc test with source files expected to remain stable (no output change between runs)")
	// TODO(meling); replace with Go 1.15 specific funcs
	// dir1 := t.TempDir()
	// dir2 := t.TempDir()
	dir1, err := ioutil.TempDir("", "gorums-stability")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1)

	protoc("sourceRelative", "zorums.proto")
	moveGenFiles(t, dir1)

	dir2, err := ioutil.TempDir("", "gorums-stability")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir2)

	protoc("sourceRelative", "zorums.proto")
	moveGenFiles(t, dir2)

	out, _ := exec.Command("diff", dir1, dir2).CombinedOutput()
	// checking only 'out' here; err would only show exit status 1 if output is different
	if string(out) != "" {
		t.Errorf("unexpected instability; observed changes between protoc runs:\n%s", string(out))
	}
}

func moveGenFiles(t *testing.T, toDir string) {
	t.Helper()
	matches, err := filepath.Glob("zorums*.pb.go")
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

var protoArgs = map[string][]string{
	"sourceRelative": {
		"--go_out=paths=source_relative:.",
		"--go-grpc_out=paths=source_relative:.",
		"--gorums_out=paths=source_relative,trace=true:.",
	},
	"module": {
		"--go_out=.",
		"--go_opt=module=" + modulePath(),
		"--go-grpc_out=.",
		"--go-grpc_opt=module=" + modulePath(),
		"--gorums_out=trace=true:.",
		"--gorums_opt=module=" + modulePath(),
	},
}

func protoc(compileType string, args ...string) {
	cmd := exec.Command("protoc", "-I.:"+repoRoot())
	cmd.Args = append(cmd.Args, protoArgs[compileType]...)
	cmd.Args = append(cmd.Args, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("executing: %v\n%s\n", strings.Join(cmd.Args, " "), out)
	}
	check(err)
}

// repoRoot returns the repository root.
func repoRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	check(err)
	return strings.TrimSpace(string(out))
}

// modulePath return the module's path.
func modulePath() string {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}")
	cmd.Dir = repoRoot()
	out, err := cmd.CombinedOutput()
	check(err)
	return strings.TrimSpace(string(out))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
