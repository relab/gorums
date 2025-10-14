package gorums

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var streamDownErr = status.Error(codes.Unavailable, "stream is down")

type request struct {
	ctx  context.Context
	msg  *Message
	opts callOptions
}

type response struct {
	nid uint32
	msg protoreflect.ProtoMessage
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
	gorumsStream ordering.Gorums_NodeStreamClient
	streamMut    sync.RWMutex
	streamCtx    context.Context
	cancelStream context.CancelFunc

	// Response routing
	responseRouters map[uint64]responseRouter
	responseMut     sync.Mutex

	// Lifecycle management
	parentCtx context.Context
}

// newChannel creates a new channel for the given node and
// starts the sender and receiver goroutines.
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

func (c *channel) enqueue(req request, responseChan chan<- response, streaming bool) {
	if responseChan != nil {
		c.responseMut.Lock()
		c.responseRouters[req.msg.Metadata.GetMessageID()] = responseRouter{responseChan, streaming}
		c.responseMut.Unlock()
	}
	// either enqueue the request on the sendQ or respond
	// with error if the node is closed
	select {
	case <-c.parentCtx.Done():
		c.routeResponse(req.msg.Metadata.GetMessageID(), response{nid: c.node.ID(), err: fmt.Errorf("channel closed")})
		return
	case c.sendQ <- req:
	}
}

func (c *channel) deleteRouter(msgID uint64) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	delete(c.responseRouters, msgID)
}

// clearStream cancels the current stream context and clears the stream reference.
// This triggers reconnection on the next send attempt.
func (c *channel) clearStream() {
	c.cancelStream()
	c.streamMut.Lock()
	c.gorumsStream = nil
	c.streamMut.Unlock()
}

// getStream returns the current stream, or nil if no stream is available.
func (c *channel) getStream() ordering.Gorums_NodeStreamClient {
	c.streamMut.RLock()
	defer c.streamMut.RUnlock()
	return c.gorumsStream
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
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{})
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

func (c *channel) sender() {
	var req request
	for {
		select {
		case <-c.parentCtx.Done():
			return
		case req = <-c.sendQ:
		}
		if err := c.ensureStream(); err != nil {
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{nid: c.node.ID(), err: err})
			continue
		}
		if err := c.sendMsg(req); err != nil {
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{nid: c.node.ID(), err: err})
		}
	}
}

func (c *channel) receiver() {
	for {
		stream := c.getStream()
		if stream == nil {
			// No stream available, wait and retry
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
			err := status.FromProto(resp.Metadata.GetStatus()).Err()
			c.routeResponse(resp.Metadata.GetMessageID(), response{nid: c.node.ID(), msg: resp.Message, err: err})
		}

		select {
		case <-c.parentCtx.Done():
			return
		default:
		}
	}
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
