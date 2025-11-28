package gorums

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// NodeResponse wraps a response value from node ID, and an error if any.
type NodeResponse[T any] struct {
	NodeID uint32
	Value  T
	Err    error
}

var streamDownErr = status.Error(codes.Unavailable, "stream is down")

type request struct {
	ctx          context.Context
	msg          *Message
	waitSendDone bool
	streaming    bool
	responseChan chan<- NodeResponse[proto.Message]
}

type channel struct {
	sendQ chan request
	node  *RawNode

	// Error and latency tracking
	mu        sync.Mutex
	lastError error
	latency   time.Duration

	// Stream management for FIFO ordering
	// gorumsStream is a bidirectional stream for FIFO message delivery
	gorumsStream grpc.ClientStream
	streamMut    sync.RWMutex
	streamCtx    context.Context
	cancelStream context.CancelFunc

	// Response routing; the map holds pending requests waiting for responses.
	// The request contains the responseChan on which to send the response
	// to the caller.
	responseRouters map[uint64]request
	responseMut     sync.Mutex

	// Lifecycle management: node close() cancels this context
	parentCtx context.Context
}

// newChannel creates a new channel for the given node and starts the sender
// and receiver goroutines.
//
// Note that we start both goroutines even though the connection and stream
// have not yet been established. This is to prevent deadlock when invoking
// a call type. The sender blocks on the sendQ and the receiver waits for
// the stream to become available.
func newChannel(n *RawNode) *channel {
	c := &channel{
		sendQ:           make(chan request, n.mgr.opts.sendBuffer),
		node:            n,
		latency:         -1 * time.Second,
		responseRouters: make(map[uint64]request),
	}
	// parentCtx controls the channel and is used to shut it down
	c.parentCtx = n.newContext()
	go c.sender()
	go c.receiver()
	return c
}

// newNodeStream creates a new stream for this channel.
// The receiver goroutine will detect the new stream and start using it.
func (c *channel) newNodeStream() (err error) {
	c.streamMut.Lock()
	c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
	c.gorumsStream, err = ordering.NewGorumsClient(c.node.conn).NodeStream(c.streamCtx)
	c.streamMut.Unlock()
	return err
}

// getStream returns the current stream, or nil if no stream is available.
func (c *channel) getStream() grpc.ClientStream {
	c.streamMut.RLock()
	defer c.streamMut.RUnlock()
	return c.gorumsStream
}

// clearStream cancels the current stream context and clears the stream reference.
// This triggers reconnection on the next send attempt.
func (c *channel) clearStream() {
	c.streamMut.Lock()
	c.cancelStream()
	c.gorumsStream = nil
	c.streamMut.Unlock()
}

// ensureStream ensures there's an active NodeStream for the sender and receiver goroutines.
// gRPC automatically handles TCP connection state when creating the stream.
func (c *channel) ensureStream() error {
	if c.isConnected() {
		return nil
	}
	return c.newNodeStream()
}

// isConnected returns true if the gRPC connection is in Ready state and we have an active stream.
func (c *channel) isConnected() bool {
	return c.node.conn.GetState() == connectivity.Ready && c.getStream() != nil
}

// enqueue adds the request to the send queue and sets up response routing if needed.
// If the node is closed, it responds with an error instead.
func (c *channel) enqueue(req request) {
	msgID := req.msg.GetMessageID()
	if req.responseChan != nil {
		c.responseMut.Lock()
		c.responseRouters[msgID] = req
		c.responseMut.Unlock()
	}
	// either enqueue the request on the sendQ or respond
	// with error if the node is closed
	select {
	case <-c.parentCtx.Done():
		// the node's close() method was called: respond with error instead of enqueueing
		c.routeResponse(msgID, NodeResponse[proto.Message]{NodeID: c.node.ID(), Err: fmt.Errorf("node closed")})
		return
	case c.sendQ <- req:
		// enqueued successfully
	}
}

// routeResponse routes the response to the appropriate response channel based on msgID.
// If no matching request is found, the response is discarded.
func (c *channel) routeResponse(msgID uint64, resp NodeResponse[proto.Message]) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	if req, ok := c.responseRouters[msgID]; ok {
		req.responseChan <- resp
		// delete the router if we are only expecting a single reply message
		if !req.streaming {
			delete(c.responseRouters, msgID)
		}
	}
}

// cancelPendingMsgs cancels all pending messages by sending an error response to each
// associated request. This is called when the stream goes down to notify all waiting calls.
func (c *channel) cancelPendingMsgs() {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	for msgID, req := range c.responseRouters {
		req.responseChan <- NodeResponse[proto.Message]{NodeID: c.node.ID(), Err: streamDownErr}
		// delete the router if we are only expecting a single reply message
		if !req.streaming {
			delete(c.responseRouters, msgID)
		}
	}
}

// deleteRouter removes the response router for the given msgID.
// This is used for cleaning up after streaming calls are done.
func (c *channel) deleteRouter(msgID uint64) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	delete(c.responseRouters, msgID)
}

// sender goroutine takes requests from the sendQ and sends them on the stream.
// If the stream is down, it tries to re-establish it.
func (c *channel) sender() {
	var req request
	for {
		select {
		case <-c.parentCtx.Done():
			// the node's close() method was called: exit sender goroutine
			return
		case req = <-c.sendQ:
			// take next request from sendQ
		}
		if err := c.ensureStream(); err != nil {
			c.routeResponse(req.msg.GetMessageID(), NodeResponse[proto.Message]{NodeID: c.node.ID(), Err: err})
			continue
		}
		if err := c.sendMsg(req); err != nil {
			c.routeResponse(req.msg.GetMessageID(), NodeResponse[proto.Message]{NodeID: c.node.ID(), Err: err})
		}
	}
}

// receiver goroutine receives messages from the stream and routes them to
// the appropriate response router. If the stream goes down, it clears the
// stream reference and cancels all pending messages with a stream down error.
func (c *channel) receiver() {
	for {
		stream := c.getStream()
		if stream == nil {
			// stream not yet available, wait and retry
			time.Sleep(10 * time.Millisecond)
			continue
		}

		resp := newMessage(responseType)
		if err := stream.RecvMsg(resp); err != nil {
			c.logf("RecvMsg error: %v", err)
			c.setLastErr(err)
			c.cancelPendingMsgs()
			c.clearStream()
		} else {
			err := resp.GetStatus().Err()
			c.routeResponse(resp.GetMessageID(), NodeResponse[proto.Message]{NodeID: c.node.ID(), Value: resp.GetProtoMessage(), Err: err})
		}

		select {
		case <-c.parentCtx.Done():
			// the node's close() method was called: exit receiver goroutine
			return
		default:
		}
	}
}

func (c *channel) sendMsg(req request) (err error) {
	defer func() {
		// For one-way call types (Unicast/Multicast), the caller can choose between two behaviors:
		//
		// 1. Default (mustWaitSendDone=true): Block until send completes
		//    - If send succeeds (err == nil): Send empty response to unblock caller immediately
		//    - If send fails (err != nil): sender() goroutine will deliver the error
		//    - Provides synchronous error handling with immediate feedback
		//
		// 2. WithNoSendWaiting option (mustWaitSendDone=false): Return immediately after enqueuing
		//    - Caller returns as soon as message is queued to sendQ
		//    - Any errors are delivered asynchronously by sender() goroutine
		//    - Provides fire-and-forget semantics
		//
		// Note: Two-way call types (RPCCall, QuorumCall) do not use this mechanism, they always
		// wait for actual server responses, so waitSendDone is false for them.
		if req.waitSendDone && err == nil {
			// Send succeeded: unblock the caller and clean up the responseRouter
			c.routeResponse(req.msg.GetMessageID(), NodeResponse[proto.Message]{})
		}
	}()

	// don't send if context is already cancelled.
	if req.ctx.Err() != nil {
		return req.ctx.Err()
	}

	stream := c.getStream()
	if stream == nil {
		return streamDownErr
	}

	done := make(chan struct{})

	// This goroutine waits for either 'done' to be closed, or the request context to be cancelled.
	// If the request context was cancelled, we have two possibilities:
	// The stream could be blocked, or the caller could be impatient.
	// We cannot know which is the case, but it seems wiser to cancel the stream as a precaution,
	// because reconnection is quite fast and cheap.
	go func() {
		select {
		case <-done:
			// all is good
		case <-req.ctx.Done():
			// Both channels could be ready at the same time, so we must check 'done' again.
			select {
			case <-done:
				// false alarm
			default:
				// trigger reconnect
				c.logf("request context cancelled: %v", req.ctx.Err())
				c.clearStream()
			}
		}
	}()

	if err = stream.SendMsg(req.msg); err != nil {
		c.logf("SendMsg error: %v", err)
		c.setLastErr(err)
		c.clearStream()
	}

	close(done)
	return err
}

func (c *channel) logf(format string, args ...any) {
	if c.node.mgr.logger == nil {
		return
	}
	c.node.mgr.logger.Printf("Node %d: %s", c.node.ID(), fmt.Sprintf(format, args...))
}

func (c *channel) setLastErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err
}

// lastErr returns the last error encountered (if any) when using this channel.
func (c *channel) lastErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

// channelLatency returns the latency between the client and this channel.
func (c *channel) channelLatency() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latency
}
