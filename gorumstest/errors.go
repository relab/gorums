package gorumstest

import (
	"maps"
	"slices"

	"github.com/relab/gorums"
	"github.com/relab/gorums/internal/conn"
)

// QuorumCallError returns a [gorums.QuorumCallError] with cause
// [gorums.ErrIncomplete] and one node error for each entry in nodeErrors.
// The node errors are in ascending node ID order.
func QuorumCallError(nodeErrors map[gorums.ID]error) gorums.QuorumCallError {
	errs := make([]conn.NodeError, 0, len(nodeErrors))
	for _, nodeID := range slices.Sorted(maps.Keys(nodeErrors)) {
		errs = append(errs, conn.NewNodeError(nodeID, nodeErrors[nodeID]))
	}
	return conn.NewQuorumCallError(gorums.ErrIncomplete, errs)
}
