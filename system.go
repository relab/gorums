package gorums

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
)

// System encapsulates the state of a Gorums system, including the server,
// listener, and any registered closers (e.g. managers).
type System struct {
	closers []io.Closer
	srv     *Server
	lis     net.Listener
	config  Configuration // auto-created outbound config; nil if not set
}

// NewSystem creates a new Gorums System listening on the specified address.
// Accepts any [DialOption]s. Server options may be passed via [WithServerOptions].
// If [WithOutboundNodes] is provided, an outbound [Configuration] is created
// automatically and can be accessed via [System.OutboundConfig].
func NewSystem(addr string, opts ...DialOption) (*System, error) {
	dialOpts := newDialOptions()
	for _, opt := range opts {
		opt(&dialOpts)
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	sys := &System{
		srv: NewServer(dialOpts.srvOpts...),
		lis: lis,
	}
	if dialOpts.outboundNodes != nil {
		cfg, err := sys.newOutboundConfig(dialOpts.outboundNodes, opts...)
		if err != nil {
			_ = lis.Close()
			return nil, fmt.Errorf("gorums: failed to create outbound config: %w", err)
		}
		sys.config = cfg
		sys.closers = append(sys.closers, cfg)
	}
	return sys, nil
}

// NewLocalSystems creates n Gorums systems listening on random localhost ports.
//
// Each system is assigned a node ID in the range 1..n and is configured to
// communicate with the others using the generated local node list. An outbound
// [Configuration] is created automatically for each system and is available via
// [System.OutboundConfig].
//
// The opts may contain any [DialOption]s. Server options may be passed via
// [WithServerOptions]. [WithOutboundNodes] is ignored by this function since
// the local node list is computed internally.
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
		sysSrvOpts := append([]ServerOption{WithConfig(myID, nodeList)}, dialOpts.srvOpts...)
		sys := &System{
			srv: NewServer(sysSrvOpts...),
			lis: listeners[i],
		}
		cfg, err := sys.newOutboundConfig(nodeList, opts...)
		if err != nil {
			for j := range i {
				_ = systems[j].Stop()
			}
			return nil, nil, fmt.Errorf("gorums: failed to create outbound config for system %d: %w", i+1, err)
		}
		sys.config = cfg
		sys.closers = append(sys.closers, cfg)
		systems[i] = sys
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

// newOutboundConfig creates an outbound [Configuration] for connecting to peers.
// It always prepends a [WithServer] option so that the remote server can dispatch
// server-initiated requests back through the bidirectional connection, regardless of
// whether this system has peer tracking configured.
func (s *System) newOutboundConfig(nodeList NodeListOption, dialOpts ...DialOption) (Configuration, error) {
	return NewConfig(nodeList, append([]DialOption{WithServer(s.srv)}, dialOpts...)...)
}

// OutboundConfig returns the auto-created outbound [Configuration], or nil if none was created.
// An outbound config is created automatically by [NewLocalSystems] and by [NewSystem] when
// [WithOutboundNodes] is provided.
func (s *System) OutboundConfig() Configuration {
	return s.config
}

// Addr returns the address the system is listening on.
func (s *System) Addr() string {
	return s.lis.Addr().String()
}

// Config returns a [Configuration] of all connected known peers, including this node.
// An empty (non-nil) Configuration is returned if no known peers are connected.
// The returned slice is replaced atomically on each connect/disconnect;
// thus, retaining a reference to an old configuration is safe.
func (s *System) Config() Configuration {
	return s.srv.Config()
}

// ClientConfig returns a [Configuration] of all connected client peers
// that can accept server-initiated requests.
// An empty (non-nil) Configuration is returned if no client peers are connected.
// The returned slice is replaced atomically on each connect/disconnect;
// thus, retaining a reference to an old configuration is safe.
func (s *System) ClientConfig() Configuration {
	return s.srv.ClientConfig()
}

// WaitForConfig blocks until cond returns true for the current known-peer
// [Configuration], or until ctx is cancelled or the system is stopped.
// The condition is checked immediately against the current configuration,
// so it may return without blocking if the condition is already satisfied.
func (s *System) WaitForConfig(ctx context.Context, cond func(Configuration) bool) error {
	return s.srv.waitForKnownConfig(ctx, cond)
}

// WaitForClientConfig blocks until cond returns true for the current
// client-peer [Configuration], or until ctx is cancelled or the system is stopped.
// The condition is checked immediately against the current configuration,
// so it may return without blocking if the condition is already satisfied.
func (s *System) WaitForClientConfig(ctx context.Context, cond func(Configuration) bool) error {
	return s.srv.waitForClientConfig(ctx, cond)
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
	// Unblock any WaitForConfig / WaitForClientConfig callers.
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
