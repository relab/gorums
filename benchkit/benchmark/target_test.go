package benchmark

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
	"github.com/relab/gorums/gorumstest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// captureDiag redirects the probe-stall self-diagnosis to a buffer for the
// duration of the test, so failure-path tests can assert on the diagnosis
// content without spamming the test log with goroutine dumps.
func captureDiag(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := diagWriter
	diagWriter = &buf
	t.Cleanup(func() { diagWriter = orig })
	return &buf
}

// localServers builds n peers with gorums.NewLocalServers and returns the raw,
// unstarted servers. It reuses the Gorums test framework's listener allocation,
// which binds every listener once and keeps it open for the lifetime of the
// server. Keeping each listener open avoids a close-to-rebind race under
// repeated test runs.
//
// The servers are returned unstarted so a test can register per-node handlers,
// serve only a subset, or stagger serving — the independent per-node control a
// real multi-process distributed run has, which single-target helpers hide.
func localServers(t *testing.T, n int, serverOpt gorums.ServerOption) []*gorums.Server {
	t.Helper()
	servers, stop, err := gorums.NewLocalServers(
		n,
		gorums.WithLocalServerOptions(serverOpt),
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)),
	)
	if err != nil {
		t.Fatalf("NewLocalServers: %v", err)
	}
	t.Cleanup(stop)
	return servers
}

// benchTarget wraps one server as a single-server SymmetricTarget with the
// benchkit Control plane and workload server attached, so it can be passed to
// AwaitReady and the other per-node setup helpers. numPeers is the full cluster
// size (arms Done tracking and sizes the exit grace period). Call before
// serving srv, since attaching registers services.
func benchTarget(srv *gorums.Server, numPeers int) *SymmetricTarget {
	ctrl := attachBenchServer(srv)
	ctrl.ArmDone(numPeers) // match SetupRemoteServer, which arms Done tracking for the exit barrier
	return &SymmetricTarget{
		servers:  []*gorums.Server{srv},
		controls: []*benchkit.Control{ctrl},
		numPeers: numPeers,
		selfAddr: srv.Addr(),
		labels:   []string{fmt.Sprintf("node %d (%s)", ctrl.SelfID(), srv.Addr())},
	}
}

// localSymmetricTargets builds n single-server SymmetricTargets over one local
// node list, each wrapping one node (target[i] is node ID i+1) and already
// serving, so a test can drive per-node setup — dedup wait, probe — in a
// controlled order, the way separate SetupRemoteServer instances do in a
// real distributed run, but without freeTCPAddrs's port-reuse race.
func localSymmetricTargets(t *testing.T, n int, serverOpt gorums.ServerOption) []*SymmetricTarget {
	t.Helper()
	servers := localServers(t, n, serverOpt)
	targets := make([]*SymmetricTarget, n)
	for i, srv := range servers {
		targets[i] = benchTarget(srv, n)
	}
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}
	return targets
}

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

// TestSetupSymmetricServersAppliesAllServerOptions verifies that
// SetupSymmetricServers forwards every option in the given slice to the
// in-process servers, not just the first. setupLocal previously called
// SetupSymmetricServers with a single gorums.ServerOption argument (only
// opts.StreamDedupOption()), so any other option opts.ServerOptions() would
// have supplied — e.g. buffer sizes — was silently dropped for local-mode
// runs; a local buffer-size sweep ran every arm with the default capacities
// while the recorded results claimed otherwise.
//
// Stream deduplication is the observable option here: it makes a peer with a
// lower ID than this node borrow that peer's channel instead of dialing its
// own, which Node.IsShared reports structurally, before any peer connects. A
// connect callback is the second, independent option, confirming that both
// elements of the slice reached the server rather than only the first.
func TestSetupSymmetricServersAppliesAllServerOptions(t *testing.T) {
	var connects atomic.Int32
	opts := []gorums.ServerOption{
		gorums.WithStreamDedup(),
		gorums.WithConnectCallback(func(context.Context) { connects.Add(1) }),
	}
	target, stop, err := SetupSymmetricServers(3, opts, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range target.servers {
		if _, err := srv.WaitForAll(ctx); err != nil {
			t.Fatalf("WaitForAll: %v", err)
		}
	}

	srv3 := target.servers[2]
	for _, node := range srv3.PeerConfig() {
		if node.ID() >= 3 {
			continue
		}
		if !node.IsShared() {
			t.Errorf("node %d: IsShared() = false, want true; the stream-dedup option did not reach the server", node.ID())
		}
	}
	if got := connects.Load(); got == 0 {
		t.Error("connect callback never fired; the connect-callback option did not reach the server")
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

// TestSetupRemoteServerAppliesServerOption verifies that the ServerOption
// reaches the server built for distributed mode. The option carries the run's
// stream topology, so dropping it would leave every cluster sweep running the
// default topology while its results were labeled otherwise.
//
// Stream deduplication is the observable case: it makes a peer with a lower ID
// than this node borrow that peer's channel instead of dialing its own, which
// Node.IsShared reports structurally, before any peer connects.
func TestSetupRemoteServerAppliesServerOption(t *testing.T) {
	// Self is the higher address, so the sorted peer list gives it ID 2 and the
	// remaining peer ID 1; only a lower-ID peer is borrowed under dedup.
	peers := []string{"127.0.0.1:0", "127.0.0.2:0"}
	tests := []struct {
		name       string
		serverOpts []gorums.ServerOption
		wantShared bool
	}{
		{name: "WithoutDedup"},
		{name: "WithDedup", serverOpts: []gorums.ServerOption{gorums.WithStreamDedup()}, wantShared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, stop, err := SetupRemoteServer(peers[1], peers, tt.serverOpts, gorumstest.InsecureDialOptions(t))
			if err != nil {
				t.Fatalf("SetupRemoteServer(%s): %v", peers[1], err)
			}
			t.Cleanup(stop)

			cfg := target.servers[0].PeerConfig()
			var peer *gorums.Node
			for _, n := range cfg.Nodes() {
				if n.ID() == 1 {
					peer = n
				}
			}
			if peer == nil {
				t.Fatalf("peer with ID 1 not in peer config %v", cfg.NodeIDs())
			}
			if got := peer.IsShared(); got != tt.wantShared {
				t.Errorf("peer 1 IsShared() = %v, want %v; the ServerOption did not reach the server",
					got, tt.wantShared)
			}
		})
	}
}

// TestSetupRemoteServerBindsWildcard verifies the distributed-mode listener
// binds the wildcard address rather than whatever the local host resolves its
// own name to. Hosts following the Debian convention map their own hostname
// to 127.0.1.1 in /etc/hosts, which would put the listener on loopback and
// make it unreachable for all peers (see doc/benchkit-troubleshooting.html).
func TestSetupRemoteServerBindsWildcard(t *testing.T) {
	// This test must exercise SetupRemoteServer directly, because it is
	// SetupRemoteServer (not the local test framework, which binds 127.0.0.1)
	// that binds the wildcard host. Port 0 lets SetupRemoteServer pick its own
	// free port, so no port is reserved and released beforehand — avoiding the
	// bind-reuse race. The two peers differ only so the sort/self-index is
	// stable; only self (the lower address) is bound.
	peers := []string{"127.0.0.1:0", "127.0.0.2:0"}
	target, stop, err := SetupRemoteServer(peers[0], peers, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupRemoteServer(%s): %v", peers[0], err)
	}
	t.Cleanup(stop)

	// ListenAndServe binds the wildcard listener in a goroutine, so wait until
	// Addr reports the concrete bound port rather than the configured ":0".
	var addr string
	if !gorumstest.WaitUntil(t, 2*time.Second, func() bool {
		addr = target.servers[0].Addr()
		_, p, e := net.SplitHostPort(addr)
		return e == nil && p != "" && p != "0"
	}) {
		t.Fatalf("listener did not bind a concrete port; Addr = %q", addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%s): %v", addr, err)
	}
	// The host must be the wildcard (empty or unspecified), never the loopback
	// or resolved hostname — the Debian 127.0.1.1 trap this guards against.
	if ip := net.ParseIP(host); host != "" && (ip == nil || !ip.IsUnspecified()) {
		t.Errorf("listener bound to host %q, want wildcard", host)
	}
	// Port 0 randomizes the bound port, so exact port preservation is not
	// asserted here; the binding must still resolve to a concrete port.
	if port == "" || port == "0" {
		t.Errorf("listener bound to port %q, want a concrete port", port)
	}
}

// TestExitGrace verifies the distributed-mode exit grace grows with cluster
// size, stays at or above the base floor, and is clamped for large clusters.
func TestExitGrace(t *testing.T) {
	const (
		base     = 3 * time.Second
		perNode  = 300 * time.Millisecond
		maxGrace = 20 * time.Second
	)
	tests := []struct {
		numNodes int
		want     time.Duration
	}{
		{0, base},
		{3, base + 3*perNode},
		{25, base + 25*perNode},
		{120, maxGrace}, // base + 120*perNode = 21s, clamped to maxGrace
	}
	for _, tt := range tests {
		if got := ExitGrace(tt.numNodes); got != tt.want {
			t.Errorf("ExitGrace(%d) = %v, want %v", tt.numNodes, got, tt.want)
		}
	}
	// The grace must never decrease as the cluster grows.
	prev := ExitGrace(1)
	for n := 2; n <= 200; n++ {
		got := ExitGrace(n)
		if got < prev {
			t.Fatalf("ExitGrace(%d) = %v < ExitGrace(%d) = %v; want non-decreasing", n, got, n-1, prev)
		}
		prev = got
	}
}

// TestAwaitReadyStaggeredRemoteStartup verifies distributed readiness tolerates
// one node starting before its peer. Both listeners are bound up front by the
// framework, so the stagger is in when each node begins serving (accepting gRPC
// streams): node 1 serves 500ms before node 2, and node 1's outbound stream to
// node 2 must retry across that gap rather than fail readiness.
func TestAwaitReadyStaggeredRemoteStartup(t *testing.T) {
	servers := localServers(t, 2, nil)
	target1, target2 := benchTarget(servers[0], 2), benchTarget(servers[1], 2)

	go func() { _ = servers[0].ListenAndServe() }()
	time.Sleep(500 * time.Millisecond)
	go func() { _ = servers[1].ListenAndServe() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	errCh := make(chan error, 2)
	go func() { errCh <- AwaitReady(ctx, target1) }()
	go func() { errCh <- AwaitReady(ctx, target2) }()

	var errs error
	for range 2 {
		if err := <-errCh; err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		t.Fatalf("AwaitReady after staggered startup: %v", errs)
	}
	if got := target1.servers[0].ConnectedPeers().Size(); got != 2 {
		t.Errorf("target1 connected config size = %d, want 2", got)
	}
	if got := target2.servers[0].ConnectedPeers().Size(); got != 2 {
		t.Errorf("target2 connected config size = %d, want 2", got)
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
		if err := AwaitReady(ctx, target); err != nil {
			t.Fatalf("AwaitReady: %v", err)
		}
	}
}

// TestAwaitPeersDoneOrGraceReturnsEarlyWhenAllSignal verifies that once every
// peer has called SignalDone, AwaitPeersDoneOrGrace returns true well before
// a deliberately long grace period elapses, instead of always sleeping it out.
func TestAwaitPeersDoneOrGraceReturnsEarlyWhenAllSignal(t *testing.T) {
	targets := localSymmetricTargets(t, 2, nil)
	target1, target2 := targets[0], targets[1]

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target1); err != nil {
		t.Fatalf("AwaitReady(target1): %v", err)
	}
	if err := AwaitReady(ctx, target2); err != nil {
		t.Fatalf("AwaitReady(target2): %v", err)
	}

	const grace = 10 * time.Second
	SignalDone(ctx, target1)
	SignalDone(ctx, target2)

	type result struct {
		allDone bool
		elapsed time.Duration
	}
	results := make(chan result, 2)
	for _, target := range []*SymmetricTarget{target1, target2} {
		go func(target *SymmetricTarget) {
			start := time.Now()
			allDone := AwaitPeersDoneOrGrace(context.Background(), target, grace)
			results <- result{allDone, time.Since(start)}
		}(target)
	}
	for range 2 {
		r := <-results
		if !r.allDone {
			t.Error("AwaitPeersDoneOrGrace = false, want true when all peers signal Done")
		}
		if r.elapsed > grace/2 {
			t.Errorf("AwaitPeersDoneOrGrace took %v, want well under grace=%v", r.elapsed, grace)
		}
	}
}

// TestAwaitPeersDoneOrGraceFallsBackWhenPeerNeverSignals verifies that a peer
// which never calls SignalDone does not hang or fail the waiter: the waiter
// falls back to the grace deadline and returns false.
func TestAwaitPeersDoneOrGraceFallsBackWhenPeerNeverSignals(t *testing.T) {
	targets := localSymmetricTargets(t, 2, nil)
	target1, target2 := targets[0], targets[1]

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target1); err != nil {
		t.Fatalf("AwaitReady(target1): %v", err)
	}
	if err := AwaitReady(ctx, target2); err != nil {
		t.Fatalf("AwaitReady(target2): %v", err)
	}

	// target2 never signals Done; only target1 does.
	const grace = 500 * time.Millisecond
	SignalDone(context.Background(), target1)

	start := time.Now()
	allDone := AwaitPeersDoneOrGrace(context.Background(), target1, grace)
	elapsed := time.Since(start)

	if allDone {
		t.Error("AwaitPeersDoneOrGrace = true, want false when a peer never signals Done")
	}
	if elapsed < grace {
		t.Errorf("AwaitPeersDoneOrGrace returned after %v, want at least grace=%v", elapsed, grace)
	}
	if elapsed > grace+2*time.Second {
		t.Errorf("AwaitPeersDoneOrGrace returned after %v, want close to grace=%v", elapsed, grace)
	}
	if got := target1.controls[0].MissingDone(); len(got) != 1 {
		t.Errorf("MissingDone() = %v, want exactly 1 missing peer", got)
	}
}

// captureProbeLog redirects the outbound-probe progress log to a buffer for
// the duration of the test, so probe tests can assert that stragglers were
// logged by node ID. The probe logs from the calling goroutine only, so the
// buffer needs no locking as long as it is read after AwaitReady returns.
func captureProbeLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := probeLogf
	probeLogf = func(format string, args ...any) { fmt.Fprintf(&buf, format, args...) }
	t.Cleanup(func() { probeLogf = orig })
	return &buf
}

// registerReplyDroppingPeer registers a QuorumCall handler on srv that silently
// sends no reply for the first drop echo requests it receives — mirroring a
// reply lost to stream churn (no error reaches the caller). Later requests echo
// normally. Call before serving srv, since it registers a service. Used to make
// one peer in a localServers mesh a reply-dropping node.
func registerReplyDroppingPeer(srv *gorums.Server, drop int64) {
	var remaining atomic.Int64
	remaining.Store(drop)
	srv.RegisterHandler("benchmark.Benchmark.QuorumCall", func(_ gorums.ServerContext, in *gorums.Message) (*gorums.Message, error) {
		if remaining.Add(-1) >= 0 {
			return nil, nil // no response and no error: nothing is sent back
		}
		return gorums.NewResponseMessage(in, gorums.AsProto[*Echo](in)), nil
	})
}

// TestAwaitReadyProbeRetriesDroppedReply verifies the outbound probe survives a
// peer that silently loses exactly one echo reply: the per-peer attempt times
// out after probeAttemptTimeout instead of consuming the whole readiness
// deadline, the straggler is logged by node ID, and a later round succeeds.
func TestAwaitReadyProbeRetriesDroppedReply(t *testing.T) {
	defer func(d time.Duration) { probeAttemptTimeout = d }(probeAttemptTimeout)
	probeAttemptTimeout = 300 * time.Millisecond
	probeLog := captureProbeLog(t)

	// node 1 probes; node 2 drops one reply.
	servers := localServers(t, 2, nil)
	target := benchTarget(servers[0], 2)
	registerReplyDroppingPeer(servers[1], 1)
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady with one dropped echo reply: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("AwaitReady took %v, want one lost reply to cost roughly one probe round", elapsed)
	}
	if got := probeLog.String(); !strings.Contains(got, "node 2") {
		t.Errorf("probe log does not name straggler node 2; got:\n%s", got)
	}
}

// TestAwaitReadyProbeFailsFastOnSilentPeer verifies that a peer which never
// answers echo probes fails the probe within the stall window — naming the
// silent peer — instead of blocking until the context deadline with the
// unattributable "incomplete call (errors: 0)" of the all-or-nothing probe.
func TestAwaitReadyProbeFailsFastOnSilentPeer(t *testing.T) {
	defer func(d time.Duration) { readyStallTimeout = d }(readyStallTimeout)
	readyStallTimeout = 500 * time.Millisecond
	defer func(d time.Duration) { probeAttemptTimeout = d }(probeAttemptTimeout)
	probeAttemptTimeout = 100 * time.Millisecond
	captureProbeLog(t)

	// node 1 probes; node 2 never answers echoes.
	servers := localServers(t, 2, nil)
	target := benchTarget(servers[0], 2)
	registerReplyDroppingPeer(servers[1], math.MaxInt64)
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}
	node2Addr := servers[1].Addr()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	err := AwaitReady(ctx, target)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("AwaitReady = nil error, want silent-peer probe failure")
	}
	for _, want := range []string{"outbound peers not ready", "node 2", node2Addr} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("AwaitReady error %q does not contain %q", err, want)
		}
	}
	if elapsed > 5*time.Second {
		t.Errorf("AwaitReady took %v, want fail-fast well under the 30s deadline", elapsed)
	}
}

// TestAwaitReadyReportsMissingRemotePeers verifies distributed readiness errors
// identify peer addresses that never respond.
func TestAwaitReadyReportsMissingRemotePeers(t *testing.T) {
	captureDiag(t)
	captureProbeLog(t)
	// node 2's address stays in node 1's node list, but node 2 is shut down
	// immediately so nothing listens there: node 1's probe never gets a
	// response and readiness reports the peer pending by address. Its port is
	// closed and never rebound, so unlike freeTCPAddrs there is no bind-reuse
	// race.
	servers := localServers(t, 2, nil)
	node2Addr := servers[1].Addr()
	servers[1].Stop()
	target := benchTarget(servers[0], 2)
	go func() { _ = servers[0].ListenAndServe() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := AwaitReady(ctx, target)
	if err == nil {
		t.Fatal("AwaitReady = nil error, want missing peer error")
	}
	for _, want := range []string{"outbound peers not ready", "node 2", node2Addr} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("AwaitReady error %q does not contain %q", err, want)
		}
	}
}

// TestAwaitReadyFailsFastOnStalledPeer verifies the outbound probe gives up
// readyStallTimeout after the last peer responded, instead of waiting out the
// full context deadline when a peer never starts. It also verifies the
// failure emits the probe-stall self-diagnosis: the bound listener address, a
// self-dial probe of the advertised address, and a goroutine dump (see
// doc/benchkit-troubleshooting.html).
func TestAwaitReadyFailsFastOnStalledPeer(t *testing.T) {
	defer func(d time.Duration) { readyStallTimeout = d }(readyStallTimeout)
	readyStallTimeout = 500 * time.Millisecond
	defer func(d time.Duration) { probeAttemptTimeout = d }(probeAttemptTimeout)
	probeAttemptTimeout = 100 * time.Millisecond
	diag := captureDiag(t)
	captureProbeLog(t)

	// node 2's address stays in node 1's node list, but node 2 is shut down
	// immediately so nothing listens there: node 1's probe stalls and must
	// give up readyStallTimeout after the last response rather than waiting
	// out the full context deadline. Its port is closed and never rebound, so
	// unlike freeTCPAddrs there is no bind-reuse race.
	servers := localServers(t, 2, nil)
	node1Addr, node2Addr := servers[0].Addr(), servers[1].Addr()
	servers[1].Stop()
	target := benchTarget(servers[0], 2)
	go func() { _ = servers[0].ListenAndServe() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	err := AwaitReady(ctx, target)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("AwaitReady = nil error, want stalled readiness error")
	}
	for _, want := range []string{"outbound peers not ready", "no outbound peer responded", node2Addr} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("AwaitReady error %q does not contain %q", err, want)
		}
	}
	if elapsed > 5*time.Second {
		t.Errorf("AwaitReady took %v, want fail-fast well under the 30s deadline", elapsed)
	}

	// The listener is up (wildcard-bound), so the self-dial probe of the
	// advertised address must succeed and the diagnosis must point the
	// blockage away from this host.
	got := diag.String()
	for _, want := range []string{
		"listener bound to ",
		"self-dial " + node1Addr + " ok",
		"goroutine dump",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("probe-stall diagnosis does not contain %q; got:\n%s", want, got)
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
