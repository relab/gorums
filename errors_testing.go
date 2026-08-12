package gorums

import (
	"testing"

	"github.com/relab/gorums/internal/conn"
)

// TestQuorumCallError creates a QuorumCallError for testing.
// The nodeErrors map contains node IDs and their corresponding errors.
func TestQuorumCallError(_ testing.TB, nodeErrors map[uint32]error) QuorumCallError {
	errs := make([]conn.NodeError, 0, len(nodeErrors))
	for nodeID, err := range nodeErrors {
		errs = append(errs, conn.NewNodeError(nodeID, err))
	}
	return conn.NewQuorumCallError(ErrIncomplete, errs)
}
