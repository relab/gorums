package stream

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// ErrNodeClosed is returned for requests enqueued after the node closed.
	ErrNodeClosed = status.Error(codes.Unavailable, "node closed")
	// ErrStreamDown is returned for requests that cannot be delivered or
	// retried because the node's stream is not available.
	ErrStreamDown = status.Error(codes.Unavailable, "stream is down")
	// ErrSendQueueFull is returned for two-way requests enqueued while the
	// node's send queue is full; one-way requests wait for space instead (see
	// [Channel]).
	ErrSendQueueFull = status.Error(codes.Unavailable, "send queue full")
	// ErrSendStalled is reported by [Channel.LastErr] while a send on the
	// node's stream has been blocked for [stallReportDelay] or longer, as when
	// the peer has stopped reading the stream.
	ErrSendStalled = status.Error(codes.Unavailable, "send stalled")
)

// stallReportDelay is how long a send must be blocked before [Channel.LastErr]
// reports [ErrSendStalled].
const stallReportDelay = time.Second
