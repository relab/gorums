package gorums

import "testing"

// TestQuorumCallError creates a QuorumCallError for testing.
// The nodeErrors map contains node IDs and their corresponding errors.
func TestQuorumCallError(_ testing.TB, nodeErrors map[uint32]error) QuorumCallError {
	errs := make([]nodeError, 0, len(nodeErrors))
	for nodeID, err := range nodeErrors {
		errs = append(errs, nodeError{cause: err, nodeID: nodeID})
	}
	return QuorumCallError{cause: ErrIncomplete, errors: errs}
}
