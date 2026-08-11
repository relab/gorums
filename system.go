package gorums

import (
	"context"
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

// NewSystem creates a new Gorums System listening on the specified address.
// Accepts any [DialOption]s. Server options may be passed via [WithServerOptions];
// pass [WithPeers] there to have the server build a peer [Configuration],
// accessible via [System.OutboundConfig].
func NewSystem(addr string, opts ...DialOption) (*System, error) {
	dialOpts := newDialOptions()
	for _, opt := range opts {
		opt(&dialOpts)
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &System{
		srv: NewServer(dialOpts.srvOpts...),
		lis: lis,
	}, nil
}

// NewLocalSystems creates n Gorums systems listening on random localhost ports.
//
// Each system is assigned a node ID in the range 1..n and is configured to
// communicate with the others using the generated local node list. A peer
// [Configuration] is created automatically for each system and is available via
// [System.OutboundConfig].
//
// The opts may contain any [DialOption]s. Server options may be passed via
// [WithServerOptions].
//
// The returned systems are not started. Call [System.Serve] after registering
// any services. The returned stop function stops all systems and should be
// called when they are no longer needed.
//
// If system creation fails, all resources acquired by this function are
// released before returning the error.
func NewLocalSystems(n int, opts ...DialOption) ([]*System, func(), error) {
	dialOpts := newDialOptions()
	for _, opt := range opts {
		opt(&dialOpts)
	}
	listeners, nodeList, err := allocateListeners(n)
	if err != nil {
		return nil, nil, err
	}
	systems := make([]*System, n)
	for i := range n {
		myID := uint32(i + 1)
		sysSrvOpts := append([]ServerOption{WithPeers(myID, nodeList, opts...)}, dialOpts.srvOpts...)
		systems[i] = &System{
			srv: NewServer(sysSrvOpts...),
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

// allocateListeners pre-allocates n TCP listeners on random localhost ports and
// returns them along with a [NodeListOption] containing their addresses. If any
// listener fails to open, all previously opened listeners are closed before
// returning the error.
func allocateListeners(n int) ([]net.Listener, NodeListOption, error) {
	listeners := make([]net.Listener, n)
	addrs := make([]string, n)
	for i := range n {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			for j := range i {
				_ = listeners[j].Close()
			}
			return nil, nil, err
		}
		listeners[i] = lis
		addrs[i] = lis.Addr().String()
	}
	return listeners, WithNodeList(addrs), nil
}

// OutboundConfig returns the server's peer [Configuration], or nil if the
// server was not configured with [WithPeers].
func (s *System) OutboundConfig() Configuration {
	return s.srv.PeerConfig()
}

// Addr returns the address the system is listening on.
func (s *System) Addr() string {
	return s.lis.Addr().String()
}

// ConnectedPeers returns the currently reachable subset of the server's peer
// [Configuration], including this node.
// An empty (non-nil) Configuration is returned if no known peers are connected.
// The returned slice is replaced atomically on each connect/disconnect;
// thus, retaining a reference to an old configuration is safe.
func (s *System) ConnectedPeers() Configuration {
	return s.srv.ConnectedPeers()
}

// ConnectedClients returns a [Configuration] of all connected client peers
// that can accept server-initiated requests.
// An empty (non-nil) Configuration is returned if no client peers are connected.
// The returned slice is replaced atomically on each connect/disconnect;
// thus, retaining a reference to an old configuration is safe.
func (s *System) ConnectedClients() Configuration {
	return s.srv.ConnectedClients()
}

// WaitForPeers blocks until cond returns true for the current connected-peer
// [Configuration], or until ctx is cancelled or the system is stopped.
// The condition is checked immediately against the current configuration,
// so it may return without blocking if the condition is already satisfied.
func (s *System) WaitForPeers(ctx context.Context, cond func(Configuration) bool) error {
	return s.srv.WaitForPeers(ctx, cond)
}

// WaitForClients blocks until cond returns true for the current
// client-peer [Configuration], or until ctx is cancelled or the system is stopped.
// The condition is checked immediately against the current configuration,
// so it may return without blocking if the condition is already satisfied.
func (s *System) WaitForClients(ctx context.Context, cond func(Configuration) bool) error {
	return s.srv.WaitForClients(ctx, cond)
}

// RegisterService registers the service with the server using the provided register function.
// The closer is added to the list of closers to be closed when the system is stopped.
//
// Example usage:
//
//	gs := NewSystem(lis)
//	impl := &srvImpl{}
//	gs.RegisterService(nil, func(srv *Server) {
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
// It is safe to call Stop before [System.Serve] to avoid resource leaks.
func (s *System) Stop() (errs error) {
	// Unblock any WaitForPeers / WaitForClients callers.
	s.srv.close()
	// We cannot use graceful stop here since multicast methods does not
	// respond to the client, and thus would block indefinitely.
	s.srv.Stop()
	// Always close the listener explicitly. If Serve was called, gRPC
	// already closed it and the second Close is a no-op error we discard.
	// If Serve was never called, this is the only place that closes it.
	_ = s.lis.Close()
	for _, closer := range s.closers {
		errs = errors.Join(errs, closer.Close())
	}
	return errs
}
