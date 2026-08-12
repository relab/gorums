package impl

import "errors"

// ErrIncomplete is the error returned by a quorum call when the call cannot be completed
// due to insufficient non-error replies to form a quorum according to the quorum function.
var ErrIncomplete = errors.New("incomplete call")

// ErrSendFailure is the error returned by a multicast call when message sending fails for one or more nodes.
var ErrSendFailure = errors.New("send failure")

// ErrSkipNode is returned when a node is skipped by request transformations.
// This allows the response iterator to account for all nodes without blocking.
var ErrSkipNode = errors.New("skip node")
