package gorums

import (
	"bytes"
	"fmt"
)

// ConfigCreationError returns an error reporting that a Configuration
// could not be created due to err.
func ConfigCreationError(err error) error {
	return fmt.Errorf("could not create configuration: %s", err.Error())
}

// A QuorumCallError is used to report that a quorum call failed.
type QuorumCallError struct {
	Reason     string
	ReplyCount int
	Errors     []Error
}

func (e QuorumCallError) Error() string {
	var b bytes.Buffer
	b.WriteString("quorum call error: ")
	b.WriteString(e.Reason)
	b.WriteString(fmt.Sprintf(" (errors: %d, replies: %d)", len(e.Errors), e.ReplyCount))
	if len(e.Errors) == 0 {
		return b.String()
	}
	b.WriteString("\ngrpc errors:\n")
	for _, err := range e.Errors {
		b.WriteByte('\t')
		b.WriteString(fmt.Sprintf("node %d: %v", err.NodeID, err.Cause))
		b.WriteByte('\n')
	}
	return b.String()
}

// Error is used to report that a single gRPC call failed.
type Error struct {
	NodeID uint32
	Cause  error
}

func (e Error) Error() string {
	return fmt.Sprintf("node %d: %v", e.NodeID, e.Cause.Error())
}
