package dev_test

import (
	"sort"

	rpc "github.com/relab/gorums/dev"
)

type MajorityQSpec struct {
	ids []uint32
	q   int
}

func NewMajorityQSpec(ids []uint32) *MajorityQSpec {
	return &MajorityQSpec{
		ids: ids,
		q:   len(ids)/2 + 1,
	}
}

func (mqs *MajorityQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

func (mqs *MajorityQSpec) WriteQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	if len(replies) < mqs.q {
		return nil, false
	}
	return replies[0], true
}

func (mqs *MajorityQSpec) IDs() []uint32 {
	return mqs.ids
}

type RegisterQSpec struct {
	ids    []uint32
	rq, wq int
}

func NewRegisterQSpec(ids []uint32, rq, wq int) *RegisterQSpec {
	return &RegisterQSpec{
		ids: ids,
		rq:  rq,
		wq:  wq,
	}
}

func (rqs *RegisterQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterQSpec) WriteQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterQSpec) IDs() []uint32 {
	return rqs.ids
}

type RegisterByTimestampQSpec struct {
	ids    []uint32
	rq, wq int
}

func NewRegisterByTimestampQSpec(ids []uint32, rq, wq int) *RegisterByTimestampQSpec {
	return &RegisterByTimestampQSpec{
		ids: ids,
		rq:  rq,
		wq:  wq,
	}
}

func (rqs *RegisterByTimestampQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	if len(replies) < rqs.rq {
		return nil, false
	}
	sort.Sort(ByTimestamp(replies))
	return replies[len(replies)-1], true
}

func (rqs *RegisterByTimestampQSpec) WriteQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	if len(replies) < rqs.wq {
		return nil, false
	}
	return replies[0], true
}

func (rqs *RegisterByTimestampQSpec) IDs() []uint32 {
	return rqs.ids
}

type NeverQSpec struct {
	ids []uint32
}

func NewNeverQSpec(ids []uint32) *NeverQSpec {
	return &NeverQSpec{ids: ids}
}
func (mqs *NeverQSpec) WriteQF(replies []*rpc.WriteResponse) (*rpc.WriteResponse, bool) {
	return nil, false
}

func (mqs *NeverQSpec) ReadQF(replies []*rpc.State) (*rpc.State, bool) {
	return nil, false
}

func (mqs *NeverQSpec) IDs() []uint32 {
	return mqs.ids
}
