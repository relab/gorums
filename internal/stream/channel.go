package stream

import (
	"cmp"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
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
	// [Channel.Enqueue]).
	ErrSendQueueFull = status.Error(codes.Unavailable, "send queue full")
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
	Oneway       bool
	ResponseChan chan<- response
	SendTime     time.Time
}

// wantServerResponse returns true if the request expects an actual
// server response and needs a router entry. It returns true for
// two-way calls (RPC, QuorumCall) and streaming calls (correctable).
func (r Request) wantServerResponse() bool {
	return r.ResponseChan != nil && !r.Oneway
}

// wantSendConfirmation returns true if the request needs send confirmation
// delivered directly on its ResponseChan, bypassing the router. It returns
// true for one-way calls (Unicast, Multicast), whose callers await the
// confirmation to learn whether the send succeeded. The nil check guards
// against delivering to a nil channel, which blocks until the request's
// context expires.
func (r Request) wantSendConfirmation() bool {
	return r.Oneway && r.ResponseChan != nil
}

// deliver sends the response on request's response channel, preferring delivery
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

// ReplyError sends err to the request's response channel if one is set.
// It is exported so callers outside this package can fail a request that
// never reaches a channel (e.g., a node with no attached channel).
func (r Request) ReplyError(nodeID uint32, err error) {
	if r.ResponseChan != nil {
		r.deliver(response{NodeID: nodeID, Err: err})
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

	// draining is the stream whose connection has received a server GOAWAY;
	// it is retired when no call is pending on it. isDraining mirrors
	// draining != nil so the receiver can test it without taking streamMut.
	draining   BidiStream
	isDraining atomic.Bool

	// eagerReconnect makes the receiver re-establish a lost stream proactively
	// instead of waiting for the next local send; see [NewOutboundChannel].
	eagerReconnect bool

	// streamUp mirrors whether the outbound stream is currently established,
	// so [Channel.StreamUp] can answer without taking streamMut. Maintained by
	// setStreamUp on every stream transition; always false for inbound and
	// local channels, which report their state structurally instead.
	streamUp atomic.Bool

	// onStreamChange, if non-nil, is invoked on every outbound stream
	// transition with the new state; see [NewOutboundChannel]. It is called
	// while internal locks are held, so it must not call back into the
	// Channel; use it only to signal or record the state elsewhere.
	onStreamChange func(up bool)

	// sendGuard serializes each request's post-Send bookkeeping in the sender
	// against that request's cancel watcher, so the watcher can distinguish a
	// Send still in flight (which it must unblock by clearing the stream) from
	// one that has already returned (the stream is healthy and must be left
	// alone); see the sender loop.
	sendGuard sync.Mutex

	// Router handles response routing for pending calls. It is owned by the
	// Node and injected into the Channel, so it survives channel replacement.
	router *MessageRouter
	// requests orders back-channel handlers for this channel. Its lifetime is
	// connCtx, which outlives individual stream reconnects.
	requests      *requestDispatch
	pendingOwner  *pendingOwner
	closeOnceFunc func() error

	// droppedReplies counts replies silently dropped by trySend: see DroppedReplies.
	droppedReplies atomic.Int64
}

// NewOutboundChannel creates a new channel for the given node and starts
// the sender, receiver, and request-dispatcher goroutines.
//
// Note that we start both goroutines even though the connection and stream
// have not yet been established. This is to prevent deadlock when invoking
// a call type. The sender blocks on the sendQ and the receiver waits for
// the stream to become available.
//
// When eagerReconnect is set, the receiver re-establishes a lost stream with
// capped exponential backoff, independently of local sends. Set it whenever a
// remote peer depends on this dialed stream staying registered on its inbound
// side: a symmetric peer (a server calling its peers via WithPeers) keeps this
// node in its connected configuration only while that stream is up, and under
// stream deduplication the peer also sends its own calls on this stream and
// cannot dial it itself. Eager reconnect keeps the peer reachable while this
// side has nothing to send.
//
// onStreamChange, if non-nil, is invoked with true when the stream is
// established and false when it is lost, on transitions only. It runs while
// internal locks are held and must not call back into the Channel.
func NewOutboundChannel(parentCtx context.Context, id uint32, sendBufferSize uint, conn *grpc.ClientConn, router *MessageRouter, eagerReconnect bool, onStreamChange func(up bool)) *Channel {
	return newChannel(parentCtx, id, sendBufferSize, conn, nil, router, eagerReconnect, onStreamChange)
}

// NewInboundChannel creates a channel from an existing server-side stream.
// The sender and request-dispatcher goroutines are started. No receiver
// goroutine is launched.
//
// Receiving from the stream is left to the caller's goroutine (e.g., the Recv
// loop of [Server.NodeStream]), which is the sole reader: it separates
// responses to pending calls from new incoming requests and routes both.
//
// Unlike outbound channels, inbound channels:
//   - Have no receiver goroutine (NodeStream's Recv loop is the sole reader)
//   - Have no grpc.ClientConn (stream accepted by the gRPC server; not dialed by us)
//   - Cannot reconnect (the client controls stream creation)
//   - Close only cancels context; it does not close the underlying connection
func NewInboundChannel(parentCtx context.Context, id uint32, sendBufferSize uint, stream BidiStream, router *MessageRouter) *Channel {
	return newChannel(parentCtx, id, sendBufferSize, nil, stream, router, false, nil)
}

// newChannel is the shared constructor for outbound and inbound channels.
// Pass a non-nil conn for outbound channels (conn.Close() is called on Close()).
// Pass a non-nil stream for inbound channels (stream is immediately ready; no reconnection).
// The receiver goroutine is started only for outbound channels; inbound callers own
// the stream's read side themselves (see NewInboundChannel for the full rationale).
func newChannel(parentCtx context.Context, id uint32, sendBufferSize uint, conn *grpc.ClientConn, stream BidiStream, router *MessageRouter, eagerReconnect bool, onStreamChange func(up bool)) *Channel {
	connCtx, connCancel := context.WithCancel(parentCtx)
	c := &Channel{
		sendQ:          make(chan Request, sendBufferSize),
		id:             id,
		conn:           conn,
		stream:         stream,
		connCtx:        connCtx,
		connCancel:     connCancel,
		router:         router,
		requests:       newRequestDispatch(defaultRequestDispatchSize),
		pendingOwner:   new(pendingOwner),
		streamReady:    make(chan struct{}, 1),
		eagerReconnect: eagerReconnect,
		onStreamChange: onStreamChange,
	}
	c.closeOnceFunc = sync.OnceValue(func() error {
		// important to cancel first to stop goroutines
		connCancel()
		c.setStreamUp(false)
		// unblocks any pending senders/receivers; an inbound channel closes
		// only with its stream, so its pending calls see the stream drop
		if c.IsInbound() {
			c.cancelPendingMsgs(ErrStreamDown)
		} else {
			c.cancelPendingMsgs(ErrNodeClosed)
		}
		if conn != nil {
			return conn.Close()
		}
		return nil
	})
	if stream != nil {
		// Signal that stream is immediately ready (inbound channel).
		c.streamReady <- struct{}{}
	}
	go c.requests.run(c.connCtx)
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
		id:           id,
		router:       router,
		pendingOwner: new(pendingOwner),
	}
	c.closeOnceFunc = sync.OnceValue(func() error { return nil })
	return c
}

// isLocal reports whether this channel dispatches in-process.
func (c *Channel) isLocal() bool {
	// The nil sendQ is the discriminator: all outbound and inbound channels always
	// allocate a sendQ via make(chan Request, ...) in newChannel.
	return c.sendQ == nil
}

// IsInbound reports whether this channel was created from a server-side stream.
func (c *Channel) IsInbound() bool {
	return c.conn == nil && c.sendQ != nil
}

// IsOutbound returns true if this channel was created as an outbound client connection.
func (c *Channel) IsOutbound() bool {
	return c.conn != nil
}

// Close closes the channel and the underlying connection exactly once.
func (c *Channel) Close() error {
	return c.closeOnceFunc()
}

// ensureStream ensures there is an active NodeStream, signals the receiver
// that the stream is ready, and returns the ensured stream. The caller sends on
// the returned stream. A concurrent [Channel.clearStream] can clear the
// channel's stream at any time; a send on the returned stream after such a
// clear fails with a stream error after the request is registered, so the
// request is requeued.
// gRPC automatically handles TCP connection state when creating the stream.
// This method is safe for concurrent use.
func (c *Channel) ensureStream() (BidiStream, error) {
	if c.IsInbound() {
		// Inbound channels cannot reconnect; just check if stream exists.
		if stream := c.getStream(); stream != nil {
			return stream, nil
		}
		return nil, ErrStreamDown
	}
	stream, err := c.ensureConnectedNodeStream()
	if err != nil {
		return nil, err
	}
	// signal receiver that stream is ready (non-blocking)
	select {
	case c.streamReady <- struct{}{}:
	default:
		// channel already has a signal pending, no need to add another
	}
	return stream, nil
}

// ensureConnectedNodeStream returns the channel's NodeStream, creating one if
// the channel has none. The receive path clears a stream that has ended and
// requeues its pending calls, and [Channel.retireOnGoAway] clears a stream on
// a draining connection. This method is safe for concurrent use.
func (c *Channel) ensureConnectedNodeStream() (BidiStream, error) {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	if c.stream != nil {
		return c.stream, nil
	}
	c.streamCtx, c.streamCancel = context.WithCancel(c.connCtx)
	stream, err := NewGorumsClient(c.conn).NodeStream(c.streamCtx)
	if err != nil {
		c.streamCancel()
		return nil, err
	}
	c.stream = stream
	c.setStreamUp(true)
	go c.retireOnGoAway(stream, c.streamCtx)
	return stream, nil
}

// retireOnGoAway marks stream as draining once its connection leaves Ready,
// which is how the client observes a server GOAWAY, and retires it once no
// call is pending on it. Pending calls thus complete on the stream that
// carries them, and the server's graceful stop or connection-age drain can
// finish. It returns when ctx, the stream's context, ends.
func (c *Channel) retireOnGoAway(stream BidiStream, ctx context.Context) {
	for c.conn.GetState() == connectivity.Ready {
		if !c.conn.WaitForStateChange(ctx, connectivity.Ready) {
			return
		}
	}
	c.streamMut.Lock()
	if c.stream == stream {
		c.draining = stream
		c.isDraining.Store(true)
	}
	c.streamMut.Unlock()
	c.retireDrained()
}

// retireDrained clears the draining stream, if any, when no call is pending
// on the channel, and requeues calls registered concurrently with the clear.
// The next send opens a stream on a new connection.
func (c *Channel) retireDrained() {
	c.streamMut.Lock()
	stream := c.draining
	c.streamMut.Unlock()
	if stream == nil || c.router.pendingCount(c.pendingOwner) > 0 {
		return
	}
	if c.clearStream(stream) {
		c.requeuePendingMsgs()
	}
}

// getStream returns the current stream, or nil if no stream is available.
func (c *Channel) getStream() BidiStream {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	return c.stream
}

// clearStream cancels the stream context of stale and clears the stream
// reference if stale is still the current stream, and reports whether it did.
// A stream that has replaced stale is left intact, together with the requests
// that belong to it. The next send opens a new stream.
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
	c.draining = nil
	c.isDraining.Store(false)
	c.setStreamUp(false)
	return true
}

// setStreamUp records the outbound stream's availability and invokes the
// registered onStreamChange callback on transitions only. The compare-and-swap
// makes repeated same-state calls no-ops, so callers may invoke it
// unconditionally after each stream mutation.
func (c *Channel) setStreamUp(up bool) {
	if c.streamUp.CompareAndSwap(!up, up) && c.onStreamChange != nil {
		c.onStreamChange(up)
	}
}

// StreamUp reports whether the channel can currently carry requests, without
// taking locks: local channels always can, inbound channels can for as long
// as they exist (they are discarded when their stream ends), and outbound
// channels can while their stream is established.
func (c *Channel) StreamUp() bool {
	if c.isLocal() || c.IsInbound() {
		return true
	}
	return c.streamUp.Load()
}

// Enqueue adds the request to the send queue. A local channel dispatches it
// in-process, and a closed channel replies [ErrNodeClosed]. When the queue is
// full, a two-way request fails fast with [ErrSendQueueFull], while a one-way
// request waits for space until its context ends, which paces the producer.
// Replies sent from a receive or dispatch loop use [Channel.trySend].
// Enqueue panics if req is both Oneway and Streaming.
func (c *Channel) Enqueue(req Request) {
	if req.Oneway && req.Streaming {
		panic("gorums: Oneway and Streaming are mutually exclusive")
	}
	if c.isLocal() {
		c.router.DispatchLocalRequest(c.id, req)
		return
	}
	// Two-stage select: the outer non-blocking check catches the already-closed
	// case deterministically. Go's select only falls through to default when no
	// other case is ready, so if connCtx.Done() is already closed it always
	// wins — unlike a plain single select, where Go randomly picks between a
	// ready Done channel and a buffered sendQ.
	// The inner selects handle the case where the node closes concurrently
	// while we are waiting for sendQ space; there a narrow race remains, but
	// drainSendQ (deferred in sender) will drain and ReplyError any entry that
	// slips through after sender exits.
	select {
	case <-c.connCtx.Done():
		// the node's close() method was called: respond with error instead of enqueueing
		req.ReplyError(c.id, ErrNodeClosed)
		return
	default:
	}
	if req.wantServerResponse() {
		// Two-way request: never wait for queue space.
		c.trySend(req)
		return
	}
	select {
	case <-c.connCtx.Done():
		// the node's close() method was called: respond with error instead of enqueueing
		req.ReplyError(c.id, ErrNodeClosed)
	case <-req.Ctx.Done():
		// The request's own context ended while waiting for queue space. The
		// sender checks the context again when it dequeues, so both wait points
		// honor the request context.
		req.ReplyError(c.id, req.Ctx.Err())
	case c.sendQ <- req:
		// enqueued successfully
	}
}

// TrySend enqueues req without waiting for queue space, as [Channel.trySend]
// does. A local channel dispatches req in-process, which can briefly wait for
// the router's dispatch lock.
func (c *Channel) TrySend(req Request) {
	if c.isLocal() {
		c.router.DispatchLocalRequest(c.id, req)
		return
	}
	c.trySend(req)
}

// trySend enqueues req without ever blocking the caller: if the node has
// closed it replies ErrNodeClosed, and if the send queue is full it replies
// ErrSendQueueFull instead of waiting for space. A request with no
// ResponseChan (a back-channel reply) is simply dropped when the queue is
// full, since there is no channel to deliver the error on; each such drop is
// counted (see [Channel.DroppedReplies]).
//
// Two callers depend on it never blocking: two-way requests from
// [Channel.Enqueue], and replies sent from a receive or dispatch loop, which
// keeps reading inbound frames while the reply is queued; see
// [Channel.dispatchInbound] for the client side and [Server.NodeStream] for
// the server side.
func (c *Channel) trySend(req Request) {
	// Deterministic already-closed check: see the equivalent select in Enqueue.
	select {
	case <-c.connCtx.Done():
		if req.ResponseChan == nil {
			c.droppedReplies.Add(1)
		}
		req.ReplyError(c.id, ErrNodeClosed)
		return
	default:
	}
	select {
	case c.sendQ <- req:
		// enqueued successfully
	default:
		if req.ResponseChan == nil {
			c.droppedReplies.Add(1)
		}
		req.ReplyError(c.id, ErrSendQueueFull)
	}
}

// DroppedReplies returns the number of replies this channel has silently
// dropped: requests with no ResponseChan (back-channel or inbound replies
// dispatched from a receive/dispatch loop) that [Channel.trySend] could not
// enqueue because the node had closed or the send queue was full. Two-way
// requests are never counted here, since their caller already observes the
// failure directly via ErrSendQueueFull or ErrNodeClosed.
func (c *Channel) DroppedReplies() int64 {
	return c.droppedReplies.Load()
}

// cancelPendingMsgs cancels this channel's pending messages by sending an
// error response to each.
func (c *Channel) cancelPendingMsgs(err error) {
	for _, req := range c.router.cancelPending(c.pendingOwner) {
		req.ReplyError(c.id, err)
	}
}

// cancelInflightSend is the sender's per-request cancel watcher on an
// outbound channel. It clears stream, which ends a Send blocked by flow
// control, and requeues the requests pending on it.
//
// The sender sets sendDone under sendGuard once Send returns, which makes a
// watcher that runs after that point a no-op: a caller that cancels its
// context on receiving the response leaves the stream intact. A watcher that
// takes sendGuard after Send returns but before sendDone is set clears the
// stream; its requeued requests retry on a new stream, opened by the next
// send or by eager reconnect.
func (c *Channel) cancelInflightSend(sendDone *bool, stream BidiStream) {
	c.sendGuard.Lock()
	defer c.sendGuard.Unlock()
	if *sendDone {
		return
	}
	if c.clearStream(stream) {
		c.requeuePendingMsgs()
	}
}

// requeuePendingMsgs moves pending non-streaming requests back to sendQ for
// retry on the next stream. Streaming requests (correctable calls) are cancelled
// with ErrStreamDown because they cannot be safely retried.
//
// Only two-way requests are registered in the router, so every requeued entry
// takes Enqueue's non-blocking fail-fast path. Calling Enqueue directly from
// the sender goroutine (the sole sendQ reader) therefore cannot deadlock;
// entries that do not fit are failed with [ErrSendQueueFull]. If the node closed meanwhile, Enqueue replies ErrNodeClosed and
// drainSendQ (deferred in sender) drains any entries that slipped through.
func (c *Channel) requeuePendingMsgs() {
	requeue, cancel := c.router.requeuePending(c.pendingOwner)
	for _, req := range cancel {
		req.ReplyError(c.id, ErrStreamDown)
	}
	for _, req := range requeue {
		c.Enqueue(req)
	}
}

// drainSendQ is deferred in sender() and drains any remaining requests from
// sendQ when the sender goroutine exits, replying to each with ErrNodeClosed.
// This handles both requests already in the queue and any that slip through
// the narrow race window in Enqueue after connCtx is cancelled.
// sendQ stays open for the channel's lifetime, since a concurrent Enqueue can
// pass the outer connCtx check and then send on it.
func (c *Channel) drainSendQ() {
	for {
		select {
		case req := <-c.sendQ:
			req.ReplyError(c.id, ErrNodeClosed)
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
//   - Pre-registration exits (stream ensure error, cancelled request context):
//     ReplyError + continue. The request never enters the router.
//   - Send failure: requeuePendingMsgs handles registered two-way entries (requeue or cancel).
//     One-way errors are delivered directly via ReplyError.
//   - Send success, one-way call: confirm send directly on ResponseChan.
//   - Send success, two-way call: the router entry stays alive for receiver()
//     to deliver the actual server response.
func (c *Channel) sender() {
	defer c.drainSendQ()

	// eager connect; ignored if stream is down (will be retried on send)
	_, _ = c.ensureStream()

	var req Request
	for {
		select {
		case <-c.connCtx.Done():
			// the node's close() method was called: exit sender goroutine
			return
		case req = <-c.sendQ:
			// take next request from sendQ
		}

		stream, err := c.ensureStream()
		if err != nil {
			// Failing to reach the peer is a fact about the node, not only
			// about this request: record it for [Channel.LastErr] before
			// reporting it to the caller.
			c.recordHealth(err)
			req.ReplyError(c.id, err)
			continue
		}
		if req.Ctx.Err() != nil {
			req.ReplyError(c.id, req.Ctx.Err())
			continue
		}

		// One-way calls bypass the router and confirm directly after Send below.
		if req.wantServerResponse() {
			// Register only for two-way/streaming calls that expect server responses.
			c.router.register(c.pendingOwner, req.Msg.GetMessageSeqNo(), req)
		}

		// Watch for per-request cancellation while Send is in flight; see
		// [Channel.cancelInflightSend]. An inbound channel has no watcher,
		// since it cannot open a replacement stream.
		var sendDone bool
		stop := func() bool { return false }
		if !c.IsInbound() {
			stop = context.AfterFunc(req.Ctx, func() {
				c.cancelInflightSend(&sendDone, stream)
			})
		}
		err = stream.Send(req.Msg)
		c.sendGuard.Lock()
		sendDone = true
		c.sendGuard.Unlock()
		stop()
		// A completed send proves the channel usable; a failed one condemns it.
		c.recordHealth(err)
		if err != nil {
			c.clearStream(stream)
			c.requeuePendingMsgs() // handles registered two-way entries
			// One-way calls are not registered in the router to receive server responses,
			// so requeuePendingMsgs won't handle them. Deliver error directly to caller.
			if !req.wantServerResponse() {
				// prefer context error when cancellation caused the failure.
				req.ReplyError(c.id, cmp.Or(req.Ctx.Err(), err))
			}
			continue
		}

		// For one-way calls, confirm successful send directly (no router round-trip).
		if req.wantSendConfirmation() {
			req.deliver(response{NodeID: c.id})
		}
	}
}

// eagerReconnectBaseDelay and eagerReconnectMaxDelay pace the receiver's
// redial loop between failed attempts when eager reconnection is enabled
// (see [NewOutboundChannel]). The delay doubles per failed attempt from the
// base to the cap; the underlying gRPC connection's own dial backoff paces
// actual TCP connection attempts underneath.
const (
	eagerReconnectBaseDelay = 50 * time.Millisecond
	eagerReconnectMaxDelay  = 2 * time.Second
)

// receiver goroutine receives messages from the stream and routes them to
// the appropriate response router. If the stream goes down, it clears the
// stream reference and requeues pending requests for retry on a new stream.
//
// With eagerReconnect set, the receiver also re-establishes a lost stream
// itself, with capped exponential backoff; see [NewOutboundChannel].
func (c *Channel) receiver() {
	reconnectDelay := eagerReconnectBaseDelay
	for {
		stream := c.getStream()
		if stream == nil {
			if !c.eagerReconnect {
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
			if c.connCtx.Err() != nil {
				// the node's close() method was called: exit receiver goroutine
				return
			}
			if _, err := c.ensureStream(); err != nil {
				// The sender records a failed stream creation only when it has
				// a request to send. This loop redials on a timer instead, so
				// while the caller sends nothing to this node it is the only
				// place the peer's unreachability is observed.
				c.recordHealth(err)
				// Creating the stream failed; pace the next attempt. Do not
				// reset the backoff merely because creation later succeeds — a
				// stream must demonstrate viability first (see below).
				if !c.pauseReconnect(&reconnectDelay) {
					return
				}
			}
			continue
		}

		streamStart := time.Now()
		msg, e := stream.Recv()
		// A received frame proves the channel usable; a broken receive condemns it.
		c.recordHealth(e)
		if e != nil {
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
			if c.eagerReconnect {
				// A newly created stream that the server immediately rejects
				// bypasses the ensureStream error path above: creation succeeds,
				// then this Recv fails at once. Pace those redials too, so a
				// server that rejects every stream cannot spin this loop. A
				// stream that stayed up past the reconnect cap has proven
				// viable, so reset the backoff before pacing the next attempt.
				if time.Since(streamStart) >= eagerReconnectMaxDelay {
					reconnectDelay = eagerReconnectBaseDelay
				}
				if !c.pauseReconnect(&reconnectDelay) {
					return
				}
			}
		} else {
			// A received frame proves the stream is viable: reset the backoff.
			reconnectDelay = eagerReconnectBaseDelay
			c.dispatchInbound(msg)
			if c.isDraining.Load() {
				c.retireDrained()
			}
		}
	}
}

// pauseReconnect waits out the current eager-reconnect backoff delay before the
// next redial attempt, then doubles the delay up to the cap. It returns early
// without growing the delay if the sender re-established the stream during the
// wait, and returns false only when the node closed, signaling the receiver to
// exit.
func (c *Channel) pauseReconnect(delay *time.Duration) bool {
	// A stream already in place is ready to read. The backoff applies only
	// while the channel has none.
	if c.getStream() != nil {
		return true
	}
	// Drain a stale readiness signal left by our own ensureStream so it cannot
	// satisfy the wait instantly; only a signal delivered during the wait — the
	// sender re-establishing the stream — should shorten the backoff.
	select {
	case <-c.streamReady:
	default:
	}
	timer := time.NewTimer(*delay)
	defer timer.Stop()
	select {
	case <-c.streamReady:
		// the sender re-established the stream; retry without growing the delay
	case <-timer.C:
		*delay = min(2*(*delay), eagerReconnectMaxDelay)
	case <-c.connCtx.Done():
		return false
	}
	return true
}

// dispatchInbound routes one message received by the receiver loop.
// Responses to pending calls are delivered here. Server-initiated requests
// are appended to the channel's request queue, and the dispatcher starts the
// next one only after the previous handler calls release or returns. This
// method waits only when that queue is full. Stale (cancelled) calls are
// silently dropped.
//
// A back-channel handler's reply is sent via [Channel.trySend], never the
// blocking [Channel.Enqueue]. The handler can then release while the send
// queue is full, and the dispatcher can start the next request.
func (c *Channel) dispatchInbound(msg *Message) {
	if isServerSequenceNumber(msg.GetMessageSeqNo()) {
		c.requests.enqueue(c.connCtx, func(release func()) {
			c.dispatchBackChannel(msg, release)
		})
		return
	}
	c.router.RouteMessage(c.connCtx, c.id, msg, c.trySend)
}

// dispatchBackChannel runs one server-initiated request. release is the
// dispatcher's callback; the handler calls it to let the next request start.
func (c *Channel) dispatchBackChannel(msg *Message, release func()) {
	handler := c.router.handler
	if handler == nil {
		release()
		return
	}
	send := func(reply *Message) {
		c.trySend(Request{Ctx: c.connCtx, Msg: reply})
	}
	handler.HandleRequest(msg.AppendToIncomingContext(c.connCtx), msg, release, send)
}

// recordHealth records the outcome of a stream operation as this channel's
// [Channel.LastErr]: a non-nil err replaces it, a nil err clears it. A nil
// err comes only from an operation that moves data, a completed send or a
// received frame, since establishing a stream does not prove the channel
// usable; a failed stream creation is recorded as an error.
func (c *Channel) recordHealth(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err
}

// LastErr returns the last error encountered (if any) when using this channel:
// a stream that could not be established, a failed send, or a broken receive.
// It reports the channel's current health, not the outcome of any one request:
// it is last-write-wins across concurrent requests and reverts to nil once
// traffic flows again.
func (c *Channel) LastErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}
