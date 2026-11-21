package benchmark

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
	"github.com/relab/gorums/gorumstest"
	proto "google.golang.org/protobuf/proto"
)

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

// peerTarget builds the peer config for one distributed node and registers
// the workload server. Call it before srv serves.
func peerTarget(srv *gorums.Server) peerConfig {
	return peerConfig{newBenchPeer(srv)}
}

// peerTargets returns a serving peerTarget for each of n local servers, so a
// test can drive each node on its own, as separate processes do.
func peerTargets(t *testing.T, n int, serverOpt gorums.ServerOption) []peerConfig {
	t.Helper()
	servers := localServers(t, n, serverOpt)
	targets := make([]peerConfig, n)
	for i, srv := range servers {
		targets[i] = peerTarget(srv)
	}
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}
	return targets
}

// symmetricServers starts n local peers in one process,
// with serverOpts applied to every server, and stops them when the test ends.
func symmetricServers(t *testing.T, n int, serverOpts ...gorums.ServerOption) peerConfig {
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
	srv.RegisterHandler("benchmark.Benchmark.QuorumCall", func(_ gorums.ServerContext, in *gorums.Message, send func(proto.Message)) error {
		if remaining.Add(-1) >= 0 {
			return nil // no send and no error: the server sends no reply
		}
		send(gorums.AsProto[*Echo](in))
		return nil
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
	for _, p := range target {
		if _, err := p.Server().WaitForAll(ctx); err != nil {
			t.Fatalf("WaitForAll: %v", err)
		}
	}

	srv3 := target[2].Server()
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

	size := target[0].Server().PeerConfig().Size()
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
	target[0].Control().ArmDone(size)
	for id := 1; id <= size; id++ {
		target[0].Control().Done(gorumstest.ServerContext(t, gorums.ID(id)), &benchkit.DoneRequest{})
	}
	if !anyPeerFinished(target) {
		t.Error("anyPeerFinished = false after all peers Done, want true")
	}
	if !quorumRunOver(target, quorum) {
		t.Error("quorumRunOver = false after all peers Done, want true")
	}
}

// TestRunSymmetricQuorumCallDefaultsToMajority verifies that
// runSymmetricQuorumCall completes a short run without an explicit
// QuorumSize, falling back to a majority of the outbound peer count.
func TestRunSymmetricQuorumCallDefaultsToMajority(t *testing.T) {
	targets := peerTargets(t, 3, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, target := range targets {
		if err := awaitReady(ctx, target); err != nil {
			t.Fatalf("awaitReady: %v", err)
		}
	}

	opts := benchkit.Options{Duration: 100 * time.Millisecond, Rate: 100, Workers: 1}
	result, err := runSymmetricQuorumCall(t.Context(), targets[0], opts)
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
	target, target2 := peerTarget(servers[0]), peerTarget(servers[1])
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
	target[0].Control().Done(gorumstest.ServerContext(t, 2), &benchkit.DoneRequest{})

	// Duration is deliberately long relative to the expected stop: a regular
	// (non-run-over) failure is only counted via Stats.RecordError and the
	// run keeps retrying until Duration elapses, so a prompt return is what
	// distinguishes the run-over path (which calls cancel() on the first
	// failed call) from ordinary failure counting — both paths return a nil
	// top-level error either way.
	const duration = 2 * time.Second
	opts := benchkit.Options{Duration: duration, Rate: 50, Workers: 1}
	start := time.Now()
	result, err := runSymmetricQuorumCall(t.Context(), target, opts)
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
	target := peerTarget(servers[0])
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
	result, err := runSymmetricQuorumCall(t.Context(), target, opts)
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

	result, err := runSymmetricMulticast(t.Context(), target, benchkit.Options{
		Workers:  1,
		Duration: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("runSymmetricMulticast: %v", err)
	}

	wantSamples := int(result.GetTotalOps()) * len(target)
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

	result, err := runSymmetricMulticast(t.Context(), target, benchkit.Options{
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
	if wantSamples := result.GetTotalOps() * uint64(len(target)); total != wantSamples {
		t.Errorf("histogram counts sum = %d, want %d", total, wantSamples)
	}
}

// TestSymmetricMulticastLatencySeries verifies that interval latency covers
// every receive, including the final drain, while throughput counts sends.
func TestSymmetricMulticastLatencySeries(t *testing.T) {
	for _, mode := range []benchkit.StatsMode{benchkit.StatsMode_EXACT, benchkit.StatsMode_HDR} {
		t.Run(mode.String(), func(t *testing.T) {
			target := symmetricServers(t, 3)
			ctx := gorumstest.Context(t, 5*time.Second)
			if err := awaitReady(ctx, target); err != nil {
				t.Fatal(err)
			}
			r, err := runSymmetricMulticast(t.Context(), target, benchkit.Options{
				Workers: 1, Duration: 20 * time.Millisecond,
				Interval: 5 * time.Millisecond, StatsMode: mode,
			})
			if err != nil {
				t.Fatal(err)
			}
			var samples, sends uint64
			var lastOffset int64
			var intervals int
			for _, e := range r.GetEvents() {
				if e.GetOffset() < lastOffset {
					t.Fatal("events are not ordered by time")
				}
				lastOffset = e.GetOffset()
				sends += e.GetThroughput().GetOps()
				if lat := e.GetLatency(); lat != nil {
					samples += lat.GetCount()
					intervals++
				}
			}
			if want := r.GetTotalOps() * uint64(len(target)); samples != want || samples == 0 {
				t.Errorf("latency interval samples = %d, want %d", samples, want)
			}
			if sends != r.GetTotalOps() {
				t.Errorf("throughput interval sends = %d, want %d", sends, r.GetTotalOps())
			}
			if intervals < 2 {
				t.Errorf("latency intervals = %d, want multiple intervals", intervals)
			}
			out := t.TempDir()
			if _, err := benchkit.WriteTimeSeriesCSVs(out, []benchkit.TimeSeriesGroup{{
				Benchmark: "SymmetricMulticast",
				Nodes:     []benchkit.TimeSeriesNode{{Node: "local", Events: r.GetEvents()}},
			}}, 0); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(out, "SymmetricMulticast_latency.csv"))
			if err != nil {
				t.Fatal(err)
			}
			rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != intervals+1 {
				t.Fatalf("CSV rows = %d, want %d", len(rows), intervals+1)
			}
			var csvSamples uint64
			for _, row := range rows[1:] {
				n, err := strconv.ParseUint(row[3], 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				csvSamples += n
			}
			if csvSamples != samples {
				t.Errorf("CSV samples = %d, want %d", csvSamples, samples)
			}
		})
	}
}
