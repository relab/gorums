package main

import (
	"slices"
	"testing"

	"github.com/relab/gorums/benchkit"
)

func TestSweepRunSpecsIncludeStreamModes(t *testing.T) {
	sc := sweepConfig{
		numNodes:    []int{3},
		workers:     []int{1},
		payloads:    []int{0},
		rates:       []int{0},
		benchmarks:  []string{"Q"},
		streamModes: []string{"dual", "dedup"},
		reps:        2,
	}
	var got []runSpec
	for p := range sc.runSpecs() {
		got = append(got, p)
	}
	if len(got) != 4 {
		t.Fatalf("runSpecs = %d, want 4", len(got))
	}
	modes := []string{got[0].StreamMode, got[1].StreamMode, got[2].StreamMode, got[3].StreamMode}
	wantModes := []string{"dual", "dedup", "dual", "dedup"}
	if !slices.Equal(modes, wantModes) {
		t.Fatalf("stream modes = %v, want %v", modes, wantModes)
	}
	if got := countRuns(sc); got != 4 {
		t.Fatalf("countRuns = %d, want 4", got)
	}
}

func TestRunBaseIncludesStreamMode(t *testing.T) {
	p := runSpec{
		Dimensions: benchkit.Dimensions{
			Benchmark: "Symmetric", Nodes: 9, Workers: 4, Payload: 64, Rate: 1000, StreamMode: "dedup",
		},
		Rep: 3,
	}
	if got, want := runBase("e1", p), "e1_Symmetric_N9_W4_P64_R1000_Sdedup_r3"; got != want {
		t.Fatalf("runBase = %q, want %q", got, want)
	}
}

// TestRunBaseBufferSizes verifies that swept buffer capacities reach the run
// name, so runs differing only by an effective buffer size land in distinct
// files. Zero selects the default and therefore adds no suffix.
func TestRunBaseBufferSizes(t *testing.T) {
	tests := []struct {
		name       string
		sendBuffer int
		recvBuffer int
		want       string
	}{
		{"Defaults", 0, 0, "e1_Q_N3_W1_P0_Sdual_r1"},
		{"SendOnly", 256, 0, "e1_Q_N3_W1_P0_SB256_Sdual_r1"},
		{"RecvOnly", 0, 16, "e1_Q_N3_W1_P0_RB16_Sdual_r1"},
		{"Both", 4096, 16, "e1_Q_N3_W1_P0_SB4096_RB16_Sdual_r1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := runSpec{
				Dimensions: benchkit.Dimensions{
					Benchmark: "Q", Nodes: 3, Workers: 1, StreamMode: "dual",
					SendBuffer: tt.sendBuffer, RecvBuffer: tt.recvBuffer,
				},
				Rep: 1,
			}
			if got := runBase("e1", p); got != tt.want {
				t.Errorf("runBase = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmptyAndExplicitZeroBuffersHaveSameIdentity(t *testing.T) {
	base := sweepConfig{
		numNodes: []int{3}, workers: []int{1}, payloads: []int{0}, rates: []int{0},
		benchmarks: []string{"Q"}, streamModes: []string{"dual"}, reps: 1,
	}
	explicit := base
	explicit.sendBuffers = []int{0}
	explicit.recvBuffers = []int{0}

	only := func(sc sweepConfig) runSpec {
		var specs []runSpec
		for spec := range sc.runSpecs() {
			specs = append(specs, spec)
		}
		if len(specs) != 1 {
			t.Fatalf("runSpecs = %d, want 1", len(specs))
		}
		return specs[0]
	}
	unset, zero := only(base), only(explicit)
	if unset != zero {
		t.Fatalf("unset buffers = %+v, explicit zeros = %+v", unset, zero)
	}
	if runBase("e1", unset) != runBase("e1", zero) {
		t.Fatalf("run bases differ: %q vs %q", runBase("e1", unset), runBase("e1", zero))
	}
}

// TestSweepRunSpecsBufferAxes verifies that the buffer lists multiply into the
// parameter product, that countRuns agrees with what runSpecs yields, and that
// an unset axis contributes exactly one unset combination.
func TestSweepRunSpecsBufferAxes(t *testing.T) {
	tests := []struct {
		name        string
		sendBuffers []int
		recvBuffers []int
		want        int
	}{
		{"NeitherSet", nil, nil, 1},
		{"SendOnly", []int{64, 256, 1024}, nil, 3},
		{"RecvOnly", nil, []int{0, 16}, 2},
		{"Both", []int{64, 256}, []int{0, 16}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := sweepConfig{
				numNodes: []int{3}, workers: []int{1}, payloads: []int{0}, rates: []int{0},
				benchmarks: []string{"Q"}, streamModes: []string{"dual"}, reps: 1,
				sendBuffers: tt.sendBuffers, recvBuffers: tt.recvBuffers,
			}
			var got []runSpec
			for p := range sc.runSpecs() {
				got = append(got, p)
			}
			if len(got) != tt.want {
				t.Fatalf("runSpecs = %d, want %d", len(got), tt.want)
			}
			if n := countRuns(sc); n != tt.want {
				t.Errorf("countRuns = %d, want %d", n, tt.want)
			}
			// Every combination must produce a distinct output file.
			names := make(map[string]bool, len(got))
			for _, p := range got {
				base := runBase("e1", p)
				if names[base] {
					t.Errorf("duplicate run base %q", base)
				}
				names[base] = true
			}
			if len(tt.sendBuffers) == 0 && got[0].SendBuffer != 0 {
				t.Error("unswept send buffer did not select the default")
			}
		})
	}
}
