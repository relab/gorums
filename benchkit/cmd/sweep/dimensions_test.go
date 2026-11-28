package main

import (
	"slices"
	"testing"

	"github.com/relab/gorums/benchkit"
)

func TestDimensionProjections(t *testing.T) {
	full := benchkit.Dimensions{
		Benchmark: "Q", Nodes: 9, Workers: 8, Payload: 1024, Rate: 5000,
		SendBuffer: 256, RecvBuffer: 16, StreamMode: "dual", Custom: custom("d", "20"),
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
		Benchmark: "Q", Nodes: 9, StreamMode: "dual", Custom: custom("d", "20"),
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

// custom returns the CustomDimensions holding the given name-value pairs.
func custom(pairs ...string) benchkit.CustomDimensions {
	m := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return benchkit.NewCustomDimensions(m)
}

// TestDimensionSchemaCustomColumns verifies that the schema of a data set puts
// its custom dimensions after the built-in ones, sorted by name, and that a
// configuration without a value for one of them reads as "".
func TestDimensionSchemaCustomColumns(t *testing.T) {
	configs := []benchkit.Dimensions{
		{Benchmark: "Baseline", Nodes: 3, Custom: custom("t", "1")},
		{Benchmark: "Idea0", Nodes: 3, Custom: custom("t", "2", "d", "20")},
	}
	schema := newDimensionSchema(configs)
	want := []string{"benchmark", "nodes", "workers", "payload", "rate", "send_buffer", "recv_buffer", "stream_mode", "d", "t"}
	if got := schema.columns(); !slices.Equal(got, want) {
		t.Errorf("columns = %v, want %v", got, want)
	}
	if got, want := schema.values(configs[0], "workers", "payload", "rate", "send_buffer", "recv_buffer"),
		[]string{"Baseline", "3", "", "", "1"}; !slices.Equal(got, want) {
		t.Errorf("values = %q, want %q", got, want)
	}
	if got, want := schema.axes(), []string{"nodes", "workers", "payload", "rate", "send_buffer", "recv_buffer", "d", "t"}; !slices.Equal(got, want) {
		t.Errorf("axes = %v, want %v", got, want)
	}
	if got := newDimensionSchema([]benchkit.Dimensions{{Benchmark: "Q"}}).columns(); !slices.Equal(got, want[:8]) {
		t.Errorf("columns without custom dimensions = %v, want %v", got, want[:8])
	}
	if got := dimensionValue(configs[1], "d"); got != "20" {
		t.Errorf(`dimensionValue(d) = %q, want "20"`, got)
	}
	if got := dimensionLabel("d"); got != "d" {
		t.Errorf(`dimensionLabel(d) = %q, want "d"`, got)
	}
	if got := dimensionValue(configs[1], ""); got != "" {
		t.Errorf(`dimensionValue("") = %q, want ""`, got)
	}
}

// TestConfigLabelCustomDimensions verifies that a varying custom dimension is
// named as name=value in a compact label, after the built-in dimensions, and
// that a custom value "0" is a real value rather than the unset marker.
func TestConfigLabelCustomDimensions(t *testing.T) {
	configs := []benchkit.Dimensions{
		{Benchmark: "Q", Nodes: 3, Custom: custom("t", "0", "ready", "all")},
		{Benchmark: "Q", Nodes: 5, Custom: custom("t", "1", "ready", "all")},
	}
	varying := varyingDimensions(configs)
	if !varying["t"] || varying["ready"] {
		t.Errorf("varying = %v, want t and not ready", varying)
	}
	if got, want := configLabel(configs[0], varying), "N3 t=0"; got != want {
		t.Errorf("configLabel = %q, want %q", got, want)
	}
}

// TestCompareDimensionsNumericCustom verifies that custom values that look like
// numbers order by value, before the values that do not, which order as strings.
func TestCompareDimensionsNumericCustom(t *testing.T) {
	values := []string{"tree", "10", "all", "9", "-1", "2.5", "1e5"}
	configs := make([]benchkit.Dimensions, len(values))
	for i, v := range values {
		configs[i] = benchkit.Dimensions{Benchmark: "Q", Custom: custom("x", v)}
	}
	slices.SortFunc(configs, compareDimensions)
	var got []string
	for _, c := range configs {
		got = append(got, c.Custom.Get("x"))
	}
	if want := []string{"-1", "2.5", "9", "10", "1e5", "all", "tree"}; !slices.Equal(got, want) {
		t.Errorf("sorted = %v, want %v", got, want)
	}
	// A configuration without the dimension sorts before one with a value.
	if c := compareDimensions(benchkit.Dimensions{Benchmark: "Q"}, configs[0]); c >= 0 {
		t.Errorf("compareDimensions(absent, -1) = %d, want < 0", c)
	}
}
