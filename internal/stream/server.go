package stream

import (
	"context"
)

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
