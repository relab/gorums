package gorums_test

import (
	"bytes"
	"context"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
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
