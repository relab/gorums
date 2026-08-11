package benchkit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// TestWriteLoadReport verifies binary round-trip fidelity.
func TestWriteLoadReport(t *testing.T) {
	want := Report_builder{
		Label: "run-1",
		Results: []*Result{
			Result_builder{
				Config:     RunConfig_builder{Name: "QuorumCall"}.Build(),
				Throughput: 12345.6,
				TotalOps:   5000,
				Latencies:  []int64{100, 200, 300, 400, 500},
			}.Build(),
			Result_builder{
				Config:     RunConfig_builder{Name: "Multicast"}.Build(),
				Throughput: 999.0,
				TotalOps:   100,
				Latencies:  []int64{50, 75},
			}.Build(),
		},
	}.Build()

	path := filepath.Join(t.TempDir(), "results.binpb")
	if err := WriteReport(want, path); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, err := LoadReport(path)
	if err != nil {
		t.Fatalf("LoadReport: %v", err)
	}

	if got.GetLabel() != want.GetLabel() {
		t.Errorf("label = %q, want %q", got.GetLabel(), want.GetLabel())
	}
	if len(got.GetResults()) != len(want.GetResults()) {
		t.Fatalf("results count = %d, want %d", len(got.GetResults()), len(want.GetResults()))
	}
	for i, w := range want.GetResults() {
		g := got.GetResults()[i]
		if g.GetConfig().GetName() != w.GetConfig().GetName() {
			t.Errorf("[%d] name = %q, want %q", i, g.GetConfig().GetName(), w.GetConfig().GetName())
		}
		if g.GetThroughput() != w.GetThroughput() {
			t.Errorf("[%d] throughput = %v, want %v", i, g.GetThroughput(), w.GetThroughput())
		}
		if len(g.GetLatencies()) != len(w.GetLatencies()) {
			t.Errorf("[%d] latencies count = %d, want %d", i, len(g.GetLatencies()), len(w.GetLatencies()))
			continue
		}
		for j, lat := range w.GetLatencies() {
			if g.GetLatencies()[j] != lat {
				t.Errorf("[%d] latencies[%d] = %d, want %d", i, j, g.GetLatencies()[j], lat)
			}
		}
	}
}

// TestConfigDelta verifies that [ConfigDelta] reports every semantic field
// that differs between two configs and reports nothing when they match, so
// [PrintComparison] can flag an incompatible comparison instead of silently
// treating it as apples-to-apples.
func TestConfigDelta(t *testing.T) {
	base := RunConfig_builder{
		Name: "QuorumCall", NumNodes: 4, Mode: "local", Duration: int64(time.Second),
		Workers: 2, Payload: 16, Rate: 100, Interval: int64(50 * time.Millisecond),
		QuorumSize: 3, MaxAsync: 500, RateStep: 50, RateStepMax: 200,
		CallTimeout: int64(20 * time.Millisecond), StatsMode: StatsMode_EXACT, StreamMode: "dual",
	}.Build()

	t.Run("IdenticalConfigsHaveNoDelta", func(t *testing.T) {
		other := RunConfig_builder{
			Name: "QuorumCall", NumNodes: 4, Mode: "local", Duration: int64(time.Second),
			Workers: 2, Payload: 16, Rate: 100, Interval: int64(50 * time.Millisecond),
			QuorumSize: 3, MaxAsync: 500, RateStep: 50, RateStepMax: 200,
			CallTimeout: int64(20 * time.Millisecond), StatsMode: StatsMode_EXACT, StreamMode: "dual",
		}.Build()
		if delta := ConfigDelta(base, other); len(delta) != 0 {
			t.Errorf("ConfigDelta(identical configs) = %v, want empty", delta)
		}
	})

	t.Run("NameDeltaIsIgnored", func(t *testing.T) {
		other := RunConfig_builder{
			Name: "Multicast", NumNodes: 4, Mode: "local", Duration: int64(time.Second),
			Workers: 2, Payload: 16, Rate: 100, Interval: int64(50 * time.Millisecond),
			QuorumSize: 3, MaxAsync: 500, RateStep: 50, RateStepMax: 200,
			CallTimeout: int64(20 * time.Millisecond), StatsMode: StatsMode_EXACT, StreamMode: "dual",
		}.Build()
		if delta := ConfigDelta(base, other); len(delta) != 0 {
			t.Errorf("ConfigDelta(only name differs) = %v, want empty (name is the comparison key, not a semantic field)", delta)
		}
	})

	tests := []struct {
		name    string
		mutate  func(*RunConfig)
		wantHit string
	}{
		{"QuorumSize", func(c *RunConfig) { c.SetQuorumSize(4) }, "quorum_size"},
		{"MaxAsync", func(c *RunConfig) { c.SetMaxAsync(1000) }, "max_async"},
		{"RateStep", func(c *RunConfig) { c.SetRateStep(100) }, "rate_step"},
		{"RateStepMax", func(c *RunConfig) { c.SetRateStepMax(400) }, "rate_step_max"},
		{"CallTimeout", func(c *RunConfig) { c.SetCallTimeout(int64(time.Second)) }, "call_timeout"},
		{"StreamMode", func(c *RunConfig) { c.SetStreamMode("dedup") }, "stream_mode"},
		{"StatsMode", func(c *RunConfig) { c.SetStatsMode(StatsMode_HDR) }, "stats_mode"},
		{"NumNodes", func(c *RunConfig) { c.SetNumNodes(8) }, "num_nodes"},
		{"SendBuffer", func(c *RunConfig) { c.SetSendBuffer(4096) }, "send_buffer"},
		{"RecvBuffer", func(c *RunConfig) { c.SetRecvBuffer(4096) }, "recv_buffer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := proto.Clone(base).(*RunConfig)
			tt.mutate(other)
			delta := ConfigDelta(base, other)
			found := false
			for _, d := range delta {
				if strings.HasPrefix(d, tt.wantHit+":") {
					found = true
				}
			}
			if !found {
				t.Errorf("ConfigDelta after mutating %s = %v, want an entry prefixed %q", tt.name, delta, tt.wantHit+":")
			}
		})
	}
}

// TestPrintComparisonFlagsConfigMismatch verifies that PrintComparison warns
// about a semantic config mismatch (e.g. differing quorum size) between
// matched baseline and experiment results, instead of comparing them as if
// they were run under the same settings.
func TestPrintComparisonFlagsConfigMismatch(t *testing.T) {
	baseline := Report_builder{
		Label: "baseline",
		Results: []*Result{Result_builder{
			Config:     RunConfig_builder{Name: "QuorumCall", QuorumSize: 2}.Build(),
			Throughput: 100,
		}.Build()},
	}.Build()
	experiment := Report_builder{
		Label: "experiment",
		Results: []*Result{Result_builder{
			Config:     RunConfig_builder{Name: "QuorumCall", QuorumSize: 4}.Build(),
			Throughput: 120,
		}.Build()},
	}.Build()

	var buf bytes.Buffer
	PrintComparison(baseline, experiment, &buf)
	out := buf.String()
	if !strings.Contains(out, "warning") || !strings.Contains(out, "quorum_size") {
		t.Errorf("PrintComparison output missing quorum_size mismatch warning; got:\n%s", out)
	}
}

// TestLoadReportRejectsNonBinary verifies that LoadReport returns an error for
// a file that does not carry the binary magic header.
func TestLoadReportRejectsNonBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-binary.json")
	if err := os.WriteFile(path, []byte(`{"label":"x","results":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReport(path); err == nil {
		t.Error("LoadReport(non-binary file) = nil error, want error")
	}
}

// TestDecodeReport verifies that [DecodeReport] reads back what [WriteReport]
// wrote, and rejects data that does not carry the binary magic header, so a
// caller that reads the file itself gets the same guarantees as [LoadReport].
func TestDecodeReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.binpb")
	if err := WriteLabeledReport(nil, "baseline", path); err != nil {
		t.Fatalf("WriteLabeledReport: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := DecodeReport(data)
	if err != nil {
		t.Fatalf("DecodeReport: %v", err)
	}
	if got.GetLabel() != "baseline" {
		t.Errorf("label = %q, want %q", got.GetLabel(), "baseline")
	}

	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"short", data[:len(binaryMagic)-1]},
		{"wrong magic", append([]byte("BKRSv9\n\x00"), data[len(binaryMagic):]...)},
		{"payload without magic", data[len(binaryMagic):]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeReport(tt.data); err == nil {
				t.Error("DecodeReport = nil error, want error")
			}
		})
	}
}

// TestWriteLabeledReport verifies that [WriteLabeledReport] wraps results in
// a [Report] carrying the given label and that [LoadReport] reads it back
// unchanged.
func TestWriteLabeledReport(t *testing.T) {
	results := []*Result{Result_builder{
		Config:     RunConfig_builder{Name: "QuorumCall"}.Build(),
		Throughput: 42,
	}.Build()}

	path := filepath.Join(t.TempDir(), "results.binpb")
	if err := WriteLabeledReport(results, "baseline", path); err != nil {
		t.Fatalf("WriteLabeledReport: %v", err)
	}
	got, err := LoadReport(path)
	if err != nil {
		t.Fatalf("LoadReport: %v", err)
	}
	if got.GetLabel() != "baseline" {
		t.Errorf("label = %q, want %q", got.GetLabel(), "baseline")
	}
	if len(got.GetResults()) != 1 || got.GetResults()[0].GetConfig().GetName() != "QuorumCall" {
		t.Errorf("results = %v, want one QuorumCall result", got.GetResults())
	}
}

// TestCompareWithBaseline verifies that [CompareWithBaseline] loads the
// baseline file, wraps results under label, and writes the same output
// [PrintComparison] would.
func TestCompareWithBaseline(t *testing.T) {
	baseline := Report_builder{
		Label: "baseline",
		Results: []*Result{Result_builder{
			Config:     RunConfig_builder{Name: "QuorumCall"}.Build(),
			Throughput: 100,
		}.Build()},
	}.Build()
	path := filepath.Join(t.TempDir(), "baseline.binpb")
	if err := WriteReport(baseline, path); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	experimentResults := []*Result{Result_builder{
		Config:     RunConfig_builder{Name: "QuorumCall"}.Build(),
		Throughput: 120,
	}.Build()}

	var got bytes.Buffer
	if err := CompareWithBaseline(path, "experiment", experimentResults, &got); err != nil {
		t.Fatalf("CompareWithBaseline: %v", err)
	}

	var want bytes.Buffer
	experiment := Report_builder{Label: "experiment", Results: experimentResults}.Build()
	PrintComparison(baseline, experiment, &want)

	if got.String() != want.String() {
		t.Errorf("CompareWithBaseline output = %q, want %q", got.String(), want.String())
	}

	if err := CompareWithBaseline(filepath.Join(t.TempDir(), "missing.binpb"), "experiment", experimentResults, &got); err == nil {
		t.Error("CompareWithBaseline(missing file) = nil error, want error")
	}
}

// TestListBenches verifies that ListBenches renders one aligned line per
// bench naming its description, and nothing for an empty list.
func TestListBenches(t *testing.T) {
	var buf bytes.Buffer
	ListBenches(&buf, []Bench{
		{Name: "QuorumCall", Description: "quorum call workload"},
		{Name: "Multicast", Description: "multicast workload"},
	})
	out := buf.String()
	for _, want := range []string{"QuorumCall:", "quorum call workload", "Multicast:", "multicast workload"} {
		if !strings.Contains(out, want) {
			t.Errorf("ListBenches output missing %q; got:\n%s", want, out)
		}
	}

	var empty bytes.Buffer
	ListBenches(&empty, nil)
	if empty.Len() != 0 {
		t.Errorf("ListBenches(nil) wrote %q, want empty output", empty.String())
	}
}
