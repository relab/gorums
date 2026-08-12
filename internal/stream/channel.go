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
	// node's send queue is at capacity. A full queue means the peer is not
	// draining sends (stopped reading, exhausted flow control); failing fast
	// lets quorum logic count the peer as failed instead of stalling the
	// caller behind it. One-way requests block instead (see Enqueue).
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
	router        *MessageRouter
	pendingOwner  *pendingOwner
	closeOnceFunc func() error

	// droppedReplies counts replies silently dropped by trySend: see DroppedReplies.
	droppedReplies atomic.Int64
}

// NewOutboundChannel creates a new channel for the given node and starts
// the sender and receiver goroutines.
//
// Note that we start both goroutines even though the connection and stream
// have not yet been established. This is to prevent deadlock when invoking
// a call type. The sender blocks on the sendQ and the receiver waits for
// the stream to become available.
//
// When eagerReconnect is set, the receiver re-establishes a lost stream
// proactively with capped exponential backoff instead of waiting for the next
// local send. Use this whenever a remote peer depends on this dialed stream
// staying registered on its inbound side. Any symmetric peer (a server calling
// its peers via WithPeers) drops out of the remote's connected
// configuration when the stream it dialed goes idle and dies. A stream lost
// while this side has nothing to send would otherwise leave the peer stalled
// until the next local send.
//
// onStreamChange, if non-nil, is invoked with true when the stream is
// established and false when it is lost, on transitions only. It runs while
// internal locks are held and must not call back into the Channel.
func NewOutboundChannel(parentCtx context.Context, id uint32, sendBufferSize uint, conn *grpc.ClientConn, router *MessageRouter, eagerReconnect bool, onStreamChange func(up bool)) *Channel {
	return newChannel(parentCtx, id, sendBufferSize, conn, nil, router, eagerReconnect, onStreamChange)
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
		pendingOwner:   new(pendingOwner),
		streamReady:    make(chan struct{}, 1),
		eagerReconnect: eagerReconnect,
		onStreamChange: onStreamChange,
	}
	c.closeOnceFunc = sync.OnceValue(func() error {
		// important to cancel first to stop goroutines
		connCancel()
		c.setStreamUp(false)
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
		id:           id,
		router:       router,
		pendingOwner: new(pendingOwner),
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

// IsOutbound returns true if this channel was created as an outbound client connection.
func (c *Channel) IsOutbound() bool {
	return c.conn != nil
}

// Close closes the channel and the underlying connection exactly once.
func (c *Channel) Close() error {
	return c.closeOnceFunc()
}

// ensureStream ensures there is an active NodeStream, signals the receiver
// that the stream is ready, and returns the ensured stream. The caller must
// use the returned stream rather than re-reading it with getStream: a
// concurrent clearStream — the receiver observing a broken stream, or a
// cancel watcher — can clear the stream between the two steps, and a request
// that never obtains a stream is failed without ever being registered for
// retry. Sending on the returned stream after such a clear fails instead with
// a stream error, after registration, so the request is requeued.
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

// ensureConnectedNodeStream returns the active NodeStream over a ready
// connection, creating a new stream if there is none.
// This method is safe for concurrent use.
func (c *Channel) ensureConnectedNodeStream() (BidiStream, error) {
	c.streamMut.Lock()
	defer c.streamMut.Unlock()
	// if we already have a ready connection and an active stream, do nothing
	if c.conn.GetState() == connectivity.Ready && c.stream != nil {
		return c.stream, nil
	}
	// Cancel any stream left behind by a previous attempt before replacing
	// it, so it does not stay alive server-side as an orphan.
	if c.streamCancel != nil {
		c.streamCancel()
	}
	c.streamCtx, c.streamCancel = context.WithCancel(c.connCtx)
	var err error
	c.stream, err = NewGorumsClient(c.conn).NodeStream(c.streamCtx)
	c.setStreamUp(c.stream != nil)
	return c.stream, err
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

// Enqueue adds the request to the send queue, blocking the caller if the
// queue is full.
//
// If it is a local channel, the request is dispatched in-process via
// the registered RequestHandler without touching the network.
// If the node is closed, it responds with an error instead.
//
// Two-way requests never wait here: a full queue means the peer is not
// draining sends, and failing fast with ErrSendQueueFull beats stalling every
// caller behind one slow peer (see [Channel.trySend]). One-way client calls
// (Unicast, Multicast) do wait: with no reply to await, backpressure on the
// caller is the only thing pacing the producer. Both wait points (here and at
// the sender's dequeue) honor the request's context, so a bounded or
// cancellable context still releases the caller; a context with no deadline
// can block indefinitely behind a peer that stopped draining.
//
// Enqueue must never be used for a reply sent from a receive/dispatch loop —
// use [Channel.trySend] instead. See [Channel.dispatchInbound].
//
// Requests cannot combine Oneway and Streaming; they are mutually exclusive:
//   - one-way calls (Unicast, Multicast) do not expect server responses.
//   - streaming (correctable) calls expect multiple server responses and
//     require the router entry to stay alive for the duration of the stream.
//
// Combining them would cause double delivery on the response channel.
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
		// The request's own context was cancelled while waiting for queue space.
		// Without this case a caller could block here indefinitely behind a peer
		// that stopped reading; the sender applies the same check when it dequeues,
		// so both wait points honor the request context.
		req.ReplyError(c.id, req.Ctx.Err())
	case c.sendQ <- req:
		// enqueued successfully
	}
}

// TrySend is [Channel.trySend] exported for callers outside this package —
// currently the server-side inbound reply path; see [PeerNode.TrySend]. Like
// trySend, it never blocks on network I/O; unlike trySend, a local channel's
// in-process dispatch can briefly block acquiring the dispatch lock.
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
// Two callers rely on this never blocking: two-way requests (see [Channel.Enqueue]
// for why a full queue should fail fast rather than stall the caller), and
// replies sent from a receive/dispatch loop, which must keep reading inbound
// frames and would deadlock if a reply blocked instead — see
// [Channel.dispatchInbound] for the client-side case and [Server.NodeStream]
// for the server-side case.
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

// cancelInflightSend is the sender's per-request cancel watcher: it clears
// the stream to unblock a Send that the request's canceled context would
// otherwise leave blocked forever (a Send stalled by flow control returns
// only when its stream dies), requeueing the pending requests stranded on the
// cleared stream.
//
// sendDone — set by the sender under sendGuard once Send returns — makes a
// watcher that runs late a no-op. The caller may cancel its context the
// moment it has the response, landing the cancellation between Send returning
// and the sender's stop call, and the watcher goroutine spawned by that
// cancellation may then run arbitrarily late; with nothing left to unblock,
// clearing would sever a healthy stream that later requests depend on.
//
// One narrow window remains: between Send returning and the sender acquiring
// sendGuard to set sendDone, a watcher can win the guard, observe sendDone
// still false, and clear a stream whose Send already completed. This is
// accepted rather than closed because it is self-healing and strands nothing:
// the requeued requests retry, and the stream is re-established on the next
// send (immediately when eager reconnect is set). The cost is a spurious
// reconnect, not lost or misrouted traffic.
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
// entries that no longer fit are failed with ErrSendQueueFull rather than
// retried. If the node closed meanwhile, Enqueue replies ErrNodeClosed and
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
// sendQ must never be closed: closing it could panic a concurrent Enqueue
// that passes the outer connCtx check and then sends on a closed channel.
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

		// Watch for per-request cancellation while Send is in flight: a Send
		// blocked by flow control returns only when its stream dies, so the
		// watcher unblocks it by clearing the stream. sendDone, set under
		// sendGuard once Send returns, neutralizes a watcher that fires late:
		// the caller may cancel its context the moment the response arrives —
		// before this goroutine resumes to call stop — and the watcher
		// goroutine spawned by that cancellation may then run arbitrarily
		// late; see [Channel.cancelInflightSend].
		var sendDone bool
		stop := context.AfterFunc(req.Ctx, func() {
			c.cancelInflightSend(&sendDone, stream)
		})
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
// itself, with capped exponential backoff, instead of leaving reconnection to
// the sender's next request: a symmetric peer depends on this dialed stream to
// stay registered on its inbound side, so on a node with nothing to send the
// peer would otherwise stall until this node's next request.
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
		}
	}
}

// pauseReconnect waits out the current eager-reconnect backoff delay before the
// next redial attempt, then doubles the delay up to the cap. It returns early
// without growing the delay if the sender re-established the stream during the
// wait, and returns false only when the node closed, signaling the receiver to
// exit.
func (c *Channel) pauseReconnect(delay *time.Duration) bool {
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

// dispatchInbound routes one message received by the receiver loop: it delivers
// responses to pending calls and dispatches server-initiated back-channel
// requests to the handler. Stale (cancelled) calls are silently dropped.
//
// A back-channel handler's reply is sent via [Channel.trySend], never the
// blocking [Channel.Enqueue]. The handler runs while holding the router's
// dispatch lock, and this same receiver goroutine must keep reading inbound
// frames; if the reply blocked on a full send queue, the handler would never
// return, the lock would never release, and the receiver would stop making
// progress.
func (c *Channel) dispatchInbound(msg *Message) {
	c.router.RouteMessage(c.connCtx, c.id, msg, c.trySend)
}

// recordHealth records the outcome of a stream operation as this channel's
// [Channel.LastErr]: a non-nil err replaces it, a nil err clears it. Call it
// only for operations that move data, a completed send or a received frame;
// establishing a stream does not prove the channel usable.
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
