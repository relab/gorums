package main

import "testing"

func TestParseSuggested(t *testing.T) {
	report := `# github.com/relab/gorums/ordering
## incompatible changes
package removed

# summary
Inferred base version: v0.11.0
Suggested version: v0.12.0
`
	got, err := parseSuggested(report)
	if err != nil || got != "v0.12.0" {
		t.Errorf("parseSuggested() = %q, %v; want v0.12.0", got, err)
	}
	if _, err := parseSuggested("# summary\nnothing useful\n"); err == nil {
		t.Error("expected an error when no suggestion is present")
	}
}

func TestParseBase(t *testing.T) {
	if got := parseBase("# summary\nInferred base version: v0.11.0\nSuggested version: v0.12.0\n"); got != "v0.11.0" {
		t.Errorf("parseBase() = %q, want v0.11.0", got)
	}
	if got := parseBase("no base here\n"); got != "" {
		t.Errorf("parseBase() = %q, want empty", got)
	}
}

func TestReportSummary(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"with summary", "# pkg\n## incompatible changes\nx: removed\n\n# summary\nInferred base version: v0.11.0\nSuggested version: v0.12.0\n",
			"# summary\nInferred base version: v0.11.0\nSuggested version: v0.12.0"},
		{"without summary", "v0.12.0 is a valid semantic version for this release.\n",
			"v0.12.0 is a valid semantic version for this release."},
	}
	for _, tt := range tests {
		if got := reportSummary(tt.in); got != tt.want {
			t.Errorf("%s: reportSummary() = %q, want %q", tt.name, got, tt.want)
		}
	}
}
