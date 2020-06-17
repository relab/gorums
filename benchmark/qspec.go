package benchmark

// QSpec is the quorum specification object for the benchmark
type QSpec struct {
	CfgSize int
	QSize   int
}

func (qspec *QSpec) qf(replies []*Echo) (*Echo, bool) {
	if len(replies) < qspec.QSize {
		return nil, false
	}
	return replies[0], true
}

// StartServerBenchmarkQF is the quorum function for the StartServerBenchmark quorumcall.
// It requires a response from all nodes.
func (qspec *QSpec) StartServerBenchmarkQF(_ *StartRequest, replies []*StartResponse) (*StartResponse, bool) {
	if len(replies) < qspec.CfgSize {
		return nil, false
	}
	return replies[0], true
}

// StopServerBenchmarkQF is the quorum function for the StopServerBenchmark quorumcall.
// It requires a response from all nodes.
func (qspec *QSpec) StopServerBenchmarkQF(_ *StopRequest, replies []*Result) (*Result, bool) {
	if len(replies) < qspec.CfgSize {
		return nil, false
	}
	// combine results, calculating averages and pooled variance
	resp := &Result{Name: replies[0].Name}
	for _, reply := range replies {
		resp.TotalOps += reply.TotalOps
		resp.TotalTime += reply.TotalTime
		resp.Throughput += reply.Throughput
		resp.LatencyAvg += reply.LatencyAvg * float64(reply.TotalOps)
		resp.ServerStats = append(resp.ServerStats, &MemoryStat{
			Allocs: reply.AllocsPerOp * resp.TotalOps,
			Memory: reply.MemPerOp * resp.TotalOps,
		})
	}
	resp.LatencyAvg /= float64(resp.TotalOps)
	for _, reply := range replies {
		resp.LatencyVar += float64(reply.TotalOps-1) * reply.LatencyVar
	}
	resp.LatencyVar /= float64(resp.TotalOps) - float64(len(replies))
	resp.TotalOps /= uint64(len(replies))
	resp.TotalTime /= int64(len(replies))
	resp.Throughput /= float64(len(replies))
	return resp, true
}

// StartBenchmarkQF is the quorum function for the StartBenchmark quorumcall.
// It requires a response from all nodes.
func (qspec *QSpec) StartBenchmarkQF(_ *StartRequest, replies []*StartResponse) (*StartResponse, bool) {
	if len(replies) < qspec.CfgSize {
		return nil, false
	}
	return replies[0], true
}

// StopBenchmarkQF is the quorum function for the StopBenchmark quorumcall.
// It requires a response from all nodes.
func (qspec *QSpec) StopBenchmarkQF(_ *StopRequest, replies []*MemoryStat) (*MemoryStatList, bool) {
	if len(replies) < qspec.CfgSize {
		return nil, false
	}
	return &MemoryStatList{MemoryStats: replies}, true
}

// UnorderedQCQF is the quorum function for the UnorderedQC quorumcall
func (qspec *QSpec) UnorderedQCQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// OrderedQCQF is the quorum function for the OrderedQC quorumcall
func (qspec *QSpec) OrderedQCQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// ConcurrentQCQF is the quorum function for the ConcurrentQC quorumcall
func (qspec *QSpec) ConcurrentQCQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// UnorderedAsyncQF is the quorum function for the UnorderedAsync quorumcall
func (qspec *QSpec) UnorderedAsyncQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// OrderedAsyncQF is the quorum function for the OrderedAsync quorumcall
func (qspec *QSpec) OrderedAsyncQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// ConcurrentAsyncQF is the quorum function for the ConcurrentAsync quorumcall
func (qspec *QSpec) ConcurrentAsyncQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// UnorderedSlowServerQF is the quorum function for the UnorderedSlowServer quorumcall
func (qspec *QSpec) UnorderedSlowServerQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// OrderedSlowServerQF is the quorum function for the OrderedSlowServer quorumcall
func (qspec *QSpec) OrderedSlowServerQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// ConcurrentSlowServerQF is the quorum function for the ConcurrentSlowServer quorumcall
func (qspec *QSpec) ConcurrentSlowServerQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}
