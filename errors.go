package gorums

import (
	"errors"
	"fmt"
)

// Incomplete is the error returned by a quorum call when the call cannot completed
// due insufficient non-error replies to form a quorum according to the quorum function.
var Incomplete = errors.New("incomplete call")

// QuorumCallError reports on a failed quorum call.
type QuorumCallError struct {
	cause error
}

// Is reports whether the target error is the same as the cause of the QuorumCallError.
func (e QuorumCallError) Is(target error) bool {
	if t, ok := target.(QuorumCallError); ok {
		return e.cause == t.cause
	}
	return e.cause == target
}

func (e QuorumCallError) Error() string {
	return fmt.Sprintf("quorum call error: %s", e.cause)
}

// nodeError reports on a failed RPC call.
type nodeError struct {
	cause  error
	nodeID uint32
}

func (e nodeError) Error() string {
	return fmt.Sprintf("node %d: %v", e.nodeID, e.cause)
}
