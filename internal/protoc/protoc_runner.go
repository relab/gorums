package protoc

import (
	"os/exec"
	"strings"
)

// Run runs the protoc generator, using either sourceRelative or module compileType,
// with additional arguments, the last of which should be the proto filename.
func Run(compileType string, args ...string) (string, error) {
	cmd := exec.Command("protoc", "-I.:"+repoRoot())
	cmd.Args = append(cmd.Args, protoArgs[compileType]...)
	cmd.Args = append(cmd.Args, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

var protoArgs = map[string][]string{
	"sourceRelative": {
		"--go_out=paths=source_relative:.",
		"--gorums_out=paths=source_relative:.",
	},
	"module": {
		"--go_out=.",
		"--go_opt=module=" + modulePath(),
		"--gorums_out=.",
		"--gorums_opt=module=" + modulePath(),
	},
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
