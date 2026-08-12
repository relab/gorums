package conn

import "github.com/relab/gorums/internal/stream"

// This file collects exported constructors that exist only to support tests in
// other packages (package conn's own tests use unexported helpers directly).
// They live in a non-test file because Go test files are not importable across
// packages; keeping them here separates them from production code.

// NewNodeForTest builds a node with the given transport and no owning manager,
// for tests in other packages that assemble a [Config] by hand. This function
// should only be used in tests.
func NewNodeForTest(id uint32, transport *stream.Transport) *Node {
	return newNode(id, "", nil, transport)
}
