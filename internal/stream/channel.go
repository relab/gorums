package stream

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
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

// deliver sends the response on request's ResponseChan, preferring delivery
// even if request's context is already canceled. If the channel is full,
// it falls back to respecting context cancellation to avoid blocking forever.
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

type Channel struct {
	sendQ chan Request
	id    uint32

	// Connection lifecycle management: node close() cancels the
	// connection context to stop all goroutines and the NodeStream
	conn       *grpc.ClientConn
	connCtx    context.Context
	connCancel context.CancelFunc

	// Error tracking
	mu        sync.Mutex
	lastError error

	// Stream lifecycle management for FIFO ordered message delivery
	// stream is a bidirectional stream for
	// sending and receiving stream.Message messages.
	stream       BidiStream
	streamMut    sync.Mutex
	streamCtx    context.Context
	streamCancel context.CancelFunc
	streamReady  chan struct{} // signals receiver when stream becomes available

	// Router handles response routing for pending calls. It is owned by the
	// Node and injected into the Channel, so it survives channel replacement.
	router        *MessageRouter
	closeOnceFunc func() error

	// localMu is the serialization gate for local dispatch, mirroring
	// NodeStream's lock+release pattern to ensure handlers are serialized.
	localMu sync.Mutex
}

// NewOutboundChannel creates a new channel for the given node and starts
// the sender and receiver goroutines.
//
// Note that we start both goroutines even though the connection and stream
// have not yet been established. This is to prevent deadlock when invoking
// a call type. The sender blocks on the sendQ and the receiver waits for
// the stream to become available.
func NewOutboundChannel(parentCtx context.Context, id uint32, sendBufferSize uint, conn *grpc.ClientConn, router *MessageRouter) *Channel {
	return newChannel(parentCtx, id, sendBufferSize, conn, nil, router)
}

// NewInboundChannel creates a channel from an existing server-side stream.
// Only the sender goroutine is started; no receiver goroutine is launched.
//
// Receiving from the stream is intentionally left to the caller's goroutine
// (e.g. NodeStream's Recv loop), which is the sole authoritative reader.
// Starting a second receiver goroutine would race with that loop and would
// intercept response messages that NodeStream must route after demultiplexing
// them from new incoming requests.
//
// Unlike outbound channels, inbound channels:
//   - Have no receiver goroutine (NodeStream's Recv loop is the sole reader)
//   - Have no grpc.ClientConn (stream accepted by the gRPC server; not dialed by us)
//   - Cannot reconnect (the client controls stream creation)
//   - Close only cancels context; it does not close the underlying connection
func NewInboundChannel(parentCtx context.Context, id uint32, sendBufferSize uint, stream BidiStream, router *MessageRouter) *Channel {
	return newChannel(parentCtx, id, sendBufferSize, nil, stream, router)
}

// newChannel is the shared constructor for outbound and inbound channels.
// Pass a non-nil conn for outbound channels (conn.Close() is called on Close()).
// Pass a non-nil stream for inbound channels (stream is immediately ready; no reconnection).
// The receiver goroutine is started only for outbound channels; inbound callers own
// the stream's read side themselves (see NewInboundChannel for the full rationale).
func newChannel(parentCtx context.Context, id uint32, sendBufferSize uint, conn *grpc.ClientConn, stream BidiStream, router *MessageRouter) *Channel {
	connCtx, connCancel := context.WithCancel(parentCtx)
	c := &Channel{
		sendQ:       make(chan Request, sendBufferSize),
		id:          id,
		conn:        conn,
		stream:      stream,
		connCtx:     connCtx,
		connCancel:  connCancel,
		router:      router,
		streamReady: make(chan struct{}, 1),
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
	if conn != nil {
		// Outbound channels need a receiver goroutine to route call responses
		// back to waiting callers. Inbound channels must not start a receiver
		// goroutine: the gRPC server's NodeStream Recv loop is the sole reader
		// of the stream.
		go c.receiver()
	}
	return c
}

// NewLocalChannel creates a Channel that dispatches requests in-process,
// bypassing the network entirely. The provided router must carry the
// RequestHandler used to serve incoming call types on this node.
// No goroutines are started; the channel's Close is a no-op.
func NewLocalChannel(id uint32, router *MessageRouter) *Channel {
	c := &Channel{
		id:     id,
		router: router,
	}
	c.closeOnceFunc = sync.OnceValue(func() error { return nil })
	return c
}

// isLocal returns true if this channel dispatches in-process rather than over a
// network connection.
func (c *Channel) isLocal() bool {
	// The nil sendQ is the discriminator: all outbound and inbound channels always
	// allocate a sendQ via make(chan Request, ...) in newChannel.
	return c.sendQ == nil
}

// IsInbound returns true if this channel was created from a server-side stream
// rather than an outbound client connection.
func (c *Channel) IsInbound() bool {
	return c.conn == nil && c.sendQ != nil
}

// NewChannelWithState creates a new Channel with a specific state for testing.
// This function should only be used in tests.
func NewChannelWithState(lastErr error) *Channel {
	return &Channel{
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
// It returns true if stale was still current and was cleared, false otherwise.
// This triggers reconnection on the next send attempt.
func (c *Channel) clearStream(stale BidiStream) bool {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	if c.stream != stale {
		// stale is already gone; a new stream has been established — do not cancel it
		return false
	}
	if c.streamCancel != nil {
		c.streamCancel()
	}
	c.stream = nil
	return true
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
//
// If it is a local channel, the request is dispatched in-process via
// the registered RequestHandler without touching the network.
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
	if c.isLocal() {
		c.dispatchLocalRequest(req)
		return
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

// dispatchLocalRequest handles a request in-process for the self-node,
// bypassing the network. It explicitly acquires the channel's localMu to ensure
// identical sequential execution semantics as remote nodes, where execution
// is serialized until the handler returns or explicitly calls ServerCtx.Release().
//
// For one-way calls (WaitSendDone=true), the send completion confirmation
// is delivered before HandleRequest runs, matching remote-node semantics
// where confirmation is sent after stream.Send() completes, but decoupled
// from the handler runtime. For two-way calls, the send closure delivers
// the response directly to the request's ResponseChan.
func (c *Channel) dispatchLocalRequest(req Request) {
	if req.Ctx.Err() != nil {
		c.replyError(req, req.Ctx.Err())
		return
	}
	rh, ok := c.router.RequestHandler()
	if !ok {
		c.replyError(req, status.Error(codes.Unimplemented, "no request handler registered"))
		return
	}

	if req.WaitSendDone && req.ResponseChan != nil {
		if !req.deliver(response{NodeID: c.id}) {
			return
		}
	}
	// For two-way calls, deliver the response via the send closure.
	// For one-way calls (WaitSendDone=true or ResponseChan==nil), send is a no-op:
	// the confirmation was already delivered above, and a second write would either
	// race with the caller consuming the channel or block on a full response channel.
	send := func(msg *Message) {
		if req.WaitSendDone || req.ResponseChan == nil {
			return
		}
		req.deliver(response{NodeID: c.id, Value: msg, Err: msg.ErrorStatus()})
	}

	c.localMu.Lock()
	var once sync.Once
	release := func() { once.Do(c.localMu.Unlock) }

	go rh.HandleRequest(req.Msg.AppendToIncomingContext(req.Ctx), req.Msg, release, send)
}

// cancelPendingMsgs cancels all pending messages by sending an error response to each.
// This is called during node shutdown to notify all waiting calls.
func (c *Channel) cancelPendingMsgs(err error) {
	for _, req := range c.router.CancelPending() {
		c.replyError(req, err)
	}
}

// requeuePendingMsgs moves pending non-streaming requests back to sendQ for
// retry on the next stream. Streaming requests (correctable calls) are cancelled
// with ErrStreamDown because they cannot be safely retried.
func (c *Channel) requeuePendingMsgs() {
	requeue, cancel := c.router.RequeuePending()
	for _, req := range cancel {
		c.replyError(req, ErrStreamDown)
	}
	// The requeue is performed in a separate goroutine because this method is called
	// from sender(), which is the sole reader of sendQ. Calling Enqueue directly from
	// the sender would deadlock: Enqueue writes to the unbuffered sendQ, but no one
	// is reading it because the sender itself is blocked in the Enqueue call.
	// The goroutine writes to sendQ while the sender returns to its read loop.
	//
	// If connCtx is cancelled before the goroutine finishes, each Enqueue call will
	// take the connCtx.Done() branch and replyError with ErrNodeClosed, and
	// drainSendQ (deferred in sender) will drain any items that made it to sendQ.
	if len(requeue) > 0 {
		go func() {
			for _, req := range requeue {
				c.Enqueue(req)
			}
		}()
	}
}

// replyError sends err to the request's response channel if one is set.
// This is used for requests that have not yet been registered in responseRouters
// and therefore cannot be reached via routeResponse.
func (c *Channel) replyError(req Request, err error) {
	if req.ResponseChan != nil {
		req.deliver(response{NodeID: c.id, Err: err})
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

		// Register call in the response router only for calls that are genuinely
		// in-flight on the current stream, after all early-exit checks pass.
		if req.ResponseChan != nil {
			c.router.Register(req.Msg.GetMessageSeqNo(), req)
		}

		// Watch for per-request cancellation while Send is in-flight.
		// If req.Ctx is done before Send returns, clearStream unblocks
		// the blocked Send by cancelling the stream context and the
		// goroutine that wins the clear is also responsible for
		// requeueing/canceling pending requests.
		//
		// stop() is advisory only: it may return false if the callback
		// already started around the same time Send returned.
		stop := context.AfterFunc(req.Ctx, func() {
			if c.clearStream(stream) {
				c.requeuePendingMsgs()
			}
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
			c.router.RouteResponse(req.Msg.GetMessageSeqNo(), response{NodeID: c.id})
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

		msg, e := stream.Recv()
		if e != nil {
			c.setLastErr(e)
			// A stale receiver may observe an error after a newer stream has already
			// replaced this one. Only the goroutine that actually clears the current
			// stream may requeue pending requests.
			if c.clearStream(stream) {
				c.requeuePendingMsgs()
			}
			// Check for shutdown before attempting reconnection
			if c.connCtx.Err() != nil {
				// the node's close() method was called: exit receiver goroutine
				return
			}
		} else {
			err := msg.ErrorStatus()
			// Two-way calls (RPCCall/QuorumCall) deliver server response.
			if c.router.RouteResponse(msg.GetMessageSeqNo(), response{NodeID: c.id, Value: msg, Err: err}) {
				continue
			}
			// Server-initiated request — dispatch via registered handler.
			if rh, ok := c.router.RequestHandler(); ok {
				go c.dispatchRequest(rh, msg)
			}
		}
	}
}

// dispatchRequest handles a server-initiated request on the client side.
// It uses the RequestHandler to process the request and enqueues the response
// back through this channel.
func (c *Channel) dispatchRequest(rh RequestHandler, reqMsg *Message) {
	send := func(msg *Message) {
		c.Enqueue(Request{Ctx: c.connCtx, Msg: msg})
	}
	// We pass an empty release function since there is no server-side mutex on the client side.
	rh.HandleRequest(c.connCtx, reqMsg, func() {}, send)
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
