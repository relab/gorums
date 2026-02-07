package gorums

import (
	"context"
	"sync"
	"time"

	"github.com/relab/gorums/ordering"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

// NodeResponse wraps a response value from node ID, and an error if any.
type NodeResponse[T any] struct {
	NodeID uint32
	Value  T
	Err    error
}

// newNodeResponse converts a NodeResponse[msg] to a NodeResponse[Resp].
// This is necessary because the channel layer's response router returns a
// NodeResponse[msg], while the calltype expects a NodeResponse[Resp].
func newNodeResponse[Resp msg](r NodeResponse[msg]) NodeResponse[Resp] {
	res := NodeResponse[Resp]{
		NodeID: r.NodeID,
		Err:    r.Err,
	}
	if r.Err == nil {
		if val, ok := r.Value.(Resp); ok {
			res.Value = val
		} else {
			res.Err = ErrTypeMismatch
		}
	}
	return res
}

var (
	nodeClosedErr = status.Error(codes.Unavailable, "node closed")
	streamDownErr = status.Error(codes.Unavailable, "stream is down")
)

type request struct {
	ctx          context.Context
	msg          *Message
	waitSendDone bool
	streaming    bool
	responseChan chan<- NodeResponse[msg]
	sendTime     time.Time
}

type channel struct {
	sendQ chan request
	id    uint32

	// Connection lifecycle management: node close() cancels the
	// connection context to stop all goroutines and the NodeStream
	conn       *grpc.ClientConn
	connCtx    context.Context
	connCancel context.CancelFunc

	// Error and latency tracking
	mu        sync.Mutex
	lastError error
	latency   time.Duration

	// Stream lifecycle management for FIFO ordered message delivery
	// gorumsStream is a bidirectional stream for
	// sending and receiving ordering.Metadata messages.
	gorumsStream ordering.Gorums_NodeStreamClient
	streamMut    sync.Mutex
	streamCtx    context.Context
	streamCancel context.CancelFunc
	streamReady  chan struct{} // signals receiver when stream becomes available

	// Response routing; the map holds pending requests waiting for responses.
	// The request contains the responseChan on which to send the response
	// to the caller.
	responseRouters map[uint64]request
	responseMut     sync.Mutex
	closeOnceFunc   func() error
}

// newChannel creates a new channel for the given node and starts the sender
// and receiver goroutines.
//
// Note that we start both goroutines even though the connection and stream
// have not yet been established. This is to prevent deadlock when invoking
// a call type. The sender blocks on the sendQ and the receiver waits for
// the stream to become available.
func newChannel(parentCtx context.Context, conn *grpc.ClientConn, id uint32, sendBufferSize uint) *channel {
	ctx, connCancel := context.WithCancel(parentCtx)
	c := &channel{
		sendQ:           make(chan request, sendBufferSize),
		id:              id,
		conn:            conn,
		connCtx:         ctx,
		connCancel:      connCancel,
		latency:         -1 * time.Second,
		responseRouters: make(map[uint64]request),
		streamReady:     make(chan struct{}, 1),
	}
	c.closeOnceFunc = sync.OnceValue(func() error {
		// important to cancel first to stop goroutines
		c.connCancel()
		if c.conn != nil {
			return c.conn.Close()
		}
		return nil
	})
	go c.sender()
	go c.receiver()
	return c
}

// close closes the channel and the underlying connection exactly once.
func (c *channel) close() error {
	return c.closeOnceFunc()
}

// ensureStream ensures there is an active NodeStream for the sender and
// receiver goroutines, and signals the receiver when the stream is ready.
// gRPC automatically handles TCP connection state when creating the stream.
// This method is safe for concurrent use.
func (c *channel) ensureStream() error {
	if err := c.ensureConnectedNodeStream(); err != nil {
		return err
	}
	// signal receiver that stream is ready (non-blocking)
	select {
	case c.streamReady <- struct{}{}:
	default:
		// channel already has a signal pending, no need to add another
	}
	return nil
}

// ensureConnectedNodeStream ensures there is an active and connected
// NodeStream, or creates a new stream if one doesn't already exist.
// This method is safe for concurrent use.
func (c *channel) ensureConnectedNodeStream() (err error) {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	// if we already have a ready connection and an active stream, do nothing
	// (cannot reuse isConnected() here since it uses the streamMut lock)
	if c.conn.GetState() == connectivity.Ready && c.gorumsStream != nil {
		return nil
	}
	c.streamCtx, c.streamCancel = context.WithCancel(c.connCtx)
	c.gorumsStream, err = ordering.NewGorumsClient(c.conn).NodeStream(c.streamCtx)
	return err
}

// getStream returns the current stream, or nil if no stream is available.
func (c *channel) getStream() grpc.ClientStream {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	return c.gorumsStream
}

// clearStream cancels the current stream context and clears the stream reference.
// This triggers reconnection on the next send attempt.
func (c *channel) clearStream() {
	c.streamMut.Lock()
	c.streamCancel()
	c.gorumsStream = nil
	c.streamMut.Unlock()
}

// isConnected returns true if the gRPC connection is in Ready state and we have an active stream.
// This method is safe for concurrent use.
func (c *channel) isConnected() bool {
	return c.conn.GetState() == connectivity.Ready && c.getStream() != nil
}

// enqueue adds the request to the send queue and sets up response routing if needed.
// If the node is closed, it responds with an error instead.
func (c *channel) enqueue(req request) {
	if req.responseChan != nil {
		req.sendTime = time.Now()
		msgID := req.msg.GetMessageID()
		c.responseMut.Lock()
		c.responseRouters[msgID] = req
		c.responseMut.Unlock()
	}
	// either enqueue the request on the sendQ or respond
	// with error if the node is closed
	select {
	case <-c.connCtx.Done():
		// the node's close() method was called: respond with error instead of enqueueing
		c.routeResponse(req.msg.GetMessageID(), NodeResponse[msg]{NodeID: c.id, Err: nodeClosedErr})
		return
	case c.sendQ <- req:
		// enqueued successfully
	}
}

// routeResponse routes the response to the appropriate response channel based on msgID.
// If no matching request is found, the response is discarded.
func (c *channel) routeResponse(msgID uint64, resp NodeResponse[msg]) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	if req, ok := c.responseRouters[msgID]; ok {
		if resp.Err == nil {
			c.updateLatency(time.Since(req.sendTime))
		}
		req.responseChan <- resp
		// delete the router if we are only expecting a single reply message
		if !req.streaming {
			delete(c.responseRouters, msgID)
		}
	}
}

// cancelPendingMsgs cancels all pending messages by sending an error response to each
// associated request. This is called when the stream goes down to notify all waiting calls.
func (c *channel) cancelPendingMsgs(err error) {
	c.responseMut.Lock()
	defer c.responseMut.Unlock()
	for msgID, req := range c.responseRouters {
		req.responseChan <- NodeResponse[msg]{NodeID: c.id, Err: err}
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
	// eager connect; ignored if stream is down (will be retried on send)
	_ = c.ensureStream()

	var req request
	for {
		select {
		case <-c.connCtx.Done():
			// the node's close() method was called: exit sender goroutine
			return
		case req = <-c.sendQ:
			// take next request from sendQ
		}
		if err := c.ensureStream(); err != nil {
			c.routeResponse(req.msg.GetMessageID(), NodeResponse[msg]{NodeID: c.id, Err: err})
			continue
		}
		if err := c.sendMsg(req); err != nil {
			c.routeResponse(req.msg.GetMessageID(), NodeResponse[msg]{NodeID: c.id, Err: err})
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
			// Stream not yet available; wait for signal or shutdown
			select {
			case <-c.streamReady:
				// Stream is now available, continue to get it
				continue
			case <-c.connCtx.Done():
				// the node's close() method was called: exit receiver goroutine
				return
			}
		}

		resp := newMessage(responseType)
		if err := stream.RecvMsg(resp); err != nil {
			c.setLastErr(err)
			c.cancelPendingMsgs(err)
			c.clearStream()
		} else {
			err := resp.GetStatus().Err()
			c.routeResponse(resp.GetMessageID(), NodeResponse[msg]{NodeID: c.id, Value: resp.GetProtoMessage(), Err: err})
		}

		select {
		case <-c.connCtx.Done():
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
		// 2. With the IgnoreErrors option (mustWaitSendDone=false): Return immediately after enqueuing
		//    - Caller returns as soon as message is queued to sendQ
		//    - Any errors are delivered asynchronously by sender() goroutine
		//    - Provides fire-and-forget semantics
		//
		// Note: Two-way call types (RPCCall, QuorumCall) do not use this mechanism, they always
		// wait for actual server responses, so waitSendDone is false for them.
		if req.waitSendDone && err == nil {
			// Send succeeded: unblock the caller and clean up the responseRouter
			c.routeResponse(req.msg.GetMessageID(), NodeResponse[msg]{})
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
				c.clearStream()
			}
		}
	}()

	if err = stream.SendMsg(req.msg); err != nil {
		c.setLastErr(err)
		c.clearStream()
	}

	close(done)
	return err
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

// channelLatency returns the latency between the client and the server associated with this channel.
func (c *channel) channelLatency() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latency
}

// updateLatency updates the latency between the client and the server associated with this channel.
// It uses a simple moving average to calculate the latency.
func (c *channel) updateLatency(rtt time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.latency < 0 {
		c.latency = rtt
	} else {
		// Use simple moving average (alpha=0.2)
		c.latency = time.Duration(0.8*float64(c.latency) + 0.2*float64(rtt))
	}
}
