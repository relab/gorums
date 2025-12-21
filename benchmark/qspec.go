package benchmark

import (
	"github.com/relab/gorums"
)

// StopServerBenchmarkQF aggregates StopServerBenchmark responses from all nodes.
// It combines results, calculating averages and pooled variance.
func StopServerBenchmarkQF(replies map[uint32]*Result) (*Result, error) {
	if len(replies) == 0 {
		return nil, gorums.ErrIncomplete
	}
	// combine results, calculating averages and pooled variance
	resp := &Result{}
	for _, reply := range replies {
		if resp.GetName() != "" {
			resp.SetName(reply.GetName())
		}
		resp.SetTotalOps(resp.GetTotalOps() + reply.GetTotalOps())
		resp.SetTotalTime(resp.GetTotalTime() + reply.GetTotalTime())
		resp.SetThroughput(resp.GetThroughput() + reply.GetThroughput())
		resp.SetLatencyAvg(resp.GetLatencyAvg() + reply.GetLatencyAvg()*float64(reply.GetTotalOps()))
		resp.SetServerStats(append(resp.GetServerStats(), MemoryStat_builder{
			Allocs: reply.GetAllocsPerOp() * resp.GetTotalOps(),
			Memory: reply.GetMemPerOp() * resp.GetTotalOps(),
		}.Build()))
	}
	resp.SetLatencyAvg(resp.GetLatencyAvg() / float64(resp.GetTotalOps()))
	for _, reply := range replies {
		resp.SetLatencyVar(resp.GetLatencyVar() + float64(reply.GetTotalOps()-1)*reply.GetLatencyVar())
	}
	resp.SetLatencyVar(resp.GetLatencyVar() / (float64(resp.GetTotalOps()) - float64(len(replies))))
	resp.SetTotalOps(resp.GetTotalOps() / uint64(len(replies)))
	resp.SetTotalTime(resp.GetTotalTime() / int64(len(replies)))
	resp.SetThroughput(resp.GetThroughput() / float64(len(replies)))
	return resp, nil
}
