package dev_test

import (
	"sort"

	qc "github.com/relab/gorums/dev"
)

type MajorityQSpec struct {
	q int
}

func NewMajorityQSpec(n int) qc.QuorumSpec {
	return &MajorityQSpec{q: n/2 + 1}
}

func (mqs *MajorityQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

func (mqs *MajorityQSpec) ReadFutureQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

func (mqs *MajorityQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.MyState, bool) {
	state, ok := mqs.ReadQF(replies)
	if !ok {
		return nil, false
	}
	myState := &qc.MyState{
		Value:     state.Value,
		Timestamp: state.Timestamp,
		Extra:     123,
	}
	return myState, ok
}

func (mqs *MajorityQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (mqs *MajorityQSpec) ReadPrelimQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (mqs *MajorityQSpec) WriteQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

func (mqs *MajorityQSpec) WriteFutureQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

func (mqs *MajorityQSpec) WritePerNodeQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

type StorageQSpec struct {
	rq, wq int
}

func NewStorageQSpec(rq, wq int) qc.QuorumSpec {
	return &StorageQSpec{
		rq: rq,
		wq: wq,
	}
}

func (sqs *StorageQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < sqs.rq {
		return nil, false
	}
	return replies[0], true
}

func (sqs *StorageQSpec) ReadFutureQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < sqs.rq {
		return nil, false
	}
	return replies[0], true
}

func (sqs *StorageQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.MyState, bool) {
	state, ok := sqs.ReadQF(replies)
	if !ok {
		return nil, false
	}
	myState := &qc.MyState{
		Value:     state.Value,
		Timestamp: state.Timestamp,
		Extra:     123,
	}
	return myState, ok
}

func (sqs *StorageQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (sqs *StorageQSpec) ReadPrelimQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (sqs *StorageQSpec) WriteQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < sqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (sqs *StorageQSpec) WriteFutureQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < sqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (sqs *StorageQSpec) WritePerNodeQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < sqs.wq {
		return nil, false
	}
	return replies[0], true
}

const (
	LevelWeak   = 1
	LevelStrong = 2
)

type StorageByTimestampQSpec struct {
	rq, wq int
}

func NewStorageByTimestampQSpec(rq, wq int) qc.QuorumSpec {
	return &StorageByTimestampQSpec{
		rq: rq,
		wq: wq,
	}
}

func (sqs *StorageByTimestampQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < sqs.rq {
		return nil, false
	}
	sort.Sort(ByTimestamp(replies))
	return replies[len(replies)-1], true
}

func (sqs *StorageByTimestampQSpec) ReadFutureQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < sqs.rq {
		return nil, false
	}
	sort.Sort(ByTimestamp(replies))
	return replies[len(replies)-1], true
}

func (sqs *StorageByTimestampQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.MyState, bool) {
	state, ok := sqs.ReadQF(replies)
	if !ok {
		return nil, false
	}
	myState := &qc.MyState{
		Value:     state.Value,
		Timestamp: state.Timestamp,
		Extra:     123,
	}
	return myState, ok
}

func (sqs *StorageByTimestampQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	if len(replies) == 0 {
		return nil, qc.LevelNotSet, false
	}
	sort.Sort(ByTimestamp(replies))
	if len(replies) < sqs.rq {
		return replies[len(replies)-1], LevelWeak, false
	}
	return replies[len(replies)-1], LevelStrong, true
}

func (sqs *StorageByTimestampQSpec) ReadPrelimQF(replies []*qc.State) (*qc.State, int, bool) {
	if len(replies) == 0 {
		return nil, qc.LevelNotSet, false
	}
	sort.Sort(ByTimestamp(replies))
	if len(replies) < sqs.rq {
		return replies[len(replies)-1], LevelWeak, false
	}
	return replies[len(replies)-1], LevelStrong, true
}

func (sqs *StorageByTimestampQSpec) WriteQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < sqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (sqs *StorageByTimestampQSpec) WriteFutureQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < sqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (sqs *StorageByTimestampQSpec) WritePerNodeQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < sqs.wq {
		return nil, false
	}
	// Note: This quorum function is designed to tolerate nil values, since some nodes
	// may not actually be invoked, but simply ignored with a nil from the perNodeArg function.
	for _, reply := range replies {
		if reply != nil {
			return reply, true
		}
	}
	return nil, false
}

type NeverQSpec struct{}

func (*NeverQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	return nil, false
}

func (*NeverQSpec) ReadFutureQF(replies []*qc.State) (*qc.State, bool) {
	return nil, false
}

func (*NeverQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.MyState, bool) {
	return nil, false
}

func (*NeverQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	return nil, qc.LevelNotSet, false
}

func (*NeverQSpec) ReadPrelimQF(replies []*qc.State) (*qc.State, int, bool) {
	return nil, qc.LevelNotSet, false
}

func (*NeverQSpec) WriteQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	return nil, false
}

func (*NeverQSpec) WriteFutureQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	return nil, false
}

func (*NeverQSpec) WritePerNodeQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	return nil, false
}

type ReadPrelimTestQSpec struct{}

func (*ReadPrelimTestQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	panic("not implemented")
}

func (*ReadPrelimTestQSpec) ReadFutureQF(replies []*qc.State) (*qc.State, bool) {
	panic("not implemented")
}

func (*ReadPrelimTestQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.MyState, bool) {
	panic("not implemented")
}

func (*ReadPrelimTestQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (*ReadPrelimTestQSpec) ReadPrelimQF(replies []*qc.State) (*qc.State, int, bool) {
	switch len(replies) {
	case 0:
		return nil, qc.LevelNotSet, false
	case 1:
		return replies[len(replies)-1], 1, false
	case 2:
		return replies[len(replies)-1], 2, false
	case 3:
		return replies[len(replies)-1], 3, false
	case 4:
		return replies[len(replies)-1], 4, true
	default:
		return replies[len(replies)-1], 42, true
	}
}

func (*ReadPrelimTestQSpec) WriteQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	panic("not implemented")
}

func (*ReadPrelimTestQSpec) WriteFutureQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	panic("not implemented")
}

func (*ReadPrelimTestQSpec) WritePerNodeQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	panic("not implemented")
}
