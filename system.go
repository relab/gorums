package gorums

import (
	"errors"
	"io"
	"net"
)

// System encapsulates the state of a Gorums system, including the server,
// listener, and any registered closers (e.g. managers).
type System struct {
	closers []io.Closer
	srv     *Server
	lis     net.Listener
}

// NewSystem creates a new Gorums System with the provided listener and server options.
func NewSystem(lis net.Listener, opts ...ServerOption) *System {
	return &System{
		srv: NewServer(opts...),
		lis: lis,
	}
}

// RegisterService registers the service with the server using the provided register function.
// The closer is added to the list of closers to be closed when the system is stopped.
//
// Example usage:
//
//	gs := NewSystem(lis)
//	mgr := NewManager(...)
//	impl := &srvImpl{}
//	gs.RegisterService(mgr, func(srv *Server) {
//		pb.RegisterMultiPaxosServer(srv, impl)
//	})
func (s *System) RegisterService(closer io.Closer, registerFunc func(*Server)) {
	if closer != nil {
		s.closers = append(s.closers, closer)
	}
	registerFunc(s.srv)
}

// Serve starts the server.
func (s *System) Serve() error {
	return s.srv.Serve(s.lis)
}

// Stop stops the Gorums server and closes all registered closers.
// It immediately closes all open connections and listeners. It cancels
// all active RPCs on the server side and the corresponding pending RPCs
// on the client side will get notified by connection errors.
func (s *System) Stop() (errs error) {
	// We cannot use graceful stop here since multicast methods does not
	// respond to the client, and thus would block indefinitely.
	// The server's listener is closed by s.srv.Stop().
	s.srv.Stop()
	for _, closer := range s.closers {
		errs = errors.Join(errs, closer.Close())
	}
	return errs
}
