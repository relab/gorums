package gorums

import (
	"cmp"
	"errors"
	"fmt"
	"strings"
)

// Incomplete is the error returned by a quorum call when the call cannot completed
// due insufficient non-error replies to form a quorum according to the quorum function.
var Incomplete = errors.New("incomplete call")

// QuorumCallError reports on a failed quorum call.
type QuorumCallError[idType cmp.Ordered] struct {
	cause   error
	errors  []nodeError[idType]
	replies int
}

// Is reports whether the target error is the same as the cause of the QuorumCallError.
func (e QuorumCallError[idType]) Is(target error) bool {
	if t, ok := target.(QuorumCallError[idType]); ok {
		return e.cause == t.cause
	}
	return e.cause == target
}

func (e QuorumCallError[idType]) Error() string {
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
type nodeError[idType cmp.Ordered] struct {
	cause  error
	nodeID idType
}

func (e nodeError[idType]) Error() string {
	return fmt.Sprintf("node %v: %v", e.nodeID, e.cause)
}
