// Package version records versioning information about this module.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// These constants determine the current version of this module.
//
// Steps for cutting a new release is described in doc/release-guide.md.
const (
	Major      = 0
	Minor      = 9
	Patch      = 0
	PreRelease = "devel"
)

// String formats the version string for this module in semver format.
//
// Examples:
//
//	v1.20.1
//	v1.21.0-rc.1
//	v1.21.0-devel+<commit-hash>
func String() string {
	v := fmt.Sprintf("v%d.%d.%d", Major, Minor, Patch)
	if PreRelease != "" {
		v += "-" + PreRelease

		bi, ok := debug.ReadBuildInfo()
		var metadata string
		if ok {
			for _, setting := range bi.Settings {
				if setting.Key == "vcs.revision" {
					metadata = setting.Value[0:8]
					break
				}
			}
		}
		if strings.Contains(PreRelease, "devel") && metadata != "" {
			v += "+" + metadata
		}
	}
	return v
}
