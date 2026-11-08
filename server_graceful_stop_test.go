package gorums_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/mockcall"
	"google.golang.org/protobuf/proto"
)

// TestServerGracefulStopWaitsForOneWayHandlers verifies that GracefulStop
// returns only after the handlers of accepted one-way requests have returned:
// one that runs and one that waits behind it. Neither has a pending reply on
// the client. It also covers a handler that calls Release before it blocks.
func TestServerGracefulStopWaitsForOneWayHandlers(t *testing.T) {
	for _, release := range []bool{false, true} {
		name := "Blocked"
		if release {
			name = "ReleasedThenBlocked"
		}
		t.Run(name, func(t *testing.T) {
			gate := make(chan struct{})
			started := make(chan struct{}, 2)
			var finished atomic.Int32
			srv := gorums.NewServer()
			srv.RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, _ *gorums.Message, _ func(proto.Message)) error {
				if release {
					ctx.Release()
				}
				started <- struct{}{}
				<-gate
				finished.Add(1)
				return nil
			})
			node := gorumstest.Node(t, func(int) gorumstest.ServerIface { return srv })

			for range 2 {
				ctx := gorumstest.Context(t, 2*time.Second)
				if err := mockcall.Unicast(ctx, node, "x").Send(); err != nil {
					t.Fatalf("Unicast: %v", err)
				}
			}
			<-started

			stopped := make(chan struct{})
			go func() {
				srv.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
				t.Fatal("GracefulStop returned while accepted one-way handlers were blocked")
			case <-time.After(200 * time.Millisecond):
			}

			close(gate)
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("GracefulStop did not return after the handlers returned")
			}
			if got := finished.Load(); got != 2 {
				t.Errorf("finished handlers = %d, want 2", got)
			}
		})
	}
}

// TestServerGracefulStopUnblocksWaiters verifies that GracefulStop unblocks
// WaitForPeers and WaitForClients before it waits for handlers, so a handler
// that waits in one of them returns with ErrStopped instead of blocking
// GracefulStop.
func TestServerGracefulStopUnblocksWaiters(t *testing.T) {
	never := func(gorums.Config) bool { return false }
	tests := []struct {
		name string
		srv  func(*testing.T) *gorums.Server
		node func(*testing.T, *gorums.Server) *gorums.Node
		wait func(*gorums.Server) error
	}{
		{
			name: "WaitForPeersInLocalCall",
			srv:  func(t *testing.T) *gorums.Server { return gorumstest.LocalServers(t, 1)[0] },
			node: func(_ *testing.T, srv *gorums.Server) *gorums.Node { return srv.PeerConfig().Node(srv.NodeID()) },
			wait: func(srv *gorums.Server) error { return srv.WaitForPeers(context.Background(), never) },
		},
		{
			name: "WaitForClientsInStreamCall",
			srv:  func(*testing.T) *gorums.Server { return gorums.NewServer() },
			node: func(t *testing.T, srv *gorums.Server) *gorums.Node {
				return gorumstest.Node(t, func(int) gorumstest.ServerIface { return srv })
			},
			wait: func(srv *gorums.Server) error { return srv.WaitForClients(context.Background(), never) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.srv(t)
			started := make(chan struct{}, 1)
			waitErr := make(chan error, 1)
			srv.RegisterHandler(mock.TestMethod, func(gorums.ServerContext, *gorums.Message, func(proto.Message)) error {
				started <- struct{}{}
				waitErr <- tt.wait(srv)
				return nil
			})
			ctx := gorumstest.Context(t, 2*time.Second)
			if err := mockcall.Unicast(ctx, tt.node(t, srv), "x").Send(); err != nil {
				t.Fatalf("Unicast: %v", err)
			}
			<-started

			stopped := make(chan struct{})
			go func() {
				srv.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("GracefulStop did not return while a handler waited for a configuration")
			}
			if err := <-waitErr; !errors.Is(err, gorums.ErrStopped) {
				t.Errorf("wait in handler: err = %v, want %v", err, gorums.ErrStopped)
			}
		})
	}
}

// TestServerGracefulStopWaitsForLocalCalls verifies that GracefulStop waits
// for the handler of a one-way call that the server sent to itself, through
// either of its configurations that include its own node.
func TestServerGracefulStopWaitsForLocalCalls(t *testing.T) {
	tests := []struct {
		name string
		cfg  func(*gorums.Server) gorums.Config
	}{
		{name: "PeerConfig", cfg: (*gorums.Server).PeerConfig},
		{name: "ConnectedPeers", cfg: (*gorums.Server).ConnectedPeers},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := gorumstest.LocalServers(t, 1)[0]
			gate := make(chan struct{})
			started := make(chan struct{}, 1)
			var finished atomic.Bool
			srv.RegisterHandler(mock.TestMethod, func(gorums.ServerContext, *gorums.Message, func(proto.Message)) error {
				started <- struct{}{}
				<-gate
				finished.Store(true)
				return nil
			})
			self := tt.cfg(srv).Node(srv.NodeID())
			if self == nil {
				t.Fatalf("%s has no node %d", tt.name, srv.NodeID())
			}
			ctx := gorumstest.Context(t, 2*time.Second)
			if err := mockcall.Unicast(ctx, self, "x").Send(); err != nil {
				t.Fatalf("Unicast to self: %v", err)
			}
			<-started

			stopped := make(chan struct{})
			go func() {
				srv.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
				t.Fatal("GracefulStop returned while a local handler was blocked")
			case <-time.After(200 * time.Millisecond):
			}
			close(gate)
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("GracefulStop did not return after the local handler returned")
			}
			if !finished.Load() {
				t.Error("GracefulStop returned before the local handler finished")
			}
		})
	}
}

// TestServerGracefulStopWaitsForRequestsOnDialedStreams verifies that, with
// stream dedup, GracefulStop waits for the handler of a one-way request that
// a peer sent on a stream that this server dialed.
func TestServerGracefulStopWaitsForRequestsOnDialedStreams(t *testing.T) {
	srvs := setupDedupServers(t, 2)
	// The server with the lower ID dials; the other sends its requests to it
	// on that stream.
	dialer, peer := srvs[0], srvs[1]
	if dialer.NodeID() > peer.NodeID() {
		dialer, peer = peer, dialer
	}
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	var finished atomic.Bool
	dialer.RegisterHandler(mock.TestMethod, func(gorums.ServerContext, *gorums.Message, func(proto.Message)) error {
		started <- struct{}{}
		<-gate
		finished.Store(true)
		return nil
	})
	ctx := gorumstest.Context(t, 2*time.Second)
	if err := mockcall.Unicast(ctx, gorumstest.PeerNode(t, peer.PeerConfig(), dialer.NodeID()), "x").Send(); err != nil {
		t.Fatalf("Unicast to the dialer: %v", err)
	}
	<-started

	stopped := make(chan struct{})
	go func() {
		dialer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("GracefulStop returned while a handler on a dialed stream was blocked")
	case <-time.After(200 * time.Millisecond):
	}
	close(gate)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("GracefulStop did not return after the handler returned")
	}
	if !finished.Load() {
		t.Error("GracefulStop returned before the handler finished")
	}
}

// TestServerGracefulStopDrainsExtendedPeerNodes verifies that GracefulStop
// also ends the streams of the nodes that Config.Extend added to the peer
// configuration in the same way as the other dialed streams: it returns only
// after the peer has handled the requests sent through such a node.
func TestServerGracefulStopDrainsExtendedPeerNodes(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	var finished atomic.Bool
	peer := gorums.NewServer()
	peer.RegisterHandler(mock.TestMethod, func(gorums.ServerContext, *gorums.Message, func(proto.Message)) error {
		started <- struct{}{}
		<-gate
		finished.Store(true)
		return nil
	})
	addrs := gorumstest.Servers(t, 1, func(int) gorumstest.ServerIface { return peer })
	srv := gorumstest.LocalServers(t, 1)[0]
	// Stop the peer before the goroutine-leak check that LocalServers
	// registered; Servers stops it again, which does nothing.
	t.Cleanup(peer.Stop)
	peerID := srv.NodeID() + 1
	cfg, err := srv.PeerConfig().Extend(gorums.WithNodes(map[gorums.ID]mock.NodeAddr{peerID: mock.NodeAddr(addrs[0])}))
	if err != nil {
		t.Fatalf("Extend: %v", err)
	}
	ctx := gorumstest.Context(t, 2*time.Second)
	if err := mockcall.Unicast(ctx, cfg.Node(peerID), "x").Send(); err != nil {
		t.Fatalf("Unicast to the added node: %v", err)
	}
	<-started

	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("GracefulStop returned while the peer was handling a request sent through an added node")
	case <-time.After(200 * time.Millisecond):
	}
	close(gate)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("GracefulStop did not return after the peer handled the request")
	}
	if !finished.Load() {
		t.Error("GracefulStop returned before the peer finished handling the request")
	}
}
