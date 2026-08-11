package benchkit

import (
	"io"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestClockOffset(t *testing.T) {
	tests := []struct {
		name                  string
		t1, serverTime, t4    int64
		wantOffset, wantDelay int64
	}{
		{name: "PeerAhead", t1: 1000, serverTime: 1600, t4: 1200, wantOffset: 500, wantDelay: 200},
		{name: "PeerBehind", t1: 1000, serverTime: 800, t4: 1200, wantOffset: -300, wantDelay: 200},
		{name: "SameClock", t1: 1000, serverTime: 1100, t4: 1200, wantOffset: 0, wantDelay: 200},
		{name: "ZeroDelay", t1: 5000, serverTime: 5000, t4: 5000, wantOffset: 0, wantDelay: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, delay := clockOffset(tt.t1, tt.serverTime, tt.t4)
			if offset != tt.wantOffset || delay != tt.wantDelay {
				t.Errorf("clockOffset(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.t1, tt.serverTime, tt.t4, offset, delay, tt.wantOffset, tt.wantDelay)
			}
		})
	}
}

func TestAverageOffsets(t *testing.T) {
	tests := []struct {
		name string
		a, b map[uint32]int64
		want map[uint32]int64
	}{
		{
			name: "SharedKeys",
			a:    map[uint32]int64{1: 100, 2: -40},
			b:    map[uint32]int64{1: 200, 2: 0},
			want: map[uint32]int64{1: 150, 2: -20},
		},
		{
			name: "KeyOnlyInA",
			a:    map[uint32]int64{1: 100, 3: 50},
			b:    map[uint32]int64{1: 100},
			want: map[uint32]int64{1: 100, 3: 50},
		},
		{
			name: "KeyOnlyInB",
			a:    map[uint32]int64{1: 100},
			b:    map[uint32]int64{1: 100, 4: 80},
			want: map[uint32]int64{1: 100, 4: 80},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AverageOffsets(tt.a, tt.b)
			if !maps.Equal(got, tt.want) {
				t.Errorf("AverageOffsets = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCorrectLatencies(t *testing.T) {
	tests := []struct {
		name   string
		in     []int64
		offset int64
		want   []int64
	}{
		{name: "SubtractPositive", in: []int64{300, 400, 450}, offset: 100, want: []int64{200, 300, 350}},
		{name: "SubtractNegative", in: []int64{200, 100}, offset: -50, want: []int64{250, 150}},
		{name: "ZeroOffsetUnchanged", in: []int64{10, 20}, offset: 0, want: []int64{10, 20}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Result_builder{Latencies: slices.Clone(tt.in)}.Build()
			CorrectLatencies(r, tt.offset)
			if got := r.GetLatencies(); !slices.Equal(got, tt.want) {
				t.Errorf("CorrectLatencies(%v, %d) = %v, want %v", tt.in, tt.offset, got, tt.want)
			}
		})
	}
}

// TestCorrectLatenciesHistogram verifies that in StatsMode_HDR, where a result
// carries a histogram instead of raw samples, CorrectLatencies subtracts the
// clock offset from the histogram bucket values (re-quantized onto the canonical
// layout) while preserving the sample count, and leaves a zero-offset result
// untouched.
func TestCorrectLatenciesHistogram(t *testing.T) {
	src := hist(20_000, 20_000, 20_000) // 3 samples at 20µs

	r := Result_builder{Histogram: src}.Build()
	CorrectLatencies(r, 5_000) // subtract 5µs
	if got := totalCount(r.GetHistogram()); got != 3 {
		t.Fatalf("count after correction = %d, want 3", got)
	}
	if got := p50(r.GetHistogram()); math.Abs(float64(got-15*time.Microsecond)) > 50 {
		t.Errorf("p50 after -5µs correction = %v, want ≈15µs", got)
	}

	// A zero offset must leave the histogram untouched (same pointer, no
	// re-quantization).
	unchanged := Result_builder{Histogram: src}.Build()
	CorrectLatencies(unchanged, 0)
	if unchanged.GetHistogram() != src {
		t.Error("zero-offset correction replaced the histogram, want unchanged")
	}
}

// captureStderr redirects os.Stderr for the duration of f and returns everything
// written to it, restoring the original stderr before returning.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	f()
	_ = w.Close()
	os.Stderr = orig
	return <-done
}

// TestLogOffsetsAlwaysEmits verifies the clock-offset summary is written to
// stderr even when verbose logging is disabled, so a sweep's collected per-run
// log always records how a server-measured latency was corrected. It guards the
// switch from the -verbose Logf to the unconditional benchkit.Printf.
func TestLogOffsetsAlwaysEmits(t *testing.T) {
	SetVerbose(false)
	before := map[uint32]int64{5: 1_000_000, 2: -2_000_000}
	after := map[uint32]int64{5: 1_500_000, 2: -2_000_000}
	out := captureStderr(t, func() { LogOffsets("servers", before, after) })

	for _, want := range []string{
		// Peers are printed in sorted node-ID order.
		"[offsets servers] peer 2:",
		"[offsets servers] peer 5:",
		// Drift is after minus before; peer 5 moved by 500µs.
		"drift=500µs",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("offset log missing %q; got:\n%s", want, out)
		}
	}
}
