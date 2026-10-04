package benchkit

import "time"

// SetClientTotals sets x's TotalOps, TotalTime, and Throughput from the
// client-side send count and the measurement window. The client send count is
// the authoritative measure of work done when each send reaches several
// servers, so it replaces any server-side totals. Throughput is left unchanged
// when sent is zero.
func (x *Result) SetClientTotals(sent uint64, elapsed time.Duration) {
	x.SetTotalOps(sent)
	x.SetTotalTime(int64(elapsed))
	if sent > 0 {
		x.SetThroughput(float64(sent) / elapsed.Seconds())
	}
}

// SetPerOpMemoryFromServerStats sets x's per-op allocation and memory counters
// from its ServerStats, divided by sent, the client send count. Every local
// server reads the same process-wide memory statistics, so the largest server's
// counters cover the whole process; summing them would count the process once
// per local server. The counters are left unset when sent is zero or x has no
// ServerStats.
func (x *Result) SetPerOpMemoryFromServerStats(sent uint64) {
	if sent == 0 {
		return
	}
	var allocs, mem uint64
	for _, s := range x.GetServerStats() {
		allocs = max(allocs, s.GetAllocs())
		mem = max(mem, s.GetMemory())
	}
	if allocs == 0 && mem == 0 {
		return
	}
	x.SetAllocsPerOp(allocs / sent)
	x.SetMemPerOp(mem / sent)
}
