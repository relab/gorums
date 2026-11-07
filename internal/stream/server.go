package stream

import (
	"context"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

	mu       sync.Mutex
	channels map[*InboundChannel]struct{} // streams being served
	draining bool                         // new streams are refused
}

// NewServer returns a Server that accepts streams with acceptor and calls
// onConnect, if non-nil, for each accepted stream.
func NewServer(onConnect func(context.Context), acceptor PeerAcceptor) *Server {
	return &Server{onConnect: onConnect, acceptor: acceptor, channels: make(map[*InboundChannel]struct{})}
}

// NodeStream serves one client stream on the channel the acceptor returns for
// it, until receiving from the stream fails.
func (s *Server) NodeStream(srv Gorums_NodeStreamServer) error {
	ch, cleanup, err := s.acceptor.AcceptPeer(srv.Context(), srv)
	if err != nil {
		return err
	}
	defer cleanup()
	if !s.track(ch) {
		return status.Error(codes.Unavailable, "server is draining")
	}
	defer s.untrack(ch)
	if s.onConnect != nil {
		s.onConnect(srv.Context())
	}
	return ch.Serve()
}

// track registers ch and reports false if the server is draining.
func (s *Server) track(ch *InboundChannel) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draining {
		return false
	}
	s.channels[ch] = struct{}{}
	return true
}

// untrack removes ch.
func (s *Server) untrack(ch *InboundChannel) {
	s.mu.Lock()
	delete(s.channels, ch)
	s.mu.Unlock()
}

// Drain refuses new streams and runs the drain protocol on every stream that
// the server serves, whether or not the stream is tracked in a configuration.
// It waits until each stream's dialer has sent its last request and this
// server has finished the requests it accepted, and reports whether that
// happened before ctx ended. Streams whose dialer does not implement the
// protocol are not waited for.
func (s *Server) Drain(ctx context.Context) bool {
	s.mu.Lock()
	s.draining = true
	var waits []<-chan struct{}
	for ch := range s.channels {
		if done := ch.Drain(); done != nil {
			waits = append(waits, done)
		}
	}
	s.mu.Unlock()
	for _, done := range waits {
		select {
		case <-done:
		case <-ctx.Done():
			return false
		}
	}
	return true
}
