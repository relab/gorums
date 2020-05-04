package gengorums

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

//TODO(meling) consider whether or not we might use a compatibility table for different Gorums types along with error messages.

//TODO(meling) add proto file; load proto file at start of test to get access to file.Services etc that can be used to test.
// methods to tests:
// qspecMethods
// qspecServices
//

func TestQSpecMethods(t *testing.T) {
	protoc("testprotos/calltypes/calltypes.proto")

}

func protoc(args ...string) {
	cmd := exec.Command("protoc", "-I.", "-I../../..", //TODO(meling) replace relative path with repoRoot()
		"--go_out=paths=source_relative:.",
		"--go-grpc_out=paths=source_relative:.",
		"--gorums_out=paths=source_relative,trace=true:.",
	)
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
