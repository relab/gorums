package benchkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/testutils/mock"
	"github.com/relab/gorums/internal/testutils/mockcall"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// lockedBuffer is a [bytes.Buffer] that several peers of one test can write
// at the same time.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureDiag sends the self-diagnosis of [Peer.AwaitReady] to a buffer until
// the test ends.
func captureDiag(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := new(lockedBuffer)
	orig := diagWriter
	diagWriter = buf
	t.Cleanup(func() { diagWriter = orig })
	return buf
}

// shortenReadiness sets the attempt and stall bounds of [AwaitMesh] to the
// given values until the test ends.
func shortenReadiness(t *testing.T, attempt, stall time.Duration) {
	t.Helper()
	origAttempt, origStall := meshAttemptTimeout, meshStallTimeout
	meshAttemptTimeout, meshStallTimeout = attempt, stall
	t.Cleanup(func() { meshAttemptTimeout, meshStallTimeout = origAttempt, origStall })
}

// meshPeers returns a serving Peer for each of servers.
func meshPeers(servers []*gorums.Server) []*Peer {
	peers := make([]*Peer, len(servers))
	for i, srv := range servers {
		peers[i] = NewPeer(srv)
	}
	serve(servers...)
	return peers
}

// registerPeerViewDropper registers a PeerView handler on srv that sends no
// reply to the first drop calls and a complete view to later calls. It returns
// the counter of calls left to drop, which is negative once a later call got
// an answer. Call it before srv serves.
func registerPeerViewDropper(srv *gorums.Server, drop int64) *atomic.Int64 {
	var remaining atomic.Int64
	remaining.Store(drop)
	srv.RegisterHandler("benchkit.Control.PeerView", func(_ gorums.ServerContext, _ *gorums.Message, send func(proto.Message)) error {
		if remaining.Add(-1) >= 0 {
			return nil // no reply and no error
		}
		send(PeerViewResponse_builder{}.Build())
		return nil
	})
	return &remaining
}

// TestPeerListenPeerAppliesServerOptions verifies that the server options of
// opts reach the server of [ListenPeer]. Stream deduplication is the
// observable case: a peer with a lower ID than this node shares the channel of
// that peer's stream, which [gorums.Node.IsShared] reports before any peer
// connects.
func TestPeerListenPeerAppliesServerOptions(t *testing.T) {
	// Self is the higher address, so the sorted list gives it ID 2.
	remotes := []string{"127.0.0.1:0", "127.0.0.2:0"}
	tests := []struct {
		name       string
		streamMode string
		wantShared bool
	}{
		{name: "Dual"},
		{name: "Dedup", streamMode: "dedup", wantShared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ListenPeer(remotes[1], remotes, Options{StreamMode: tt.streamMode}, gorumstest.InsecureDialOptions(t))
			if err != nil {
				t.Fatalf("ListenPeer: %v", err)
			}
			t.Cleanup(p.Stop)
			if got := p.Server().NodeID(); got != 2 {
				t.Errorf("NodeID() = %d, want 2", got)
			}
			peer := gorumstest.PeerNode(t, p.Server().PeerConfig(), 1)
			if got := peer.IsShared(); got != tt.wantShared {
				t.Errorf("peer 1 IsShared() = %v, want %v", got, tt.wantShared)
			}
		})
	}
}

// TestPeerListenPeerRejectsUnknownSelf verifies that [ListenPeer] returns an
// error when self is not in remotes.
func TestPeerListenPeerRejectsUnknownSelf(t *testing.T) {
	_, err := ListenPeer("127.0.0.3:0", []string{"127.0.0.1:0", "127.0.0.2:0"}, Options{})
	if err == nil {
		t.Fatal("ListenPeer = nil error, want error for a self address outside remotes")
	}
}

// TestPeerListenPeerBindsWildcard verifies that the server of [ListenPeer]
// listens on the wildcard address. Hosts that follow the Debian convention map
// their own hostname to 127.0.1.1, which would put a hostname-bound listener
// on loopback, out of reach of every peer. Port 0 lets the server pick a free
// port, so the test reserves no port beforehand.
func TestPeerListenPeerBindsWildcard(t *testing.T) {
	remotes := []string{"127.0.0.1:0", "127.0.0.2:0"}
	p, err := ListenPeer(remotes[0], remotes, Options{}, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("ListenPeer: %v", err)
	}
	t.Cleanup(p.Stop)
	serve(p.Server())

	var addr string
	if !gorumstest.WaitUntil(t, 2*time.Second, func() bool {
		addr = p.Server().Addr()
		_, port, err := net.SplitHostPort(addr)
		return err == nil && port != "" && port != "0"
	}) {
		t.Fatalf("listener did not bind a concrete port; Addr = %q", addr)
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%s): %v", addr, err)
	}
	if ip := net.ParseIP(host); host != "" && (ip == nil || !ip.IsUnspecified()) {
		t.Errorf("listener bound to host %q, want wildcard", host)
	}
}

// TestPeerExitGrace verifies that the exit grace grows with the run size,
// starts at the base, and is clamped for large runs.
func TestPeerExitGrace(t *testing.T) {
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
		{120, maxGrace}, // base + 120*perNode = 39s, clamped to maxGrace
	}
	for _, tt := range tests {
		if got := ExitGrace(tt.numNodes); got != tt.want {
			t.Errorf("ExitGrace(%d) = %v, want %v", tt.numNodes, got, tt.want)
		}
	}
	prev := ExitGrace(1)
	for n := 2; n <= 200; n++ {
		got := ExitGrace(n)
		if got < prev {
			t.Fatalf("ExitGrace(%d) = %v < ExitGrace(%d) = %v; want non-decreasing", n, got, n-1, prev)
		}
		prev = got
	}
}

// TestPeerAwaitReady verifies that [Peer.AwaitReady] passes in both stream
// modes, and that every peer then has every peer in its connected view. In
// dedup mode it passes without a prior [gorums.Server.WaitForAll], and the
// lower-ID peers of node 3 then use shared inbound streams.
func TestPeerAwaitReady(t *testing.T) {
	for _, mode := range streamModes {
		t.Run(mode.name, func(t *testing.T) {
			peers := meshPeers(meshServers(t, 3, mode.opts, nil))
			if err := peers[0].AwaitReady(gorumstest.Context(t, 10*time.Second)); err != nil {
				t.Fatalf("AwaitReady: %v", err)
			}
			for _, p := range peers {
				if got := p.Server().ConnectedPeers().Size(); got != 3 {
					t.Errorf("%s: ConnectedPeers().Size() = %d, want 3", p, got)
				}
			}
			if mode.name != "Dedup" {
				return
			}
			for _, node := range peers[2].Server().PeerConfig() {
				if node.ID() < 3 && (node.IsOutbound() || !node.IsInbound()) {
					t.Errorf("node 3 to node %d: IsOutbound() = %v, IsInbound() = %v, want a shared inbound stream",
						node.ID(), node.IsOutbound(), node.IsInbound())
				}
			}
		})
	}
}

// TestPeerAwaitReadyStaggeredStartup verifies that readiness tolerates a node
// that starts serving 500 ms after its peer. Both listeners are bound up front,
// so the stagger is in when each node begins to accept streams.
func TestPeerAwaitReadyStaggeredStartup(t *testing.T) {
	servers := meshServers(t, 2, nil, nil)
	peers := []*Peer{NewPeer(servers[0]), NewPeer(servers[1])}
	serve(servers[0])
	time.AfterFunc(500*time.Millisecond, func() { serve(servers[1]) })

	ctx := gorumstest.Context(t, 5*time.Second)
	errCh := make(chan error, len(peers))
	for _, p := range peers {
		go func() { errCh <- p.AwaitReady(ctx) }()
	}
	var errs error
	for range peers {
		errs = errors.Join(errs, <-errCh)
	}
	if errs != nil {
		t.Fatalf("AwaitReady after a staggered startup: %v", errs)
	}
}

// TestPeerAwaitReadyRetriesDroppedReply verifies that readiness survives a
// peer that drops one PeerView reply: the call times out after the attempt
// bound, and a later round passes.
func TestPeerAwaitReadyRetriesDroppedReply(t *testing.T) {
	shortenReadiness(t, 300*time.Millisecond, meshStallTimeout)

	servers := meshServers(t, 2, nil, nil)
	p := NewPeer(servers[0])
	remaining := registerPeerViewDropper(servers[1], 1)
	serve(servers...)

	start := time.Now()
	if err := p.AwaitReady(gorumstest.Context(t, 10*time.Second)); err != nil {
		t.Fatalf("AwaitReady with one dropped reply: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < meshAttemptTimeout || elapsed > 5*time.Second {
		t.Errorf("AwaitReady took %v, want one lost reply to cost about one round of %v", elapsed, meshAttemptTimeout)
	}
	if got := remaining.Load(); got >= 0 {
		t.Errorf("node 2 has %d PeerView calls left to drop, want the dropped call and a later answer", got+1)
	}
}

// TestPeerAwaitReadyFailsFast verifies that readiness fails within the stall
// bound, well before a long deadline, when a peer never answers or is gone.
// The error names the peer and its address, and the self-diagnosis reports
// the bound listener, a self-dial of the advertised address, and a goroutine
// dump.
func TestPeerAwaitReadyFailsFast(t *testing.T) {
	tests := []struct {
		name  string
		node2 func(srv *gorums.Server)
	}{
		{name: "SilentPeer", node2: func(srv *gorums.Server) {
			registerPeerViewDropper(srv, math.MaxInt64)
			serve(srv)
		}},
		{name: "StoppedPeer", node2: func(srv *gorums.Server) { srv.Stop() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shortenReadiness(t, 100*time.Millisecond, 500*time.Millisecond)
			diag := captureDiag(t)

			servers := meshServers(t, 2, nil, nil)
			node1Addr, node2Addr := servers[0].Addr(), servers[1].Addr()
			p := NewPeer(servers[0])
			tt.node2(servers[1])
			serve(servers[0])

			start := time.Now()
			err := p.AwaitReady(gorumstest.Context(t, 30*time.Second))
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Errorf("AwaitReady took %v, want it to fail well before the 30s deadline", elapsed)
			}
			if err == nil {
				t.Fatal("AwaitReady = nil, want an error")
			}
			for _, want := range []string{"peers not ready", "no further node reported a complete view", "node 2 (" + node2Addr + ")"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("AwaitReady error %q does not contain %q", err, want)
				}
			}
			got := diag.String()
			for _, want := range []string{"listener bound to ", "self-dial " + node1Addr + " ok", "goroutine dump"} {
				if !strings.Contains(got, want) {
					t.Errorf("self-diagnosis does not contain %q; got:\n%s", want, got)
				}
			}
		})
	}
}

// TestPeerFlushOutbound verifies that once [Peer.FlushOutbound] returns, every
// peer has handled the one-way requests sent before it.
func TestPeerFlushOutbound(t *testing.T) {
	const sends = 200
	servers := meshServers(t, 3, nil, nil)
	received := make([]atomic.Int64, len(servers))
	for i, srv := range servers {
		srv.RegisterHandler(mock.TestMethod, func(gorums.ServerContext, *gorums.Message, func(proto.Message)) error {
			received[i].Add(1)
			return nil
		})
	}
	peers := meshPeers(servers)
	ctx := gorumstest.Context(t, 10*time.Second)
	if err := peers[0].AwaitReady(ctx); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}

	cfg := peers[0].Server().PeerConfig()
	for range sends {
		if err := mockcall.Multicast(ctx, cfg, "x").Async().Wait(); err != nil {
			t.Fatalf("Multicast: %v", err)
		}
	}
	if err := peers[0].FlushOutbound(ctx); err != nil {
		t.Fatalf("FlushOutbound: %v", err)
	}
	for i := range received {
		if got := received[i].Load(); got != sends {
			t.Errorf("node %d handled %d requests before the flush returned, want %d", i+1, got, sends)
		}
	}
}

// TestPeerAwaitPeersDone verifies that [Peer.AwaitPeersDone] returns nil soon
// after every peer signals Done, and that it returns an error naming the
// missing peer when one peer never signals.
func TestPeerAwaitPeersDone(t *testing.T) {
	t.Run("AllSignal", func(t *testing.T) {
		peers := meshPeers(meshServers(t, 2, nil, nil))
		ctx := gorumstest.Context(t, 10*time.Second)
		if err := peers[0].AwaitReady(ctx); err != nil {
			t.Fatalf("AwaitReady: %v", err)
		}
		for _, p := range peers {
			p.SignalDone(ctx)
		}
		for _, p := range peers {
			if err := p.AwaitPeersDone(ctx); err != nil {
				t.Errorf("%s: AwaitPeersDone: %v", p, err)
			}
		}
	})
	t.Run("PeerNeverSignals", func(t *testing.T) {
		peers := meshPeers(meshServers(t, 2, nil, nil))
		if err := peers[0].AwaitReady(gorumstest.Context(t, 10*time.Second)); err != nil {
			t.Fatalf("AwaitReady: %v", err)
		}
		peers[0].SignalDone(t.Context()) // node 2 never signals

		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()
		err := peers[0].AwaitPeersDone(ctx)
		if err == nil {
			t.Fatal("AwaitPeersDone = nil, want an error when node 2 never signals")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("AwaitPeersDone error = %v, want it to wrap %v", err, context.DeadlineExceeded)
		}
		if want := "node(s) [2]"; !strings.Contains(err.Error(), want) {
			t.Errorf("AwaitPeersDone error = %q, want it to contain %q", err, want)
		}
	})
}

// TestPeerSignalDoneSurvivesImmediateStop verifies that a node whose Done
// signal is the last one a peer awaits can stop its server right after
// [Peer.SignalDone] returns, and the peer still receives the signal. The sender
// first queues a 16 MiB backlog of one-way requests to the waiter, so Done is
// still queued behind them when the stop begins. A stop that discards queued
// requests loses Done in almost every subtest.
//
// The stop bound here is longer than [stopFlushBound], so a slow test machine
// cannot reach the Stop fallback while the backlog drains.
func TestPeerSignalDoneSurvivesImmediateStop(t *testing.T) {
	const (
		backlog   = 256
		stopBound = 10 * time.Second
	)
	payload := strings.Repeat("x", 64<<10)
	for i := range 10 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			servers := meshServers(t, 2, nil, nil)
			for _, srv := range servers {
				srv.RegisterHandler(mock.TestMethod, func(gorums.ServerContext, *gorums.Message, func(proto.Message)) error {
					return nil
				})
			}
			peers := meshPeers(servers)
			sender, waiter := peers[0], peers[1]
			ctx := gorumstest.Context(t, 2*stopBound)
			if err := sender.AwaitReady(ctx); err != nil {
				t.Fatalf("AwaitReady: %v", err)
			}

			waiter.SignalDone(ctx) // the waiter's own Done, so only the sender's can be missing
			cfg := sender.Server().PeerConfig()
			for range backlog {
				if err := mockcall.Multicast(ctx, cfg, payload).Async().Wait(); err != nil {
					t.Fatalf("Multicast: %v", err)
				}
			}
			sender.SignalDone(ctx)
			stopFlushed(sender.Server(), stopBound) // the process exits right after the stop

			waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := waiter.AwaitPeersDone(waitCtx); err != nil {
				t.Fatalf("peer missed Done from a node that stopped right after SignalDone: %v", err)
			}
		})
	}
}

// pingWorkload returns a [Workload] whose one benchmark sends sends one-way
// requests from each peer to every peer, itself included. Its final hook sets
// the TotalOps of the result of each peer to the number of requests that the
// peer received.
func pingWorkload(sends int) Workload {
	var received [8]atomic.Int64 // indexed by node ID
	return Workload{
		Setup: func(p *Peer) ([]Bench, error) {
			id := p.Server().NodeID()
			p.Server().RegisterHandler(mock.TestMethod, func(gorums.ServerContext, *gorums.Message, func(proto.Message)) error {
				received[id].Add(1)
				return nil
			})
			ping := func(context.Context, Options) (*Result, error) {
				cfg := p.Server().PeerConfig()
				for range sends {
					if err := mockcall.Multicast(context.Background(), cfg, "ping").Async().Wait(); err != nil {
						return nil, err
					}
				}
				r := &Result{}
				r.SetTotalOps(uint64(sends))
				return r, nil
			}
			return []Bench{{Name: "Ping", Run: ping}}, nil
		},
		Final: func(p *Peer, results []*Result) error {
			results[0].SetTotalOps(uint64(received[p.Server().NodeID()].Load()))
			return nil
		},
	}
}

// localOpts returns the local server options of a test run of [RunLocal] in
// the given stream mode.
func localOpts(t *testing.T, serverOpts []gorums.ServerOption) []gorums.LocalServerOption {
	return []gorums.LocalServerOption{
		gorums.WithLocalServerOptions(serverOpts...),
		gorums.WithLocalDialOptions(gorumstest.InsecureDialOptions(t)),
	}
}

// pingOpts are the benchmark options of the tests of [Peer.Run].
var pingOpts = Options{Workers: 1, Duration: 10 * time.Millisecond}

// TestPeerRunLocal verifies the lifecycle of [Peer.Run] through [RunLocal] in
// both stream modes. The final hook of every peer observes the requests of all
// peers, so it ran after every peer finished, and the change of the hook to the
// results shows in the returned results.
func TestPeerRunLocal(t *testing.T) {
	const n, sends = 3, 50
	for _, mode := range streamModes {
		t.Run(mode.name, func(t *testing.T) {
			results, err := RunLocal(gorumstest.Context(t, 30*time.Second), n, regexp.MustCompile("Ping"),
				pingWorkload(sends), pingOpts, localOpts(t, mode.opts)...)
			if err != nil {
				t.Fatalf("RunLocal: %v", err)
			}
			if len(results) != n {
				t.Fatalf("len(results) = %d, want %d", len(results), n)
			}
			for i, r := range results {
				if len(r) != 1 {
					t.Fatalf("node %d: len(results) = %d, want 1", i+1, len(r))
				}
				if got, want := r[0].GetTotalOps(), uint64(n*sends); got != want {
					t.Errorf("node %d: final hook saw %d requests, want %d", i+1, got, want)
				}
				if got := r[0].GetConfig().GetName(); got != "Ping" {
					t.Errorf("node %d: result name = %q, want %q", i+1, got, "Ping")
				}
			}
		})
	}
}

// namedMetricsWorkload returns a [Workload] with two benchmarks that record
// named metrics. On every node, Local records sends samples of a client-side
// series and a counter. On node 1, Server runs [ServerMeasured] against every
// peer, and the handler of each peer records a server-measured series and a
// counter for each request; on the other nodes, Server returns an empty result.
func namedMetricsWorkload(sends int) Workload {
	return Workload{
		Setup: func(p *Peer) ([]Bench, error) {
			stats := p.Control().Stats()
			deliver := stats.Series("deliver")
			received := stats.Counter("received", "msgs")
			p.Server().RegisterHandler(mock.TestMethod, func(ctx gorums.ServerContext, _ *gorums.Message, _ func(proto.Message)) error {
				stats.AddLatency(time.Microsecond)
				deliver.AddBySender(ctx.SenderID(), time.Microsecond)
				received.Add(1)
				return nil
			})
			local := func(_ context.Context, opts Options) (*Result, error) {
				m := StartMeasurement(opts)
				encode := m.Stats.Series("encode")
				blocks := m.Stats.Counter("blocks", "blocks")
				for i := range sends {
					m.Stats.AddLatency(time.Microsecond)
					encode.Add(time.Duration(i+1) * time.Microsecond)
					blocks.Add(1)
				}
				return m.Finish(), nil
			}
			server := func(ctx context.Context, opts Options) (*Result, error) {
				if p.Server().NodeID() != 1 {
					return &Result{}, nil
				}
				setup := func(ctx context.Context, cfg Config, _ Options) func() error {
					return func() error { return mockcall.Multicast(ctx, cfg, "ping").Async().Wait() }
				}
				return ServerMeasured(p.Server().PeerConfig(), setup)(ctx, opts)
			}
			return []Bench{{Name: "Local", Run: local}, {Name: "Server", Run: server}}, nil
		},
	}
}

// TestPeerRunLocalNamedMetrics verifies that named metrics reach the results of
// a local run: the client-side metrics of Local on every node, the metrics
// that the servers recorded for Server merged across nodes, and both in the
// protojson form of the results.
func TestPeerRunLocalNamedMetrics(t *testing.T) {
	const n, sends = 3, 20
	opts := Options{Workers: 1, Duration: 20 * time.Millisecond}
	results, err := RunLocal(gorumstest.Context(t, 30*time.Second), n, regexp.MustCompile(".*"),
		namedMetricsWorkload(sends), opts, localOpts(t, nil)...)
	if err != nil {
		t.Fatalf("RunLocal: %v", err)
	}
	for i, r := range results {
		local := r[0]
		if got := len(metricByName(local, "encode").GetDistribution().GetSamples()); got != sends {
			t.Errorf("node %d: encode samples = %d, want %d", i+1, got, sends)
		}
		if c := metricByName(local, "blocks").GetCounter(); c.GetSum() != sends || c.GetNodes() != 1 {
			t.Errorf("node %d: blocks = (sum %d, nodes %d), want (%d, 1)", i+1, c.GetSum(), c.GetNodes(), sends)
		}
	}

	server := results[0][1]
	ops := server.GetTotalOps()
	if ops == 0 {
		t.Fatal("node 1: Server recorded no operations")
	}
	deliver := metricByName(server, "deliver")
	if got := deliver.GetMeasurementMode(); got != MeasurementMode_SERVER_MEASURED {
		t.Errorf("deliver measurement mode = %v, want SERVER_MEASURED", got)
	}
	if got := uint64(len(deliver.GetDistribution().GetSamples())); got != ops {
		t.Errorf("deliver samples = %d, want %d, one per received request", got, ops)
	}
	if c := metricByName(server, "received").GetCounter(); c.GetSum() != ops || c.GetNodes() != n || c.GetMax() > ops {
		t.Errorf("received = (sum %d, max %d, nodes %d), want (%d, <= %d, %d)", c.GetSum(), c.GetMax(), c.GetNodes(), ops, ops, n)
	}

	// The protojson form names every metric and renders a counter's sum.
	type jsonMetric struct {
		Name    string `json:"name"`
		Counter *struct {
			Sum string `json:"sum"`
		} `json:"counter"`
	}
	var got []string
	for _, r := range []*Result{results[0][0], server} {
		data, err := protojson.Marshal(r)
		if err != nil {
			t.Fatalf("protojson.Marshal: %v", err)
		}
		var decoded struct {
			Metrics []jsonMetric `json:"metrics"`
		}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		for _, m := range decoded.Metrics {
			got = append(got, m.Name)
			if m.Name == "blocks" && (m.Counter == nil || m.Counter.Sum != fmt.Sprint(sends)) {
				t.Errorf("protojson blocks counter = %+v, want sum %d", m.Counter, sends)
			}
		}
	}
	if want := []string{"blocks", "encode", "deliver", "received"}; !slices.Equal(got, want) {
		t.Errorf("protojson metric names = %v, want %v", got, want)
	}
}

// TestPeerRunLocalSetupError verifies that a setup error on one peer makes
// [RunLocal] return an error that names that peer and wraps the setup error,
// without waiting out the readiness bounds of the other peers.
func TestPeerRunLocalSetupError(t *testing.T) {
	captureDiag(t)
	errSetup := errors.New("setup error")
	w := pingWorkload(1)
	setup := w.Setup
	w.Setup = func(p *Peer) ([]Bench, error) {
		if p.Server().NodeID() == 2 {
			return nil, errSetup
		}
		return setup(p)
	}
	start := time.Now()
	_, err := RunLocal(gorumstest.Context(t, 30*time.Second), 3, regexp.MustCompile("Ping"), w, pingOpts, localOpts(t, nil)...)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("RunLocal took %v, want the setup error to end the run at once", elapsed)
	}
	if !errors.Is(err, errSetup) {
		t.Fatalf("RunLocal error = %v, want it to wrap %v", err, errSetup)
	}
	if want := "node 2"; !strings.Contains(err.Error(), want) {
		t.Errorf("RunLocal error = %q, want it to contain %q", err, want)
	}
}

// TestPeerRunLocalIncompleteMesh verifies that [Peer.Run] fails at readiness
// when one stream of the mesh never comes up: node 3 refuses the streams of
// node 2.
func TestPeerRunLocalIncompleteMesh(t *testing.T) {
	captureDiag(t)
	gate, _ := streamGate(2)
	opts := append(localOpts(t, nil), gorums.WithLocalServerOptionsFunc(func(id gorums.ID) []gorums.ServerOption {
		if id == 3 {
			return []gorums.ServerOption{gate}
		}
		return nil
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err := RunLocal(ctx, 3, regexp.MustCompile("Ping"), pingWorkload(1), pingOpts, opts...)
	if err == nil {
		t.Fatal("RunLocal = nil error, want a readiness error")
	}
	for _, want := range []string{"peers not ready", "is missing peers [3]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("RunLocal error = %q, want it to contain %q", err, want)
		}
	}
}

// TestPeerRunCanceledSkipsFinal verifies that [Peer.Run] returns the error of a
// canceled context, instead of results, when the context ends during the wait
// for the Done signals, and that it skips the final hook. Node 1 cancels the
// run at the end of its benchmark, before it signals Done, so no peer can
// receive every Done signal before the cancel.
func TestPeerRunCanceledSkipsFinal(t *testing.T) {
	ctx, cancel := context.WithCancel(gorumstest.Context(t, 30*time.Second))
	defer cancel()
	var finals atomic.Int32
	w := pingWorkload(1)
	setup := w.Setup
	w.Setup = func(p *Peer) ([]Bench, error) {
		benches, err := setup(p)
		if err != nil || p.Server().NodeID() != 1 {
			return benches, err
		}
		run := benches[0].Run
		benches[0].Run = func(ctx context.Context, opts Options) (*Result, error) {
			defer cancel()
			return run(ctx, opts)
		}
		return benches, nil
	}
	w.Final = func(*Peer, []*Result) error {
		finals.Add(1)
		return nil
	}
	_, err := RunLocal(ctx, 3, regexp.MustCompile("Ping"), w, pingOpts, localOpts(t, nil)...)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunLocal error = %v, want it to wrap %v", err, context.Canceled)
	}
	if got := finals.Load(); got != 0 {
		t.Errorf("final hook ran %d times, want 0", got)
	}
}

// TestPeerRunLocalRejectsPeerCount verifies that [RunLocal] returns an error
// for fewer than two peers.
func TestPeerRunLocalRejectsPeerCount(t *testing.T) {
	for _, n := range []int{-1, 0, 1} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			results, err := RunLocal(t.Context(), n, regexp.MustCompile("Ping"), pingWorkload(1), pingOpts, localOpts(t, nil)...)
			if err == nil {
				t.Fatalf("RunLocal(%d) = %v, nil error; want an error", n, results)
			}
			if want := "at least 2 peers"; !strings.Contains(err.Error(), want) {
				t.Errorf("RunLocal(%d) error = %q, want it to contain %q", n, err, want)
			}
		})
	}
}

// TestPeerRunLocalCancelsRunningBenchmarks verifies that a benchmark error on
// one peer ends the running benchmarks of the other peers at once, because
// [RunLocal] cancels their context and [Peer.Run] passes it to each benchmark.
func TestPeerRunLocalCancelsRunningBenchmarks(t *testing.T) {
	captureDiag(t)
	errBench := errors.New("benchmark error")
	w := Workload{Setup: func(p *Peer) ([]Bench, error) {
		run := func(ctx context.Context, opts Options) (*Result, error) {
			if p.Server().NodeID() == 2 {
				return nil, errBench
			}
			benchCtx, cancel := BenchContext(ctx, opts)
			defer cancel()
			<-benchCtx.Done()
			return &Result{}, nil
		}
		return []Bench{{Name: "Wait", Run: run}}, nil
	}}
	opts := pingOpts
	opts.Duration = time.Minute
	start := time.Now()
	_, err := RunLocal(gorumstest.Context(t, 2*time.Minute), 3, regexp.MustCompile("Wait"), w, opts, localOpts(t, nil)...)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("RunLocal took %v, want the benchmark error to end the run at once", elapsed)
	}
	if !errors.Is(err, errBench) {
		t.Fatalf("RunLocal error = %v, want it to wrap %v", err, errBench)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RunLocal error = %v, want the other peers to return %v", err, context.Canceled)
	}
}
