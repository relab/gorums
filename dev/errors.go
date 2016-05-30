package dev

import (
	"fmt"
	"time"
)

// A NodeNotFoundError reports that a specified node could not be found.
type NodeNotFoundError uint32

func (e NodeNotFoundError) Error() string {
	return fmt.Sprintf("node not found: %d", e)
}

// A ConfigNotFoundError reports that a specified configuration could not be
// found.
type ConfigNotFoundError uint32

func (e ConfigNotFoundError) Error() string {
	return fmt.Sprintf("configuration not found: %d", e)
}

// An IncompleteRPCError reports that a quorum RPC call failed.
type IncompleteRPCError struct {
	ErrCount, ReplyCount int
}

func (e IncompleteRPCError) Error() string {
	return fmt.Sprintf(
		"incomplete rpc (errors: %d, replies: %d)",
		e.ErrCount, e.ReplyCount,
	)
}

// An TimeoutRPCError reports that a quorum RPC call timed out.
type TimeoutRPCError struct {
	Waited                 time.Duration
	ErrCount, RepliesCount int
}

func (e TimeoutRPCError) Error() string {
	return fmt.Sprintf(
		"rpc timed out: waited %v (errors: %d, replies: %d)",
		e.Waited, e.ErrCount, e.RepliesCount,
	)
}

// An IllegalConfigError reports that a specified configuration could not be
// created.
type IllegalConfigError string

func (e IllegalConfigError) Error() string {
	return "illegal configuration: " + string(e)
}

// ManagerCreationError returns an error reporting that a Manager could not be
// created due to err.
func ManagerCreationError(err error) error {
	return fmt.Errorf("could not create manager: %s", err.Error())
}
