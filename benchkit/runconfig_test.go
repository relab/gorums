package benchkit

import "testing"

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
