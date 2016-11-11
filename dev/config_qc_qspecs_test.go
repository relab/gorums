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

func (mqs *MajorityQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (mqs *MajorityQSpec) WriteQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
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

func (rqs *RegisterQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	panic("not implemented")
}

func (rqs *RegisterQSpec) WriteQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
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

func (rqs *RegisterByTimestampQSpec) WriteQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

type NeverQSpec struct{}

func NewNeverQSpec() qc.QuorumSpec {
	return &NeverQSpec{}
}

func (mqs *NeverQSpec) ReadQF(replies []*qc.State) (*qc.State, bool) {
	return nil, false
}

func (mqs *NeverQSpec) ReadCorrectableQF(replies []*qc.State) (*qc.State, int, bool) {
	return nil, qc.LevelNotSet, false
}

func (mqs *NeverQSpec) WriteQF(replies []*qc.WriteResponse) (*qc.WriteResponse, bool) {
	return nil, false
}
