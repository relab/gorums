package stream

import (
	"context"
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
)

// BidiStream abstracts both client-side and server-side bidirectional streams.
// Both grpc.BidiStreamingClient[Message, Message] and
// grpc.BidiStreamingServer[Message, Message] satisfy this interface.
type BidiStream interface {
	Send(*Message) error
	Recv() (*Message, error)
}

// Request is a message to send to a node, together with how to report its outcome.
type Request struct {
	Ctx          context.Context
	Msg          *Message
	Streaming    bool
	Oneway       bool
	ResponseChan chan<- response
	SendTime     time.Time
}

// wantServerResponse reports whether the request expects server responses:
// two-way calls (RPC, quorum call) and streaming calls (correctable).
func (r Request) wantServerResponse() bool {
	return r.ResponseChan != nil && !r.Oneway
}

// wantSendConfirmation reports whether the request expects confirmation that
// it was sent: one-way calls (unicast, multicast) with a response channel.
func (r Request) wantSendConfirmation() bool {
	return r.Oneway && r.ResponseChan != nil
}

// deliver sends resp on the request's response channel, preferring delivery
// even if the request's context has ended. If the channel is full, it waits
// until the context ends. It reports whether resp was delivered.
func (r Request) deliver(resp response) bool {
	select {
	case r.ResponseChan <- resp:
		return true
	default:
	}
	select {
	case r.ResponseChan <- resp:
		return true
	case <-r.Ctx.Done():
		return false
	}
}

// ReplyError sends err on the request's response channel, if it has one.
func (r Request) ReplyError(nodeID uint32, err error) {
	if r.ResponseChan != nil {
		r.deliver(response{NodeID: nodeID, Err: err})
	}
}
