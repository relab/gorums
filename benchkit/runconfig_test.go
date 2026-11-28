package benchkit

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
)

func TestRunConfigDimensionsRoundTrip(t *testing.T) {
	want := Dimensions{
		Benchmark:  "SymmetricQuorumCall",
		Nodes:      9,
		Workers:    8,
		Payload:    1024,
		Rate:       5000,
		SendBuffer: 256,
		RecvBuffer: 16,
		StreamMode: "dedup",
	}
	cfg := NewRunConfig(want)
	if got := cfg.Dimensions(); got != want {
		t.Fatalf("Dimensions() = %+v, want %+v", got, want)
	}
	replacement := Dimensions{Benchmark: "Broadcast", Nodes: 3, Workers: 2}
	cfg.ApplyDimensions(replacement)
	if got := cfg.Dimensions(); got != replacement {
		t.Fatalf("Dimensions() after ApplyDimensions = %+v, want %+v", got, replacement)
	}

	var nilConfig *RunConfig
	if got := nilConfig.Dimensions(); got != (Dimensions{}) {
		t.Fatalf("nil Dimensions() = %+v, want zero value", got)
	}
}

func TestRunConfigDimensionsWithFallback(t *testing.T) {
	cfg := NewRunConfig(Dimensions{
		Benchmark: "Q",
		Nodes:     3,
		Rate:      100,
	})
	fallback := Dimensions{
		Benchmark:  "fallback",
		Nodes:      9,
		Workers:    8,
		Payload:    1024,
		Rate:       5000,
		SendBuffer: 256,
		RecvBuffer: 16,
		StreamMode: "dual",
	}
	want := Dimensions{
		Benchmark:  "Q",
		Nodes:      3,
		Workers:    8,
		Payload:    1024,
		Rate:       100,
		SendBuffer: 256,
		RecvBuffer: 16,
		StreamMode: "dual",
	}
	if got := cfg.DimensionsWithFallback(fallback); got != want {
		t.Fatalf("DimensionsWithFallback() = %+v, want %+v", got, want)
	}
}

// TestRunConfigCustomDimensionsCanonical verifies that equal sets of custom dimension
// values give equal [CustomDimensions], whatever the order they were added in, so a
// [Dimensions] holding them stays usable as a map key.
func TestRunConfigCustomDimensionsCanonical(t *testing.T) {
	a := NewCustomDimensions(map[string]string{"t": "2", "d": "20", "ready": "tree"})
	b := NewCustomDimensions(map[string]string{"ready": "tree", "d": "20", "t": "2"})
	if a != b {
		t.Fatalf("NewCustomDimensions differs by insertion order: %q != %q", a, b)
	}
	if got, want := a.Names(), []string{"d", "ready", "t"}; !slices.Equal(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
	if got := a.Get("ready"); got != "tree" {
		t.Errorf(`Get("ready") = %q, want "tree"`, got)
	}
	if got := a.Get("missing"); got != "" {
		t.Errorf(`Get("missing") = %q, want ""`, got)
	}
	var names []string
	for name, value := range a.All() {
		names = append(names, name+"="+value)
	}
	if want := []string{"d=20", "ready=tree", "t=2"}; !slices.Equal(names, want) {
		t.Errorf("All() = %v, want %v", names, want)
	}
	if got := NewCustomDimensions(nil); got != "" {
		t.Errorf("NewCustomDimensions(nil) = %q, want empty", got)
	}
	if got := CustomDimensions("").Map(); got != nil {
		t.Errorf("empty Map() = %v, want nil", got)
	}
}

// TestRunConfigCustomDimensionsRoundTrip verifies that custom dimension values survive
// [RunConfig.ApplyDimensions] and [RunConfig.Dimensions], and that a result
// without them takes the fallback's values.
func TestRunConfigCustomDimensionsRoundTrip(t *testing.T) {
	want := Dimensions{
		Benchmark: "Idea0", Nodes: 9,
		Custom: NewCustomDimensions(map[string]string{"d": "20", "ready": "all"}),
	}
	cfg := NewRunConfig(want)
	if got := cfg.Dimensions(); got != want {
		t.Fatalf("Dimensions() = %+v, want %+v", got, want)
	}
	if got := cfg.GetCustomDimensions(); !maps.Equal(got, map[string]string{"d": "20", "ready": "all"}) {
		t.Errorf("GetCustomDimensions() = %v", got)
	}
	bare := NewRunConfig(Dimensions{Benchmark: "Idea0"})
	if got := bare.DimensionsWithFallback(want); got.Custom != want.Custom {
		t.Errorf("DimensionsWithFallback().Custom = %q, want %q", got.Custom, want.Custom)
	}
	if got := cfg.DimensionsWithFallback(Dimensions{Custom: NewCustomDimensions(map[string]string{"d": "22"})}); got.Custom != want.Custom {
		t.Errorf("DimensionsWithFallback() replaced recorded custom dimensions: %q, want %q", got.Custom, want.Custom)
	}
}

// TestRunConfigCustomDimensionsJSON verifies that [CustomDimensions] encodes as a JSON object of
// name-value pairs, and that a [Dimensions] without custom values omits the
// field, so manifests of sweeps without custom dimensions keep their form.
func TestRunConfigCustomDimensionsJSON(t *testing.T) {
	dims := Dimensions{Benchmark: "Q", Custom: NewCustomDimensions(map[string]string{"t": "1", "d": "20"})}
	data, err := json.Marshal(dims)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	obj, ok := raw["custom"].(map[string]any)
	if !ok || obj["d"] != "20" || obj["t"] != "1" || len(obj) != 2 {
		t.Fatalf("custom = %#v in %s, want {d: 20, t: 1}", raw["custom"], data)
	}
	var back Dimensions
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back != dims {
		t.Errorf("JSON round trip = %+v, want %+v", back, dims)
	}

	data, err = json.Marshal(Dimensions{Benchmark: "Q"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\"custom\"") {
		t.Errorf("Dimensions without custom dimensions encodes a custom field: %s", data)
	}
	if err := json.Unmarshal([]byte(`{"custom": ["d"]}`), &back); err == nil {
		t.Error("Unmarshal of a non-object custom succeeded, want an error")
	}
}

// FuzzRunConfigCustomDimensions verifies that any set of custom dimension values survives
// the trip through [RunConfig], its wire encoding, and JSON unchanged.
func FuzzRunConfigCustomDimensions(f *testing.F) {
	f.Add("d", "20", "ready", "tree")
	f.Add("a&b", "x=y", "", "%")
	f.Add("t", "", "t2", "1.5")
	f.Fuzz(func(t *testing.T, k1, v1, k2, v2 string) {
		for _, s := range []string{k1, v1, k2, v2} {
			if !utf8.ValidString(s) {
				t.Skip("proto and JSON strings are UTF-8")
			}
		}
		want := map[string]string{k1: v1, k2: v2}
		customDims := NewCustomDimensions(want)
		if got := customDims.Map(); !maps.Equal(got, want) {
			t.Fatalf("NewCustomDimensions(%v).Map() = %v", want, got)
		}
		data, err := proto.Marshal(NewRunConfig(Dimensions{Custom: customDims}))
		if err != nil {
			t.Fatal(err)
		}
		cfg := &RunConfig{}
		if err := proto.Unmarshal(data, cfg); err != nil {
			t.Fatal(err)
		}
		if got := cfg.Dimensions().Custom; got != customDims {
			t.Fatalf("proto round trip = %q, want %q", got, customDims)
		}
		js, err := json.Marshal(customDims)
		if err != nil {
			t.Fatal(err)
		}
		var back CustomDimensions
		if err := json.Unmarshal(js, &back); err != nil {
			t.Fatal(err)
		}
		if back != customDims {
			t.Fatalf("JSON round trip = %q, want %q", back, customDims)
		}
	})
}
