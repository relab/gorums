// Package version records versioning information about this module.
package version

import (
	"fmt"
	"strings"
)

// These constants determine the current version of this module.
//
// Steps for cutting a new release is described in doc/release-guide.md.
const (
	Major      = 0
	Minor      = 7
	Patch      = 0
	PreRelease = "devel"
)

// String formats the version string for this module in semver format.
//
// Examples:
//	v1.20.1
//	v1.21.0-rc.1
func String() string {
	v := fmt.Sprintf("v%d.%d.%d", Major, Minor, Patch)
	if PreRelease != "" {
		v += "-" + PreRelease

		// TODO: Add metadata about the commit or build hash.
		// See https://golang.org/issue/29814
		// See https://golang.org/issue/33533
		// See https://github.com/golang/go/issues/37475
		var metadata string
		if strings.Contains(PreRelease, "devel") && metadata != "" {
			v += "+" + metadata
		}
	}
	return v
}
