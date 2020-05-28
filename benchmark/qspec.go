package benchmark

// QSpec is the quroum specification object for the benchmark
type QSpec struct {
	QSize int
}

func (qspec *QSpec) qf(replies []*Echo) (*Echo, bool) {
	if len(replies) < qspec.QSize {
		return nil, false
	}
	return replies[0], true
}

// UnorderedQCQF is the quorum function for the UnorderedQC quorumcall
func (qspec *QSpec) UnorderedQCQF(_ *Echo, replies []*Echo) (*Echo, bool) {
	return qspec.qf(replies)
}

// OrderedQCQF is the quorum function for the OrderedQC quorumcall
func (qspec *QSpec) OrderedQCQF(_ *Echo, replies []*Echo) (*Echo, bool) {
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
