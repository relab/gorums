package dev_test

import (
	"sort"

	rpc "github.com/relab/gorums/dev"
)

type MajorityQSpec struct {
	q int
}

func NewMajorityQSpec(n int) rpc.QuorumSpec {
	return &MajorityQSpec{q: n/2 + 1}
}

func (mqs *MajorityQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

func (mqs *MajorityQSpec) ReadCorrectableQF(replies []*rpc.State) (*rpc.State, int, bool) {
	panic("not implemented")
}

func (mqs *MajorityQSpec) WriteQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

type RegisterQSpec struct {
	rq, wq int
}

func NewRegisterQSpec(rq, wq int) rpc.QuorumSpec {
	return &RegisterQSpec{
		rq: rq,
		wq: wq,
	}
}

func (rqs *RegisterQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterQSpec) ReadCorrectableQF(replies []*rpc.State) (*rpc.State, int, bool) {
	panic("not implemented")
}

func (rqs *RegisterQSpec) WriteQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
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

func NewRegisterByTimestampQSpec(rq, wq int) rpc.QuorumSpec {
	return &RegisterByTimestampQSpec{
		rq: rq,
		wq: wq,
	}
}

func (rqs *RegisterByTimestampQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	sort.Sort(ByTimestamp(replies))
	return replies[len(replies)-1], true
}

func (rqs *RegisterByTimestampQSpec) ReadCorrectableQF(replies []*rpc.State) (*rpc.State, int, bool) {
	if len(replies) == 0 {
		return nil, rpc.LevelNotSet, false
	}
	sort.Sort(ByTimestamp(replies))
	if len(replies) < rqs.rq {
		return replies[len(replies)-1], LevelWeak, false
	}
	return replies[len(replies)-1], LevelStrong, true
}

func (rqs *RegisterByTimestampQSpec) WriteQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

type NeverQSpec struct{}

func NewNeverQSpec() rpc.QuorumSpec {
	return &NeverQSpec{}
}

func (mqs *NeverQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	return nil, false
}

func (mqs *NeverQSpec) ReadCorrectableQF(replies []*rpc.State) (*rpc.State, int, bool) {
	return nil, rpc.LevelNotSet, false
}

func (mqs *NeverQSpec) WriteQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	return nil, false
}
