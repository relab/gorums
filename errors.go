package gorums

import (
	"fmt"
	"strings"
)

// QuorumCallError reports on a failed quorum call.
type QuorumCallError struct {
	Reason  string
	errors  []nodeError
	replies int
}

func (e QuorumCallError) Error() string {
	s := fmt.Sprintf("quorum call error: %s (errors: %d, replies: %d)", e.Reason, len(e.errors), e.replies)
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
