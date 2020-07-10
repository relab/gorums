package zorums_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// stability: installgorums
// 	@echo "Running protoc test with source files expected to remain stable (no output change between runs)"
// 	@protoc -I. \
// 		--go_out=paths=source_relative:. \
// 		--go-grpc_out=paths=source_relative:. \
// 		--gorums_out=paths=source_relative,trace=true:. $(tests_zorums_proto) \
// 	|| (echo "unexpected failure with exit code: $$?")
// 	@cp $(tests_zorums_gen) $(tmp_dir)/x_gorums.pb.go
// 	@protoc -I. \
// 		--go_out=paths=source_relative:. \
// 		--go-grpc_out=paths=source_relative:. \
// 		--gorums_out=paths=source_relative,trace=true:. $(tests_zorums_proto) \
// 	|| (echo "unexpected failure with exit code: $$?")
// 	@diff $(tests_zorums_gen) $(tmp_dir)/x_gorums.pb.go \
// 	|| (echo "unexpected instability, observed changes between protoc runs: $$?")
// 	@rm -rf $(tmp_dir)

func TestGorumsStability(t *testing.T) {
	t.Log("Running protoc test with source files expected to remain stable (no output change between runs)")
	// Go 1.15 specific funcs
	// dir1 := t.TempDir()
	// dir2 := t.TempDir()
	// cp zorums.proto dir1
	// cp zorums.proto dir2
	protoc("zorums.proto")
	// protoc("dir2/zorums.proto")
	// diff dir1 dir2
	// return error if different; maybe use cmp package to show differences.
}

// protoc --go_out=. --go_opt=module=google.golang.org/protobuf google/protobuf/empty.proto

func protoc(args ...string) {
	// cmd := exec.Command("protoc", "-I.", "-I"+repoRoot(),
	// 	"--go_out=paths=source_relative:.",
	// 	"--go-grpc_out=paths=source_relative:.",
	// 	"--gorums_out=paths=source_relative,trace=true:.",
	// )
	cmd := exec.Command("protoc", "-I.", "-I"+repoRoot(),
		"--go_opt=module=github.com/relab/gorums",
		"--go-grpc_opt=module=github.com/relab/gorums",
		"--go_out=.",
		"--go-grpc_out=.",
		"--gorums_out=trace=true:.",
	)
	cmd.Args = append(cmd.Args, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("executing: %v\n%s\n", strings.Join(cmd.Args, " "), out)
	}
	check(err)
	_ = modulePath() //TODO(meling) remove; currently used only to avoid compile error due to unused func.
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
