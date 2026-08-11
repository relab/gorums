package main

import (
	"slices"
	"strings"
	"testing"
)

// TestFormatRunListTimestamp verifies that list output uses a human-readable
// local date and time at second precision.
func TestFormatRunListTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		want      string
	}{
		{
			name:      "microseconds",
			timestamp: "2026-07-29T16:37:33.616046-07:00",
			want:      "2026-07-29 16:37:33",
		},
		{
			name:      "fractional seconds",
			timestamp: "2026-07-28T13:54:33.51098-07:00",
			want:      "2026-07-28 13:54:33",
		},
		{
			name:      "seconds",
			timestamp: "2026-07-27T22:57:41-07:00",
			want:      "2026-07-27 22:57:41",
		},
		{
			name:      "stat fallback",
			timestamp: "2026-07-27 22:57:41",
			want:      "2026-07-27 22:57:41",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatRunListTimestamp(tt.timestamp); got != tt.want {
				t.Errorf("formatRunListTimestamp(%q) = %q, want %q", tt.timestamp, got, tt.want)
			}
		})
	}
}

// TestWriteDriverListAlignsColumns verifies that values of different lengths
// do not shift columns away from their headings.
func TestWriteDriverListAlignsColumns(t *testing.T) {
	const (
		namespace = "/tmp/sweep-meling"
		latest    = namespace + "/sweep-driver-dedup-qc-v3-20260729_163732"
	)
	rows := strings.Join([]string{
		"2026-07-29T16:37:33.616046-07:00\tdedup-qc-v3\tcompleted\t0\t117M\t" + latest,
		"2026-07-28T13:54:33.51098-07:00\tsymmetric-qc-dedup-eval-v14\traw-pending\t0\t1.2G\t" + namespace + "/sweep-driver-symmetric-qc-dedup-eval-v14-20260728_135433",
	}, "\n")

	var output strings.Builder
	if err := writeDriverList(&output, "bb1", namespace, latest, rows); err != nil {
		t.Fatalf("writeDriverList() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("output has %d lines, want 4:\n%s", len(lines), output.String())
	}
	if strings.Contains(output.String(), "\t") {
		t.Fatalf("output contains unexpanded tabs:\n%s", output.String())
	}

	columns := [][]string{
		{"STARTED", "LABEL", "STATUS", "EXIT", "SIZE", "PATH"},
		{"2026-07-29 16:37:33", "dedup-qc-v3", "completed", "0", "117M", latest},
		{"2026-07-28 13:54:33", "symmetric-qc-dedup-eval-v14", "raw-pending", "0", "1.2G", namespace + "/sweep-driver-symmetric-qc-dedup-eval-v14-20260728_135433"},
	}
	starts := columnStarts(t, lines[1], columns[0])
	for i, fields := range columns[1:] {
		if got := columnStarts(t, lines[i+2], fields); !slices.Equal(got, starts) {
			t.Errorf("line %d column starts = %v, want %v:\n%s", i+3, got, starts, lines[i+2])
		}
	}
	if !strings.HasSuffix(lines[2], latest+"  (latest)") {
		t.Errorf("latest row missing marker:\n%s", lines[2])
	}
}

func columnStarts(t *testing.T, line string, fields []string) []int {
	t.Helper()
	starts := make([]int, 0, len(fields))
	from := 0
	for _, field := range fields {
		pos := strings.Index(line[from:], field)
		if pos < 0 {
			t.Fatalf("field %q not found in %q", field, line)
		}
		pos += from
		starts = append(starts, pos)
		from = pos + len(field)
	}
	return starts
}
