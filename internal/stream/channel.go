package stream

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// Channel carries requests to one node. It is implemented by
// [OutboundChannel], [InboundChannel], and [LocalChannel].
type Channel interface {
	// Enqueue sends req, reporting its outcome on req's response channel.
	// When the send queue is full, a two-way request fails with
	// [ErrSendQueueFull] and a one-way request waits for space until its
	// context ends. A closed stream channel fails req with [ErrNodeClosed].
	// Enqueue panics if req is both Oneway and Streaming.
	Enqueue(req Request)
	// StreamUp reports whether the channel can currently carry requests.
	StreamUp() bool
	// LastErr returns [ErrSendStalled] while a send has been blocked for
	// [stallReportDelay] or longer, and otherwise the error of the most recent
	// stream operation, or nil if it succeeded. After a refusal, it reverts to
	// nil only once the server accepts a new stream.
	LastErr() error
	// DroppedReplies returns the number of handler replies dropped because
	// the send queue was full or closed, or during a refusal backoff.
	DroppedReplies() int64
	// UnmatchedErrors returns the number of error replies from the peer that
	// no caller observes: replies that match no pending call, and replies to
	// calls whose contexts have ended. Each one is also logged if the channel
	// has a logger.
	UnmatchedErrors() int64
	// PendingCount returns the number of two-way calls awaiting responses.
	// The count may include calls whose callers are done but that have not
	// yet been removed.
	PendingCount() int
	// Close closes the channel and fails its queued and pending requests.
	Close() error
}

// RequestHandler is the interface that wraps the HandleRequest method.
//
// HandleRequest handles a request message from the stream and dispatches it
// to the method handler that the message's method field names. A channel
// calls HandleRequest for one request at a time, in arrival order, on a
// goroutine other than the one that receives from the stream.
//
// The senderID is the ID of the channel that received the request: the
// peer's ID on a stream, 0 if the peer has no ID, and the local node's own
// ID on a [LocalChannel].
//
// The release function is idempotent. Calling it lets the next request from
// the stream start before HandleRequest returns; otherwise the next request
// starts when HandleRequest returns.
//
// The send function must be non-nil and delivers a response message to the
// peer. On a stream, the send function does not wait for queue space, except
// on an [InboundChannel] created with [InboundOptions.WaitingReplies].
// For two-way call types, send may be called zero or more times (e.g., for
// streaming correctable calls). For one-way call types, the peer has no
// pending call to receive a response. The peer counts and logs an error
// response as described for [Channel.UnmatchedErrors], and drops any other
// response.
type RequestHandler interface {
	HandleRequest(ctx context.Context, senderID ID, msg *Message, release func(), send func(*Message))
}

// endpoint holds the state that a stream channel shares across its streams.
type endpoint struct {
	id       ID
	ctx      context.Context // the channel's lifetime
	cancel   context.CancelFunc
	queue    *sendQueue
	requests *dispatcher
	handler  RequestHandler // may be nil
	latency  *Latency       // may be nil
	lastErr  atomic.Pointer[error]

	// sendStart is when the send in progress began, in Unix nanoseconds, or 0
	// if there is none. A channel sends on one goroutine at a time.
	sendStart atomic.Int64

	// waitingReplies makes handler replies wait for queue space until the
	// channel closes, instead of being dropped when the queue is full.
	waitingReplies bool

	unmatchedErrors
}

// newEndpoint returns an endpoint that lives until parent ends or cancel is
// called. The recvBufferSize must be positive.
func newEndpoint(parent context.Context, id ID, sendBufferSize, recvBufferSize uint, handler RequestHandler, latency *Latency) endpoint {
	ctx, cancel := context.WithCancel(parent)
	return endpoint{
		id:       id,
		ctx:      ctx,
		cancel:   cancel,
		queue:    newSendQueue(id, sendBufferSize, ctx.Done()),
		requests: newDispatcher(ctx.Done(), recvBufferSize),
		handler:  handler,
		latency:  latency,
	}
}

// Enqueue implements [Channel.Enqueue].
func (e *endpoint) Enqueue(req Request) {
	if req.Oneway && req.Streaming {
		panic("gorums: Oneway and Streaming are mutually exclusive")
	}
	e.queue.push(req, !req.wantServerResponse())
}

// reply queues a handler's reply to the peer. Without waitingReplies, it does
// not wait for queue space, so receiving never waits on a reply.
func (e *endpoint) reply(msg *Message) {
	e.queue.push(Request{Ctx: e.ctx, Msg: msg}, e.waitingReplies)
}

// DroppedReplies implements [Channel.DroppedReplies].
func (e *endpoint) DroppedReplies() int64 {
	return e.queue.dropped.Load()
}

// unmatchedErrors counts the error replies from a channel's peer that no
// caller observes, and logs each one if it has a logger. The server replies
// with an error when a request names an unknown method or cannot be decoded,
// even for a one-way call, which has no pending call. An error reply to a
// two-way call whose context has ended is counted too, whether or not the call
// is still pending.
type unmatchedErrors struct {
	logger *log.Logger // may be nil
	count  atomic.Int64
}

// UnmatchedErrors implements [Channel.UnmatchedErrors].
func (u *unmatchedErrors) UnmatchedErrors() int64 {
	return u.count.Load()
}

// record counts msg, an error reply with err from node id, and logs it. It
// logs before it counts, so a reader that sees the count also sees the log.
func (u *unmatchedErrors) record(id ID, msg *Message, err error) {
	if u.logger != nil {
		u.logger.Printf("node %d: %s: error reply to message %d matches no pending call: %v",
			id, msg.GetMethod(), msg.GetMessageSeqNo(), err)
	}
	u.count.Add(1)
}

// LastErr implements [Channel.LastErr]. It reports the channel's current
// health, not the outcome of any one request: [ErrSendStalled] while a send
// has been blocked for [stallReportDelay] or longer, and otherwise the outcome
// of the latest stream operation.
func (e *endpoint) LastErr() error {
	if start := e.sendStart.Load(); start != 0 && time.Since(time.Unix(0, start)) >= stallReportDelay {
		return ErrSendStalled
	}
	if err := e.lastErr.Load(); err != nil {
		return *err
	}
	return nil
}

// recordHealth records err as the outcome of the latest stream operation.
// Record nil only for an operation that moved data.
func (e *endpoint) recordHealth(err error) {
	if err == nil {
		if e.lastErr.Load() != nil {
			e.lastErr.Store(nil)
		}
		return
	}
	e.lastErr.Store(&err)
}

// Compile-time assertions: each channel type implements [Channel].
var (
	_ Channel = (*OutboundChannel)(nil)
	_ Channel = (*InboundChannel)(nil)
	_ Channel = (*LocalChannel)(nil)
)
