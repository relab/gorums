package gorums

import (
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
// Accepts any mix of [ServerOption], [ManagerOption], and [NodeListOption].
// If a [NodeListOption] is provided, an outbound [Configuration] is created
// automatically and can be accessed via [System.OutboundConfig].
func NewSystem(addr string, opts ...Option) (*System, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	var srvOpts []ServerOption
	var mgrOpts []ManagerOption
	var nodeListOpt NodeListOption
	for _, opt := range opts {
		switch o := opt.(type) {
		case ServerOption:
			srvOpts = append(srvOpts, o)
		case ManagerOption:
			mgrOpts = append(mgrOpts, o)
		case NodeListOption:
			nodeListOpt = o
		}
	}
	sys := &System{
		srv: NewServer(srvOpts...),
		lis: lis,
	}
	if nodeListOpt != nil {
		cfg, err := sys.createOutboundConfig(nodeListOpt, mgrOpts)
		if err != nil {
			_ = lis.Close()
			return nil, fmt.Errorf("gorums: failed to create outbound config: %w", err)
		}
		sys.config = cfg
		sys.closers = append(sys.closers, cfg.Manager())
	}
	return sys, nil
}

// NewLocalSystems creates n Gorums systems listening on random localhost ports.
// It pre-allocates all listeners before creating any system, so the full list
// of addresses is available when configuring each system — solving the
// chicken-and-egg problem of needing peer addresses before binding.
//
// Each system is automatically configured with [WithNodeList] and [WithConfig]
// derived from the allocated addresses, assigning node IDs 1..n in order.
// Accepts any mix of [ServerOption] and [ManagerOption] via opts.
// If [ManagerOption]s are provided (e.g., dial options), an outbound [Configuration]
// is created automatically for each system and can be accessed via [System.OutboundConfig].
//
// The returned systems are not started; the caller must call [System.Serve]
// (typically via a goroutine) after registering any services. This ensures
// that handlers are registered before the server begins accepting connections.
//
// The returned stop function stops all systems and should be called when done,
// e.g. via defer. If any listener cannot be created, all previously opened
// listeners are closed and an error is returned.
func NewLocalSystems(n int, opts ...Option) ([]*System, func(), error) {
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
	var srvOpts []ServerOption
	var mgrOpts []ManagerOption
	for _, opt := range opts {
		switch o := opt.(type) {
		case ServerOption:
			if o == nil {
				continue
			}
			srvOpts = append(srvOpts, o)
		case ManagerOption:
			if o == nil {
				continue
			}
			mgrOpts = append(mgrOpts, o)
		}
	}
	nodeList := WithNodeList(addrs)
	systems := make([]*System, n)
	for i := range n {
		myID := uint32(i + 1)
		sysSrvOpts := append([]ServerOption{WithConfig(myID, nodeList)}, srvOpts...)
		sys := &System{
			srv: NewServer(sysSrvOpts...),
			lis: listeners[i],
		}
		if len(mgrOpts) > 0 {
			cfg, err := sys.createOutboundConfig(nodeList, mgrOpts)
			if err != nil {
				for j := range i {
					systems[j].Stop()
				}
				return nil, nil, fmt.Errorf("gorums: failed to create outbound config for system %d: %w", i+1, err)
			}
			sys.config = cfg
			sys.closers = append(sys.closers, cfg.Manager())
		}
		systems[i] = sys
	}
	stop := func() {
		for _, sys := range systems {
			_ = sys.Stop()
		}
	}
	return systems, stop, nil
}

// createOutboundConfig creates an outbound [Configuration] by combining nodeList,
// a self-routing request handler (when peer tracking is configured), and mgrOpts.
func (s *System) createOutboundConfig(nodeList NodeListOption, mgrOpts []ManagerOption) (Configuration, error) {
	opts := []Option{nodeList}
	if s.srv.inboundManager != nil {
		opts = append(opts, withRequestHandler(s.srv, s.srv.NodeID()))
	}
	for _, o := range mgrOpts {
		opts = append(opts, o)
	}
	return NewConfig(opts...)
}

// OutboundConfig returns the auto-created outbound [Configuration], or nil if none was created.
// An outbound config is created automatically by [NewLocalSystems] when dial options are provided,
// and by [NewSystem] when a [NodeListOption] is provided.
func (s *System) OutboundConfig() Configuration {
	return s.config
}

// NewOutboundConfig creates an outbound [Configuration] for connecting to peers.
// When peer tracking is configured ([WithConfig] or [WithClientConfig]), the system's
// request handler is automatically included, enabling the remote server to dispatch
// server-initiated requests back through the bidirectional connection.
// The caller must provide a [NodeListOption] and any dial options as [ManagerOption]s.
// For symmetric configurations created by [NewLocalSystems], the outbound config is
// auto-created and accessible via [System.OutboundConfig]; use [NewOutboundConfig]
// for asymmetric setups where per-system node lists differ.
func (s *System) NewOutboundConfig(opts ...Option) (Configuration, error) {
	var base []Option
	if s.srv.inboundManager != nil {
		base = append(base, withRequestHandler(s.srv, s.srv.NodeID()))
	}
	return NewConfig(append(base, opts...)...)
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
