package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// customDimension is one sweep dimension that a -dim flag declares: its name
// and the values the sweep takes.
type customDimension struct {
	name   string
	values []string
}

// String renders the dimension as its -dim flag value: name=v1,v2.
func (d customDimension) String() string {
	return d.name + "=" + strings.Join(d.values, ",")
}

var (
	// dimensionNamePattern matches a valid custom dimension name. It is
	// lowercase, so two names never differ only in case.
	dimensionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

	// dimensionValuePattern matches a valid custom dimension value. The value
	// appears in run names and node commands, so it holds only characters that
	// file systems and shells pass through unchanged.
	dimensionValuePattern = regexp.MustCompile(`^[A-Za-z0-9.+-]+$`)
)

// reservedDimensionNames are the names a custom dimension cannot take: the
// built-in dimensions and their flag spellings, every flag that sweep forwards
// to a node, the help flags, and the other columns of the CSV files sweep writes.
var reservedDimensionNames = []string{
	// Built-in dimensions and their flags.
	"benchmark", "benchmarks", "nodes", "n", "workers", "payload", "rate",
	"send-buffer", "recv-buffer", "stream-mode",
	// Flags that sweep forwards to every node.
	"self", "remotes", "time", "output", "interval", "stats-mode",
	"rate-step", "rate-step-max", "cpuprofile", "memprofile", "verbose",
	// Help flags.
	"h", "help",
	// Other CSV columns.
	"base", "label", "status", "rep", "reps", "throughput", "goodput",
	"node", "host", "samples", "prob", "group", "col", "row", "rel", "runs",
	"total", "succeeded", "degraded", "failed", "skipped", "completed", "share",
	"baseline", "modes",
}

// dimFlag is the repeatable -dim flag. Each use declares one custom dimension
// as name=v1,v2,...; the declared dimensions are kept sorted by name.
type dimFlag struct {
	dst *[]customDimension
}

// String renders the declared dimensions as their space-separated flag values.
func (f dimFlag) String() string {
	if f.dst == nil {
		return ""
	}
	parts := make([]string, len(*f.dst))
	for i, d := range *f.dst {
		parts[i] = d.String()
	}
	return strings.Join(parts, " ")
}

// Set declares one custom dimension. It returns an error for an invalid or
// reserved name, a name declared before, an invalid value, and two values that
// differ only in case, which a case-insensitive file system cannot tell apart
// in run names.
func (f dimFlag) Set(value string) error {
	name, list, ok := strings.Cut(value, "=")
	if !ok {
		return fmt.Errorf("%q: want name=v1,v2", value)
	}
	if !dimensionNamePattern.MatchString(name) {
		return fmt.Errorf("invalid name %q: want lowercase letters, digits, and dashes, starting with a letter", name)
	}
	if slices.Contains(reservedDimensionNames, name) {
		return fmt.Errorf("name %q is reserved by sweep", name)
	}
	if slices.ContainsFunc(*f.dst, func(d customDimension) bool { return d.name == name }) {
		return fmt.Errorf("repeated -dim %s", name)
	}
	values := strings.Split(list, ",")
	for i, v := range values {
		if !dimensionValuePattern.MatchString(v) {
			return fmt.Errorf("%s: invalid value %q: want letters, digits, '.', '+', and '-'", name, v)
		}
		for _, earlier := range values[:i] {
			if strings.EqualFold(v, earlier) {
				return fmt.Errorf("%s: repeated value %q (values must differ in more than case)", name, v)
			}
		}
	}
	dim := customDimension{name: name, values: values}
	i, _ := slices.BinarySearchFunc(*f.dst, name, func(d customDimension, name string) int {
		return strings.Compare(d.name, name)
	})
	*f.dst = slices.Insert(*f.dst, i, dim)
	return nil
}

// validateDimensionArgs returns an error when a custom dimension is also given
// as a flag in extraArgs, since the node would then receive that flag twice. It
// also returns an error when extraArgs has an unterminated quote. The node's
// shell splits extraArgs into words, so this function splits it by the same
// rules (see [shellWords]). A flag word is -name, --name, -name=value, or
// --name=value. The words after a bare -- are arguments, as for Go's flag
// package.
func validateDimensionArgs(dims []customDimension, extraArgs string) error {
	words, err := shellWords(extraArgs)
	if err != nil {
		return fmt.Errorf("-extra-args: %w", err)
	}
	for _, word := range words {
		if word == "--" {
			return nil
		}
		name, ok := flagWordName(word)
		if !ok {
			continue
		}
		if slices.ContainsFunc(dims, func(d customDimension) bool { return d.name == name }) {
			return fmt.Errorf("-dim %s: also given in -extra-args", name)
		}
	}
	return nil
}

// flagWordName returns the flag name that word sets and true when word has the
// form -name, --name, -name=value, or --name=value.
func flagWordName(word string) (string, bool) {
	name, ok := strings.CutPrefix(word, "-")
	if !ok {
		return "", false
	}
	name = strings.TrimPrefix(name, "-")
	name, _, _ = strings.Cut(name, "=")
	return name, name != ""
}

// shellWords splits s into the words that a POSIX shell passes to a command,
// after quote removal. Unquoted blanks and newlines separate words. A
// backslash outside quotes escapes the next character. Single quotes keep
// every character up to the next single quote. Double quotes keep every
// character up to the next unescaped double quote; inside them, a backslash
// escapes only $, `, ", \, and newline, and stays literal before any other
// character. An escaped newline joins the lines. shellWords keeps every other
// character literally, including the $ of a parameter expansion that the shell
// would expand. It returns an error for an unterminated quote.
func shellWords(s string) ([]string, error) {
	var (
		words  []string
		word   strings.Builder
		inWord bool // a quote starts a word even when it holds no characters
	)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ', '\t', '\n':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		case '\\':
			if i+1 < len(s) && s[i+1] == '\n' {
				i++ // an escaped newline joins the lines
				continue
			}
			inWord = true
			if i+1 < len(s) {
				i++
				word.WriteByte(s[i])
			}
		case '\'':
			inWord = true
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return nil, fmt.Errorf("unterminated single quote in %q", s)
			}
			word.WriteString(s[i+1 : i+1+end])
			i += 1 + end
		case '"':
			inWord = true
			closed := false
			for i++; i < len(s); i++ {
				c := s[i]
				if c == '"' {
					closed = true
					break
				}
				if c == '\\' && i+1 < len(s) && strings.IndexByte("$`\"\\\n", s[i+1]) >= 0 {
					i++
					c = s[i]
					if c == '\n' {
						continue
					}
				}
				word.WriteByte(c)
			}
			if !closed {
				return nil, fmt.Errorf("unterminated double quote in %q", s)
			}
		default:
			inWord = true
			word.WriteByte(c)
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}
