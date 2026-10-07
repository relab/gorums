package stream

import (
	"context"
	"time"
)

// Request is a message to send to a node, together with how to report its outcome.
type Request struct {
	Ctx          context.Context
	Msg          *Message
	Streaming    bool
	Oneway       bool
	ResponseChan chan<- response
	sendTime     time.Time
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

// replyError sends err on the request's response channel, if it has one.
func (r Request) replyError(nodeID uint32, err error) {
	if r.ResponseChan != nil {
		r.deliver(response{NodeID: nodeID, Err: err})
	}
}

// NodeResponse wraps a response value from node ID, and an error if any.
type NodeResponse[T any] struct {
	NodeID uint32
	Value  T
	Err    error
}

// response is a type alias for NodeResponse[*Message] to avoid long type names.
type response = NodeResponse[*Message]
