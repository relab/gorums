package main

import (
	"errors"
	"flag"
	"fmt"
	"path"
	"slices"
	"strings"
)

// optionalValueFlag is a flag whose value is optional. A bare flag stores
// bare; "-name value" and "-name=value" store value. Empty and "false" values
// are rejected, so a bare flag never collides with an explicit value.
type optionalValueFlag struct {
	value *string
	bare  string
}

func (f optionalValueFlag) String() string {
	if f.value == nil {
		return ""
	}
	return *f.value
}

func (f optionalValueFlag) Set(value string) error {
	switch value {
	case "true":
		value = f.bare
	case "", "false":
		return errors.New("omit the flag, or pass it bare or with a value")
	}
	*f.value = value
	return nil
}

func (optionalValueFlag) IsBoolFlag() bool { return true }

// normalizeOptionalArgs rewrites "-name value" to "-name=value" for every
// optionalValueFlag registered in fs. The flag package parses a bool-like
// flag's value only in the "-name=value" form.
func normalizeOptionalArgs(fs *flag.FlagSet, args []string) []string {
	out := slices.Clone(args)
	for i := 0; i < len(out); i++ {
		if out[i] == "--" {
			break
		}
		// Accept "-name" and "--name", like the flag package.
		name, ok := strings.CutPrefix(out[i], "-")
		if !ok {
			continue
		}
		f := fs.Lookup(strings.TrimPrefix(name, "-"))
		if f == nil {
			continue
		}
		if _, ok := f.Value.(optionalValueFlag); !ok {
			continue
		}
		if i+1 < len(out) && !strings.HasPrefix(out[i+1], "-") {
			out[i] += "=" + out[i+1]
			out = slices.Delete(out, i+1, i+2)
		}
	}
	return out
}

// removedFlagError reports a flag that sweep no longer accepts, with the
// replacement to use. Replay and collect scripts from older runs still pass
// -driver, so they get a migration hint instead of a bare usage error.
func removedFlagError(args []string) error {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		name, ok := strings.CutPrefix(arg, "-")
		if !ok {
			continue
		}
		name, _, _ = strings.Cut(strings.TrimPrefix(name, "-"), "=")
		if name == "driver" {
			return errors.New("-driver was removed: launch with -detach <host> (remote runs always detach), and collect with -collect <host>:<path>")
		}
	}
	return nil
}

// splitRemoteRun splits a "host:path" run reference, as taken by -collect and
// -collect-now, into the driver host and its absolute remote work directory.
func splitRemoteRun(ref string) (host, workDir string, err error) {
	host, workDir, ok := strings.Cut(ref, ":")
	if !ok || host == "" || !path.IsAbs(workDir) {
		return "", "", fmt.Errorf("run %q: want <host>:<remote-work-dir> with an absolute path", ref)
	}
	return host, workDir, nil
}
