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

// NewLocalSystems creates n Gorums systems listening on random localhost ports.
// It pre-allocates all listeners before creating any system, so the full list
// of addresses is available when configuring each system — solving the
// chicken-and-egg problem of needing peer addresses before binding.
//
// Each system is automatically configured with [WithNodeList] and [WithConfig]
// derived from the allocated addresses, assigning node IDs 1..n in order.
// Additional server options can be provided via opts.
//
// The returned systems are not started; the caller must call [System.Serve]
// (typically via a goroutine) after registering any services. This ensures
// that handlers are registered before the server begins accepting connections.
//
// The returned stop function stops all systems and should be called when done,
// e.g. via defer. If any listener cannot be created, all previously opened
// listeners are closed and an error is returned.
func NewLocalSystems(n int, opts ...ServerOption) ([]*System, func(), error) {
	listeners := make([]net.Listener, n)
	addrs := make([]string, n)
	for i := range n {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			for j := range i {
				listeners[j].Close()
			}
			return nil, nil, err
		}
		listeners[i] = lis
		addrs[i] = lis.Addr().String()
	}
	nodeList := WithNodeList(addrs)
	systems := make([]*System, n)
	for i := range n {
		sysOpts := append([]ServerOption{WithConfig(uint32(i+1), nodeList)}, opts...)
		systems[i] = &System{
			srv: NewServer(sysOpts...),
			lis: listeners[i],
		}
	}
	stop := func() {
		for _, sys := range systems {
			_ = sys.Stop()
		}
	}
	return systems, stop, nil
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

// NewOutboundConfig creates an outbound [Configuration] for connecting to peers.
// When peer tracking / symmetric configuration is enabled, it automatically
// includes this system's NodeID in the connection metadata, enabling the
// remote server to identify this replica.
func (s *System) NewOutboundConfig(opts ...Option) (Configuration, error) {
	if s.srv.inboundManager == nil {
		return NewConfig(opts...)
	}
	return NewConfig(append([]Option{
		withRequestHandler(s.srv, s.srv.NodeID()),
	}, opts...)...)
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
