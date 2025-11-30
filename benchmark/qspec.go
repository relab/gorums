package benchmark

import (
	"maps"
	"slices"

	"github.com/relab/gorums"
)

// qf returns a quorum function for the QuorumCall, AsyncQuorumCall, and SlowServer benchmarks.
// The quorum function collects responses from a quorum of nodes, and returns any one of them.
func qf(quorumSize int) gorums.QuorumFunc[*Echo, *Echo, *Echo] {
	return func(ctx *gorums.ClientCtx[*Echo, *Echo]) (*Echo, error) {
		replies := ctx.Responses().CollectN(quorumSize)
		if len(replies) < quorumSize {
			return nil, gorums.ErrIncomplete
		}
		var resp *Echo
		for _, r := range replies {
			resp = r
			break
		}
		return resp, nil
	}
}

// StartServerBenchmarkQF returns the quorum function for the StartServerBenchmark quorumcall.
// The quorum function collects responses from all nodes, and returns any one of them.
func StartServerBenchmarkQF(configSize int) gorums.QuorumFunc[*StartRequest, *StartResponse, *StartResponse] {
	return func(ctx *gorums.ClientCtx[*StartRequest, *StartResponse]) (*StartResponse, error) {
		replies := ctx.Responses().CollectN(configSize)
		if len(replies) < configSize {
			return nil, gorums.ErrIncomplete
		}
		// return any response
		var resp *StartResponse
		for _, r := range replies {
			resp = r
			break
		}
		return resp, nil
	}
}

// StopServerBenchmarkQF returns the quorum function for the StopServerBenchmark quorumcall.
// The quorum function collects responses from all nodes, and returns the combined results from all nodes.
func StopServerBenchmarkQF(configSize int) gorums.QuorumFunc[*StopRequest, *Result, *Result] {
	return func(ctx *gorums.ClientCtx[*StopRequest, *Result]) (*Result, error) {
		replies := ctx.Responses().CollectN(configSize)
		if len(replies) < configSize {
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
}

// StartBenchmarkQF returns the quorum function for the StartBenchmark quorumcall.
// The quorum function collects responses from all nodes, and returns any one of them.
func StartBenchmarkQF(configSize int) gorums.QuorumFunc[*StartRequest, *StartResponse, *StartResponse] {
	return func(ctx *gorums.ClientCtx[*StartRequest, *StartResponse]) (*StartResponse, error) {
		replies := ctx.Responses().CollectN(configSize)
		if len(replies) < configSize {
			return nil, gorums.ErrIncomplete
		}
		// return any response
		var resp *StartResponse
		for _, r := range replies {
			resp = r
			break
		}
		return resp, nil
	}
}

// StopBenchmarkQF returns the quorum function for the StopBenchmark quorumcall.
// The quorum function collects responses from all nodes, and returns the combined results from all nodes.
func StopBenchmarkQF(configSize int) gorums.QuorumFunc[*StopRequest, *MemoryStat, *MemoryStatList] {
	return func(ctx *gorums.ClientCtx[*StopRequest, *MemoryStat]) (*MemoryStatList, error) {
		replies := ctx.Responses().CollectN(configSize)
		if len(replies) < configSize {
			return nil, gorums.ErrIncomplete
		}
		return MemoryStatList_builder{
			MemoryStats: slices.Collect(maps.Values(replies)),
		}.Build(), nil
	}
}
