package stream

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// ErrNodeClosed is exported and documented as gorums.ErrNodeClosed.
	ErrNodeClosed = status.Error(codes.Unavailable, "node closed")
	// ErrStreamDown is exported and documented as gorums.ErrStreamDown.
	ErrStreamDown = status.Error(codes.Unavailable, "stream is down")
	// ErrSendQueueFull is exported and documented as gorums.ErrSendQueueFull.
	ErrSendQueueFull = status.Error(codes.Unavailable, "send queue full")
	// ErrSendStalled is exported and documented as gorums.ErrSendStalled.
	ErrSendStalled = status.Error(codes.Unavailable, "send stalled")
)

// stallReportDelay is how long a send must be blocked before [Channel.LastErr]
// reports [ErrSendStalled].
const stallReportDelay = time.Second

// isResourceExhausted reports whether err is due to resource exhaustion.
func isResourceExhausted(err error) bool {
	return status.Code(err) == codes.ResourceExhausted
}

// isRefusal reports whether err is due to a refused stream, including a
// stream that a draining server refused.
func isRefusal(err error) bool {
	switch status.Code(err) {
	case codes.Unauthenticated, codes.PermissionDenied, codes.InvalidArgument, codes.Unimplemented:
		return true
	}
	return isServerDraining(err)
}
