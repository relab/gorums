package gengorums

import (
	"fmt"
	"strings"
)

const (
	versionMajor      = 0
	versionMinor      = 2
	versionPatch      = 0
	versionPreRelease = "devel"
)

// VersionString formats the version string for this module in semver format.
//
// Examples:
//	v1.20.1
//	v1.21.0-rc.1
func VersionString() string {
	v := fmt.Sprintf("v%d.%d.%d", versionMajor, versionMinor, versionPatch)
	if versionPreRelease != "" {
		v += "-" + versionPreRelease

		// TODO: Add metadata about the commit or build hash.
		// See https://golang.org/issue/29814
		// See https://golang.org/issue/33533
		var versionMetadata string
		if strings.Contains(versionPreRelease, "devel") && versionMetadata != "" {
			v += "+" + versionMetadata
		}
	}
	return v
}
