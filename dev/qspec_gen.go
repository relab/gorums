package dev

// QuorumSpec is the interface that wraps every quorum function in addition to a
// general method for querying the IDs of configuration/quorum spec-combination.
// All interface methods except IDs are generated for each specific use case.
type QuorumSpec interface {
	// ReadQF is the quorum function for the Read RPC method.
	ReadQF(replies []*State) (*State, bool)
	// WriteQF is the quorum function for the Write RPC method.
	WriteQF(replies []*WriteResponse) (*WriteResponse, bool)
	// IDs returns a slice containing the Gorums identifiers for every node
	// in the configuration for the quorum specification.
	IDs() []uint32
}
