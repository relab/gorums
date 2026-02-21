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

// BidiStream abstracts both client-side and server-side bidirectional streams.
// Both grpc.BidiStreamingClient[Message, Message] and
// grpc.BidiStreamingServer[Message, Message] satisfy this interface.
type BidiStream interface {
	Send(*Message) error
	Recv() (*Message, error)
}

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
	stream       BidiStream
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

// NewOutboundChannel creates a new channel for the given node and starts
// the sender and receiver goroutines.
//
// Note that we start both goroutines even though the connection and stream
// have not yet been established. This is to prevent deadlock when invoking
// a call type. The sender blocks on the sendQ and the receiver waits for
// the stream to become available.
func NewOutboundChannel(parentCtx context.Context, id uint32, sendBufferSize uint, conn *grpc.ClientConn) *Channel {
	return newChannel(parentCtx, id, sendBufferSize, conn, nil)
}

// NewInboundChannel creates a channel from an existing server-side stream.
// Unlike outbound channels, inbound channels:
//   - Have no grpc.ClientConn (stream accepted by the gRPC server; not dialed by us)
//   - Cannot reconnect (the client controls stream creation)
//   - Close only cancels context; it does not close the underlying connection
func NewInboundChannel(parentCtx context.Context, id uint32, sendBufferSize uint, stream BidiStream) *Channel {
	return newChannel(parentCtx, id, sendBufferSize, nil, stream)
}

// newChannel is the shared constructor for outbound and inbound channels.
// Pass a non-nil conn for outbound channels (conn.Close() is called on Close()).
// Pass a non-nil stream for inbound channels (stream is immediately ready; no reconnection).
func newChannel(parentCtx context.Context, id uint32, sendBufferSize uint, conn *grpc.ClientConn, stream BidiStream) *Channel {
	connCtx, connCancel := context.WithCancel(parentCtx)
	c := &Channel{
		sendQ:           make(chan Request, sendBufferSize),
		id:              id,
		conn:            conn,
		stream:          stream,
		connCtx:         connCtx,
		connCancel:      connCancel,
		latency:         -1 * time.Second,
		responseRouters: make(map[uint64]Request),
		streamReady:     make(chan struct{}, 1),
	}
	c.closeOnceFunc = sync.OnceValue(func() error {
		// important to cancel first to stop goroutines
		connCancel()
		// unblocks any pending senders/receivers
		c.cancelPendingMsgs(ErrNodeClosed)
		if conn != nil {
			return conn.Close()
		}
		return nil
	})
	if stream != nil {
		// Signal that stream is immediately ready (inbound channel).
		c.streamReady <- struct{}{}
	}
	go c.sender()
	go c.receiver()
	return c
}

// IsInbound returns true if this channel was created from a server-side stream
// rather than an outbound client connection.
func (c *Channel) IsInbound() bool {
	return c.conn == nil
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
	if c.IsInbound() {
		// Inbound channels cannot reconnect; just check if stream exists.
		if c.getStream() == nil {
			return ErrStreamDown
		}
		return nil
	}
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
func (c *Channel) getStream() BidiStream {
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
func (c *Channel) clearStream(stale BidiStream) {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	if c.stream != stale {
		// stale is already gone; a new stream has been established — do not cancel it
		return
	}
	if c.streamCancel != nil {
		c.streamCancel()
	}
	c.stream = nil
}

// isConnected returns true if the channel has an active stream.
// For outbound channels, also requires the gRPC connection to be in Ready state.
// This method is safe for concurrent use.
func (c *Channel) isConnected() bool {
	if c.IsInbound() {
		return c.connCtx.Err() == nil && c.getStream() != nil
	}
	return c.conn.GetState() == connectivity.Ready && c.getStream() != nil
}

// Enqueue adds the request to the send queue.
// If the node is closed, it responds with an error instead.
//
// WaitSendDone and Streaming are mutually exclusive: WaitSendDone is for one-way
// calls that want send-completion confirmation, while Streaming is for
// correctable calls that keep the router entry alive for multiple server responses.
// Combining them would cause double delivery on the response channel.
func (c *Channel) Enqueue(req Request) {
	if req.WaitSendDone && req.Streaming {
		panic("stream: WaitSendDone and Streaming are mutually exclusive")
	}
	// Two-stage select: the outer non-blocking check catches the already-closed
	// case deterministically. Go's select only falls through to default when no
	// other case is ready, so if connCtx.Done() is already closed it always
	// wins — unlike a plain single select, where Go randomly picks between a
	// ready Done channel and a buffered sendQ.
	// The inner select handles the case where the node closes concurrently
	// while we are waiting for sendQ space; there a narrow race remains, but
	// drainSendQ (deferred in sender) will drain and replyError any entry that
	// slips through after sender exits.
	select {
	case <-c.connCtx.Done():
		// the node's close() method was called: respond with error instead of enqueueing
		c.replyError(req, ErrNodeClosed)
	default:
		select {
		case <-c.connCtx.Done():
			// the node's close() method was called: respond with error instead of enqueueing
			c.replyError(req, ErrNodeClosed)
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
// retry on the next stream. Streaming requests (correctable calls) are instead
// cancelled with ErrStreamDown because they cannot be safely retried: the caller
// may have already received partial responses on the response channel, and a silent
// re-send would start a fresh server-side computation and deliver a second,
// independent result sequence on the same channel, violating the correctable call
// contract. The caller receives the error and can decide whether to issue a new call.
//
// The sender goroutine will pick up requeued requests, call ensureStream to
// create a new stream, and resend them. The natural retry limit is the
// request's context timeout.
func (c *Channel) requeuePendingMsgs() {
	c.responseMut.Lock()
	toRequeue := make([]Request, 0, len(c.responseRouters))
	toCancel := make([]Request, 0, len(c.responseRouters))
	for msgID, req := range c.responseRouters {
		delete(c.responseRouters, msgID)
		if req.Streaming {
			toCancel = append(toCancel, req)
			continue
		}
		toRequeue = append(toRequeue, req)
	}
	c.responseMut.Unlock()

	// requeue non-streaming requests for retry on the new stream without holding the lock
	for _, req := range toRequeue {
		c.Enqueue(req)
	}
	// cancel streaming requests with error without holding the lock
	for _, req := range toCancel {
		c.replyError(req, ErrStreamDown)
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

// drainSendQ is deferred in sender() and drains any remaining requests from
// sendQ when the sender goroutine exits, replying to each with ErrNodeClosed.
// This handles both requests already in the queue and any that slip through
// the narrow race window in Enqueue after connCtx is cancelled.
// sendQ must never be closed: closing it could panic a concurrent Enqueue
// that passes the outer connCtx check and then sends on a closed channel.
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

		// Watch for per-request cancellation while Send is in-flight.
		// If req.Ctx is done before Send returns, clearStream unblocks
		// the blocked Send by cancelling the stream context.
		// stop() disarms the watcher after Send returns, preventing a
		// spurious clearStream on a still-valid stream.
		stop := context.AfterFunc(req.Ctx, func() {
			c.clearStream(stream)
		})
		if err := stream.Send(req.Msg); err != nil {
			stop()
			c.setLastErr(err)
			c.clearStream(stream)
			c.requeuePendingMsgs() // removes and requeues/cancels all router entries
			continue
		}
		stop()

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
