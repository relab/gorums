package benchkit

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeTestEvents builds a minimal event stream for testing plotters.
func makeTestEvents(t *testing.T) []*Event {
	t.Helper()
	b := newEventBuffer()

	start := time.Unix(1, 0)
	const halfSec = 500 * time.Millisecond
	b.emitPhase(start, PhaseMarker_START, 100)
	b.emitThroughput(start.Add(500*time.Millisecond), 50, halfSec)
	b.emitLatency(start.Add(500*time.Millisecond), 200.0, 30.0, 50)
	b.emitThroughput(start.Add(1500*time.Millisecond), 100, halfSec)
	b.emitLatency(start.Add(1500*time.Millisecond), 150.0, 20.0, 100)
	b.emitPhase(start.Add(2000*time.Millisecond), PhaseMarker_RATE_STEP, 200)
	b.emitThroughput(start.Add(2500*time.Millisecond), 180, halfSec)
	b.emitPhase(start.Add(3000*time.Millisecond), PhaseMarker_STOP, 0)

	return b.Events()
}

// render runs one plotter over the test event stream, tagged with node, and
// returns its rendered CSV.
func render(t *testing.T, p Plotter, node string) string {
	t.Helper()
	NewEventReader(0, p).Read(node, makeTestEvents(t))
	var buf bytes.Buffer
	if err := p.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

// TestPlotterRender verifies that each plotter emits its documented header and
// one row per event it collects, with no leading comment line: the CSVs are
// read by front ends with no comment syntax.
func TestPlotterRender(t *testing.T) {
	tests := []struct {
		name     string
		plotter  Plotter
		header   string
		wantRows int
	}{
		// 3 ThroughputInterval events.
		{"throughput", &ThroughputTimePlotter{}, "offset_s,throughput_ops_s,phase,node", 3},
		// 2 LatencyInterval events.
		{"latency", &LatencyTimePlotter{}, "offset_s,mean_ns,stddev_ns,count,phase,node", 2},
		// START + 1 RATE_STEP → 2 levels.
		{"saturation", &SaturationCurvePlotter{}, "offered_rate,throughput_ops_s,mean_latency_ns,node", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := csvLines(render(t, tt.plotter, "bb1:9000"))
			if len(lines) == 0 || lines[0] != tt.header {
				t.Fatalf("first line = %q, want the header %q", lines, tt.header)
			}
			if got := len(lines) - 1; got != tt.wantRows {
				t.Errorf("rows = %d, want %d", got, tt.wantRows)
			}
		})
	}
}

func TestEventReaderFansToAllPlotters(t *testing.T) {
	events := makeTestEvents(t)
	tp := &ThroughputTimePlotter{}
	lp := &LatencyTimePlotter{}
	NewEventReader(0, tp, lp).Read("bb1:9000", events)
	if len(tp.rows) == 0 {
		t.Error("ThroughputTimePlotter received no rows")
	}
	if len(lp.rows) == 0 {
		t.Error("LatencyTimePlotter received no rows")
	}
}

// TestEventReaderTrim verifies that a trim threshold drops interval events
// recorded before the offset while phase markers still pass through. The test
// stream has interval events at 0.5s, 1.5s, and 2.5s; trimming at 1s drops the
// 0.5s pair only. The RATE_STEP marker still reaches the saturation plotter, so
// it keeps both rate levels.
func TestEventReaderTrim(t *testing.T) {
	events := makeTestEvents(t)
	tp := &ThroughputTimePlotter{}
	lp := &LatencyTimePlotter{}
	sc := &SaturationCurvePlotter{}
	NewEventReader(int64(time.Second), tp, lp, sc).Read("bb1:9000", events)
	// 3 throughput intervals total; the one at 0.5s is dropped, leaving 2.
	if got := len(tp.rows); got != 2 {
		t.Errorf("throughput rows after trim = %d, want 2", got)
	}
	// 2 latency intervals total; the one at 0.5s is dropped, leaving 1.
	if got := len(lp.rows); got != 1 {
		t.Errorf("latency rows after trim = %d, want 1", got)
	}
	// START and RATE_STEP markers pass through, so both rate levels remain.
	if got := len(sc.levels); got != 2 {
		t.Errorf("saturation levels after trim = %d, want 2", got)
	}
}

// TestPlottersTagRowsWithNode verifies that two nodes' event streams stay
// distinguishable in the rendered CSVs: every row carries its node identity,
// and the saturation plotter keeps one set of rate levels per node instead of
// merging them.
func TestPlottersTagRowsWithNode(t *testing.T) {
	events := makeTestEvents(t)
	tp := &ThroughputTimePlotter{}
	sc := &SaturationCurvePlotter{}
	reader := NewEventReader(0, tp, sc)
	for _, node := range []string{"bb1:9000", "bb2:9000"} {
		reader.Read(node, events)
	}

	var buf bytes.Buffer
	if err := tp.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	rows := csvLines(buf.String())[1:] // skip header
	if got := len(rows); got != 6 {
		t.Fatalf("throughput rows = %d, want 6 (3 per node)", got)
	}
	for i, row := range rows {
		wantNode := "bb1:9000"
		if i >= 3 {
			wantNode = "bb2:9000"
		}
		if !strings.HasSuffix(row, ","+wantNode) {
			t.Errorf("row %d = %q, want node suffix %q", i, row, wantNode)
		}
	}

	// Each node's START + RATE_STEP opens its own levels: 2 levels per node.
	if got := len(sc.levels); got != 4 {
		t.Errorf("saturation levels = %d, want 4 (2 per node)", got)
	}
	for i, lv := range sc.levels {
		wantNode := "bb1:9000"
		if i >= 2 {
			wantNode = "bb2:9000"
		}
		if lv.node != wantNode {
			t.Errorf("level %d node = %q, want %q", i, lv.node, wantNode)
		}
	}
}

// TestWriteTimeSeriesCSVs verifies the group-level pipeline: one benchmark's
// per-node event streams are rendered into the three CSVs, each carrying the
// node column, and the benchmark is reported as available.
func TestWriteTimeSeriesCSVs(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "plots")
	groups := []TimeSeriesGroup{{
		Benchmark: "QuorumCall",
		Nodes:     []TimeSeriesNode{{Node: "bb1:9000", Events: makeTestEvents(t)}},
	}}

	available, err := WriteTimeSeriesCSVs(outDir, groups, 0)
	if err != nil {
		t.Fatalf("WriteTimeSeriesCSVs: %v", err)
	}
	if len(available) != 1 || available[0] != "QuorumCall" {
		t.Errorf("available = %v, want [QuorumCall]", available)
	}
	for _, name := range []string{
		"QuorumCall_throughput.csv",
		"QuorumCall_latency.csv",
		"QuorumCall_saturation.csv",
	} {
		data, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Errorf("missing CSV: %v", err)
			continue
		}
		if !strings.Contains(string(data), ",bb1:9000") {
			t.Errorf("%s: no row carries the node identity:\n%s", name, data)
		}
	}
}

// TestWriteTimeSeriesCSVsSkipsEmptyGroup verifies that a group whose streams
// hold no interval event writes no CSV and is absent from the returned names,
// so the caller plans no figure for it: a header-only CSV would leave the
// figure's node list empty.
func TestWriteTimeSeriesCSVsSkipsEmptyGroup(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "plots")
	groups := []TimeSeriesGroup{
		{Benchmark: "Empty", Nodes: []TimeSeriesNode{{Node: "bb1:9000"}}},
		{Benchmark: "Trimmed", Nodes: []TimeSeriesNode{{Node: "bb1:9000", Events: makeTestEvents(t)}}},
	}

	// A trim past the end of the stream drops every interval of both groups.
	available, err := WriteTimeSeriesCSVs(outDir, groups, time.Hour)
	if err != nil {
		t.Fatalf("WriteTimeSeriesCSVs: %v", err)
	}
	if available != nil {
		t.Errorf("available = %v, want none", available)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("wrote %d file(s), want none", len(entries))
	}
}

// TestPlotterRenderQuotesCommaInNode verifies that a node identity or phase
// containing a comma round-trips through a CSV reader instead of corrupting
// the file: node/phase values come from report labels and event data, which
// are not under this package's control. All three plotters share the same
// encoding/csv-based Render, so ThroughputTimePlotter stands in for the
// others.
func TestPlotterRenderQuotesCommaInNode(t *testing.T) {
	b := newEventBuffer()
	start := time.Unix(1, 0)
	b.emitPhase(start, PhaseMarker_START, 100)
	b.emitThroughput(start.Add(500*time.Millisecond), 50, 500*time.Millisecond)

	p := &ThroughputTimePlotter{}
	nodeWithComma := "bb1:9000, region=eu"
	NewEventReader(0, p).Read(nodeWithComma, b.Events())

	var buf bytes.Buffer
	if err := p.Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("csv.ReadAll: %v", err)
	}
	if len(records) != 2 { // header + one data row
		t.Fatalf("got %d CSV records, want 2 (header + 1 row)", len(records))
	}
	if got := records[1][3]; got != nodeWithComma {
		t.Errorf("node field = %q, want %q (comma must round-trip, not split the row)", got, nodeWithComma)
	}
}

// csvLines returns the non-empty lines of a rendered CSV.
func csvLines(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
