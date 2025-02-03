package benchmark

// QSpec is the quorum specification object for the benchmark
type QSpec struct {
	CfgSize int
	QSize   int
}

func (qspec *QSpec) qf(replies map[uint32]*Echo) (*Echo, bool) {
	if len(replies) < qspec.QSize {
		return nil, false
	}
	var resp *Echo
	for _, r := range replies {
		resp = r
		break
	}
	return resp, true
}

// StartServerBenchmarkQF is the quorum function for the StartServerBenchmark quorumcall.
// It requires a response from all nodes.
func (qspec *QSpec) StartServerBenchmarkQF(_ *StartRequest, replies map[uint32]*StartResponse) (*StartResponse, bool) {
	if len(replies) < qspec.CfgSize {
		return nil, false
	}
	// return any response
	var resp *StartResponse
	for _, r := range replies {
		resp = r
		break
	}
	return resp, true
}

// StopServerBenchmarkQF is the quorum function for the StopServerBenchmark quorumcall.
// It requires a response from all nodes.
func (qspec *QSpec) StopServerBenchmarkQF(_ *StopRequest, replies map[uint32]*Result) (*Result, bool) {
	if len(replies) < qspec.CfgSize {
		return nil, false
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
	return resp, true
}

// StartBenchmarkQF is the quorum function for the StartBenchmark quorumcall.
// It requires a response from all nodes.
func (qspec *QSpec) StartBenchmarkQF(_ *StartRequest, replies map[uint32]*StartResponse) (*StartResponse, bool) {
	if len(replies) < qspec.CfgSize {
		return nil, false
	}
	// return any response
	var resp *StartResponse
	for _, r := range replies {
		resp = r
		break
	}
	return resp, true
}

// StopBenchmarkQF is the quorum function for the StopBenchmark quorumcall.
// It requires a response from all nodes.
func (qspec *QSpec) StopBenchmarkQF(_ *StopRequest, replies map[uint32]*MemoryStat) (*MemoryStatList, bool) {
	if len(replies) < qspec.CfgSize {
		return nil, false
	}
	replyList := make([]*MemoryStat, 0, len(replies))
	for _, v := range replies {
		replyList = append(replyList, v)
	}
	return MemoryStatList_builder{MemoryStats: replyList}.Build(), true
}

// QuorumCallQF is the quorum function for the QuorumCall quorumcall
func (qspec *QSpec) QuorumCallQF(_ *Echo, replies map[uint32]*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// AsyncQuorumCallQF is the quorum function for the AsyncQuorumCall quorumcall
func (qspec *QSpec) AsyncQuorumCallQF(_ *Echo, replies map[uint32]*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// SlowServerQF is the quorum function for the SlowServer quorumcall
func (qspec *QSpec) SlowServerQF(_ *Echo, replies map[uint32]*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}
