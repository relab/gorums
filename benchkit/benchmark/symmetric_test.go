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

// captureProbeLog redirects the outbound-probe progress log to a buffer for
// the duration of the test, so probe tests can assert that stragglers were
// logged by node ID. The probe logs from the calling goroutine only, so the
// buffer needs no locking as long as it is read after awaitReady returns.
func captureProbeLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := probeLogf
	probeLogf = func(format string, args ...any) { fmt.Fprintf(&buf, format, args...) }
	t.Cleanup(func() { probeLogf = orig })
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

// singleServerTarget wraps one server as a single-server SymmetricTarget with the
// benchkit Control plane and workload server attached, so it can be passed to
// awaitReady and the other per-node setup helpers. numPeers is the full cluster
// size (arms Done tracking and sizes the exit grace period). Call before
// serving srv, since attaching registers services.
func singleServerTarget(srv *gorums.Server, numPeers int) *SymmetricTarget {
	ctrl := attachBenchServer(srv)
	ctrl.ArmDone(numPeers) // match setupRemoteServer, which arms Done tracking for the exit barrier
	return &SymmetricTarget{
		servers:  []*gorums.Server{srv},
		controls: []*benchkit.Control{ctrl},
		numPeers: numPeers,
		selfAddr: srv.Addr(),
		labels:   []string{fmt.Sprintf("node %d (%s)", ctrl.ID(), srv.Addr())},
	}
}

// localSymmetricTargets builds n single-server SymmetricTargets over one local
// node list, each wrapping one node (target[i] is node ID i+1) and already
// serving, so a test can drive per-node setup — dedup wait, probe — in a
// controlled order, the way separate setupRemoteServer instances do in a
// real distributed run, but without freeTCPAddrs's port-reuse race.
func localSymmetricTargets(t *testing.T, n int, serverOpt gorums.ServerOption) []*SymmetricTarget {
	t.Helper()
	servers := localServers(t, n, serverOpt)
	targets := make([]*SymmetricTarget, n)
	for i, srv := range servers {
		targets[i] = singleServerTarget(srv, n)
	}
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}
	return targets
}

// symmetricServers starts n local servers as one in-process SymmetricTarget,
// with serverOpts applied to every server, and stops them when the test ends.
func symmetricServers(t *testing.T, n int, serverOpts ...gorums.ServerOption) *SymmetricTarget {
	t.Helper()
	target, stop, err := setupSymmetricServers(n, serverOpts, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("setupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)
	return target
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

// TestSetupSymmetricServersAppliesAllServerOptions verifies that
// setupSymmetricServers forwards every option in the given slice to the
// in-process servers, not just the first. setupLocal previously called
// setupSymmetricServers with a single gorums.ServerOption argument (only
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
	target, stop, err := setupSymmetricServers(3, opts, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("setupSymmetricServers: %v", err)
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
			target, stop, err := setupRemoteServer(peers[1], peers, tt.serverOpts, gorumstest.InsecureDialOptions(t))
			if err != nil {
				t.Fatalf("setupRemoteServer(%s): %v", peers[1], err)
			}
			t.Cleanup(stop)

			peer := gorumstest.PeerNode(t, target.servers[0].PeerConfig(), 1)
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
// make it unreachable for all peers.
func TestSetupRemoteServerBindsWildcard(t *testing.T) {
	// This test must exercise setupRemoteServer directly, because it is
	// setupRemoteServer (not the local test framework, which binds 127.0.0.1)
	// that binds the wildcard host. Port 0 lets setupRemoteServer pick its own
	// free port, so no port is reserved and released beforehand — avoiding the
	// bind-reuse race. The two peers differ only so the sort/self-index is
	// stable; only self (the lower address) is bound.
	peers := []string{"127.0.0.1:0", "127.0.0.2:0"}
	target, stop, err := setupRemoteServer(peers[0], peers, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("setupRemoteServer(%s): %v", peers[0], err)
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

// TestAwaitPeersDoneOrGraceReturnsEarlyWhenAllSignal verifies that once every
// peer has called SignalDone, AwaitPeersDoneOrGrace returns true well before
// a deliberately long grace period elapses, instead of always sleeping it out.
func TestAwaitPeersDoneOrGraceReturnsEarlyWhenAllSignal(t *testing.T) {
	targets := localSymmetricTargets(t, 2, nil)
	target1, target2 := targets[0], targets[1]

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target1); err != nil {
		t.Fatalf("awaitReady(target1): %v", err)
	}
	if err := awaitReady(ctx, target2); err != nil {
		t.Fatalf("awaitReady(target2): %v", err)
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
	if err := awaitReady(ctx, target1); err != nil {
		t.Fatalf("awaitReady(target1): %v", err)
	}
	if err := awaitReady(ctx, target2); err != nil {
		t.Fatalf("awaitReady(target2): %v", err)
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

func TestRunComplete(t *testing.T) {
	tests := []struct {
		name                string
		outSize, done, need int
		want                bool
	}{
		{"NoneDone", 5, 0, 3, false},
		{"OneDoneQuorumStillPossible", 5, 1, 3, false}, // alive 4 >= 3
		{"EnoughDoneQuorumImpossible", 5, 3, 3, true},  // alive 2 < 3
		{"AllPeersNeededOneDone", 5, 1, 5, true},       // alive 4 < 5
		{"AllPeersNeededNoneDone", 5, 0, 5, false},     // never masks a startup fault
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runComplete(tt.outSize, tt.done, tt.need); got != tt.want {
				t.Errorf("runComplete(%d, %d, %d) = %v, want %v",
					tt.outSize, tt.done, tt.need, got, tt.want)
			}
		})
	}
}

// TestSymmetricRunOver checks the live classifiers flip from false to true only
// after peers signal Done, over a real (in-process) symmetric target.
func TestSymmetricRunOver(t *testing.T) {
	target := symmetricServers(t, 3)

	size := target.servers[0].PeerConfig().Size()
	quorum := size/2 + 1

	// No peer has signaled Done yet: nothing is over, so a failure now would
	// still be reported as a fault.
	if anyPeerFinished(target) {
		t.Error("anyPeerFinished = true before any Done, want false")
	}
	if quorumRunOver(target, quorum) {
		t.Error("quorumRunOver = true before any Done, want false")
	}

	// Signal every peer done on the first server; the run is now winding down.
	target.controls[0].ArmDone(size)
	for id := 1; id <= size; id++ {
		target.controls[0].Done(gorums.ServerContext{}, benchkit.DoneRequest_builder{SenderId: uint32(id)}.Build())
	}
	if !anyPeerFinished(target) {
		t.Error("anyPeerFinished = false after all peers Done, want true")
	}
	if !quorumRunOver(target, quorum) {
		t.Error("quorumRunOver = false after all peers Done, want true")
	}
}

// TestAwaitReadyStaggeredRemoteStartup verifies distributed readiness tolerates
// one node starting before its peer. Both listeners are bound up front by the
// framework, so the stagger is in when each node begins serving (accepting gRPC
// streams): node 1 serves 500ms before node 2, and node 1's outbound stream to
// node 2 must retry across that gap rather than fail readiness.
func TestAwaitReadyStaggeredRemoteStartup(t *testing.T) {
	servers := localServers(t, 2, nil)
	target1, target2 := singleServerTarget(servers[0], 2), singleServerTarget(servers[1], 2)

	go func() { _ = servers[0].ListenAndServe() }()
	time.Sleep(500 * time.Millisecond)
	go func() { _ = servers[1].ListenAndServe() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	errCh := make(chan error, 2)
	go func() { errCh <- awaitReady(ctx, target1) }()
	go func() { errCh <- awaitReady(ctx, target2) }()

	var errs error
	for range 2 {
		if err := <-errCh; err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		t.Fatalf("awaitReady after staggered startup: %v", errs)
	}
	if got := target1.servers[0].ConnectedPeers().Size(); got != 2 {
		t.Errorf("target1 connected config size = %d, want 2", got)
	}
	if got := target2.servers[0].ConnectedPeers().Size(); got != 2 {
		t.Errorf("target2 connected config size = %d, want 2", got)
	}
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
	target := singleServerTarget(servers[0], 2)
	registerReplyDroppingPeer(servers[1], 1)
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady with one dropped echo reply: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("awaitReady took %v, want one lost reply to cost roughly one probe round", elapsed)
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
	target := singleServerTarget(servers[0], 2)
	registerReplyDroppingPeer(servers[1], math.MaxInt64)
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}
	node2Addr := servers[1].Addr()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	err := awaitReady(ctx, target)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("awaitReady = nil error, want silent-peer probe failure")
	}
	for _, want := range []string{"outbound peers not ready", "node 2", node2Addr} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("awaitReady error %q does not contain %q", err, want)
		}
	}
	if elapsed > 5*time.Second {
		t.Errorf("awaitReady took %v, want fail-fast well under the 30s deadline", elapsed)
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
	target := singleServerTarget(servers[0], 2)
	go func() { _ = servers[0].ListenAndServe() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := awaitReady(ctx, target)
	if err == nil {
		t.Fatal("awaitReady = nil error, want missing peer error")
	}
	for _, want := range []string{"outbound peers not ready", "node 2", node2Addr} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("awaitReady error %q does not contain %q", err, want)
		}
	}
}

// TestAwaitReadyFailsFastOnStalledPeer verifies the outbound probe gives up
// readyStallTimeout after the last peer responded, instead of waiting out the
// full context deadline when a peer never starts. It also verifies the
// failure emits the probe-stall self-diagnosis: the bound listener address, a
// self-dial probe of the advertised address, and a goroutine dump.
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
	target := singleServerTarget(servers[0], 2)
	go func() { _ = servers[0].ListenAndServe() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	err := awaitReady(ctx, target)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("awaitReady = nil error, want stalled readiness error")
	}
	for _, want := range []string{"outbound peers not ready", "no outbound peer responded", node2Addr} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("awaitReady error %q does not contain %q", err, want)
		}
	}
	if elapsed > 5*time.Second {
		t.Errorf("awaitReady took %v, want fail-fast well under the 30s deadline", elapsed)
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

// TestRunSymmetricQuorumCallDefaultsToMajority verifies that
// runSymmetricQuorumCall completes a short run without an explicit
// QuorumSize, falling back to a majority of the outbound peer count.
func TestRunSymmetricQuorumCallDefaultsToMajority(t *testing.T) {
	targets := localSymmetricTargets(t, 3, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, target := range targets {
		if err := awaitReady(ctx, target); err != nil {
			t.Fatalf("awaitReady: %v", err)
		}
	}

	opts := benchkit.Options{Duration: 100 * time.Millisecond, Rate: 100, Workers: 1}
	result, err := runSymmetricQuorumCall(targets[0], opts)
	if err != nil {
		t.Fatalf("runSymmetricQuorumCall: %v", err)
	}
	if result.GetTotalOps() == 0 {
		t.Error("TotalOps = 0, want at least one completed quorum call")
	}
}

// TestRunSymmetricQuorumCallStragglerEndsCleanly verifies the straggler path:
// when a quorum call fails after enough peers have signaled Done that the
// quorum can no longer form, runSymmetricQuorumCall treats it as the expected
// end of the run (benchkit.ErrRunOver) rather than propagating the failure,
// so a node outliving its peers still returns a usable partial Result instead
// of failing the whole benchmark.
func TestRunSymmetricQuorumCallStragglerEndsCleanly(t *testing.T) {
	servers := localServers(t, 2, nil)
	target := singleServerTarget(servers[0], 2) // numPeers=2 arms Done tracking for IDs 1 and 2
	target2 := singleServerTarget(servers[1], 2)
	go func() { _ = servers[0].ListenAndServe() }()
	go func() { _ = servers[1].ListenAndServe() }()

	// Both sides probe: a dual-mode stream is only fully up once each side has
	// dialed out, so waiting on only one direction can stall the other peer's
	// half of the handshake.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := make(chan error, 2)
	go func() { errCh <- awaitReady(ctx, target) }()
	go func() { errCh <- awaitReady(ctx, target2) }()
	var errs error
	for range 2 {
		if err := <-errCh; err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		t.Fatalf("awaitReady: %v", errs)
	}

	// Node 2 (the only outbound peer) finishes and exits; its calls now fail,
	// and it has signaled Done, so quorumRunOver must recognize this as the
	// expected end of the run rather than a fault.
	servers[1].Stop()
	target.controls[0].Done(gorums.ServerContext{}, benchkit.DoneRequest_builder{SenderId: 2}.Build())

	// Duration is deliberately long relative to the expected stop: a regular
	// (non-run-over) failure is only counted via Stats.RecordError and the
	// run keeps retrying until Duration elapses, so a prompt return is what
	// distinguishes the run-over path (which calls cancel() on the first
	// failed call) from ordinary failure counting — both paths return a nil
	// top-level error either way.
	const duration = 2 * time.Second
	opts := benchkit.Options{Duration: duration, Rate: 50, Workers: 1}
	start := time.Now()
	result, err := runSymmetricQuorumCall(target, opts)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("runSymmetricQuorumCall = %v, want nil error (straggler run-over should be a clean stop)", err)
	}
	if elapsed >= duration/2 {
		t.Errorf("runSymmetricQuorumCall took %v, want well under %v (run-over should cancel on the first failed call)", elapsed, duration)
	}
	if got := result.GetFailedOps(); got != 0 {
		t.Errorf("FailedOps = %d, want 0 (the failing call must be classified as run-over, not counted as a failure)", got)
	}
}

// TestRunSymmetricQuorumCallHonorsCallTimeout verifies that the symmetric
// runner's -call-timeout branch bounds each call to opts.CallTimeout instead
// of hanging behind an unresponsive peer until the run's own deadline
// elapses, mirroring TestQuorumCallHonorsCallTimeout for the coordinator
// path.
func TestRunSymmetricQuorumCallHonorsCallTimeout(t *testing.T) {
	servers := localServers(t, 2, nil)
	target := singleServerTarget(servers[0], 2)
	// Node 2 drops every reply from the start, mirroring
	// TestQuorumCallHonorsCallTimeout: a call without CallTimeout would hang
	// until BenchContext's own (30s+) deadline instead of the short one
	// below. awaitReady is not used here since its own probe is a QuorumCall
	// against the same handler and would never succeed against a
	// permanently unresponsive peer.
	registerReplyDroppingPeer(servers[1], 1<<30)
	go func() { _ = servers[0].ListenAndServe() }()
	go func() { _ = servers[1].ListenAndServe() }()

	// QuorumSize=2 requires both replies: server0's own PeerConfig includes
	// itself alongside server1, so a threshold of 1 would be satisfied by the
	// self entry alone without ever needing server1's (dropped) reply.
	opts := benchkit.Options{
		Workers: 1, Duration: 100 * time.Millisecond, QuorumSize: 2,
		CallTimeout: 20 * time.Millisecond,
	}
	start := time.Now()
	result, err := runSymmetricQuorumCall(target, opts)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("runSymmetricQuorumCall: %v", err)
	}
	const wantBound = 5 * time.Second // generous vs. what an ignored CallTimeout would look like
	if elapsed > wantBound {
		t.Errorf("took %v, want well under %v (CallTimeout should bound each call against the unresponsive peer)", elapsed, wantBound)
	}
	if result.GetFailedOps() == 0 {
		t.Error("FailedOps = 0, want > 0 (every call should time out against the unresponsive peer)")
	}
}

func TestRunSymmetricMulticastDrainsServerMessages(t *testing.T) {
	target := symmetricServers(t, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
	}

	result, err := runSymmetricMulticast(target, benchkit.Options{
		Workers:  1,
		Duration: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("runSymmetricMulticast: %v", err)
	}

	wantSamples := int(result.GetTotalOps()) * target.numPeers
	if got := len(result.GetLatencies()); got != wantSamples {
		t.Fatalf("len(Latencies) = %d, want %d", got, wantSamples)
	}
}

// TestRunSymmetricMulticastHDR verifies that a symmetric server-measured run
// honors StatsMode_HDR end to end: the per-sender stores, offset correction,
// and cross-server aggregation carry a bounded histogram (Result.Histogram)
// instead of raw samples (Result.Latencies nil).
func TestRunSymmetricMulticastHDR(t *testing.T) {
	target := symmetricServers(t, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
	}

	result, err := runSymmetricMulticast(target, benchkit.Options{
		Workers:   1,
		Duration:  20 * time.Millisecond,
		StatsMode: benchkit.StatsMode_HDR,
	})
	if err != nil {
		t.Fatalf("runSymmetricMulticast: %v", err)
	}
	if got := result.GetLatencies(); got != nil {
		t.Errorf("Latencies in HDR mode = %v, want nil", got)
	}
	h := result.GetHistogram()
	if h == nil {
		t.Fatal("Histogram in HDR mode = nil, want non-nil")
	}
	var total uint64
	for _, c := range h.GetCount() {
		total += c
	}
	if wantSamples := result.GetTotalOps() * uint64(target.numPeers); total != wantSamples {
		t.Errorf("histogram counts sum = %d, want %d", total, wantSamples)
	}
}
