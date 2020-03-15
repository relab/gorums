// Code generated by protoc-gen-gorums. DO NOT EDIT.

package dev

// QuorumSpec is the interface of quorum functions for ReaderService.
type QuorumSpec interface {

	// ReadQuorumCallQF is the quorum function for the ReadQuorumCall
	// quorum call method.
	ReadQuorumCallQF(replies []*ReadResponse) (*ReadResponse, bool)

	// ReadQuorumCallPerNodeArgQF is the quorum function for the ReadQuorumCallPerNodeArg
	// quorum call method.
	ReadQuorumCallPerNodeArgQF(replies []*ReadResponse) (*ReadResponse, bool)

	// ReadQuorumCallQFWithRequestArgQF is the quorum function for the ReadQuorumCallQFWithRequestArg
	// quorum call method.
	ReadQuorumCallQFWithRequestArgQF(in *ReadRequest, replies []*ReadResponse) (*ReadResponse, bool)

	// ReadQuorumCallCustomReturnTypeQF is the quorum function for the ReadQuorumCallCustomReturnType
	// quorum call method.
	ReadQuorumCallCustomReturnTypeQF(replies []*ReadResponse) (*MyReadResponse, bool)

	// ReadQuorumCallComboQF is the quorum function for the ReadQuorumCallCombo
	// quorum call method.
	ReadQuorumCallComboQF(in *ReadRequest, replies []*ReadResponse) (*MyReadResponse, bool)

	// ReadQuorumCallFutureQF is the quorum function for the ReadQuorumCallFuture
	// asynchronous quorum call method.
	ReadQuorumCallFutureQF(replies []*ReadResponse) (*ReadResponse, bool)

	// ReadCorrectableQF is the quorum function for the ReadCorrectable
	// correctable quorum call method.
	ReadCorrectableQF(replies []*ReadResponse) (*MyReadResponse, int, bool)

	// ReadCorrectableStreamQF is the quorum function for the ReadCorrectableStream
	// correctable stream quorum call method.
	ReadCorrectableStreamQF(replies []*ReadResponse) (*ReadResponse, int, bool)
}