package gorums

import (
	"errors"
	"fmt"
	"strings"
)

// ErrIncomplete is the error returned by a quorum call when the call cannot be completed
// due to insufficient non-error replies to form a quorum according to the quorum function.
var ErrIncomplete = errors.New("incomplete call")

// ErrTypeMismatch is returned when a response cannot be cast to the expected type.
var ErrTypeMismatch = errors.New("response type mismatch")

// ErrSendFailure is the error returned by a multicast call when message sending fails for one or more nodes.
var ErrSendFailure = errors.New("send failure")

// QuorumCallError reports on a failed quorum call.
type QuorumCallError struct {
	cause   error
	errors  []nodeError
	replies int
}

// Is reports whether the target error is the same as the cause of the QuorumCallError.
func (e QuorumCallError) Is(target error) bool {
	if t, ok := target.(QuorumCallError); ok {
		return e.cause == t.cause
	}
	return e.cause == target
}

func (e QuorumCallError) Error() string {
	s := fmt.Sprintf("quorum call error: %s (errors: %d, replies: %d)", e.cause, len(e.errors), e.replies)
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

// nodeError reports on a failed RPC call.
type nodeError struct {
	cause  error
	nodeID uint32
}

func (e nodeError) Error() string {
	return fmt.Sprintf("node %d: %v", e.nodeID, e.cause)
}
