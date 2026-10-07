package gorums

import (
	"net"
	"testing"

	"github.com/relab/gorums/internal/conn"
)

// ServerIface is implemented by servers supported by the test helpers.
//
// Package [github.com/relab/gorums/internal/testutils/servers] declares a
// structurally identical interface, since it does not import gorums; a value
// satisfying either satisfies both.
type ServerIface interface {
	Serve(net.Listener) error
	Stop()
}

// TestQuorumCallError creates a QuorumCallError for testing.
// The nodeErrors map contains node IDs and their corresponding errors.
func TestQuorumCallError(_ testing.TB, nodeErrors map[uint32]error) QuorumCallError {
	errs := make([]conn.NodeError, 0, len(nodeErrors))
	for nodeID, err := range nodeErrors {
		errs = append(errs, conn.NewNodeError(nodeID, err))
	}
	return conn.NewQuorumCallError(ErrIncomplete, errs)
}
