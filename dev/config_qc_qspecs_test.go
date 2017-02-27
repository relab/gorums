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

func (mqs *MajorityQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.State, bool) {
	return mqs.ReadQF(replies)
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

type RegisterQSpec struct {
	rq, wq int
}

func NewRegisterQSpec(rq, wq int) qc.QuorumSpec {
	return &RegisterQSpec{
		rq: rq,
		wq: wq,
	}
}

func (rqs *RegisterQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterQSpec) ReadFutureQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.State, bool) {
	return rqs.ReadQF(replies)
}

func (rqs *RegisterQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (rqs *RegisterQSpec) ReadPrelimQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (rqs *RegisterQSpec) WriteQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterQSpec) WriteFutureQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterQSpec) WritePerNodeQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

const (
	LevelWeak   = 1
	LevelStrong = 2
)

type RegisterByTimestampQSpec struct {
	rq, wq int
}

func NewRegisterByTimestampQSpec(rq, wq int) qc.QuorumSpec {
	return &RegisterByTimestampQSpec{
		rq: rq,
		wq: wq,
	}
}

func (rqs *RegisterByTimestampQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	sort.Sort(ByTimestamp(replies))
	return replies[len(replies)-1], true
}

func (rqs *RegisterByTimestampQSpec) ReadFutureQF(replies []*qc.State) (*qc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	sort.Sort(ByTimestamp(replies))
	return replies[len(replies)-1], true
}

func (rqs *RegisterByTimestampQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.State, bool) {
	return rqs.ReadQF(replies)
}

func (rqs *RegisterByTimestampQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	if len(replies) == 0 {
		return nil, qc.LevelNotSet, false
	}
	sort.Sort(ByTimestamp(replies))
	if len(replies) < rqs.rq {
		return replies[len(replies)-1], LevelWeak, false
	}
	return replies[len(replies)-1], LevelStrong, true
}

func (rqs *RegisterByTimestampQSpec) ReadPrelimQF(replies []*qc.State) (*qc.State, int, bool) {
	if len(replies) == 0 {
		return nil, qc.LevelNotSet, false
	}
	sort.Sort(ByTimestamp(replies))
	if len(replies) < rqs.rq {
		return replies[len(replies)-1], LevelWeak, false
	}
	return replies[len(replies)-1], LevelStrong, true
}

func (rqs *RegisterByTimestampQSpec) WriteQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterByTimestampQSpec) WriteFutureQF(req *qc.State, replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterByTimestampQSpec) WritePerNodeQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

type NeverQSpec struct{}

func (*NeverQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	return nil, false
}

func (*NeverQSpec) ReadFutureQF(replies []*qc.State) (*qc.State, bool) {
	return nil, false
}

func (*NeverQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.State, bool) {
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

func (*ReadPrelimTestQSpec) ReadCustomReturnQF(replies []*qc.State) (*qc.State, bool) {
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
