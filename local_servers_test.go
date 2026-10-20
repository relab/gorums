package gorums_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
)

// TestLocalServersMissingDialOptions verifies that NewLocalServers reports an
// invalid peer configuration as an error, and returns no servers to stop.
func TestLocalServersMissingDialOptions(t *testing.T) {
	servers, stop, err := gorums.NewLocalServers(4)
	if err == nil {
		if stop != nil {
			stop()
		}
		t.Fatal("NewLocalServers without dial options succeeded, want an error")
	}
	if servers != nil || stop != nil {
		t.Errorf("NewLocalServers returned servers=%v stop=%v with an error", servers, stop != nil)
	}
}

// TestLocalServersInvalidPeers verifies that NewLocalServers reports peers
// that the inbound manager rejects as an error instead of panicking.
func TestLocalServersInvalidPeers(t *testing.T) {
	servers, stop, err := gorums.NewLocalServers(2,
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)),
		gorums.WithLocalServerOptions(gorums.WithPeers(1, gorums.WithNodeList([]string{"not-an-address"}))),
	)
	if err == nil {
		stop()
		t.Fatal("NewLocalServers with an invalid peer address succeeded, want an error")
	}
	if servers != nil || stop != nil {
		t.Errorf("NewLocalServers returned servers=%v stop=%v with an error", servers, stop != nil)
	}
}

// TestLocalServersStopClosesPreallocatedListener verifies that the stop
// function closes a server's preallocated listener even when the server
// was started on a different listener with Serve.
func TestLocalServersStopClosesPreallocatedListener(t *testing.T) {
	servers, stop, err := gorums.NewLocalServers(2, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatal(err)
	}
	preallocated := servers[0].Addr()
	// The test serves on its own listener, which is what the server must not
	// confuse with the preallocated one.
	own, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = servers[0].Serve(own) }()
	go func() { _ = servers[1].ListenAndServe() }()
	gorumstest.WaitUntil(t, 2*time.Second, func() bool { return servers[0].Addr() == own.Addr().String() })

	stop()
	if c, err := net.DialTimeout("tcp", preallocated, time.Second); err == nil {
		_ = c.Close()
		t.Errorf("preallocated listener %s still accepts connections after stop", preallocated)
	}
}

// TestLocalServersStopBeforeServeClosesListeners verifies that the stop
// function returned by NewLocalServers closes all pre-allocated listeners even
// when none of the servers has had ListenAndServe called yet, so no file
// descriptors are leaked.
func TestLocalServersStopBeforeServeClosesListeners(t *testing.T) {
	servers, stop, err := gorums.NewLocalServers(3, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	addrs := make([]string, len(servers))
	for i, srv := range servers {
		addrs[i] = srv.Addr()
	}
	stop() // called before any Serve()
	// Every pre-allocated listener must be closed. Assert this by dialing each
	// address and expecting a refused connection; re-binding the port would
	// race with anything else on the machine that grabs the free ephemeral
	// port.
	for _, addr := range addrs {
		if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
			_ = conn.Close()
			t.Errorf("connected to %s after stop (without Serve); expected the listener to be closed", addr)
		}
	}
}

// TestLocalServersAssignsSequentialNodeIDs verifies that NewLocalServers
// assigns node IDs 1..n in the order the servers are returned.
func TestLocalServersAssignsSequentialNodeIDs(t *testing.T) {
	const n = 4
	servers, stop, err := gorums.NewLocalServers(n, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)

	for i, srv := range servers {
		if want := uint32(i + 1); srv.NodeID() != want {
			t.Errorf("servers[%d].NodeID() = %d, want %d", i, srv.NodeID(), want)
		}
	}
}

// TestLocalServersPeerConfigSize verifies that each server's peer
// configuration includes every node in the symmetric group, without stream
// deduplication enabled.
func TestLocalServersPeerConfigSize(t *testing.T) {
	const n = 4
	servers, stop, err := gorums.NewLocalServers(n, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)

	for i, srv := range servers {
		if got := srv.PeerConfig().Size(); got != n {
			t.Errorf("servers[%d].PeerConfig().Size() = %d, want %d", i, got, n)
		}
	}
}

// TestLocalServersAppliesServerOptions verifies that a ServerOption passed
// via WithLocalServerOptions is applied to every server, not just the first.
func TestLocalServersAppliesServerOptions(t *testing.T) {
	const n = 3
	var connects atomic.Int32
	servers, stop, err := gorums.NewLocalServers(
		n,
		gorums.WithLocalServerOptions(gorums.WithConnectCallback(func(context.Context) { connects.Add(1) })),
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)),
	)
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}

	// Wait on the callback counter directly: WaitForPeers observes this
	// server's own connections, which can be established before any peer's
	// inbound connect callback has run.
	if !gorumstest.WaitUntil(t, 5*time.Second, func() bool { return connects.Load() > 0 }) {
		t.Error("WithConnectCallback never fired; ServerOption was not applied to the local servers")
	}
}

// TestLocalServersAppliesDialOptions verifies that a DialOption passed via
// WithLocalDialOptions is applied to every server's outbound configuration.
// WithLogger's "ready" line is written synchronously when the outbound
// manager is constructed, so this does not depend on any network activity.
func TestLocalServersAppliesDialOptions(t *testing.T) {
	const n = 3
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	_, stop, err := gorums.NewLocalServers(
		n,
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t), gorums.WithLogger(logger)),
	)
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)

	if got := strings.Count(buf.String(), "ready"); got != n {
		t.Errorf("logger recorded %d \"ready\" lines, want %d (one per server); DialOption was not applied to every server", got, n)
	}
}

// TestLocalServersAppliesPerServerOptions verifies that
// WithLocalServerOptionsFunc and WithLocalDialOptionsFunc give each server the
// options for its own node ID, after the shared options.
func TestLocalServersAppliesPerServerOptions(t *testing.T) {
	const n = 3
	var mu sync.Mutex
	sharedChanges := 0
	sawOwnNode := make(map[gorums.ID]bool)
	var sharedLog, perServerLog bytes.Buffer
	_, stop, err := gorums.NewLocalServers(n,
		gorums.WithLocalServerOptions(gorums.WithPeerChange(func(gorums.Config) {
			mu.Lock()
			defer mu.Unlock()
			sharedChanges++
		})),
		gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
			return []gorums.ServerOption{gorums.WithPeerChange(func(cfg gorums.Config) {
				mu.Lock()
				defer mu.Unlock()
				if cfg.Node(id) != nil {
					sawOwnNode[id] = true
				}
			})}
		}),
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t), gorums.WithLogger(log.New(&sharedLog, "", 0))),
		gorums.WithLocalDialOptionsFunc(func(id gorums.ID) []gorums.DialOption {
			return []gorums.DialOption{gorums.WithLogger(log.New(&perServerLog, fmt.Sprintf("server %d: ", id), 0))}
		}),
	)
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)

	mu.Lock()
	defer mu.Unlock()
	// The per-server options come after the shared ones, so their
	// WithPeerChange callback and WithLogger replace the shared ones.
	if sharedChanges != 0 {
		t.Errorf("shared WithPeerChange callback ran %d times; want 0", sharedChanges)
	}
	if sharedLog.Len() != 0 {
		t.Errorf("shared logger recorded %q; want nothing", sharedLog.String())
	}
	for id := gorums.ID(1); id <= n; id++ {
		if !sawOwnNode[id] {
			t.Errorf("server %d: per-server WithPeerChange callback did not see its own node", id)
		}
		if want := fmt.Sprintf("server %d: ready", id); !strings.Contains(perServerLog.String(), want) {
			t.Errorf("per-server log = %q; want a line %q", perServerLog.String(), want)
		}
	}
}

// TestLocalServersNodes verifies that the node factory runs once per listener,
// in ID order, and supplies every server's peer configuration.
func TestLocalServersNodes(t *testing.T) {
	tests := []struct {
		name    string
		factory bool
	}{
		{name: "AddressOnlyFactory", factory: true},
		{name: "NilFactory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ids []gorums.ID
			addrs := make(map[gorums.ID]string)
			var fn func(gorums.ID, string) gorums.NodeAddress
			if tt.factory {
				fn = func(id gorums.ID, addr string) gorums.NodeAddress {
					ids = append(ids, id)
					addrs[id] = addr
					return mock.NodeAddr(addr)
				}
			}
			servers := gorumstest.LocalServers(t, 3, gorums.WithLocalNodes(fn))
			gorumstest.WaitForPeers(t, servers)
			if tt.factory && !slices.Equal(ids, []gorums.ID{1, 2, 3}) {
				t.Errorf("factory called for IDs %v; want [1 2 3]", ids)
			}
			for _, srv := range servers {
				if tt.factory && addrs[srv.NodeID()] != srv.Addr() {
					t.Errorf("factory address for node %d = %q; want %q", srv.NodeID(), addrs[srv.NodeID()], srv.Addr())
				}
				for _, peer := range servers {
					if got := srv.PeerConfig().Node(peer.NodeID()).Address(); got != peer.Addr() {
						t.Errorf("server %d: peer %d address = %q; want %q", srv.NodeID(), peer.NodeID(), got, peer.Addr())
					}
				}
			}
		})
	}
}

// TestLocalServersStopIsIdempotent verifies that the stop function
// returned by NewLocalServers can be called more than once without panicking.
func TestLocalServersStopIsIdempotent(t *testing.T) {
	_, stop, err := gorums.NewLocalServers(3, gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)))
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	stop()
	stop()
}
