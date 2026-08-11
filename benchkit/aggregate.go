package benchkit

import (
	"maps"
	"slices"

	"github.com/relab/gorums"
)

// AppendServerStats attaches per-server memory statistics from Stop RPC replies
// to an existing result. It is used by client-measured benchmarks (QuorumCall,
// AsyncQuorumCall) where the client's latency samples are already in result but
// the server-side allocations must be collected separately.
func AppendServerStats(result *Result, replies map[uint32]*Result) {
	for _, id := range slices.Sorted(maps.Keys(replies)) {
		r := replies[id]
		result.SetServerStats(append(result.GetServerStats(), MemoryStat_builder{
			Allocs: r.GetAllocsPerOp() * r.GetTotalOps(),
			Memory: r.GetMemPerOp() * r.GetTotalOps(),
		}.Build()))
	}
}

// AggregateServerResults combines per-server Stop replies into a single
// cluster-wide Result. TotalOps and Throughput are summed across servers so
// the reported value reflects the cluster's aggregate work; TotalTime is the
// maximum across servers so it reflects the measurement window's wall-clock
// time rather than an N-fold sum. Latency samples from every reply are
// concatenated so that LatencyMean, LatencyMeanAndStdDev and Percentiles
// recompute from the full cluster-wide distribution. In StatsMode_HDR, where
// replies carry a histogram instead of raw samples, the per-server histograms
// are merged onto one canonical histogram (see [mergeHistograms]) instead.
//
// Per-server memory and alloc counters are attached as ServerStats in a stable
// node-ID order so the output columns do not reshuffle between runs.
func AggregateServerResults(replies map[uint32]*Result) (*Result, error) {
	if len(replies) == 0 {
		return nil, gorums.ErrIncomplete
	}

	resp := &Result{}
	var allSamples []int64
	var hists []*LatencyHistogram
	for _, id := range slices.Sorted(maps.Keys(replies)) {
		reply := replies[id]
		// The benchmark name lives in RunConfig and is stamped by Run on the
		// aggregated result; per-server replies carry no config.
		resp.SetTotalOps(resp.GetTotalOps() + reply.GetTotalOps())
		resp.SetTotalTime(max(resp.GetTotalTime(), reply.GetTotalTime()))
		resp.SetThroughput(resp.GetThroughput() + reply.GetThroughput())
		allSamples = append(allSamples, reply.GetLatencies()...)
		if h := reply.GetHistogram(); h != nil {
			hists = append(hists, h)
		}
		resp.SetServerStats(append(resp.GetServerStats(), MemoryStat_builder{
			Allocs: reply.GetAllocsPerOp() * reply.GetTotalOps(),
			Memory: reply.GetMemPerOp() * reply.GetTotalOps(),
		}.Build()))
	}
	if len(allSamples) > 0 {
		resp.SetLatencies(allSamples)
	}
	if len(hists) > 0 {
		resp.SetHistogram(mergeHistograms(hists...))
	}
	return resp, nil
}
