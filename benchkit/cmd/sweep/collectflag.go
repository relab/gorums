package main

import (
	"errors"
	"strings"
)

// optionalPathFlag behaves as a boolean flag when no value is supplied and as
// a path flag for both "-collect path" and "-collect=path".
type optionalPathFlag struct {
	value *string
}

func (f optionalPathFlag) String() string {
	if f.value == nil {
		return ""
	}
	return *f.value
}

func (f optionalPathFlag) Set(value string) error {
	if value == "true" {
		value = latestRunSentinel
	}
	*f.value = value
	return nil
}

func (optionalPathFlag) IsBoolFlag() bool { return true }

func normalizeOptionalArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		if out[i] == "--" {
			break
		}
		if out[i] != "-detach" && out[i] != "--detach" && out[i] != "-collect" && out[i] != "-collect-now" && out[i] != "--collect" && out[i] != "--collect-now" {
			continue
		}
		if i+1 < len(out) && !strings.HasPrefix(out[i+1], "-") {
			out[i] += "=" + out[i+1]
			out = append(out[:i+1], out[i+2:]...)
		}
	}
	return out
}

// optionalHostFlag selects the first host when -detach has no argument.
type optionalHostFlag struct{ value *string }

func (f optionalHostFlag) String() string {
	if f.value == nil {
		return ""
	}
	return *f.value
}

func (f optionalHostFlag) Set(value string) error {
	switch value {
	case "true":
		value = "first"
	case "", "false":
		return errors.New("use -detach [hostname], or omit -detach for local execution")
	}
	*f.value = value
	return nil
}

func (optionalHostFlag) IsBoolFlag() bool { return true }
