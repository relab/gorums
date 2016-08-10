package dev

type QuorumSpec interface {
	ReadQF(replies []*State) (*State, bool)
	WriteQF(replies []*WriteResponse) (*WriteResponse, bool)
	IDs() []uint32
}
