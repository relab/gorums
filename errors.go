package gorums

import (
	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/impl"
	"github.com/relab/gorums/internal/stream"
)

// ErrIncomplete is the error returned by a quorum call when the call cannot be completed
// due to insufficient non-error replies to form a quorum according to the quorum function.
var ErrIncomplete = impl.ErrIncomplete

// ErrSendFailure is the error returned by a multicast call when message sending fails for one or more nodes.
var ErrSendFailure = impl.ErrSendFailure

// ErrTypeMismatch is returned when a response cannot be cast to the expected type.
var ErrTypeMismatch = stream.ErrTypeMismatch

// ErrStreamDown is returned for a call that cannot be delivered or retried
// because the target node's stream is unavailable. Under [WithStreamDedup] a
// higher-ID peer borrows the lower-ID peer's dialed stream and cannot re-dial
// it, so calls to a lower-ID peer report ErrStreamDown until that peer connects
// (see [Server.WaitForAll]) and again if the shared stream later drops. It is a
// gRPC status error with the Unavailable code; match its identity with
// [errors.Is], including against a node error inside a [QuorumCallError].
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
var ErrSkipNode = impl.ErrSkipNode

// ErrStopped is returned by [Server.WaitForPeers], [Server.WaitForClients],
// and [Server.WaitForAll] when the server is stopped before the condition is met.
var ErrStopped = conn.ErrStopped

// QuorumCallError reports on a failed quorum call.
// It provides detailed information about which nodes failed.
type QuorumCallError = conn.QuorumCallError
