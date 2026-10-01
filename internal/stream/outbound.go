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

// OutboundOptions configures an [OutboundChannel].
type OutboundOptions struct {
	// SendBufferSize is the send queue capacity.
	SendBufferSize uint
	// Handler serves requests the server sends on the stream; may be nil.
	Handler RequestHandler
	// Latency receives round-trip samples; may be nil.
	Latency *Latency
	// EagerReconnect makes the channel re-establish a lost stream with capped
	// exponential backoff, instead of on the next request.
	EagerReconnect bool
	// OnStreamChange, if non-nil, is called on the channel's goroutine each
	// time the stream comes up or goes down; [OutboundChannel.StreamUp]
	// reports the new state.
	OnStreamChange func()
}

// OutboundChannel is a [Channel] over streams this node dials to a server. It
// opens a stream on creation and re-establishes a lost one, and requests that
// were pending on a lost stream are sent again on the next one, except
// streaming requests, which fail with [ErrStreamDown].
type OutboundChannel struct {
	endpoint
	conn           *grpc.ClientConn
	eagerReconnect bool
	onStreamChange func()
	streamUp       atomic.Bool

	mu       sync.Mutex
	sessions map[*session]struct{} // sessions whose receive loop is running

	wg        sync.WaitGroup
	closeOnce func() error
}

// NewOutboundChannel returns a channel to the server behind conn and starts
// its goroutine. The channel lives until ctx ends or it is closed; closing it
// also closes conn.
func NewOutboundChannel(ctx context.Context, id uint32, conn *grpc.ClientConn, opts OutboundOptions) *OutboundChannel {
	c := &OutboundChannel{
		endpoint:       newEndpoint(ctx, id, opts.SendBufferSize, 0, opts.Handler, opts.Latency),
		conn:           conn,
		eagerReconnect: opts.EagerReconnect,
		onStreamChange: opts.OnStreamChange,
		sessions:       make(map[*session]struct{}),
	}
	c.closeOnce = sync.OnceValue(func() error {
		c.cancel()
		c.queue.close()
		c.wg.Wait()
		return c.conn.Close()
	})
	c.wg.Add(1)
	go c.run()
	return c
}

// run opens sessions and sends queued requests on them until the channel
// closes. The first stream is opened at once. With eager reconnect, a draining
// stream is replaced at once and a lost one after a backoff delay; otherwise,
// either is replaced on the next request.
func (c *OutboundChannel) run() {
	defer c.wg.Done()
	defer c.queue.close()
	delay := eagerReconnectBaseDelay
	var retry <-chan time.Time
	var req *Request
	connect := true
	for {
		if !connect {
			select {
			case <-c.ctx.Done():
				return
			case r := <-c.queue.ch:
				req = &r
			case <-retry:
			}
		}
		if c.ctx.Err() != nil {
			if req != nil {
				req.ReplyError(c.id, ErrNodeClosed)
			}
			return
		}
		s, err := c.open()
		if err != nil {
			c.recordHealth(err)
			if req != nil {
				req.ReplyError(c.id, err)
				req = nil
			}
			retry, connect = c.pace(&delay), false
			continue
		}
		start := time.Now()
		req = s.sendLoop(req)
		c.setStreamUp(false)
		// A stream that received a frame or outlived the maximum delay has
		// proven viable, so the backoff starts over.
		if s.received.Load() || time.Since(start) >= eagerReconnectMaxDelay {
			delay = eagerReconnectBaseDelay
		}
		connect = req != nil || s.draining.Load() && c.eagerReconnect
		retry = nil
		if !connect {
			retry = c.pace(&delay)
		}
	}
}

// pace returns a timer for the next eager reconnect attempt and doubles delay
// up to the cap. Without eager reconnect it returns nil.
func (c *OutboundChannel) pace(delay *time.Duration) <-chan time.Time {
	if !c.eagerReconnect {
		return nil
	}
	timer := time.After(*delay)
	*delay = min(2*(*delay), eagerReconnectMaxDelay)
	return timer
}

// open opens a stream and starts the session's receive loop and GOAWAY watcher.
func (c *OutboundChannel) open() (*session, error) {
	ctx, cancel := context.WithCancel(c.ctx)
	st, err := NewGorumsClient(c.conn).NodeStream(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	s := newSession(&c.endpoint, st, ctx, cancel, true, true)
	c.mu.Lock()
	c.sessions[s] = struct{}{}
	c.mu.Unlock()
	c.setStreamUp(true)
	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		_ = s.receive()
		c.mu.Lock()
		delete(c.sessions, s)
		c.mu.Unlock()
	}()
	go func() {
		defer c.wg.Done()
		c.watchGoAway(ctx, s)
	}()
	return s, nil
}

// watchGoAway drains s once the connection leaves Ready, which is how the
// client observes a server GOAWAY. Pending calls then complete on s while new
// requests go to a new stream. It returns when ctx ends.
func (c *OutboundChannel) watchGoAway(ctx context.Context, s *session) {
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

// StreamUp reports whether a stream is established and accepting requests.
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
