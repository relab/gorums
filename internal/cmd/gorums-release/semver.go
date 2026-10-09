package main

import (
	"cmp"
	"fmt"
	"regexp"
	"strconv"
)

// semver is a semantic version without build metadata.
type semver struct {
	major, minor, patch int
	pre                 string
}

var semverRE = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$`)

// parseSemver parses a version such as v0.12.0 or v0.12.0-rc.1.
func parseSemver(s string) (semver, error) {
	m := semverRE.FindStringSubmatch(s)
	if m == nil {
		return semver{}, fmt.Errorf("invalid version %q: want vMAJOR.MINOR.PATCH[-PRERELEASE]", s)
	}
	var v semver
	for i, dst := range []*int{&v.major, &v.minor, &v.patch} {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return semver{}, fmt.Errorf("invalid version %q: %w", s, err)
		}
		*dst = n
	}
	v.pre = m[4]
	return v, nil
}

// String formats the version with a leading "v".
func (v semver) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
	if v.pre != "" {
		s += "-" + v.pre
	}
	return s
}

// compareCore compares major, minor, and patch, and ignores the pre-release.
func (v semver) compareCore(o semver) int {
	return cmp.Or(
		cmp.Compare(v.major, o.major),
		cmp.Compare(v.minor, o.minor),
		cmp.Compare(v.patch, o.patch),
	)
}
