package benchmark

import (
	fmt "fmt"

	gorums "github.com/relab/gorums"
	"google.golang.org/protobuf/proto"
)

// stopServerBenchmarkQF is the quorum function for the StopServerBenchmark quorumcall.
// It requires a response from all nodes.
func stopServerBenchmarkQF(replies gorums.Iterator[*Result], cfgSize int) (*Result, error) {
	// combine results, calculating averages and pooled variance
	resp := &Result_builder{}
	replyCount := 0
	for reply := range replies.IgnoreErrors() {
		replyCount++
		msg := reply.Msg
		if resp.Name != "" {
			resp.Name = msg.GetName()
		}
		resp.TotalOps += msg.GetTotalOps()
		resp.TotalTime += msg.GetTotalTime()
		resp.Throughput += msg.GetThroughput()
		resp.LatencyAvg += msg.GetLatencyAvg() * float64(msg.GetTotalOps())
		resp.LatencyVar += float64(msg.GetTotalOps()-1) * msg.GetLatencyVar()
		resp.ServerStats = append(resp.ServerStats, MemoryStat_builder{
			Allocs: msg.GetAllocsPerOp() * resp.TotalOps,
			Memory: msg.GetMemPerOp() * resp.TotalOps,
		}.Build())
	}

	if replyCount < cfgSize {
		return nil, fmt.Errorf("missing benchmark results got %d want %d", replyCount, cfgSize)
	}

	resp.LatencyAvg /= float64(resp.TotalOps)
	resp.LatencyVar /= float64(resp.TotalOps) - float64(replyCount)
	resp.TotalOps /= uint64(replyCount)
	resp.TotalTime /= int64(replyCount)
	resp.Throughput /= float64(replyCount)
	return resp.Build(), nil
}

// stopBenchmarkQF is the quorum function for the StopBenchmark quorumcall.
// It requires a response from all nodes.
func stopBenchmarkQF(replies gorums.Iterator[*MemoryStat], cfgSize int) ([]*MemoryStat, error) {
	replyList := make([]*MemoryStat, 0)
	replyCount := 0
	for reply := range replies.IgnoreErrors() {
		replyCount++
		replyList = append(replyList, reply.Msg)
		if len(replyList) < cfgSize {
			continue
		}
		return replyList, nil
	}
	return nil, fmt.Errorf("quorum not found, got %d responses want %d", replyCount, cfgSize)
}

// qf is a generic quorum function which does not return anything.
// It requires a response from a specified quorum amount of nodes.
func qf[responseType proto.Message](replies gorums.Iterator[responseType], quorum int) error {
	replyCount := 0
	for range replies.IgnoreErrors() {
		replyCount++
		if replyCount < quorum {
			continue
		}
		return nil
	}
	return fmt.Errorf("quorum not found, got %d responses want %d", replyCount, quorum)
}
