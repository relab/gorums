package main

import (
	"errors"
	"strings"
)

// parseSuggested extracts the version from the "Suggested version:" line of a
// gorelease report.
func parseSuggested(report string) (string, error) {
	for line := range strings.Lines(report) {
		if v, ok := strings.CutPrefix(line, "Suggested version:"); ok {
			if v = strings.TrimSpace(v); v != "" {
				return v, nil
			}
		}
	}
	return "", errors.New("gorelease report has no suggested version")
}

// reportSummary returns the "# summary" section of a gorelease report, or the
// whole report if it has none.
func reportSummary(report string) string {
	if _, tail, ok := strings.Cut(report, "# summary"); ok {
		return "# summary" + strings.TrimRight(tail, "\n")
	}
	return strings.TrimSpace(report)
}

// parseBase extracts the base version from the "Inferred base version:" line
// of a gorelease report, or returns "" if there is none.
func parseBase(report string) string {
	for line := range strings.Lines(report) {
		if v, ok := strings.CutPrefix(line, "Inferred base version:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
