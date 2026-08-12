package benchkit

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/relab/gorums"
)

func TestAggregateServerResults(t *testing.T) {
	const eps = 1e-9

	// reply is a compact per-node stub used to build the input map.
	// samples stands in for per-op latency measurements in nanoseconds; the
	// aggregator concatenates them across servers so the cluster-wide mean
	// and stddev can be recomputed from the full distribution.
	type reply struct {
		tTime    int64
		tput     float64
		samples  []int64
		allocsPO uint64
		memPO    uint64
	}
	newReplies := func(in map[uint32]reply) map[uint32]*Result {
		out := make(map[uint32]*Result, len(in))
		for id, r := range in {
			out[id] = Result_builder{
				TotalOps:    uint64(len(r.samples)),
				TotalTime:   r.tTime,
				Throughput:  r.tput,
				Latencies:   r.samples,
				AllocsPerOp: r.allocsPO,
				MemPerOp:    r.memPO,
			}.Build()
		}
		return out
	}

	tests := []struct {
		name    string
		replies map[uint32]reply
		wantErr error
		// For non-error cases, the expected aggregate values.
		wantTotalOps      uint64
		wantTotalTime     int64
		wantThroughput    float64
		wantSamplesConcat []int64  // feeds expected mean/stddev via Result methods
		wantServerMem     []uint64 // per-server memory in sorted node-ID order
		wantServerAlloc   []uint64 // per-server allocs in sorted node-ID order
	}{
		{
			name:    "EmptyRepliesReturnsIncomplete",
			replies: nil,
			wantErr: gorums.ErrIncomplete,
		},
		{
			name: "SingleReplyPassesThrough",
			replies: map[uint32]reply{
				1: {tTime: 1_000_000, tput: 100, samples: []int64{10, 20, 30, 40}, allocsPO: 3, memPO: 64},
			},
			wantTotalOps:      4,
			wantTotalTime:     1_000_000,
			wantThroughput:    100,
			wantSamplesConcat: []int64{10, 20, 30, 40},
			wantServerMem:     []uint64{64 * 4},
			wantServerAlloc:   []uint64{3 * 4},
		},
		{
			name: "ThreeRepliesConcatenateSamples",
			replies: map[uint32]reply{
				1: {tTime: 1, tput: 10, samples: []int64{10, 12}, allocsPO: 2, memPO: 8},
				2: {tTime: 2, tput: 20, samples: []int64{20, 22, 24}, allocsPO: 3, memPO: 16},
				3: {tTime: 3, tput: 30, samples: []int64{30, 32, 34, 36}, allocsPO: 5, memPO: 32},
			},
			wantTotalOps:      9,
			wantTotalTime:     3, // max across servers, not the sum
			wantThroughput:    60,
			wantSamplesConcat: []int64{10, 12, 20, 22, 24, 30, 32, 34, 36},
			wantServerMem:     []uint64{8 * 2, 16 * 3, 32 * 4}, // sorted by node ID: 1, 2, 3
			wantServerAlloc:   []uint64{2 * 2, 3 * 3, 5 * 4},
		},
		{
			name: "UnsortedIDsProduceSortedServerStats",
			replies: map[uint32]reply{
				// Insertion order does not matter; sorted ID order determines ServerStats[i].
				7: {tTime: 1, tput: 1, samples: []int64{5, 5, 5}, allocsPO: 1, memPO: 1},
				3: {tTime: 1, tput: 1, samples: []int64{5, 5, 5}, allocsPO: 2, memPO: 2},
				5: {tTime: 1, tput: 1, samples: []int64{5, 5, 5}, allocsPO: 3, memPO: 3},
			},
			wantTotalOps:      9,
			wantTotalTime:     1, // max across servers, not the sum
			wantThroughput:    3,
			wantSamplesConcat: []int64{5, 5, 5, 5, 5, 5, 5, 5, 5},
			wantServerMem:     []uint64{2 * 3, 3 * 3, 1 * 3}, // node IDs sorted: 3, 5, 7 -> mem 2,3,1
			wantServerAlloc:   []uint64{2 * 3, 3 * 3, 1 * 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AggregateServerResults(newReplies(tt.replies))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.GetTotalOps() != tt.wantTotalOps {
				t.Errorf("TotalOps = %d, want %d", got.GetTotalOps(), tt.wantTotalOps)
			}
			if got.GetTotalTime() != tt.wantTotalTime {
				t.Errorf("TotalTime = %d, want %d", got.GetTotalTime(), tt.wantTotalTime)
			}
			if math.Abs(got.GetThroughput()-tt.wantThroughput) > eps {
				t.Errorf("Throughput = %v, want %v", got.GetThroughput(), tt.wantThroughput)
			}

			// Reference values come from invoking the same methods on a
			// Result built from the expected concatenation. time.Duration
			// is int64 so equality is exact once the truncation boundary
			// is shared between got and want.
			want := Result_builder{Latencies: tt.wantSamplesConcat}.Build()
			gotMean, gotSD := got.LatencyMeanAndStdDev()
			wantMean, wantSD := want.LatencyMeanAndStdDev()
			if gotMean != wantMean {
				t.Errorf("LatencyMean = %v, want %v", gotMean, wantMean)
			}
			if gotSD != wantSD {
				t.Errorf("LatencyStdDev = %v, want %v", gotSD, wantSD)
			}

			if n := len(got.GetServerStats()); n != len(tt.wantServerMem) {
				t.Fatalf("len(ServerStats) = %d, want %d", n, len(tt.wantServerMem))
			}
			for i, s := range got.GetServerStats() {
				if s.GetMemory() != tt.wantServerMem[i] {
					t.Errorf("ServerStats[%d].Memory = %d, want %d", i, s.GetMemory(), tt.wantServerMem[i])
				}
				if s.GetAllocs() != tt.wantServerAlloc[i] {
					t.Errorf("ServerStats[%d].Allocs = %d, want %d", i, s.GetAllocs(), tt.wantServerAlloc[i])
				}
			}
		})
	}
}

// TestAggregateServerResultsHistogram verifies that in StatsMode_HDR, where
// per-server replies carry a histogram instead of raw samples, the aggregate
// merges the per-server histograms (Latencies nil, Histogram set) with counts
// summed across servers, and that TotalOps and Throughput still sum as usual.
func TestAggregateServerResultsHistogram(t *testing.T) {
	replies := map[uint32]*Result{
		1: Result_builder{TotalOps: 3, TotalTime: 2, Throughput: 30, Histogram: hist(1_000, 1_000, 1_000)}.Build(),
		2: Result_builder{TotalOps: 2, TotalTime: 1, Throughput: 20, Histogram: hist(1_000, 2_000)}.Build(),
	}
	got, err := AggregateServerResults(replies)
	if err != nil {
		t.Fatalf("AggregateServerResults: %v", err)
	}
	if got.GetLatencies() != nil {
		t.Errorf("Latencies = %v, want nil in HDR mode", got.GetLatencies())
	}
	if got.GetTotalOps() != 5 {
		t.Errorf("TotalOps = %d, want 5", got.GetTotalOps())
	}
	if got.GetThroughput() != 50 {
		t.Errorf("Throughput = %v, want 50", got.GetThroughput())
	}
	h := got.GetHistogram()
	if h == nil {
		t.Fatal("Histogram = nil, want merged histogram")
	}
	if n := totalCount(h); n != 5 {
		t.Errorf("merged histogram count = %d, want 5", n)
	}
	// Merged distribution {1,1,1,1,2}µs has median 1µs, reproduced within
	// HDR precision.
	if p := got.Percentiles(0.5); p == nil || math.Abs(float64(p[0]-time.Microsecond)) > 50 {
		t.Errorf("Percentiles(0.5) = %v, want ≈1µs", p)
	}
}
