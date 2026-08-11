package conn

import (
	"errors"
	"fmt"
	"strings"
)

// ErrStopped is returned by the server wait methods ([InboundManager.WaitForPeers],
// [InboundManager.WaitForClients]) when the server is stopped before the
// condition is met.
var ErrStopped = errors.New("server stopped")

// QuorumCallError reports on a failed quorum call.
// It provides detailed information about which nodes failed.
type QuorumCallError struct {
	cause  error
	errors []NodeError
}

// NewQuorumCallError builds a [QuorumCallError] from an overall cause and the
// per-node errors that contributed to it. It is used by the call engine.
func NewQuorumCallError(cause error, errs []NodeError) QuorumCallError {
	return QuorumCallError{cause: cause, errors: errs}
}

// Cause returns the underlying cause of the quorum call failure.
func (e QuorumCallError) Cause() error {
	return e.cause
}

// NumErrors returns the number of nodes that failed during the quorum call.
func (e QuorumCallError) NumErrors() int {
	return len(e.errors)
}

// Is reports whether the target error is the same as the cause of the QuorumCallError.
func (e QuorumCallError) Is(target error) bool {
	if t, ok := target.(QuorumCallError); ok {
		return e.cause == t.cause
	}
	return e.cause == target
}

// Unwrap returns all the underlying node errors as a slice.
// This allows the error to work with errors.Is and errors.As for any wrapped errors.
func (e QuorumCallError) Unwrap() (errs []error) {
	for _, ne := range e.errors {
		errs = append(errs, ne.cause)
	}
	return errs
}

func (e QuorumCallError) Error() string {
	s := fmt.Sprintf("quorum call error: %s (errors: %d)", e.cause, len(e.errors))
	var b strings.Builder
	b.WriteString(s)
	if len(e.errors) == 0 {
		return b.String()
	}
	b.WriteString("\nnode errors:\n")
	for _, err := range e.errors {
		b.WriteByte('\t')
		b.WriteString(err.Error())
		b.WriteByte('\n')
	}
	return b.String()
}

// NodeError reports on a failed RPC call from a specific node.
type NodeError struct {
	cause  error
	nodeID uint32
}

// NewNodeError builds a [NodeError] for the given node ID and cause. It is used
// by the call engine to record per-node failures in a [QuorumCallError].
func NewNodeError(nodeID uint32, cause error) NodeError {
	return NodeError{cause: cause, nodeID: nodeID}
}

func (e NodeError) Error() string {
	return fmt.Sprintf("node %d: %v", e.nodeID, e.cause)
}
