package dev

// QuorumSpec is the interface that wraps every quorum function.
type QuorumSpec interface {
	// ReadQF is the quorum function for the Read RPC method.
	ReadQF(replies []*State) (*State, bool)
	// WriteQF is the quorum function for the Write RPC method.
	WriteQF(replies []*WriteResponse) (*WriteResponse, bool)
}
