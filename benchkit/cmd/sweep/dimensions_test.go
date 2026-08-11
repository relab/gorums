package main

import (
	"testing"

	"github.com/relab/gorums/benchkit"
)

func TestDimensionProjections(t *testing.T) {
	full := benchkit.Dimensions{
		Benchmark: "Q", Nodes: 9, Workers: 8, Payload: 1024, Rate: 5000,
		SendBuffer: 256, RecvBuffer: 16, StreamMode: "dual",
	}

	comparison := comparisonDimensions(full)
	wantComparison := full
	wantComparison.StreamMode = ""
	if comparison != wantComparison {
		t.Errorf("comparisonDimensions = %+v, want %+v", comparison, wantComparison)
	}

	if got, want := loadScaleDimensions(full, []string{"workers"}), (benchkit.Dimensions{
		Payload: 1024, Rate: 5000, SendBuffer: 256, RecvBuffer: 16,
	}); got != want {
		t.Errorf("loadScaleDimensions = %+v, want %+v", got, want)
	}
	// A curve traced along the offered rate must not band by it: its points
	// differ only in the rate, so they belong to one curve and one band.
	if got, want := loadScaleDimensions(full, []string{"rate"}), (benchkit.Dimensions{
		Payload: 1024, SendBuffer: 256, RecvBuffer: 16,
	}); got != want {
		t.Errorf("loadScaleDimensions with rate traced = %+v, want %+v", got, want)
	}
	if got, want := nodeHealthDimensions(full), (benchkit.Dimensions{
		Benchmark: "Q", Nodes: 9, StreamMode: "dual",
	}); got != want {
		t.Errorf("nodeHealthDimensions = %+v, want %+v", got, want)
	}
}

// TestConfigLabel verifies that a compact configuration label names only the
// dimensions that vary between the labeled configurations: what they all share
// identifies none of them and belongs in the report's experiment line, and an
// unset numeric dimension describes no part of the experiment at all.
func TestConfigLabel(t *testing.T) {
	configs := []benchkit.Dimensions{
		{Benchmark: "Q", Nodes: 9, Workers: 32, Payload: 4096, Rate: 1000, SendBuffer: 4096, StreamMode: "dedup"},
		{Benchmark: "Q", Nodes: 15, Workers: 32, Payload: 16384, Rate: 1000, SendBuffer: 4096, StreamMode: "dual"},
	}
	varying := varyingDimensions(configs)
	for _, name := range []string{"nodes", "payload", "stream_mode"} {
		if !varying[name] {
			t.Errorf("varyingDimensions omits %q, which differs between the configurations", name)
		}
	}
	for _, name := range []string{"benchmark", "workers", "rate", "send_buffer", "recv_buffer"} {
		if varying[name] {
			t.Errorf("varyingDimensions includes %q, which every configuration shares", name)
		}
	}
	if got, want := configLabel(configs[1], varying), "N15 P16384 dual"; got != want {
		t.Errorf("configLabel = %q, want %q", got, want)
	}
	// A single configuration varies in nothing, so it has no label of its own.
	if got := configLabel(configs[0], varyingDimensions(configs[:1])); got != "" {
		t.Errorf("configLabel of a lone configuration = %q, want empty", got)
	}
}

func TestExcludedByBufferDimension(t *testing.T) {
	dims := benchkit.Dimensions{SendBuffer: 256, RecvBuffer: 0}
	if !excludedByDim(map[string]map[string]bool{"send_buffer": {"256": true}}, dims) {
		t.Error("send_buffer=256 did not exclude matching dimensions")
	}
	if !excludedByDim(map[string]map[string]bool{"recv_buffer": {"0": true}}, dims) {
		t.Error("recv_buffer=0 did not exclude matching dimensions")
	}
	if excludedByDim(map[string]map[string]bool{"send_buffer": {"64": true}}, dims) {
		t.Error("send_buffer=64 excluded non-matching dimensions")
	}
}
