package stream

import (
	"context"
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
// It is implemented by [github.com/relab/gorums/internal/conn.InboundManager].
type PeerAcceptor interface {
	// AcceptPeer returns the channel for an accepted stream and a cleanup
	// function to call when the stream ends, or an error that rejects the
	// stream. ctx is the stream's context.
	AcceptPeer(ctx context.Context, stream BidiStream) (*InboundChannel, func(), error)
}

// Server handles NodeStream connections.
type Server struct {
	onConnect func(context.Context)
	acceptor  PeerAcceptor
	UnimplementedGorumsServer
}

// NewServer returns a Server that accepts streams with acceptor and calls
// onConnect, if non-nil, for each accepted stream.
func NewServer(onConnect func(context.Context), acceptor PeerAcceptor) *Server {
	return &Server{onConnect: onConnect, acceptor: acceptor}
}

// NodeStream serves one client stream on the channel the acceptor returns for
// it, until receiving from the stream fails.
func (s *Server) NodeStream(srv Gorums_NodeStreamServer) error {
	ch, cleanup, err := s.acceptor.AcceptPeer(srv.Context(), srv)
	if err != nil {
		return err
	}
	defer cleanup()
	if s.onConnect != nil {
		s.onConnect(srv.Context())
	}
	return ch.Serve()
}
