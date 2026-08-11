package gorums

import (
	"errors"
	"fmt"
	"strings"

	"github.com/relab/gorums/internal/stream"
)

// ErrIncomplete is the error returned by a quorum call when the call cannot be completed
// due to insufficient non-error replies to form a quorum according to the quorum function.
var ErrIncomplete = errors.New("incomplete call")

// ErrSendFailure is the error returned by a multicast call when message sending fails for one or more nodes.
var ErrSendFailure = errors.New("send failure")

// ErrTypeMismatch is returned when a response cannot be cast to the expected type.
var ErrTypeMismatch = stream.ErrTypeMismatch

// ErrStreamDown is returned for a call that cannot be delivered or retried
// because the target node's stream is unavailable. It is a gRPC status error
// with the Unavailable code; match its identity with [errors.Is], including
// against a node error inside a [QuorumCallError].
var ErrStreamDown = stream.ErrStreamDown

// ErrNodeClosed is returned for a call enqueued after its node was closed. It
// is a gRPC status error with the Unavailable code; match it with [errors.Is].
var ErrNodeClosed = stream.ErrNodeClosed

// ErrSendQueueFull is returned for a two-way call enqueued while the node's
// send queue is at capacity: a full queue means the peer is not draining sends,
// so the call fails fast (letting quorum logic count the peer as failed) rather
// than block behind it. It is a gRPC status error with the Unavailable code;
// match it with [errors.Is]. See [WithSendBufferSize] for the capacity.
var ErrSendQueueFull = stream.ErrSendQueueFull

// ErrSkipNode is returned when a node is skipped by request transformations.
// This allows the response iterator to account for all nodes without blocking.
var ErrSkipNode = errors.New("skip node")

// ErrStopped is returned by [Server.WaitForPeers] and [Server.WaitForClients]
// when the server is stopped before the condition is met.
var ErrStopped = errors.New("system stopped")

// QuorumCallError reports on a failed quorum call.
// It provides detailed information about which nodes failed.
type QuorumCallError struct {
	cause  error
	errors []nodeError
}

// Cause returns the underlying cause of the quorum call failure.
// Common causes include ErrIncomplete and ErrSendFailure.
func (e QuorumCallError) Cause() error {
	return e.cause
}

// NodeErrors returns the number of nodes that failed during the quorum call.
func (e QuorumCallError) NodeErrors() int {
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

// nodeError reports on a failed RPC call from a specific node.
type nodeError struct {
	cause  error
	nodeID uint32
}

func (e nodeError) Error() string {
	return fmt.Sprintf("node %d: %v", e.nodeID, e.cause)
}
