package stream

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// acceptedKey is the response header that a server sends on each stream it
// accepts, before any message. A dialer counts its stream as up when this
// header or the first frame arrives. A refused stream ends without it.
const acceptedKey = "gorums-accepted"

// acceptFrame returns the frame that a server sends on an accepted stream in
// place of the acceptance header, when a stream interceptor has already sent
// the response headers. Message IDs start at 1, so the frame answers no call:
// a dialer counts it as the server's first frame and then drops it.
func acceptFrame() *Message {
	return Message_builder{}.Build()
}

// PeerAcceptor identifies and registers incoming peers on a stream.
// It is implemented by [conn.InboundManager].
type PeerAcceptor interface {
	// AcceptPeer returns the channel for an accepted stream and a cleanup
	// function to call when the stream ends, or an error that rejects the
	// stream. ctx is the stream's context.
	AcceptPeer(ctx context.Context, stream InboundStream) (*InboundChannel, func(), error)
}

// Server handles NodeStream connections.
type Server struct {
	onConnect func(context.Context)
	acceptor  PeerAcceptor
	UnimplementedGorumsServer

	// work counts the accepted handlers of every stream, also after the
	// stream ends.
	work atomic.Int64

	mu           sync.Mutex
	channels     map[*InboundChannel]struct{} // streams being served
	admitting    int                          // streams admitted but not yet registered
	draining     bool                         // new streams are refused
	drainStarted []*InboundChannel            // channels that Drain started draining
}

// NewServer returns a Server that accepts streams with acceptor and calls
// onConnect, if non-nil, for each accepted stream.
func NewServer(onConnect func(context.Context), acceptor PeerAcceptor) *Server {
	return &Server{onConnect: onConnect, acceptor: acceptor, channels: make(map[*InboundChannel]struct{})}
}

// NodeStream serves one client stream on the channel the acceptor returns for
// it, until receiving from the stream fails. A draining server refuses the
// stream with [ErrServerDraining] before the acceptor sees it.
func (s *Server) NodeStream(srv Gorums_NodeStreamServer) error {
	if !s.admit() {
		return ErrServerDraining
	}
	ch, cleanup, err := s.acceptor.AcceptPeer(srv.Context(), srv)
	if err != nil {
		s.register(nil)
		return err
	}
	defer cleanup()
	// Receiving has not started, so no handler has been queued yet.
	ch.requests.shared = &s.work
	s.register(ch)
	defer s.untrack(ch)
	if s.onConnect != nil {
		s.onConnect(srv.Context())
	}
	return ch.Serve()
}

// admit reserves a place for a new stream and reports false if the server is
// draining.
func (s *Server) admit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draining {
		return false
	}
	s.admitting++
	return true
}

// register records the admitted stream's channel ch, or a refused stream if
// ch is nil. A stream admitted before the drain began is drained too.
func (s *Server) register(ch *InboundChannel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.admitting--
	if ch == nil {
		return
	}
	s.channels[ch] = struct{}{}
	if s.draining {
		s.startDrain(ch)
	}
}

// untrack removes ch.
func (s *Server) untrack(ch *InboundChannel) {
	s.mu.Lock()
	delete(s.channels, ch)
	s.mu.Unlock()
}

// startDrain starts the drain protocol on ch. The caller must hold s.mu.
func (s *Server) startDrain(ch *InboundChannel) {
	ch.session.startLocalDrain()
	s.drainStarted = append(s.drainStarted, ch)
}

// Drain refuses new streams and runs the drain protocol on every stream that
// the server serves, whether or not the stream is tracked in a configuration.
// It reports whether the following happened before ctx ended:
//   - each stream's dialer sent its last request and this server sent its
//     replies, or the stream ended;
//   - every accepted handler returned, including handlers whose stream ended.
//
// A stream that ends while it drains still runs its queued handlers.
func (s *Server) Drain(ctx context.Context) bool {
	s.mu.Lock()
	s.draining = true
	for ch := range s.channels {
		s.startDrain(ch)
	}
	s.mu.Unlock()
	tick := time.NewTicker(drainPollInterval)
	defer tick.Stop()
	for !s.isDrained() {
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
		}
	}
	return true
}

// isDrained reports whether the server has finished draining.
func (s *Server) isDrained() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admitting > 0 || s.work.Load() > 0 {
		return false
	}
	for ch := range s.channels {
		if !ch.session.isDrained() {
			return false
		}
	}
	return true
}

// Stop refuses new streams, and makes the streams that Drain started
// discard their queued handlers when they end, as streams do without a drain.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draining = true
	for _, ch := range s.drainStarted {
		ch.session.requests.keep.Store(false)
	}
	s.drainStarted = nil
}
