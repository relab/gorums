package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// Files holding the version constants, relative to the repository root.
const (
	versionFile = "internal/version/version.go"
	runtimeFile = "runtime/gorumsimpl/version.go"
)

// constRE returns a pattern that matches "name = value" in a const block and
// captures the text up to the value, and the value itself.
func constRE(name, value string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^(\s*` + name + `\s*=\s*)(` + value + `)\s*$`)
}

var (
	majorRE      = constRE("Major", `\d+`)
	minorRE      = constRE("Minor", `\d+`)
	patchRE      = constRE("Patch", `\d+`)
	preReleaseRE = constRE("PreRelease", `"[^"]*"`)
	genVersionRE = constRE("GenVersion", `\d+`)
	minVersionRE = constRE("MinVersion", `\d+`)
)

// findOne returns the value captured by re, requiring exactly one match.
func findOne(re *regexp.Regexp, src []byte) (string, error) {
	m := re.FindAllSubmatch(src, -1)
	if len(m) != 1 {
		return "", fmt.Errorf("found %d matches for %s, want 1", len(m), re)
	}
	return string(m[0][2]), nil
}

// replaceOne replaces the value matched by re, requiring exactly one match.
func replaceOne(re *regexp.Regexp, src []byte, value string) ([]byte, error) {
	if _, err := findOne(re, src); err != nil {
		return nil, err
	}
	return re.ReplaceAll(src, []byte("${1}"+value)), nil
}

// parseVersionFile reads the version constants from internal/version/version.go.
func parseVersionFile(src []byte) (semver, error) {
	var v semver
	for _, f := range []struct {
		re  *regexp.Regexp
		dst *int
	}{{majorRE, &v.major}, {minorRE, &v.minor}, {patchRE, &v.patch}} {
		s, err := findOne(f.re, src)
		if err != nil {
			return semver{}, err
		}
		if *f.dst, err = strconv.Atoi(s); err != nil {
			return semver{}, err
		}
	}
	pre, err := findOne(preReleaseRE, src)
	if err != nil {
		return semver{}, err
	}
	v.pre, err = strconv.Unquote(pre)
	return v, err
}

// rewriteVersionFile returns src with the version constants set to v.
func rewriteVersionFile(src []byte, v semver) ([]byte, error) {
	var err error
	for _, f := range []struct {
		re    *regexp.Regexp
		value string
	}{
		{majorRE, strconv.Itoa(v.major)},
		{minorRE, strconv.Itoa(v.minor)},
		{patchRE, strconv.Itoa(v.patch)},
		{preReleaseRE, strconv.Quote(v.pre)},
	} {
		if src, err = replaceOne(f.re, src, f.value); err != nil {
			return nil, err
		}
	}
	return src, nil
}

// parseRuntimeVersions reads GenVersion and MinVersion from
// runtime/gorumsimpl/version.go.
func parseRuntimeVersions(src []byte) (gen, minV int, err error) {
	for _, f := range []struct {
		re  *regexp.Regexp
		dst *int
	}{{genVersionRE, &gen}, {minVersionRE, &minV}} {
		s, err := findOne(f.re, src)
		if err != nil {
			return 0, 0, err
		}
		if *f.dst, err = strconv.Atoi(s); err != nil {
			return 0, 0, err
		}
	}
	return gen, minV, nil
}
