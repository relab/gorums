package benchkit

import (
	"fmt"
	"iter"
	"strings"
	"time"
)

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

// Row returns the result's display cells in column order: Name, Throughput,
// Latency, Std.dev, p50, p95, p99, B/op, allocs/op. [PrintResults]
// consumes these cells directly, rather than parsing Format's joined string,
// so adding, removing, or reordering columns here cannot silently break table
// rendering.
func (x *Result) Row() []string {
	mean, stddev := x.LatencyMeanAndStdDev()
	row := []string{
		x.GetConfig().GetName(),
		fmt.Sprintf("%.1f ops/sec", x.GetThroughput()),
		formatDuration(mean),
		formatDuration(stddev),
	}
	if pcts := x.Percentiles(0.5, 0.95, 0.99); pcts != nil {
		row = append(row, formatDuration(pcts[0]), formatDuration(pcts[1]), formatDuration(pcts[2]))
	} else {
		row = append(row, "n/a", "n/a", "n/a")
	}
	return append(row,
		fmt.Sprintf("%d B/op", x.GetMemPerOp()),
		fmt.Sprintf("%d allocs/op", x.GetAllocsPerOp()),
	)
}

// Format returns a tab formatted string representation of the result (see
// Row for column order). Always emits nine tab-separated columns, each
// followed by a trailing tab, so tabwriter aligns correctly.
func (x *Result) Format() string {
	return strings.Join(x.Row(), "\t") + "\t"
}

// LatencyMeanAndStdDev returns the mean and standard deviation of the recorded
// latencies. In exact mode this is the sample standard deviation over the raw
// samples; StdDev is zero when fewer than two samples have been recorded. In
// HDR mode, where no raw samples are retained, both are the population mean and
// standard deviation computed from the persisted histogram's weighted
// (value, count) pairs, matching [Histogram.Mean] and [Histogram.StdDev]'s
// HdrHistogram-mirroring convention.
func (x *Result) LatencyMeanAndStdDev() (mean, stddev time.Duration) {
	m, sd := resultDist(x).MeanAndStdDev()
	return time.Duration(m), time.Duration(sd)
}

// Percentiles returns the requested quantile values as time.Duration.
// Quantiles are in [0, 1]; e.g. Percentiles(0.5, 0.95) yields p50 and p95.
// In HDR mode the quantiles come from the persisted histogram, accurate to
// its significant figures. Returns nil when no samples have been recorded.
func (x *Result) Percentiles(quantiles ...float64) []time.Duration {
	qs := resultDist(x).Quantiles(quantiles...)
	if qs == nil {
		return nil
	}
	out := make([]time.Duration, len(qs))
	for i, q := range qs {
		out[i] = time.Duration(q)
	}
	return out
}

// ServerMemPerOp yields each server's memory (bytes) and allocations per
// operation, in server order.
func (x *Result) ServerMemPerOp() iter.Seq2[uint64, uint64] {
	return func(yield func(memPerOp, allocsPerOp uint64) bool) {
		for _, memStat := range x.GetServerStats() {
			mem, allocs := memStat.perOp(x.GetTotalOps())
			if !yield(mem, allocs) {
				return
			}
		}
	}
}

// perOp returns the memory (bytes) and allocations per operation, or zero when
// totalOps is zero.
func (m *MemoryStat) perOp(totalOps uint64) (memPerOp, allocsPerOp uint64) {
	if totalOps == 0 {
		return 0, 0
	}
	return m.GetMemory() / totalOps, m.GetAllocs() / totalOps
}
