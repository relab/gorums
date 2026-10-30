package stream

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// eagerReconnectBaseDelay and eagerReconnectMaxDelay bound the delay between
// eager reconnect attempts. The delay doubles per failed attempt; the gRPC
// connection's own dial backoff paces the TCP connection attempts underneath.
const (
	eagerReconnectBaseDelay = 50 * time.Millisecond
	eagerReconnectMaxDelay  = 2 * time.Second
)

// viableStreamAge is how long a stream that delivered no frame must live
// before it counts as viable.
const viableStreamAge = 2 * time.Second

// PeerVerifier reports whether the remote end of the stream with context ctx
// is the node with ID id. It returns nil to accept the stream, and an error
// that explains the refusal otherwise. ctx is the stream's context, which
// carries the connection's [google.golang.org/grpc/peer.Peer] and, on a
// server, the stream's incoming metadata. A PeerVerifier must be safe for
// concurrent use. It should return promptly, and return when ctx ends: on a
// dialing node it runs on the channel's goroutine, so a verifier that blocks
// delays the channel's sends and its Close.
type PeerVerifier func(ctx context.Context, id ID) error

// OutboundOptions configures an [OutboundChannel].
type OutboundOptions struct {
	// SendBufferSize is the send queue capacity.
	SendBufferSize uint
	// Handler serves requests the server sends on the stream; may be nil.
	Handler RequestHandler
	// Latency receives round-trip samples; may be nil.
	Latency *Latency
	// EagerReconnect makes the channel re-establish a lost stream with capped
	// exponential backoff, instead of on the next request. It also opens the
	// next stream when the backoff after a refusal ends.
	EagerReconnect bool
	// OnStreamChange, if non-nil, is called on the channel's goroutine each
	// time the stream comes up or goes down; [OutboundChannel.StreamUp]
	// reports the new state.
	OnStreamChange func()
	// Verify, if non-nil, checks the remote end of each new stream against
	// the channel's ID before the stream carries requests.
	Verify PeerVerifier
}

// OutboundChannel is a [Channel] over a stream that this node opens to a
// server. It opens the stream when it is created. If the stream fails, for
// example because the connection breaks or the server stops, the channel
// opens a new stream; [OutboundOptions.EagerReconnect] controls how soon.
//
// Calls still awaiting responses when the stream fails are sent again on the
// new stream. Calls that do not fit in the send queue fail with
// [ErrSendQueueFull]. Streaming calls fail with [ErrStreamDown] instead. They
// are not sent again, since they may already have received responses.
// A request rejected by the local send-size check fails with
// codes.ResourceExhausted and is not sent again.
//
// A server that shuts down gracefully first tells the client to stop using
// the connection (a GOAWAY). The channel then sends new calls on a new
// stream. Calls already sent on the old stream complete there, and the old
// stream ends once it has no calls left.
//
// A stream the server refuses ends with codes.Unauthenticated,
// codes.PermissionDenied, codes.InvalidArgument, or codes.Unimplemented
// before delivering a frame. Calls on it fail with that status and are not
// sent again. The channel then backs off before opening a new stream, and
// requests fail with the refusal in the meantime. A stream that fails
// [OutboundOptions.Verify] is treated the same way.
type OutboundChannel struct {
	endpoint
	conn           *grpc.ClientConn
	eagerReconnect bool
	onStreamChange func()
	verify         PeerVerifier
	streamUp       atomic.Bool

	mu       sync.Mutex
	sessions map[*session]struct{} // sessions still receiving; two during a GOAWAY handover

	wg        sync.WaitGroup
	closeOnce func() error
}

// NewOutboundChannel returns a channel to the server behind conn and starts
// its goroutine. The channel lives until ctx ends or it is closed; closing it
// also closes conn.
func NewOutboundChannel(ctx context.Context, id ID, conn *grpc.ClientConn, opts OutboundOptions) *OutboundChannel {
	c := &OutboundChannel{
		endpoint:       newEndpoint(ctx, id, opts.SendBufferSize, 0, opts.Handler, opts.Latency),
		conn:           conn,
		eagerReconnect: opts.EagerReconnect,
		onStreamChange: opts.OnStreamChange,
		verify:         opts.Verify,
		sessions:       make(map[*session]struct{}),
	}
	c.closeOnce = sync.OnceValue(func() error {
		c.cancel()
		c.queue.close()
		c.wg.Wait()
		return c.conn.Close()
	})
	c.wg.Go(c.run)
	return c
}

// run opens streams and sends queued requests on them until the channel
// closes.
func (c *OutboundChannel) run() {
	defer c.queue.close()
	b := newBackoff(c.eagerReconnect)
	var req *Request // taken from the queue, not yet sent
	connect := true  // open a stream without waiting for a request or timer
	for {
		if !connect {
			var ok bool
			if req, ok = c.await(&b); !ok {
				return
			}
		}
		if c.ctx.Err() != nil {
			c.failRequest(req, ErrNodeClosed)
			return
		}
		b.attempt()

		// Open a stream; on failure, fail the request in hand and back off.
		s, err := c.open(b.suspect())
		if err != nil {
			c.streamFailed(&b, req, err)
			req, connect = nil, false
			continue
		}
		// Send until the stream ends or starts draining. A request taken
		// from the queue but not sent comes back in req.
		req = s.sendLoop(req)
		c.setStreamUp(false)
		// If the stream ended while sending, it may still be viable if it
		// delivered a frame, or if it lived long enough.
		streamViable, refusalErr := s.outcome()
		if streamViable {
			b.recovered()
		}
		if refusalErr != nil {
			// The server refused the stream: fail the request and back off.
			c.streamFailed(&b, req, refusalErr)
			req, connect = nil, false
			continue
		}
		// The stream was lost or is draining. A waiting request goes to the
		// next stream at once, and so does a draining stream's replacement
		// under eager reconnect.
		connect = req != nil || s.draining.Load() && c.eagerReconnect
		if !connect {
			b.pace()
		}
	}
}

// streamFailed handles a stream that could not be opened, or that the server
// refused, with err. It fails req, which is not sent again.
func (c *OutboundChannel) streamFailed(b *backoff, req *Request, err error) {
	if c.ctx.Err() != nil {
		err = ErrNodeClosed
	} else {
		c.recordHealth(err)
		b.failed(err)
	}
	c.failRequest(req, err)
}

// await returns the next request, or nil when the retry timer fires first.
// Requests that arrive during a refusal backoff fail with the refusal. It
// returns false when the channel closes.
func (c *OutboundChannel) await(b *backoff) (req *Request, ok bool) {
	for {
		select {
		case <-c.ctx.Done():
			return nil, false
		case r := <-c.queue.ch:
			if err := b.refusing(); err != nil {
				c.queue.fail(r, err)
				continue
			}
			return &r, true
		case <-b.retry:
			return nil, true
		}
	}
}

// failRequest fails req with err. It ignores a nil req.
func (c *OutboundChannel) failRequest(req *Request, err error) {
	if req != nil {
		c.queue.fail(*req, err)
	}
}

// backoff paces the attempts to open a stream. A refusal always imposes a
// delay; other failures impose one only under eager reconnect, where the
// delay is the time to the next attempt. The delay doubles up to
// eagerReconnectMaxDelay and starts over when a stream proves viable.
type backoff struct {
	eager   bool
	delay   time.Duration
	retry   <-chan time.Time // fires when to attempt again; nil unless eager
	refusal error            // the latest refusal, until a stream proves viable
	until   time.Time        // when the delay after refusal ends
}

func newBackoff(eager bool) backoff {
	return backoff{eager: eager, delay: eagerReconnectBaseDelay}
}

// attempt discards the retry timer of the previous attempt.
func (b *backoff) attempt() {
	b.retry = nil
}

// suspect reports whether no stream has proven viable since the latest refusal.
func (b *backoff) suspect() bool {
	return b.refusal != nil
}

// recovered starts over after a stream proved viable.
func (b *backoff) recovered() {
	b.delay = eagerReconnectBaseDelay
	b.refusal = nil
}

// failed records a failed attempt, which was refused if err is a refusal.
func (b *backoff) failed(err error) {
	if !isRefusal(err) {
		b.pace()
		return
	}
	b.refusal = err
	b.until = b.next()
	if b.eager {
		b.retry = time.After(time.Until(b.until))
	}
}

// pace sets the retry timer for the next attempt under eager reconnect.
func (b *backoff) pace() {
	if b.eager {
		b.retry = time.After(time.Until(b.next()))
	}
}

// refusing returns the refusal that requests fail with, or nil once its
// delay has ended.
func (b *backoff) refusing() error {
	if b.refusal != nil && time.Now().Before(b.until) {
		return b.refusal
	}
	return nil
}

// next returns when the delay that starts now ends, and doubles the delay.
func (b *backoff) next() time.Time {
	until := time.Now().Add(b.delay)
	b.delay = min(2*b.delay, eagerReconnectMaxDelay)
	return until
}

// open returns a session on a new stream, after checking its remote end if the
// channel has a verifier. A suspect session follows a refused stream.
func (c *OutboundChannel) open(suspect bool) (*session, error) {
	ctx, cancel := context.WithCancel(c.ctx)
	st, err := NewGorumsClient(c.conn).NodeStream(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	if c.verify != nil {
		// st.Context carries the server's peer information, including
		// its TLS state. The call also commits st to its connection:
		// gRPC will not retry st on another connection, so every
		// request sent on st reaches the peer checked here. If st
		// fails, the run loop opens a new stream and checks it again.
		if err := c.verify(st.Context(), c.id); err != nil {
			cancel()
			return nil, VerificationError(c.id, err)
		}
	}
	s := newSession(ctx, cancel, &c.endpoint, st, true, true)
	s.suspect = suspect
	c.mu.Lock()
	c.sessions[s] = struct{}{}
	c.mu.Unlock()
	c.setStreamUp(true)
	c.wg.Go(func() {
		_ = s.receive()
		c.mu.Lock()
		delete(c.sessions, s)
		c.mu.Unlock()
	})
	c.wg.Go(func() {
		c.watchGoAway(ctx, s)
	})
	return s, nil
}

// watchGoAway drains s once the connection leaves Ready, which is how the
// client observes a server GOAWAY. It returns when ctx ends.
//
// The connection may still report Connecting just after the stream opens.
// That is not a GOAWAY, so the watcher waits for Ready first.
func (c *OutboundChannel) watchGoAway(ctx context.Context, s *session) {
	for c.conn.GetState() == connectivity.Connecting {
		if !c.conn.WaitForStateChange(ctx, connectivity.Connecting) {
			return
		}
	}
	for c.conn.GetState() == connectivity.Ready {
		if !c.conn.WaitForStateChange(ctx, connectivity.Ready) {
			return
		}
	}
	s.startDrain()
}

// setStreamUp records whether a stream is accepting requests and calls the
// stream-change callback on transitions.
func (c *OutboundChannel) setStreamUp(up bool) {
	if c.streamUp.CompareAndSwap(!up, up) && c.onStreamChange != nil {
		c.onStreamChange()
	}
}

// StreamUp reports whether a stream is open for sending. A stream counts as up
// until its refusal, if any, arrives.
func (c *OutboundChannel) StreamUp() bool {
	return c.streamUp.Load()
}

// PendingCount returns the number of two-way calls awaiting responses on the
// channel's streams.
func (c *OutboundChannel) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for s := range c.sessions {
		n += s.pending.len()
	}
	return n
}

// Close closes the channel and its connection, fails queued and pending
// requests with [ErrNodeClosed], and waits for the channel's goroutines to
// exit. It is idempotent.
func (c *OutboundChannel) Close() error {
	return c.closeOnce()
}
