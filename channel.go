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

var streamDownErr = status.Error(codes.Unavailable, "stream is down")

type request struct {
	ctx       context.Context
	msg       *Message
	opts      callOptions
	streaming bool
}

type response struct {
	nid uint32
	msg proto.Message
	err error
}

type responseRouter struct {
	c         chan<- response
	streaming bool
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

	// Response routing
	responseRouters map[uint64]responseRouter
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
		responseRouters: make(map[uint64]responseRouter),
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

// enqueue adds the request to the send queue and sets up a response router if needed.
// If the node is closed, it responds with an error instead.
func (c *channel) enqueue(req request, responseChan chan<- response) {
	msgID := req.msg.GetMessageID()
	if responseChan != nil {
		// allocate before critical section
		router := responseRouter{responseChan, req.streaming}
		c.responseMut.Lock()
		c.responseRouters[msgID] = router
		c.responseMut.Unlock()
	}
	// either enqueue the request on the sendQ or respond
	// with error if the node is closed
	select {
	case <-c.parentCtx.Done():
		// the node's close() method was called: respond with error instead of enqueueing
		c.routeResponse(msgID, response{nid: c.node.ID(), err: fmt.Errorf("node closed")})
		return
	case c.sendQ <- req:
		// enqueued successfully
	}
}

// routeResponse routes the response to the appropriate response router based on msgID.
// If no router is found, the response is discarded.
func (c *channel) routeResponse(msgID uint64, resp response) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	if router, ok := c.responseRouters[msgID]; ok {
		router.c <- resp
		// delete the router if we are only expecting a single reply message
		if !router.streaming {
			delete(c.responseRouters, msgID)
		}
	}
}

// cancelPendingMsgs cancels all pending messages by sending an error response to each router.
// This is typically called when the stream goes down to notify all waiting calls.
func (c *channel) cancelPendingMsgs() {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	for msgID, router := range c.responseRouters {
		router.c <- response{nid: c.node.ID(), err: streamDownErr}
		// delete the router if we are only expecting a single reply message
		if !router.streaming {
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
			c.routeResponse(req.msg.GetMessageID(), response{nid: c.node.ID(), err: err})
			continue
		}
		if err := c.sendMsg(req); err != nil {
			c.routeResponse(req.msg.GetMessageID(), response{nid: c.node.ID(), err: err})
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
			c.routeResponse(resp.GetMessageID(), response{nid: c.node.ID(), msg: resp.GetProtoMessage(), err: err})
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
		// wait for actual server responses, so mustWaitSendDone() returns false for them.
		if req.opts.mustWaitSendDone() && err == nil {
			// Send succeeded: unblock the caller and clean up the responseRouter
			c.routeResponse(req.msg.GetMessageID(), response{})
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
