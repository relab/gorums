package main

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
)

// listFlag is a flag.Value that parses a comma-separated list into a slice,
// converting each element with parse. It backs flags such as -n 1,3,5 and
// -benchmarks Symmetric,Async. Setting the flag replaces the slice wholesale,
// so the destination's initial contents act as the default.
//
// See the flag package's interval example for the single-value analogue:
// https://pkg.go.dev/flag#example-FlagSet
type listFlag[T any] struct {
	dst   *[]T
	parse func(string) (T, error)
}

// intListFlag returns a flag.Value parsing a comma-separated list of integers
// into dst.
func intListFlag(dst *[]int) flag.Value {
	return &listFlag[int]{dst: dst, parse: strconv.Atoi}
}

// stringListFlag returns a flag.Value parsing a comma-separated list of
// (trimmed, non-empty) strings into dst.
func stringListFlag(dst *[]string) flag.Value {
	return &listFlag[string]{dst: dst, parse: func(s string) (string, error) { return s, nil }}
}

// String renders the current list the way it would be entered on the command
// line, which the flag package uses to show the default value in usage text.
func (l *listFlag[T]) String() string {
	if l == nil || l.dst == nil {
		return ""
	}
	parts := make([]string, len(*l.dst))
	for i, v := range *l.dst {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, ",")
}

// Set parses a comma-separated value, trimming whitespace and skipping empty
// fields. It requires at least one valid element so an empty range is rejected.
func (l *listFlag[T]) Set(value string) error {
	var out []T
	for f := range strings.SplitSeq(value, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		v, err := l.parse(f)
		if err != nil {
			return err
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return errors.New("requires at least one value")
	}
	*l.dst = out
	return nil
}
