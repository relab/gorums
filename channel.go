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

// waitForSend returns true if the WithNoSendWaiting call option is not set.
func (req request) waitForSend() bool {
	return req.opts.callType != nil && !req.opts.noSendWaiting
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
	// receiverStarted ensures we only start one receiver goroutine
	receiverStarted bool

	// Response routing
	responseRouters map[uint64]responseRouter
	responseMut     sync.Mutex

	// Lifecycle management
	parentCtx context.Context
}

// newChannel creates a new channel for the given node and starts the sending goroutine.
//
// Note that we start the sending goroutine even though the
// connection has not yet been established. This is to prevent
// deadlock when invoking a call type, as the goroutine will
// block on the sendQ until a connection has been established.
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
	return c
}

// newNodeStream creates a stream and starts the receiving goroutine.
//
// Note that the stream could fail even though conn != nil due
// to the non-blocking dial. Hence, we need to try to connect
// to the node before starting the receiving goroutine.
func (c *channel) newNodeStream() (err error) {
	// gorumsClient creates streams over the node's ClientConn
	gorumsClient := ordering.NewGorumsClient(c.node.conn)

	c.streamMut.Lock()
	c.streamCtx, c.cancelStream = context.WithCancel(c.parentCtx)
	c.gorumsStream, err = gorumsClient.NodeStream(c.streamCtx)
	c.streamMut.Unlock()
	if err != nil {
		return err
	}

	// Start receiver goroutine once (guard against multiple receivers)
	if !c.receiverStarted {
		c.receiverStarted = true
		go c.receiver()
	}
	return nil
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

func (c *channel) sendMsg(req request) (err error) {
	defer func() {
		// While the default is to block the caller until the message has been sent, we
		// can provide the WithNoSendWaiting call option to more quickly unblock the caller.
		// Hence, after sending, we unblock the waiting caller if the call option is not set;
		// that is, waitForSend is true. Conversely, if the call option is set, the call type
		// will not block on the response channel, and the "receiver" goroutine below will
		// eventually clean up the responseRouter map by calling routeResponse.
		if req.waitForSend() {
			// unblock the caller and clean up the responseRouter map
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{})
		}
	}()

	// don't send if context is already cancelled.
	if req.ctx.Err() != nil {
		return req.ctx.Err()
	}

	c.streamMut.RLock()
	stream := c.gorumsStream
	c.streamMut.RUnlock()

	if stream == nil {
		return status.Error(codes.Unavailable, "stream not available")
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
				c.clearStream()
			}
		}
	}()

	err = stream.SendMsg(req.msg)
	if err != nil {
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

		// Ensure we have an active stream (gRPC handles TCP connection automatically)
		if err := c.ensureStream(); err != nil {
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{nid: c.node.ID(), err: err})
			continue
		}

		// Try to send message
		err := c.sendMsg(req)
		if err != nil {
			c.routeResponse(req.msg.Metadata.GetMessageID(), response{nid: c.node.ID(), err: err})
		}
	}
}

func (c *channel) receiver() {
	for {
		c.streamMut.RLock()
		stream := c.gorumsStream
		c.streamMut.RUnlock()

		if stream == nil {
			// No stream available, wait and retry
			time.Sleep(10 * time.Millisecond)
			continue
		}

		resp := newMessage(responseType)
		err := stream.RecvMsg(resp)
		if err != nil {
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

// ensureStream ensures there's an active NodeStream and starts the receiver goroutine if needed.
// gRPC automatically handles TCP connection state when creating the stream.
func (c *channel) ensureStream() error {
	if c.isConnected() {
		return nil
	}
	return c.newNodeStream()
}

// isConnected returns true if the gRPC connection is in Ready state and we have an active stream.
func (c *channel) isConnected() bool {
	c.streamMut.RLock()
	hasStream := c.gorumsStream != nil
	c.streamMut.RUnlock()

	return c.node.conn.GetState() == connectivity.Ready && hasStream
}
