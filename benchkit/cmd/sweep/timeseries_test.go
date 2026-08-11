package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums/benchkit"
)

// globResultFiles lists dir's raw result files, mirroring how generateReport
// builds generateTimeSeries' explicit file list from a run manifest's Files.
func globResultFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*"+resultExt))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestGenerateTimeSeries(t *testing.T) {
	const s = int64(1_000_000_000)
	dir := t.TempDir()
	base := "ts_Q_N1_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}

	tput := func(off int64, ops uint64) *benchkit.Event {
		return benchkit.Event_builder{
			Offset:     off,
			Throughput: benchkit.ThroughputInterval_builder{Ops: ops, Duration: s}.Build(),
		}.Build()
	}
	lat := func(off int64, mean float64, count uint64) *benchkit.Event {
		return benchkit.Event_builder{
			Offset:  off,
			Latency: benchkit.LatencyInterval_builder{Mean: mean, Stddev: 10, Count: count}.Build(),
		}.Build()
	}
	start := benchkit.Event_builder{
		Offset: 0,
		Phase:  benchkit.PhaseMarker_builder{Phase: benchkit.PhaseMarker_START, Rate: 0}.Build(),
	}.Build()

	writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
		Config: plotRunConfig("Q", 1, 1, 0, 0),
		Events: []*benchkit.Event{
			start,
			tput(0, 100), lat(0, 500, 100),
			tput(1*s, 200), lat(1*s, 600, 200),
		},
	}.Build())

	out := filepath.Join(dir, "ts")
	benches, err := generateTimeSeries(globResultFiles(t, dir), out, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(benches, []string{"Q"}) {
		t.Errorf("benches = %v, want [Q]", benches)
	}

	for _, name := range []string{"Q_throughput.csv", "Q_latency.csv", "Q_saturation.csv"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), ",node") {
			t.Errorf("%s missing expected header: %q", name, firstLine(string(data)))
		}
	}

	// Throughput CSV: 100 ops over 1s -> 100 ops/s in the first data row.
	tp, _ := os.ReadFile(filepath.Join(out, "Q_throughput.csv"))
	lines := strings.Split(strings.TrimSpace(string(tp)), "\n")
	// line 0: header, line 1: first data row.
	if len(lines) < 2 {
		t.Fatalf("throughput CSV has %d lines, want >=2", len(lines))
	}
	fields := strings.Split(lines[1], ",")
	if fields[1] != "100.000" {
		t.Errorf("first throughput_ops_s = %q, want 100.000", fields[1])
	}
}

// TestGenerateTimeSeriesNoFiles verifies an empty file list is a no-op, not
// an error: a compact-transfer directory's manifest lists raw result files
// that were never retained for successful runs, and that must read as
// "nothing to show" rather than a failure.
func TestGenerateTimeSeriesNoFiles(t *testing.T) {
	dir := t.TempDir()
	benches, err := generateTimeSeries(nil, filepath.Join(dir, "out"), nil, 0)
	if err != nil {
		t.Errorf("empty file list = %v, want nil", err)
	}
	if benches != nil {
		t.Errorf("benches = %v, want nil", benches)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); !os.IsNotExist(err) {
		t.Errorf("output dir created for an empty file list")
	}
}

// TestGenerateTimeSeriesSkipsMissingFiles verifies that a file in the list
// that cannot be read is skipped with a warning rather than failing the
// whole call, the same "expected, not exceptional" outcome as an empty list:
// a manifest's Files can include paths absent from a compact-transfer
// directory (see prepareCompactTransfer).
func TestGenerateTimeSeriesSkipsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	base := "ts_Q_N1_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
		Config: plotRunConfig("Q", 1, 1, 0, 0),
		Events: []*benchkit.Event{benchkit.Event_builder{
			Offset:     0,
			Throughput: benchkit.ThroughputInterval_builder{Ops: 100, Duration: 1_000_000_000}.Build(),
		}.Build()},
	}.Build())

	files := append(globResultFiles(t, dir), filepath.Join(dir, "missing_bb2_9000"+resultExt))
	out := filepath.Join(dir, "ts")
	benches, err := generateTimeSeries(files, out, nil, 0)
	if err != nil {
		t.Fatalf("generateTimeSeries with one missing file: %v", err)
	}
	if !slices.Equal(benches, []string{"Q"}) {
		t.Errorf("benches = %v, want [Q] (the readable file's data)", benches)
	}
	if _, err := os.Stat(filepath.Join(out, "Q_throughput.csv")); err != nil {
		t.Errorf("Q_throughput.csv not written: %v", err)
	}
}

// TestGenerateTimeSeriesAbsentFileIsQuiet verifies which unreadable inputs are
// worth reporting. A compact-transfer directory retains no raw result file for
// a successful run (see prepareCompactTransfer), so an absent file is the
// expected outcome and must stay silent; a 720-run sweep would otherwise emit
// thousands of warnings on every replot. Every other read or decode failure is
// a real problem and is still warned about.
func TestGenerateTimeSeriesAbsentFileIsQuiet(t *testing.T) {
	tests := []struct {
		name        string
		makeExtra   func(t *testing.T, path string)
		wantWarning bool
	}{
		{
			name:      "absent file",
			makeExtra: func(*testing.T, string) {}, // nothing written: the path does not exist
		},
		{
			name: "unreadable file",
			makeExtra: func(t *testing.T, path string) {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantWarning: true,
		},
		{
			name: "undecodable file",
			makeExtra: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("not a report"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantWarning: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			base := "ts_Q_N1_W1_P0"
			present := nodeAssignment{host: "bb1", port: 9000}
			writePlotReport(t, dir, base, present, "bb1:9000",
				benchkit.Result_builder{
					Config: plotRunConfig("Q", 1, 1, 0, 0),
					Events: []*benchkit.Event{benchkit.Event_builder{
						Offset:     0,
						Throughput: benchkit.ThroughputInterval_builder{Ops: 100, Duration: 1_000_000_000}.Build(),
					}.Build()},
				}.Build())
			extra := filepath.Join(dir, resultFilename(base, nodeAssignment{host: "bb2", port: 9000}, resultExt))
			test.makeExtra(t, extra)
			files := []string{filepath.Join(dir, resultFilename(base, present, resultExt)), extra}

			logged := captureLog(t)
			benches, err := generateTimeSeries(files, filepath.Join(dir, "ts"), nil, 0)
			if err != nil {
				t.Fatalf("generateTimeSeries: %v", err)
			}
			if !slices.Equal(benches, []string{"Q"}) {
				t.Errorf("benches = %v, want [Q] (the readable file's data)", benches)
			}
			if gotWarning := strings.Contains(logged.String(), "warning"); gotWarning != test.wantWarning {
				t.Errorf("warning logged = %v, want %v; log:\n%s", gotWarning, test.wantWarning, logged)
			}
		})
	}
}

// TestGenerateTimeSeriesEmptyEventStream verifies that a benchmark whose event
// stream carries no throughput or latency interval — the -interval=0 case, and
// a trim that dropped every interval — is not reported as available and gets no
// CSVs. A header-only CSV would leave the figure's node list empty, which fails
// Typst compilation in hlegend's grid(columns: 0).
func TestGenerateTimeSeriesEmptyEventStream(t *testing.T) {
	tests := []struct {
		name   string
		events []*benchkit.Event
		trim   time.Duration
	}{
		{
			name: "no events at all",
		},
		{
			name: "phase markers only",
			events: []*benchkit.Event{benchkit.Event_builder{
				Offset: 0,
				Phase:  benchkit.PhaseMarker_builder{Phase: benchkit.PhaseMarker_START}.Build(),
			}.Build()},
		},
		{
			name: "every interval trimmed",
			events: []*benchkit.Event{benchkit.Event_builder{
				Offset:     0,
				Throughput: benchkit.ThroughputInterval_builder{Ops: 100, Duration: 1_000_000_000}.Build(),
			}.Build()},
			trim: 5 * time.Second,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			base := "ts_Q_N1_W1_P0"
			n := nodeAssignment{host: "bb1", port: 9000}
			writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
				Config: plotRunConfig("Q", 1, 1, 0, 0),
				Events: test.events,
			}.Build())

			out := filepath.Join(dir, "ts")
			benches, err := generateTimeSeries(globResultFiles(t, dir), out, nil, test.trim)
			if err != nil {
				t.Fatalf("generateTimeSeries: %v", err)
			}
			if benches != nil {
				t.Errorf("benches = %v, want nil", benches)
			}
			for _, name := range []string{"Q_throughput.csv", "Q_latency.csv", "Q_saturation.csv"} {
				if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
					t.Errorf("%s written for an empty event stream", name)
				}
			}
		})
	}
}

// TestGenerateTimeSeriesSelectorExcludesAll verifies that present files whose
// benchmarks all fail the selector are reported as an error, not silently
// skipped (a mistyped selector should not look like success).
func TestGenerateTimeSeriesSelectorExcludesAll(t *testing.T) {
	dir := t.TempDir()
	base := "ts_Q_N1_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}
	writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
		Config: plotRunConfig("Q", 1, 1, 0, 0),
	}.Build())
	sel := regexp.MustCompile("NoSuchBenchmark")
	if _, err := generateTimeSeries(globResultFiles(t, dir), filepath.Join(dir, "out"), sel, 0); err == nil {
		t.Error("selector matching nothing = nil, want error")
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// captureLog redirects the standard logger for the duration of the test and
// returns the buffer it writes to, so a test can assert that a best-effort
// code path stayed quiet. [testing.T.Output] does not serve here: it is a sink
// that feeds the test's own log, not a way to read back what was logged.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}
