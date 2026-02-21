package stream

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	ErrNodeClosed = status.Error(codes.Unavailable, "node closed")
	ErrStreamDown = status.Error(codes.Unavailable, "stream is down")
)

type Request struct {
	Ctx          context.Context
	Msg          *Message
	Streaming    bool
	WaitSendDone bool
	ResponseChan chan<- response
	SendTime     time.Time
}

type Channel struct {
	sendQ chan Request
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
	// stream is a bidirectional stream for
	// sending and receiving stream.Message messages.
	stream       Gorums_NodeStreamClient
	streamMut    sync.Mutex
	streamCtx    context.Context
	streamCancel context.CancelFunc
	streamReady  chan struct{} // signals receiver when stream becomes available

	// Response routing; the map holds pending requests waiting for responses.
	// The request contains the responseChan on which to send the response
	// to the caller.
	responseRouters map[uint64]Request
	responseMut     sync.Mutex
	closeOnceFunc   func() error
}

// NewChannel creates a new channel for the given node and starts the sender
// and receiver goroutines.
//
// Note that we start both goroutines even though the connection and stream
// have not yet been established. This is to prevent deadlock when invoking
// a call type. The sender blocks on the sendQ and the receiver waits for
// the stream to become available.
func NewChannel(parentCtx context.Context, conn *grpc.ClientConn, id uint32, sendBufferSize uint) *Channel {
	ctx, connCancel := context.WithCancel(parentCtx)
	c := &Channel{
		sendQ:           make(chan Request, sendBufferSize),
		id:              id,
		conn:            conn,
		connCtx:         ctx,
		connCancel:      connCancel,
		latency:         -1 * time.Second,
		responseRouters: make(map[uint64]Request),
		streamReady:     make(chan struct{}, 1),
	}
	c.closeOnceFunc = sync.OnceValue(func() error {
		// important to cancel first to stop goroutines
		c.connCancel()
		// unblocks any pending senders/receivers
		c.cancelPendingMsgs(ErrNodeClosed)
		if c.conn != nil {
			return c.conn.Close()
		}
		return nil
	})
	go c.sender()
	go c.receiver()
	return c
}

// NewChannelWithState creates a new Channel with a specific state for testing.
// This function should only be used in tests.
func NewChannelWithState(latency time.Duration, lastErr error) *Channel {
	return &Channel{
		latency:   latency,
		lastError: lastErr,
	}
}

// Close closes the channel and the underlying connection exactly once.
func (c *Channel) Close() error {
	return c.closeOnceFunc()
}

// ensureStream ensures there is an active NodeStream for the sender and
// receiver goroutines, and signals the receiver when the stream is ready.
// gRPC automatically handles TCP connection state when creating the stream.
// This method is safe for concurrent use.
func (c *Channel) ensureStream() error {
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
func (c *Channel) ensureConnectedNodeStream() (err error) {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	// if we already have a ready connection and an active stream, do nothing
	// (cannot reuse isConnected() here since it uses the streamMut lock)
	if c.conn.GetState() == connectivity.Ready && c.stream != nil {
		return nil
	}
	c.streamCtx, c.streamCancel = context.WithCancel(c.connCtx)
	c.stream, err = NewGorumsClient(c.conn).NodeStream(c.streamCtx)
	return err
}

// getStream returns the current stream, or nil if no stream is available.
func (c *Channel) getStream() Gorums_NodeStreamClient {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	return c.stream
}

// clearStream cancels the stream context for stale and clears the stream reference,
// but only if stale is still the current stream. This guards against a race where
// the receiver calls clearStream on a stale stream after ensureStream has already
// replaced it with a new one, which would otherwise cancel the new stream's context
// and spuriously cancel requests that belong to the new stream.
// This triggers reconnection on the next send attempt.
func (c *Channel) clearStream(stale Gorums_NodeStreamClient) {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	if c.stream != stale {
		// stale is already gone; a new stream has been established — do not cancel it
		return
	}
	c.streamCancel()
	c.stream = nil
}

// isConnected returns true if the gRPC connection is in Ready state and we have an active stream.
// This method is safe for concurrent use.
func (c *Channel) isConnected() bool {
	return c.conn.GetState() == connectivity.Ready && c.getStream() != nil
}

// Enqueue adds the request to the send queue.
// If the node is closed, it responds with an error instead.
//
// WaitSendDone and Streaming are mutually exclusive: WaitSendDone is for one-way
// calls that want send-completion confirmation, while Streaming is for
// correctable calls that keep the router entry alive for multiple server responses.
// Combining them causes double delivery on the response channel.
func (c *Channel) Enqueue(req Request) {
	if req.WaitSendDone && req.Streaming {
		panic("stream: WaitSendDone and Streaming are mutually exclusive")
	}
	// Two-stage select pattern: ensures deterministic cancellation detection.
	// The outer select with default prevents racing between Done and sendQ when
	// both are ready (Go's select randomly chooses). This guarantees we always
	// detect cancellation before attempting to send, avoiding enqueuing after
	// the sender goroutine has exited.
	select {
	case <-c.connCtx.Done():
		// the node's close() method was called: respond with error instead of enqueueing
		c.replyError(req, ErrNodeClosed)
		return
	default:
		select {
		case <-c.connCtx.Done():
			// the node's close() method was called: respond with error instead of enqueueing
			c.replyError(req, ErrNodeClosed)
			return
		case c.sendQ <- req:
			// enqueued successfully
		}
	}
}

// routeResponse routes the response to the appropriate response channel based on msgID.
// If no matching request is found, the response is discarded.
func (c *Channel) routeResponse(msgID uint64, resp response) {
	c.responseMut.Lock()
	req, ok := c.responseRouters[msgID]
	if ok {
		// delete the router if we are only expecting a single reply message
		if !req.Streaming {
			delete(c.responseRouters, msgID)
		}
	}
	c.responseMut.Unlock()

	// Send response without holding the lock to avoid blocking other operations
	if ok {
		if resp.Err == nil {
			c.updateLatency(time.Since(req.SendTime))
		}
		req.ResponseChan <- resp
	}
}

// cancelPendingMsgs cancels all pending messages by sending an error response to each
// associated request. This is called during node shutdown to notify all waiting calls.
func (c *Channel) cancelPendingMsgs(err error) {
	c.responseMut.Lock()
	pendingRequests := make([]Request, 0, len(c.responseRouters))
	for msgID, req := range c.responseRouters {
		pendingRequests = append(pendingRequests, req)
		delete(c.responseRouters, msgID)
	}
	c.responseMut.Unlock()

	// Send error responses without holding the lock
	for _, req := range pendingRequests {
		c.replyError(req, err)
	}
}

// requeuePendingMsgs moves pending non-streaming requests back to sendQ for
// retry on the next stream. Streaming requests are cancelled with an error
// since they are bound to a specific stream and cannot be meaningfully retried.
//
// The sender goroutine will pick up requeued requests, call ensureStream to
// create a new stream, and resend them. The natural retry limit is the
// request's context timeout.
func (c *Channel) requeuePendingMsgs() {
	c.responseMut.Lock()
	toRequeue := make([]Request, 0, len(c.responseRouters))
	for msgID, req := range c.responseRouters {
		delete(c.responseRouters, msgID)
		if req.Streaming {
			c.replyError(req, ErrStreamDown)
			continue
		}
		toRequeue = append(toRequeue, req)
	}
	c.responseMut.Unlock()

	// requeue non-streaming requests for retry on the new stream without holding the responseMut lock
	for _, req := range toRequeue {
		c.Enqueue(req)
	}
}

// replyError sends err to the request's response channel if one is set.
// This is used for requests that have not yet been registered in responseRouters
// and therefore cannot be reached via routeResponse.
func (c *Channel) replyError(req Request, err error) {
	if req.ResponseChan != nil {
		req.ResponseChan <- response{NodeID: c.id, Err: err}
	}
}

// drainSendQ drains the sendQ and replies with err for each request.
// This is called when the sender() goroutine is exiting due to node shutdown,
// to ensure all pending requests receive an error response instead of being left hanging.
func (c *Channel) drainSendQ() {
	for {
		select {
		case req := <-c.sendQ:
			c.replyError(req, ErrNodeClosed)
		default:
			// sendQ is empty
			return
		}
	}
}

// sender goroutine takes requests from the sendQ and sends them on the stream.
// If the stream is down, it tries to re-establish it.
//
// Delivery contract:
//   - Pre-registration exits (stream error, cancelled request context, nil stream): replyError + continue.
//     The request never enters the router, so no routeResponse lookup is needed.
//   - Send failure: requeuePendingMsgs handles the registered entry (requeue or cancel).
//     continue skips routeResponse since the entry is already gone.
//   - Send success, WaitSendDone=true: routeResponse delivers the confirmation.
//   - Send success, WaitSendDone=false: the router entry stays alive for receiver()
//     to deliver the actual server response, so routeResponse is not called here.
func (c *Channel) sender() {
	// drain sendQ and return an error for all requests to the caller,
	// since the node is closed and no more sends are possible
	defer c.drainSendQ()

	// eager connect; ignored if stream is down (will be retried on send)
	_ = c.ensureStream()

	var req Request
	for {
		select {
		case <-c.connCtx.Done():
			// the node's close() method was called: exit sender goroutine
			return
		case req = <-c.sendQ:
			// take next request from sendQ
		}

		if err := c.ensureStream(); err != nil {
			c.replyError(req, err)
			continue
		}
		if req.Ctx.Err() != nil {
			c.replyError(req, req.Ctx.Err())
			continue
		}
		stream := c.getStream()
		if stream == nil {
			c.replyError(req, ErrStreamDown)
			continue
		}

		// Register in the response router only for requests that are genuinely
		// in-flight on the current stream, after all early-exit checks pass.
		if req.ResponseChan != nil {
			req.SendTime = time.Now()
			msgID := req.Msg.GetMessageSeqNo()
			c.responseMut.Lock()
			c.responseRouters[msgID] = req
			c.responseMut.Unlock()
		}

		if err := stream.Send(req.Msg); err != nil {
			c.setLastErr(err)
			c.clearStream(stream)
			c.requeuePendingMsgs() // removes and requeues/cancels all router entries
			continue
		}

		// For one-way calls (Unicast/Multicast) with WaitSendDone, confirm successful send.
		if req.WaitSendDone {
			c.routeResponse(req.Msg.GetMessageSeqNo(), response{NodeID: c.id})
		}
	}
}

// receiver goroutine receives messages from the stream and routes them to
// the appropriate response router. If the stream goes down, it clears the
// stream reference and requeues pending requests for retry on a new stream.
func (c *Channel) receiver() {
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

		respMsg, e := stream.Recv()
		if e != nil {
			c.setLastErr(e)
			c.clearStream(stream)
			c.requeuePendingMsgs()
			// Check for shutdown before attempting reconnection
			if c.connCtx.Err() != nil {
				// the node's close() method was called: exit receiver goroutine
				return
			}
		} else {
			err := respMsg.ErrorStatus()
			var resp proto.Message
			if err == nil {
				resp, err = UnmarshalResponse(respMsg)
			}
			// Two-way calls (RPCCall/QuorumCall) deliver server response.
			c.routeResponse(respMsg.GetMessageSeqNo(), response{NodeID: c.id, Value: resp, Err: err})
		}
	}
}

func (c *Channel) setLastErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err
}

// LastErr returns the last error encountered (if any) when using this channel.
func (c *Channel) LastErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

// Latency returns the latency between the client and the server associated with this channel.
func (c *Channel) Latency() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latency
}

// updateLatency updates the latency between the client and the server associated with this channel.
// It uses a simple moving average to calculate the latency.
func (c *Channel) updateLatency(rtt time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.latency < 0 {
		c.latency = rtt
	} else {
		// Use simple moving average (alpha=0.2)
		c.latency = time.Duration(0.8*float64(c.latency) + 0.2*float64(rtt))
	}
}
