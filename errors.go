package gorums

import (
	"github.com/relab/gorums/internal/conn"
	"github.com/relab/gorums/internal/impl"
	"github.com/relab/gorums/internal/stream"
)

// ErrIncomplete is the error returned by a quorum call when the call cannot be completed
// due to insufficient non-error replies to form a quorum according to the quorum function.
var ErrIncomplete = impl.ErrIncomplete

// ErrSendFailure is the error returned by a multicast call when message sending
// fails for one or more nodes.
var ErrSendFailure = impl.ErrSendFailure

// ErrTypeMismatch is returned when a response cannot be cast to the expected type.
var ErrTypeMismatch = impl.ErrTypeMismatch

// ErrStreamDown is returned for a call that cannot be delivered or retried
// because the target node's stream is unavailable, such as a call over a
// shared [WithStreamDedup] stream that is not connected (see
// [Server.WaitForAll]). It is a gRPC status error with the Unavailable code;
// test for it with [errors.Is], also inside a [QuorumCallError]. The
// Unavailable code alone does not tell it apart from other node errors.
var ErrStreamDown = stream.ErrStreamDown

// ErrNodeClosed is returned for a call enqueued after its node was closed. It
// is a gRPC status error with the Unavailable code; test for it with
// [errors.Is]. The Unavailable code alone does not tell it apart from other
// node errors.
var ErrNodeClosed = stream.ErrNodeClosed

// ErrNoNode is the error of a remote call or unicast on a nil node.
// It is a gRPC status error with the InvalidArgument code; test for it with
// [errors.Is]. The InvalidArgument code alone does not tell it apart from
// [ErrEmptyConfig].
var ErrNoNode = impl.ErrNoNode

// ErrEmptyConfig is the error of a quorum call or multicast on an empty
// configuration, such as one that [Config.Remove] or [Config.WithoutErrors]
// emptied. It is a gRPC status error with the InvalidArgument code; test for
// it with [errors.Is]. The InvalidArgument code alone does not tell it apart
// from [ErrNoNode].
var ErrEmptyConfig = impl.ErrEmptyConfig

// ErrSendQueueFull is returned for a two-way call enqueued while the node's
// send queue is full, so quorum logic counts the peer as failed; see
// [WithDialBufferSizes]. A one-way call waits for space instead. It is a gRPC
// status error with the Unavailable code; test for it with [errors.Is]. The
// Unavailable code alone does not tell it apart from other node errors.
var ErrSendQueueFull = stream.ErrSendQueueFull

// ErrSendStalled is reported by [Node.LastErr] while a send to the node has
// been blocked for a second or longer, as when the node has stopped reading
// its stream.
var ErrSendStalled = stream.ErrSendStalled

// ErrSkipNode is returned when a node is skipped by request transformations.
// This allows the response iterator to account for all nodes without blocking.
var ErrSkipNode = impl.ErrSkipNode

// ErrStopped is returned by [Server.WaitForPeers], [Server.WaitForClients],
// and [Server.WaitForAll] when the server is stopped before the condition is met.
var ErrStopped = conn.ErrStopped

// QuorumCallError reports on a failed quorum call.
// It provides detailed information about which nodes failed.
type QuorumCallError = conn.QuorumCallError
