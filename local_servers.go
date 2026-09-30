package gorums

import (
	"fmt"
	"net"
)

// localServerOptions accumulates the options [NewLocalServers] applies to
// every server it creates.
type localServerOptions struct {
	serverOpts []ServerOption
	dialOpts   []DialOption
}

// LocalServerOption configures [NewLocalServers]. Use [WithLocalServerOptions]
// and [WithLocalDialOptions] to build one.
type LocalServerOption func(*localServerOptions)

// WithLocalServerOptions applies opts to every server created by [NewLocalServers].
func WithLocalServerOptions(opts ...ServerOption) LocalServerOption {
	return func(o *localServerOptions) {
		o.serverOpts = append(o.serverOpts, opts...)
	}
}

// WithLocalDialOptions applies opts to every server's outbound configuration
// created by [NewLocalServers].
func WithLocalDialOptions(opts ...DialOption) LocalServerOption {
	return func(o *localServerOptions) {
		o.dialOpts = append(o.dialOpts, opts...)
	}
}

// NewLocalServers creates n Gorums servers listening on random localhost ports.
//
// Each server is assigned a node ID from 1 to n. Every server tracks and calls
// all the other servers. Use [WithLocalServerOptions] to add [ServerOption]s
// to every server, and [WithLocalDialOptions] to add [DialOption]s to each
// server's outbound connections.
//
// The returned servers are not started; call [Server.ListenAndServe] after
// registering any services. The returned stop function stops all servers and
// closes all allocated listeners and outbound configurations. If listener
// allocation fails, all listeners acquired so far are closed before returning
// the error.
func NewLocalServers(n int, opts ...LocalServerOption) ([]*Server, func(), error) {
	var localOpts localServerOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&localOpts)
		}
	}
	listeners, nodeSource, err := allocateListeners(n)
	if err != nil {
		return nil, nil, err
	}
	servers := make([]*Server, n)
	for i := range n {
		myID := uint32(i + 1)
		serverOpts := append(
			[]ServerOption{WithPeers(myID, nodeSource, localOpts.dialOpts...)},
			localOpts.serverOpts...,
		)
		srv, err := newServer(serverOpts...)
		if err != nil {
			closeServers(servers[:i], listeners)
			return nil, nil, fmt.Errorf("gorums: invalid peer configuration: %w", err)
		}
		srv.setListener(listeners[i])
		servers[i] = srv
	}
	stop := func() { closeServers(servers, listeners) }
	return servers, stop, nil
}

// closeServers stops servers and closes every preallocated listener, including
// those of servers that were not created or that serve on another listener.
func closeServers(servers []*Server, listeners []net.Listener) {
	for _, srv := range servers {
		srv.Stop()
	}
	for _, lis := range listeners {
		_ = lis.Close()
	}
}

// allocateListeners pre-allocates n TCP listeners on random localhost ports and
// returns them along with a [NodeSource] containing their addresses. If any
// listener fails to open, the listeners opened so far are closed before
// returning the error.
func allocateListeners(n int) ([]net.Listener, NodeSource, error) {
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
