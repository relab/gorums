package benchkit

import (
	"fmt"
	"maps"
	"slices"

	"github.com/relab/gorums"
)

// addServerStats appends the memory statistics of each server's Stop reply
// to x, in node ID order, and adds the servers' named metrics, merged by name,
// to the metrics of x. Client-measured benchmarks, such as QuorumCall and
// AsyncQuorumCall, use it: x already holds the client's latency samples, and
// each server reports its allocations in its own reply. It returns an error
// when the servers' metrics do not merge, when a server reports a
// server-measured series, which a client-measured run has no clock offsets to
// correct, or when the client and a server record a metric of the same name.
func (x *Result) addServerStats(replies map[uint32]*Result) error {
	var servers metricMerger
	for _, id := range slices.Sorted(maps.Keys(replies)) {
		x.SetServerStats(append(x.GetServerStats(), serverMemoryStat(replies[id])))
		if err := servers.merge(replies[id].GetMetrics()); err != nil {
			return fmt.Errorf("node %d: %w", id, err)
		}
	}
	merged := servers.sorted()
	if err := serverMeasuredMetric(merged, "a client-measured run"); err != nil {
		return err
	}
	set := newMetricSet(x.GetMetrics())
	if err := set.union(merged); err != nil {
		return err
	}
	x.SetMetrics(set.sorted())
	return nil
}

// serverMemoryStat returns the total allocations and bytes allocated by the
// server that sent reply, derived from its per-op counters.
func serverMemoryStat(reply *Result) *MemoryStat {
	return MemoryStat_builder{
		Allocs: reply.GetAllocsPerOp() * reply.GetTotalOps(),
		Memory: reply.GetMemPerOp() * reply.GetTotalOps(),
	}.Build()
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
// Corrected latency events with a shared bucket end are combined using their
// sample counts, means, and sample variances. Replies must use the same time
// origin and interval width, as arranged by [Measurement.TrackLatency].
//
// Named metrics merge by name: series concatenate their samples or merge their
// histograms, and counters add their sums and node counts and keep the larger
// max. It returns an error naming the node and the metric when a metric's
// unit, kind, measurement mode, or stats mode differs between replies.
func AggregateServerResults(replies map[uint32]*Result) (*Result, error) {
	if len(replies) == 0 {
		return nil, gorums.ErrIncomplete
	}

	resp := &Result{}
	var latencies distMerger
	var latencyEvents []*Event
	var metrics metricMerger
	for _, id := range slices.Sorted(maps.Keys(replies)) {
		reply := replies[id]
		// The benchmark name lives in RunConfig and is stamped by Run on the
		// aggregated result; per-server replies carry no config.
		resp.SetTotalOps(resp.GetTotalOps() + reply.GetTotalOps())
		resp.SetTotalTime(max(resp.GetTotalTime(), reply.GetTotalTime()))
		resp.SetThroughput(resp.GetThroughput() + reply.GetThroughput())
		latencies.add(reply.GetLatencies(), reply.GetHistogram())
		latencyEvents = append(latencyEvents, reply.GetEvents()...)
		resp.SetServerStats(append(resp.GetServerStats(), serverMemoryStat(reply)))
		if err := metrics.merge(reply.GetMetrics()); err != nil {
			return nil, fmt.Errorf("node %d: %w", id, err)
		}
	}
	samples, hist := latencies.dist()
	if samples != nil {
		resp.SetLatencies(samples)
	}
	resp.SetHistogram(hist)
	resp.SetEvents(mergeLatencyEvents(latencyEvents))
	resp.SetMetrics(metrics.sorted())
	return resp, nil
}
