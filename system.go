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

// NewSystem creates a new Gorums System listening on the specified address with the provided server options.
func NewSystem(addr string, opts ...ServerOption) (*System, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &System{
		srv: NewServer(opts...),
		lis: lis,
	}, nil
}

// Addr returns the address the system is listening on.
func (s *System) Addr() string {
	return s.lis.Addr().String()
}

// Config returns the Configuration of all currently connected known peers.
// Returns nil if no peer tracking is configured.
func (s *System) Config() Configuration {
	return s.srv.Config()
}

// ClientConfig returns a [Configuration] of all connected client peers.
// The returned slice is replaced atomically on each connect/disconnect;
// retaining a reference to an old value is safe.
// Returns nil if no peer tracking is configured.
func (s *System) ClientConfig() Configuration {
	return s.srv.ClientConfig()
}

// NewOutboundConfig creates an outbound Configuration for connecting to
// peers. It automatically includes this system's NodeID in the
// connection metadata, enabling the remote server to identify
// this replica.
//
// When the server has an InboundManager (symmetric configuration),
// stream deduplication is enabled: shared Node objects from the
// InboundManager are adopted, and only the lower-ID node in each pair
// creates an outbound connection. After building the configuration,
// it refreshes the InboundManager's config and sets request handlers on
// shared nodes so that server-initiated calls arriving on outbound
// channels are dispatched through the server's handler chain.
func (s *System) NewOutboundConfig(opts ...Option) (Configuration, error) {
	if s.srv.inboundMgr == nil {
		return NewConfig(opts...)
	}
	opts = append([]Option{
		WithNodeID(s.srv.inboundMgr.myID),
		withInboundManager(s.srv.inboundMgr),
	}, opts...)
	cfg, err := NewConfig(opts...)
	if err != nil {
		return nil, err
	}
	// Refresh InboundManager's config so that peers with newly-attached
	// outbound channels appear in Config().
	s.srv.inboundMgr.RefreshConfig()
	// Set the server's request handler on all shared nodes' routers so that
	// the outbound channel's receiver goroutine can dispatch incoming
	// server-initiated requests through the server's handler chain.
	for _, node := range cfg.Nodes() {
		if node.ID() != s.srv.inboundMgr.myID {
			node.setRequestHandler(s.srv)
		}
	}
	return cfg, nil
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
