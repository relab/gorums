package main

import (
	"cmp"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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

// compare orders versions by semantic version precedence: a version with a
// pre-release sorts before the same version without one.
func (v semver) compare(o semver) int {
	if c := cmp.Or(
		cmp.Compare(v.major, o.major),
		cmp.Compare(v.minor, o.minor),
		cmp.Compare(v.patch, o.patch),
	); c != 0 {
		return c
	}
	switch {
	case v.pre == o.pre:
		return 0
	case v.pre == "":
		return 1
	case o.pre == "":
		return -1
	}
	return comparePre(v.pre, o.pre)
}

// comparePre compares two non-empty pre-release strings identifier by
// identifier. Numeric identifiers sort before alphanumeric ones, and a shorter
// list sorts before a longer one with the same prefix.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aErr := strconv.Atoi(as[i])
		bn, bErr := strconv.Atoi(bs[i])
		var c int
		switch {
		case aErr == nil && bErr == nil:
			c = cmp.Compare(an, bn)
		case aErr == nil:
			c = -1
		case bErr == nil:
			c = 1
		default:
			c = strings.Compare(as[i], bs[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}
