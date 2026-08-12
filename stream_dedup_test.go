package gorums_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	gorumsimpl "github.com/relab/gorums/runtime/gorumsimpl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func setupDedupServers(t *testing.T, n int) []*gorums.Server {
	t.Helper()
	servers := gorumstest.LocalServers(t, n, gorums.WithStreamDedup())
	waitForDedup(t, servers)
	return servers
}

func waitForDedup(t *testing.T, servers []*gorums.Server) {
	t.Helper()
	ctx := gorumstest.Context(t, 10*time.Second)
	for _, srv := range servers {
		if _, err := srv.WaitForAll(ctx); err != nil {
			t.Fatalf("WaitForAll: %v", err)
		}
	}
}

func waitForDedupConcurrent(t *testing.T, servers []*gorums.Server) {
	t.Helper()
	ctx := gorumstest.Context(t, 10*time.Second)
	var wg sync.WaitGroup
	errs := make([]error, len(servers))
	// Concurrent readers exercise the shared-node topology while WaitForAll
	// runs; born-shared nodes never change transports, so readers must
	// observe a stable topology throughout.
	done := make(chan struct{})
	var readers sync.WaitGroup
	for _, srv := range servers {
		readers.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
					for _, node := range srv.PeerConfig().Nodes() {
						_ = node.IsShared()
					}
					// Yield between passes. One reader per server means the
					// largest case runs 50 of these, and without a yield they
					// would spin flat out on every core for as long as
					// WaitForAll takes, starving the work under test.
					runtime.Gosched()
				}
			}
		})
	}
	for i, srv := range servers {
		wg.Go(func() {
			_, errs[i] = srv.WaitForAll(ctx)
		})
	}
	wg.Wait()
	close(done)
	readers.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("server %d: WaitForAll: %v", i+1, err)
		}
	}
}

func TestStreamDedupWaitForAllConcurrent(t *testing.T) {
	for _, n := range []int{3, 5, 15, 50} {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			servers := gorumstest.LocalServers(t, n, gorums.WithStreamDedup())
			waitForDedupConcurrent(t, servers)
			for i, srv := range servers {
				if got := srv.PeerConfig().Size(); got != n {
					t.Fatalf("server %d outbound size = %d, want %d", i+1, got, n)
				}
			}
		})
	}
}

func TestStreamDedupWaitForAllRejectsInvalidSetup(t *testing.T) {
	tests := []struct {
		name      string
		opts      func(t *testing.T) []gorums.ServerOption
		wantError string
	}{
		{
			// A server without its own node ID cannot enable stream dedup:
			// dedup's single-dialer rule requires a nonzero local node ID.
			name: "MissingLocalNodeID",
			opts: func(t *testing.T) []gorums.ServerOption {
				return []gorums.ServerOption{
					gorums.WithStreamDedup(),
					gorums.WithPeers(0, gorums.WithNodes(map[uint32]testNode{
						1: {addr: "127.0.0.1:9081"},
					}), gorumstest.InsecureDialOptions(t)),
				}
			},
			wantError: "nonzero local node ID",
		},
		{
			// The peer list must contain the local node so it is part of the
			// deduplicated configuration.
			name: "SelfMissingFromKnownPeers",
			opts: func(t *testing.T) []gorums.ServerOption {
				return []gorums.ServerOption{
					gorums.WithStreamDedup(),
					gorums.WithPeers(2, gorums.WithNodes(map[uint32]testNode{
						1: {addr: "127.0.0.1:9081"},
					}), gorumstest.InsecureDialOptions(t)),
				}
			},
			wantError: "does not contain local node 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]gorums.ServerOption{gorums.WithAddr("127.0.0.1:0")}, tt.opts(t)...)
			srv := gorums.NewServer(opts...)
			t.Cleanup(srv.Stop)

			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			_, err := srv.WaitForAll(ctx)
			if err == nil {
				t.Fatalf("WaitForAll succeeded, want error containing %q", tt.wantError)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("WaitForAll waited for context expiry: %v", err)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("WaitForAll error = %q, want substring %q", err, tt.wantError)
			}
		})
	}
}

// TestStreamDedupConnectedPeersIncludesDialedPeers verifies that under stream
// deduplication every server eventually observes the full peer set in
// ConnectedPeers, including the higher-ID peers whose shared connections this
// server itself dials and owns.
func TestStreamDedupConnectedPeersIncludesDialedPeers(t *testing.T) {
	const n = 3
	servers := setupDedupServers(t, n)
	for i, srv := range servers {
		ctx := gorumstest.Context(t, 5*time.Second)
		if err := srv.WaitForPeers(ctx, func(cfg gorums.Config) bool {
			return cfg.Size() == n
		}); err != nil {
			t.Errorf("server %d: WaitForPeers(size==%d): %v; ConnectedPeers = %v",
				i+1, n, err, srv.ConnectedPeers().NodeIDs())
		}
	}
}

func TestStreamDedupSharedNodes(t *testing.T) {
	const n = 5
	t.Run("Dual", func(t *testing.T) {
		servers := gorumstest.LocalServers(t, n)
		awaitServerReady(t, servers)
		for i, srv := range servers {
			for _, node := range srv.PeerConfig().Nodes() {
				if node.IsShared() {
					t.Fatalf("server %d node %d IsShared = true, want false", i+1, node.ID())
				}
			}
		}
	})

	t.Run("Dedup", func(t *testing.T) {
		servers := setupDedupServers(t, n)
		for i, srv := range servers {
			shared := 0
			for _, node := range srv.PeerConfig().Nodes() {
				if node.IsShared() {
					shared++
				}
			}
			// A server with i lower-ID peers (0-indexed) has exactly that
			// many born-shared outbound nodes.
			if shared != i {
				t.Fatalf("server %d shared node count = %d, want %d", i+1, shared, i)
			}
		}
	})
}

// TestStreamDedupNodesBornShared verifies that under stream deduplication the
// outbound node for every lower-ID peer is born shared: it borrows the peer's
// inbound channel slot from construction and never dials, so the shared
// topology holds before [gorums.Server.WaitForAll] and regardless of when the
// peers connect. There is no consolidation step that could reshape the
// topology later.
func TestStreamDedupNodesBornShared(t *testing.T) {
	const n = 3
	servers := gorumstest.LocalServers(t, n, gorums.WithStreamDedup())
	for i, srv := range servers {
		localID := uint32(i + 1)
		for _, node := range srv.PeerConfig().Nodes() {
			wantShared := node.ID() < localID
			if node.IsShared() != wantShared {
				t.Errorf("server %d node %d IsShared = %t, want %t", localID, node.ID(), node.IsShared(), wantShared)
			}
			if wantShared && node.IsOutbound() {
				t.Errorf("server %d node %d IsOutbound = true, want false (a born-shared node must not dial)", localID, node.ID())
			}
		}
	}
}

// TestStreamDedupCallBeforePeerConnectsFailsFast verifies that a call to a
// born-shared node whose peer has not yet connected fails fast with the
// stream-down error instead of blocking: the node has no stream to borrow and
// cannot dial (only the lower-ID peer of the pair dials), so failing fast
// lets quorum logic count the peer as unavailable. No server is started, so
// the lower-ID peer never connects.
func TestStreamDedupCallBeforePeerConnectsFailsFast(t *testing.T) {
	servers, stop, err := gorums.NewLocalServers(
		2,
		gorums.WithLocalServerOptions(gorums.WithStreamDedup()),
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)),
	)
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)

	var node1 *gorums.Node
	for _, node := range servers[1].PeerConfig().Nodes() {
		if node.ID() == 1 {
			node1 = node
			break
		}
	}
	if node1 == nil {
		t.Fatal("server 2 outbound configuration does not contain node 1")
	}

	ctx := gorumstest.Context(t, 5*time.Second)
	start := time.Now()
	_, err = gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
		gorums.Config{node1}.Context(ctx),
		pb.String("hello"),
		mock.TestMethod,
	).Threshold(1)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("QuorumCall to a disconnected born-shared node succeeded, want stream-down failure")
	}
	if !errors.Is(err, gorums.ErrStreamDown) {
		t.Fatalf("QuorumCall error = %v, want errors.Is(err, gorums.ErrStreamDown)", err)
	}
	if elapsed > time.Second {
		t.Fatalf("QuorumCall took %v, want fail-fast well under the context deadline", elapsed)
	}
}

func TestStreamDedupRetainedConfigurationsRemainUsable(t *testing.T) {
	servers := gorumstest.LocalServers(t, 3, gorums.WithStreamDedup())
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, stringEchoHandler("echo"))
	}

	retained := servers[2].PeerConfig()
	retainedSubset := retained.Remove(3)
	waitForDedup(t, servers)
	current := servers[2].PeerConfig()

	for i, node := range retained {
		if node != current[i] {
			t.Errorf("node %d identity changed across WaitForAll", node.ID())
		}
		wantShared := node.ID() < 3
		if node.IsShared() != wantShared {
			t.Errorf("retained node %d IsShared = %t, want %t", node.ID(), node.IsShared(), wantShared)
		}
	}

	ctx := gorumstest.Context(t, 3*time.Second)
	for name, cfg := range map[string]gorums.Config{
		"Full":   retained,
		"Subset": retainedSubset,
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
				cfg.Context(ctx),
				pb.String("hello"),
				mock.TestMethod,
			).Majority()
			if err != nil {
				t.Fatalf("QuorumCall: %v", err)
			}
			if got, want := resp.GetValue(), "echo: hello"; got != want {
				t.Fatalf("QuorumCall response = %q, want %q", got, want)
			}
		})
	}
}

func TestStreamDedupMulticastAndQuorumCall(t *testing.T) {
	servers := setupDedupServers(t, 3)
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, stringEchoHandler("echo"))
	}

	ctx := gorumstest.Context(t, 3*time.Second)
	cfg := servers[0].PeerConfig()
	if err := gorumsimpl.Multicast(cfg.Context(ctx), pb.String("hello"), mock.TestMethod).Send(); err != nil {
		t.Fatalf("Multicast: %v", err)
	}
	resp, err := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
		cfg.Context(ctx),
		pb.String("hello"),
		mock.TestMethod,
	).Majority()
	if err != nil {
		t.Fatalf("QuorumCall: %v", err)
	}
	if got, want := resp.GetValue(), "echo: hello"; got != want {
		t.Fatalf("QuorumCall response = %q, want %q", got, want)
	}
}

// TestStreamDedupOwnerReconnectHealsBorrower verifies the born-shared recovery
// path end to end: after the lower-ID owner's shared stream drops while both
// peers are idle, the higher-ID borrower loses the peer, the owner
// re-establishes the stream on its own with no application send, and the
// borrower's call to that peer then succeeds again.
//
// The drop is forced with a short server-side MaxConnectionAge, which closes
// the connection the owner dialed in. Only the owner (the lower ID of a pair)
// dials, so that connection carries the single shared stream. The age limit
// recurs on every reconnect, so the drop is observed even when the owner
// reconnects immediately.
func TestStreamDedupOwnerReconnectHealsBorrower(t *testing.T) {
	const maxAge = 300 * time.Millisecond
	servers := gorumstest.LocalServers(
		t, 2,
		gorums.WithStreamDedup(),
		gorums.WithGRPCServerOptions(grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge:      maxAge,
			MaxConnectionAgeGrace: 50 * time.Millisecond,
		})),
	)
	for _, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, stringEchoHandler("echo"))
	}
	waitForDedup(t, servers)

	// servers[1] is the higher-ID borrower (node 2); it reaches node 1 over the
	// stream node 1 dialed, so node 1's state is observed from node 2's view.
	borrower := servers[1]
	const ownerID = 1
	hasOwner := func(cfg gorums.Config) bool { return cfg.Contains(ownerID) }
	missingOwner := func(cfg gorums.Config) bool { return !cfg.Contains(ownerID) }

	ctx := gorumstest.Context(t, 10*time.Second)

	// The owner's shared stream drops under MaxConnectionAge, so the borrower
	// loses the owner from its connected view.
	if err := borrower.WaitForPeers(ctx, missingOwner); err != nil {
		t.Fatalf("owner's shared stream never dropped: %v", err)
	}
	// The owner re-establishes the stream on its own — no application send
	// happens here — so the borrower regains the owner.
	if err := borrower.WaitForPeers(ctx, hasOwner); err != nil {
		t.Fatalf("owner did not reconnect its dropped idle stream: %v", err)
	}

	// Locate the borrower's shared node for the owner.
	var owner *gorums.Node
	for _, node := range borrower.PeerConfig().Nodes() {
		if node.ID() == ownerID {
			owner = node
			break
		}
	}
	if owner == nil {
		t.Fatal("borrower configuration does not contain the owner node")
	}

	// The borrower's call to the owner recovers. Retrying tolerates the
	// recurring age cycle: a drop can land between the observation above and
	// the call, after which the owner reconnects and the retry succeeds.
	recovered := gorumstest.WaitUntil(t, 10*time.Second, func() bool {
		callCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		resp, err := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
			gorums.Config{owner}.Context(callCtx),
			pb.String("hello"),
			mock.TestMethod,
		).Threshold(1)
		return err == nil && resp.GetValue() == "echo: hello"
	})
	if !recovered {
		t.Fatal("borrower's call to the owner never recovered after the idle reconnect")
	}
}

func TestStreamDedupChainedQuorumCall(t *testing.T) {
	servers := setupDedupServers(t, 3)
	for i, srv := range servers {
		myID := i + 1
		srv.RegisterHandler(mock.TestMethod, outerChainedHandler(t, myID, false, mock.EchoMethod, (*gorums.Responses[*pb.StringValue]).Majority))
		srv.RegisterHandler(mock.EchoMethod, stringEchoHandler("inner-echo"))
	}

	ctx := gorumstest.Context(t, 3*time.Second)
	res, err := gorumsimpl.QuorumCall[*pb.StringValue, *pb.StringValue](
		servers[2].PeerConfig().Context(ctx),
		pb.String("outer-call"),
		mock.TestMethod,
	).Majority()
	if err != nil {
		t.Fatalf("QuorumCall: %v", err)
	}
	if got, want := res.GetValue(), "outer-call | inner-echo: outer-call"; !strings.Contains(got, want) {
		t.Fatalf("QuorumCall response = %q, want to contain %q", got, want)
	}
}
