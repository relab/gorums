// Package protoc provides a small helper for invoking the protoc compiler,
// used by the code-generation tooling and tests to regenerate .pb.go files.
package protoc

import (
	"fmt"
	"os/exec"
	"strings"
)

// Run runs protoc with the Go and Gorums plugins, writing source-relative
// output next to each input. The last argument should be the proto filename.
// Run returns protoc's combined output, and an error if the repository root
// cannot be found or protoc fails.
func Run(args ...string) (string, error) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("protoc: find repository root: %w", err)
	}
	cmd := exec.Command("protoc", "-I.:"+strings.TrimSpace(string(root)),
		"--go_out=paths=source_relative:.",
		"--gorums_out=paths=source_relative:.",
		"--go_opt=default_api_level=API_OPAQUE",
	)
	cmd.Args = append(cmd.Args, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
