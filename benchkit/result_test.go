package benchkit

import (
	"testing"
	"time"
)

func TestResultSetClientTotals(t *testing.T) {
	tests := []struct {
		name           string
		sent           uint64
		elapsed        time.Duration
		wantThroughput float64
	}{
		{"Sends", 100, 2 * time.Second, 50},
		{"NoSends", 0, 2 * time.Second, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pre-existing server-side totals must be overridden.
			r := Result_builder{TotalOps: 999, TotalTime: 1}.Build()
			r.SetClientTotals(tt.sent, tt.elapsed)
			if got := r.GetTotalOps(); got != tt.sent {
				t.Errorf("TotalOps = %d, want %d", got, tt.sent)
			}
			if got := time.Duration(r.GetTotalTime()); got != tt.elapsed {
				t.Errorf("TotalTime = %v, want %v", got, tt.elapsed)
			}
			if got := r.GetThroughput(); got != tt.wantThroughput {
				t.Errorf("Throughput = %v, want %v", got, tt.wantThroughput)
			}
		})
	}
}

// TestResultSetPerOpMemoryFromServerStats verifies that per-op memory derives
// from the largest server's counters divided by the client send count, since
// summing the servers would count the shared process once per local server.
func TestResultSetPerOpMemoryFromServerStats(t *testing.T) {
	stat := func(allocs, mem uint64) *MemoryStat {
		return MemoryStat_builder{Allocs: allocs, Memory: mem}.Build()
	}
	tests := []struct {
		name       string
		stats      []*MemoryStat
		sent       uint64
		wantAllocs uint64
		wantMem    uint64
	}{
		{"OneServer", []*MemoryStat{stat(300, 6000)}, 10, 30, 600},
		{"LocalServersShareProcess", []*MemoryStat{stat(290, 5800), stat(310, 6200), stat(300, 6000)}, 10, 31, 620},
		{"NoSends", []*MemoryStat{stat(300, 6000)}, 0, 0, 0},
		{"NoServerStats", nil, 10, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Result_builder{ServerStats: tt.stats}.Build()
			r.SetPerOpMemoryFromServerStats(tt.sent)
			if got := r.GetAllocsPerOp(); got != tt.wantAllocs {
				t.Errorf("AllocsPerOp = %d, want %d", got, tt.wantAllocs)
			}
			if got := r.GetMemPerOp(); got != tt.wantMem {
				t.Errorf("MemPerOp = %d, want %d", got, tt.wantMem)
			}
		})
	}
}
