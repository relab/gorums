package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
	"github.com/relab/gorums/gorumstest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// TestSetupTargetLocal verifies that local mode (no self, no remotes) builds a
// ready symmetric target and fills in the topology-derived Options fields.
func TestSetupTargetLocal(t *testing.T) {
	var opts benchkit.Options
	target, cleanup, err := SetupTarget(&opts, "", nil, 3, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupTarget: %v", err)
	}
	t.Cleanup(cleanup)

	if target.Symmetric == nil {
		t.Error("Symmetric target is nil, want local symmetric servers")
	}
	if opts.Remote {
		t.Error("opts.Remote = true, want false in local mode")
	}
	if opts.NumNodes != 3 {
		t.Errorf("opts.NumNodes = %d, want 3", opts.NumNodes)
	}
}

// TestSetupTargetLocalRejectsNonPositiveConfigSize verifies that local mode
// (no self, no remotes) rejects a config size below 1 instead of creating a
// degenerate zero-server target that "succeeds" without doing any work.
func TestSetupTargetLocalRejectsNonPositiveConfigSize(t *testing.T) {
	for _, configSize := range []int{0, -1} {
		var opts benchkit.Options
		_, _, err := SetupTarget(&opts, "", nil, configSize, gorumstest.InsecureDialOptions(t))
		if err == nil {
			t.Errorf("SetupTarget(local, config-size=%d) = nil error, want error", configSize)
		}
	}
}

// TestSetupTargetDistributedRequiresPeers verifies that distributed mode with
// fewer than two remotes fails instead of running a degenerate benchmark.
func TestSetupTargetDistributedRequiresPeers(t *testing.T) {
	var opts benchkit.Options
	_, _, err := SetupTarget(&opts, "127.0.0.1:9000", []string{"127.0.0.1:9000"}, 0, gorumstest.InsecureDialOptions(t))
	if err == nil {
		t.Fatal("SetupTarget(distributed, 1 remote) = nil error, want error")
	}
}

// TestDualReconnectsDroppedIdleStream verifies that in dual mode a symmetric
// server re-establishes an outbound stream that dropped while idle — with no
// local send prompting it — so the peer stays reachable in its connected-peer
// configuration. During setup no node sends application traffic, so a stream
// that is never re-established leaves the node without its peer and stalls
// the readiness probe.
//
// The drop is forced deterministically with a short server-side
// MaxConnectionAge: gRPC sends GOAWAY and closes the connection, ending the
// outbound stream the other node dialed in. This is more reliable than a
// refused initial connect, which gRPC papers over by retrying the connection
// underneath a still-pending stream. The age limit recurs on every reconnected
// stream, so the gap is observed even if a reconnect follows immediately.
//
// The servers are built through the shared test framework
// ([gorumstest.LocalServers]), which owns listener allocation and cleanup for
// the whole test. The age limit is applied to both symmetric servers; in a
// two-node group the observed node's single outbound stream is dropped by its
// peer's age limit either way. The drop and reconnect are observed from one
// node, whose connected-peer view tracks its outbound stream state.
func TestDualReconnectsDroppedIdleStream(t *testing.T) {
	const maxAge = 300 * time.Millisecond
	servers := gorumstest.LocalServers(t, 2, gorums.WithGRPCServerOptions(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge:      maxAge,
			MaxConnectionAgeGrace: 50 * time.Millisecond,
		}),
	))

	observer := servers[0]
	const peerID = 2
	hasPeer := func(cfg gorums.Config) bool { return cfg.Contains(peerID) }
	missingPeer := func(cfg gorums.Config) bool { return !cfg.Contains(peerID) }

	ctx := gorumstest.Context(t, 10*time.Second)

	// The mesh forms from the senders' eager initial connect, with no sends.
	if err := observer.WaitForPeers(ctx, hasPeer); err != nil {
		t.Fatalf("outbound stream to the peer never came up: %v", err)
	}

	// The peer's MaxConnectionAge closes the connection the observer dialed in,
	// dropping the observer's outbound stream, so the peer leaves the observer's
	// connected view. The stream-state change broadcasts a config change, and
	// the age limit recurs on every reconnected stream, so the gap is observed
	// even if a reconnect follows immediately.
	if err := observer.WaitForPeers(ctx, missingPeer); err != nil {
		t.Fatalf("MaxConnectionAge never dropped the idle outbound stream: %v", err)
	}

	// The observer must re-establish its dropped stream on its own — no sends
	// happen here — so the peer returns to its connected view. Without a
	// self-initiated reconnect the observer has lost the peer for good.
	if err := observer.WaitForPeers(ctx, hasPeer); err != nil {
		t.Fatalf("did not reconnect the dropped idle stream: %v", err)
	}
}

// TestDedupSetupProbesSharedTopology verifies that the setup sequence
// setupDistributed/setupLocal use in dedup mode — the dedup wait
// (awaitStreamDedup, which calls Server.WaitForAll), then the outbound
// probe — leaves every lower-ID outbound peer backed by its live shared
// inbound stream before the probe runs, and that the probe then succeeds
// against that shared topology. Probing before the dedup wait would fail
// fast with ErrStreamDown for any lower-ID peer that has not yet connected.
func TestDedupSetupProbesSharedTopology(t *testing.T) {
	targets := localSymmetricTargets(t, 3, gorums.WithStreamDedup())

	// One ctx is shared across the sequential dedup-wait and probe steps
	// below. Setup is in-process over kept-open listeners, so it completes in
	// milliseconds; the timeout only bounds a genuinely stuck peer.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Step 1: the dedup wait, exactly as setupDistributed does before probing.
	for _, target := range targets {
		if err := awaitStreamDedup(ctx, target); err != nil {
			t.Fatalf("awaitStreamDedup: %v", err)
		}
	}

	// By the time the probe runs, every peer with a lower ID than node 3
	// must be a shared node backed by a live inbound stream.
	srv3 := targets[2].servers[0]
	for _, node := range srv3.PeerConfig() {
		if node.ID() >= 3 {
			continue
		}
		if node.IsOutbound() {
			t.Errorf("node %d: IsOutbound() = true, want false (expected a shared inbound stream before the probe)", node.ID())
		}
		if !node.IsInbound() {
			t.Errorf("node %d: IsInbound() = false, want true (expected a shared inbound stream before the probe)", node.ID())
		}
	}

	// Step 2: the probe must succeed against that shared topology.
	for _, target := range targets {
		if err := awaitReady(ctx, target); err != nil {
			t.Fatalf("awaitReady: %v", err)
		}
	}
}

// TestSetupTargetCoordinatorNumNodes verifies the coordinator-mode node-count
// clamping: configSize selects a prefix of the remotes when within range and
// all remotes otherwise.
func TestSetupTargetCoordinatorNumNodes(t *testing.T) {
	remotes := []string{"127.0.0.1:9001", "127.0.0.1:9002", "127.0.0.1:9003"}
	tests := []struct {
		name       string
		configSize int
		wantNodes  int
	}{
		{"WithinRangeSelectsPrefix", 2, 2},
		{"ZeroUsesAll", 0, 3},
		{"TooLargeUsesAll", 5, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts benchkit.Options
			target, cleanup, err := SetupTarget(&opts, "", remotes, tt.configSize, gorumstest.InsecureDialOptions(t))
			if err != nil {
				t.Fatalf("SetupTarget: %v", err)
			}
			t.Cleanup(cleanup)

			if target.Config == nil {
				t.Error("Config target is nil, want coordinator configuration")
			}
			if !opts.Remote {
				t.Error("opts.Remote = false, want true in coordinator mode")
			}
			if opts.NumNodes != tt.wantNodes {
				t.Errorf("opts.NumNodes = %d, want %d", opts.NumNodes, tt.wantNodes)
			}
			if got := target.Config.Size(); got != tt.wantNodes {
				t.Errorf("config size = %d, want %d", got, tt.wantNodes)
			}
		})
	}
}
