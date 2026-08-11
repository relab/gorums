package benchmark

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
	"github.com/relab/gorums/gorumstest"
)

func TestBenchmarkDescriptions(t *testing.T) {
	descs := BenchmarkDescriptions()
	wantNames := []string{
		"QuorumCall",
		"AsyncQuorumCall",
		"SlowServer",
		"Multicast",
		"AsyncMulticast",
		"SymmetricQuorumCall",
		"SymmetricMulticast",
	}
	if len(descs) != len(wantNames) {
		t.Fatalf("got %d descriptions, want %d", len(descs), len(wantNames))
	}
	seen := make(map[string]bool, len(wantNames))
	for _, d := range descs {
		if d.Description == "" {
			t.Errorf("%q: empty description", d.Name)
		}
		seen[d.Name] = true
	}
	for _, want := range wantNames {
		if !seen[want] {
			t.Errorf("missing benchmark %q", want)
		}
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
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

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

// TestRunSymmetricQuorumCallDefaultsToMajority verifies that
// runSymmetricQuorumCall completes a short run without an explicit
// QuorumSize, falling back to a majority of the outbound peer count.
func TestRunSymmetricQuorumCallDefaultsToMajority(t *testing.T) {
	targets := localSymmetricTargets(t, 3, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, target := range targets {
		if err := AwaitReady(ctx, target); err != nil {
			t.Fatalf("AwaitReady: %v", err)
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
	target := benchTarget(servers[0], 2) // numPeers=2 arms Done tracking for IDs 1 and 2
	target2 := benchTarget(servers[1], 2)
	go func() { _ = servers[0].ListenAndServe() }()
	go func() { _ = servers[1].ListenAndServe() }()

	// Both sides probe: a dual-mode stream is only fully up once each side has
	// dialed out, so waiting on only one direction can stall the other peer's
	// half of the handshake.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errCh := make(chan error, 2)
	go func() { errCh <- AwaitReady(ctx, target) }()
	go func() { errCh <- AwaitReady(ctx, target2) }()
	var errs error
	for range 2 {
		if err := <-errCh; err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		t.Fatalf("AwaitReady: %v", errs)
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
	target := benchTarget(servers[0], 2)
	// Node 2 drops every reply from the start, mirroring
	// TestQuorumCallHonorsCallTimeout: a call without CallTimeout would hang
	// until BenchContext's own (30s+) deadline instead of the short one
	// below. AwaitReady is not used here since its own probe is a QuorumCall
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

func TestGetBenchmarksTargetRouting(t *testing.T) {
	symTarget := &SymmetricTarget{}

	tests := []struct {
		name  string
		t     BenchTarget
		count int
	}{
		{"empty", BenchTarget{}, 0},
		{"symmetric only", BenchTarget{Symmetric: symTarget}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBenchmarks(tt.t)
			if len(got) != tt.count {
				t.Errorf("got %d benchmarks, want %d", len(got), tt.count)
			}
		})
	}
}

// TestGetBenchmarksExcludesConfigBenchmarksForDistributedTarget verifies that
// a distributed (multi-process) symmetric target excludes needsConfig
// benchmarks (QuorumCall, Multicast, ...), unlike a local symmetric target.
// Every distributed node runs the same binary; if a needsConfig benchmark
// were exposed, more than one node selecting it would each issue their own
// Control.Start/Stop against the same peer group concurrently, corrupting
// every other node's Stats window mid-run. Only the needsSymmetric
// benchmarks (SymmetricQuorumCall, SymmetricMulticast), designed for
// concurrent per-node execution, are safe here.
func TestGetBenchmarksExcludesConfigBenchmarksForDistributedTarget(t *testing.T) {
	peers := []string{"127.0.0.1:0", "127.0.0.2:0"}
	target, stop, err := SetupRemoteServer(peers[0], peers, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupRemoteServer: %v", err)
	}
	t.Cleanup(stop)

	got := GetBenchmarks(BenchTarget{Symmetric: target})
	for _, b := range got {
		if b.Name == "QuorumCall" || b.Name == "Multicast" || b.Name == "AsyncMulticast" || b.Name == "AsyncQuorumCall" || b.Name == "SlowServer" {
			t.Errorf("GetBenchmarks(distributed target) included needsConfig benchmark %q, want excluded", b.Name)
		}
	}
	const wantCount = 2 // SymmetricQuorumCall, SymmetricMulticast
	if len(got) != wantCount {
		t.Errorf("GetBenchmarks(distributed target) returned %d benchmarks, want %d", len(got), wantCount)
	}
}

// TestGetBenchmarksMatchesDescriptionsForFullTarget verifies that a target
// exposing both a Config and a SymmetricTarget produces exactly the
// runnable benchmarks BenchmarkDescriptions lists, by name and count: both
// views are derived from the one benchDescs table (see benchmark.go), so
// they cannot drift the way two hand-written lists could.
func TestGetBenchmarksMatchesDescriptionsForFullTarget(t *testing.T) {
	target, stop, err := SetupSymmetricServers(2, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	// Symmetric alone is enough: GetBenchmarks derives cfg from server 0's
	// outbound config when t.Config is unset, so needsConfig benchmarks are
	// also included.
	got := GetBenchmarks(BenchTarget{Symmetric: target})
	gotNames := make(map[string]bool, len(got))
	for _, b := range got {
		gotNames[b.Name] = true
	}

	wantDescs := BenchmarkDescriptions()
	if len(got) != len(wantDescs) {
		t.Fatalf("GetBenchmarks returned %d benchmarks, want %d (BenchmarkDescriptions)", len(got), len(wantDescs))
	}
	for _, d := range wantDescs {
		if !gotNames[d.Name] {
			t.Errorf("BenchmarkDescriptions lists %q but GetBenchmarks did not return it", d.Name)
		}
	}
}

// TestAsyncQCBoundsInFlight verifies that -max-async bounds the calls actually
// in flight, and that the recorded latency describes the calls rather than the
// harness. Both are checked against the same run: Little's law ties throughput
// and mean latency to the concurrency the bound permits, so a latency inflated
// by the harness's own scheduling shows up as an impossible concurrency.
func TestAsyncQCBoundsInFlight(t *testing.T) {
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}

	const maxAsync = 64
	var inFlight, peak atomic.Int64
	opts := benchkit.Options{Workers: 2, MaxAsync: maxAsync, Duration: 2 * time.Second, QuorumSize: 2}
	res, err := runAsyncQCBenchmark(opts, target.servers[0].PeerConfig(),
		func(cc *ConfigContext, in *Echo, quorumSize int) AsyncEcho {
			fut := QuorumCall(cc, in).AsyncThreshold(quorumSize)
			cur := inFlight.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			// Async.Get reads an already-closed channel, so observing the same
			// future from a second goroutine is safe.
			go func() { _, _ = fut.Get(); inFlight.Add(-1) }()
			return fut
		})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// The observer decrements after the benchmark's own completion path, so a
	// small overshoot is measurement skew rather than a broken bound.
	if got := peak.Load(); got > maxAsync*2 {
		t.Errorf("peak in flight = %d, want <= %d (2x -max-async=%d)", got, maxAsync*2, maxAsync)
	}

	lat := res.GetLatencies()
	if len(lat) == 0 {
		t.Fatal("no latency samples recorded")
	}
	var sum int64
	for _, l := range lat {
		sum += l
	}
	mean := time.Duration(sum / int64(len(lat)))
	elapsed := time.Duration(res.GetTotalTime())
	throughput := float64(res.GetTotalOps()) / elapsed.Seconds()
	concurrency := throughput * mean.Seconds()
	t.Logf("throughput=%.0f/s mean=%v peak_in_flight=%d littles_law_concurrency=%.1f",
		throughput, mean, peak.Load(), concurrency)
	if concurrency > maxAsync*4 {
		t.Errorf("throughput %.0f/s at mean latency %v implies %.0f concurrent calls, but -max-async=%d; "+
			"the recorded latency is measuring the harness, not the calls",
			throughput, mean, concurrency, maxAsync)
	}
}

// TestAsyncQCComplete verifies the per-call completion handling that keeps
// AsyncQuorumCall aligned with [benchkit.MeasureLatency]'s contract:
// [benchkit.ErrRunOver] stops the send chain without refiring and records
// neither a success nor a failure; any other error is counted via
// [benchkit.Measurement.RecordError] and the chain still refires, so a
// saturating workload degrades gracefully instead of aborting on the first
// failure; a nil error records the latency.
func TestAsyncQCComplete(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantRefire    bool
		wantTotalOps  uint64
		wantFailedOps uint64
	}{
		{"Success", nil, true, 1, 0},
		{"FailureRefiresAndCounts", errors.New("quorum call failed"), true, 0, 1},
		{"RunOverStopsWithoutCounting", benchkit.ErrRunOver, false, 0, 0},
		{"WrappedRunOverStopsWithoutCounting", fmt.Errorf("node 3: %w", benchkit.ErrRunOver), false, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := benchkit.StartMeasurement(benchkit.Options{})
			refire := asyncQCComplete(tt.err, time.Millisecond, m)
			if refire != tt.wantRefire {
				t.Errorf("refire = %v, want %v", refire, tt.wantRefire)
			}
			result := m.Finish()
			if got := result.GetTotalOps(); got != tt.wantTotalOps {
				t.Errorf("TotalOps = %d, want %d", got, tt.wantTotalOps)
			}
			if got := result.GetFailedOps(); got != tt.wantFailedOps {
				t.Errorf("FailedOps = %d, want %d", got, tt.wantFailedOps)
			}
		})
	}
}

// TestRunAsyncQCBenchmarkRejectsRateRamp verifies that runAsyncQCBenchmark
// rejects rate-ramp options instead of silently ignoring them: sends fire
// from completion callbacks gated by a shared RatedGate, not the
// runMeasure/runSchedule path that implements ramping for ClientMeasured and
// ServerMeasured. config is nil because the rejection happens before it is
// touched.
func TestRunAsyncQCBenchmarkRejectsRateRamp(t *testing.T) {
	opts := benchkit.Options{Workers: 1, Duration: time.Second, RateStep: 10, RateStepMax: 100}
	_, err := runAsyncQCBenchmark(opts, nil, func(*ConfigContext, *Echo, int) AsyncEcho {
		t.Fatal("asyncQCFunc invoked despite rejected rate-ramp options")
		return nil
	})
	if !errors.Is(err, ErrAsyncQCRampUnsupported) {
		t.Fatalf("err = %v, want %v", err, ErrAsyncQCRampUnsupported)
	}
}

// registerFailingQuorumCallPeer registers a QuorumCall handler on srv that
// always fails, mirroring a peer whose quorum calls are erroring (as opposed
// to registerReplyDroppingPeer's silent drop, which instead times out). Call
// before serving srv, since it registers a service.
func registerFailingQuorumCallPeer(srv *gorums.Server, errMsg string) {
	srv.RegisterHandler("benchmark.Benchmark.QuorumCall", func(_ gorums.ServerContext, _ *gorums.Message) (*gorums.Message, error) {
		return nil, errors.New(errMsg)
	})
}

// TestQuorumCallHonorsCallTimeout verifies that the coordinator QuorumCall
// benchmark bounds each call to opts.CallTimeout instead of hanging behind an
// unresponsive peer until the run's own deadline elapses.
func TestQuorumCallHonorsCallTimeout(t *testing.T) {
	servers := localServers(t, 1, nil)
	registerReplyDroppingPeer(servers[0], 1<<30) // drops every reply for the test's duration
	go func() { _ = servers[0].ListenAndServe() }()

	cfg, err := gorums.NewConfig(gorums.WithNodeList([]string{servers[0].Addr()}), gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	t.Cleanup(gorumstest.Closer(t, cfg))

	run := benchDescs[0].build(cfg, nil) // "QuorumCall"
	opts := benchkit.Options{
		Workers: 1, Duration: 100 * time.Millisecond, QuorumSize: 1,
		CallTimeout: 20 * time.Millisecond,
	}
	start := time.Now()
	result, err := run(opts)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("QuorumCall benchmark: %v", err)
	}
	const wantBound = 5 * time.Second // generous vs. what an ignored CallTimeout would look like
	if elapsed > wantBound {
		t.Errorf("took %v, want well under %v (CallTimeout should bound each call against the unresponsive peer)", elapsed, wantBound)
	}
	if result.GetFailedOps() == 0 {
		t.Error("FailedOps = 0, want > 0 (every call should time out against the unresponsive peer)")
	}
}

// TestRunAsyncQCBenchmarkCountsErrorsWithoutAborting verifies that a failing
// quorum call does not abort runAsyncQCBenchmark: a failed call is counted
// and the run continues, matching [benchkit.ClientMeasured]'s
// [benchkit.MeasureLatency] contract.
func TestRunAsyncQCBenchmarkCountsErrorsWithoutAborting(t *testing.T) {
	servers := localServers(t, 1, nil)
	registerFailingQuorumCallPeer(servers[0], "quorum call failed")
	go func() { _ = servers[0].ListenAndServe() }()

	cfg, err := gorums.NewConfig(gorums.WithNodeList([]string{servers[0].Addr()}), gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	t.Cleanup(gorumstest.Closer(t, cfg))

	opts := benchkit.Options{Workers: 1, Duration: 30 * time.Millisecond, MaxAsync: 10, QuorumSize: 1}
	result, err := runAsyncQCBenchmark(opts, cfg,
		func(ctx *ConfigContext, in *Echo, quorumSize int) AsyncEcho {
			return QuorumCall(ctx, in).AsyncThreshold(quorumSize)
		})
	if err != nil {
		t.Fatalf("runAsyncQCBenchmark aborted on call error: %v", err)
	}
	if result.GetFailedOps() == 0 {
		t.Error("FailedOps = 0, want > 0 (failed calls must be counted, not abort the run)")
	}
}

func TestRunSymmetricMulticastDrainsServerMessages(t *testing.T) {
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
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
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
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

// TestAsyncSendsPipelinesAndDrains verifies the two properties AsyncMulticast
// relies on: dispatch keeps at most depth sends outstanding without waiting for
// them, and drain collects the ones the send window left behind so they are not
// lost from the run.
func TestAsyncSendsPipelinesAndDrains(t *testing.T) {
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}
	cc := target.servers[0].PeerConfig().Context(ctx)

	const depth = 4
	outstanding := newAsyncSends(depth)
	// The first depth dispatches must not block on completion; the ones after
	// reap an earlier send to make room.
	for range depth * 3 {
		msg := TimedMsg_builder{SendTime: time.Now().UnixNano()}.Build()
		if err := outstanding.dispatch(func() asyncSend { return Multicast(cc, msg).Async() }); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}
	if got := len(outstanding.handles); got != depth {
		t.Errorf("outstanding sends = %d, want %d", got, depth)
	}
	if err := outstanding.drain(); err != nil {
		t.Errorf("drain: %v", err)
	}
	if got := len(outstanding.handles); got != 0 {
		t.Errorf("outstanding sends after drain = %d, want 0", got)
	}
}

type blockingAsyncSend struct {
	done   <-chan struct{}
	active *atomic.Int32
}

func (s *blockingAsyncSend) Wait() error {
	<-s.done
	s.active.Add(-1)
	return nil
}

// TestAsyncSendsReservesCapacityBeforeDispatch verifies that -max-async is a
// bound on sends actually dispatched, not merely on handles retained after
// dispatch. The third closure must not run until one of the first two handles
// has completed.
func TestAsyncSendsReservesCapacityBeforeDispatch(t *testing.T) {
	const depth = 2
	outstanding := newAsyncSends(depth)
	done := make(chan struct{}, 3)
	var active atomic.Int32
	newSend := func() asyncSend {
		active.Add(1)
		return &blockingAsyncSend{done: done, active: &active}
	}

	for range depth {
		if err := outstanding.dispatch(newSend); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}
	if got := active.Load(); got != depth {
		t.Fatalf("active sends = %d, want %d", got, depth)
	}

	dispatchStarted := make(chan struct{})
	thirdDispatched := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		close(dispatchStarted)
		errCh <- outstanding.dispatch(func() asyncSend {
			h := newSend()
			close(thirdDispatched)
			return h
		})
	}()
	<-dispatchStarted

	select {
	case <-thirdDispatched:
		t.Fatal("third send dispatched before outstanding capacity was released")
	case <-time.After(20 * time.Millisecond):
	}

	done <- struct{}{}
	select {
	case <-thirdDispatched:
	case <-time.After(time.Second):
		t.Fatal("third send did not dispatch after outstanding capacity was released")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("third dispatch: %v", err)
	}
	if got := active.Load(); got != depth {
		t.Errorf("active sends after third dispatch = %d, want %d", got, depth)
	}

	done <- struct{}{}
	done <- struct{}{}
	if err := outstanding.drain(); err != nil {
		t.Fatalf("drain: %v", err)
	}
}

// TestAsyncMulticastBenchmarkRuns verifies that the AsyncMulticast benchmark
// completes a server-measured run and records operations, exercising the
// dispatch and quiesce-drain wiring together.
func TestAsyncMulticastBenchmarkRuns(t *testing.T) {
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}

	benches := GetBenchmarks(BenchTarget{Config: target.servers[0].PeerConfig()})
	idx := slices.IndexFunc(benches, func(b benchkit.Bench) bool { return b.Name == "AsyncMulticast" })
	if idx < 0 {
		t.Fatal("AsyncMulticast benchmark not registered")
	}
	result, err := benches[idx].Run(benchkit.Options{
		Workers: 2, MaxAsync: 8, Duration: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.GetTotalOps() == 0 {
		t.Error("TotalOps = 0, want > 0")
	}
}

// TestServerMeasuredMulticastHDR verifies that the coordinator server-measured
// Multicast lifecycle honors StatsMode_HDR end to end: the mode reaches the
// server over the Start RPC, so Stop returns a histogram, clock-offset
// correction shifts the histogram, and the aggregate carries it (Latencies nil).
func TestServerMeasuredMulticastHDR(t *testing.T) {
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}

	run := benchkit.ServerMeasured(target.servers[0].PeerConfig(),
		func(_ benchkit.Options, cc *ConfigContext) func() error {
			return func() error {
				msg := TimedMsg_builder{SendTime: time.Now().UnixNano()}.Build()
				return Multicast(cc, msg).Send()
			}
		})

	result, err := run(benchkit.Options{Workers: 1, Duration: 20 * time.Millisecond, StatsMode: benchkit.StatsMode_HDR})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.GetTotalOps() == 0 {
		t.Fatal("TotalOps = 0, want > 0")
	}
	if got := result.GetLatencies(); got != nil {
		t.Errorf("Latencies in HDR mode = %v, want nil", got)
	}
	if result.GetHistogram() == nil {
		t.Error("Histogram in HDR mode = nil, want non-nil")
	}
}

// TestServerMeasuredQuiesce verifies that benchkit.ServerMeasured invokes the
// WithQuiesce drain hook after the send window and before Control.Stop
// collects the server-side statistics.
func TestServerMeasuredQuiesce(t *testing.T) {
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}

	quiesceCalls := 0
	run := benchkit.ServerMeasured(target.servers[0].PeerConfig(),
		func(_ benchkit.Options, cc *ConfigContext) func() error {
			return func() error {
				msg := TimedMsg_builder{SendTime: time.Now().UnixNano()}.Build()
				return Multicast(cc, msg).Send()
			}
		},
		benchkit.WithQuiesce(func(context.Context) error {
			quiesceCalls++
			return nil
		}))

	result, err := run(benchkit.Options{Workers: 1, Duration: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if quiesceCalls != 1 {
		t.Errorf("quiesce calls = %d, want 1", quiesceCalls)
	}
	if result.GetTotalOps() == 0 {
		t.Error("TotalOps = 0, want > 0")
	}
}

// TestServerMeasuredVerify verifies that benchkit.WithVerify receives the
// per-server Stop replies of a server-measured run and that a verify error
// fails the run before aggregation.
func TestServerMeasuredVerify(t *testing.T) {
	target, stop, err := SetupSymmetricServers(3, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}

	setup := func(_ benchkit.Options, cc *ConfigContext) func() error {
		return func() error {
			msg := TimedMsg_builder{SendTime: time.Now().UnixNano()}.Build()
			return Multicast(cc, msg).Send()
		}
	}

	var gotNodes int
	run := benchkit.ServerMeasured(target.servers[0].PeerConfig(), setup,
		benchkit.WithVerify(func(replies map[uint32]*benchkit.Result) error {
			gotNodes = len(replies)
			return nil
		}))
	if _, err := run(benchkit.Options{Workers: 1, Duration: 20 * time.Millisecond}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotNodes != 3 {
		t.Errorf("verify saw %d replies, want 3", gotNodes)
	}

	errVerify := errors.New("per-server ops diverged")
	failing := benchkit.ServerMeasured(target.servers[0].PeerConfig(), setup,
		benchkit.WithVerify(func(map[uint32]*benchkit.Result) error { return errVerify }))
	if _, err := failing(benchkit.Options{Workers: 1, Duration: 20 * time.Millisecond}); !errors.Is(err, errVerify) {
		t.Errorf("run with failing verify = %v, want %v", err, errVerify)
	}
}

// TestServerMeasuredWindowExcludesClockSync verifies that the server-measured
// throughput window (Control.Start to Control.Stop) brackets only the send
// window, not the two clock-offset estimation phases that run around it. Each
// phase is 50 sequential ClockSync round trips, so on a real network the
// window would otherwise report a TotalTime and Throughput skewed by
// clock-sync time; a bufconn round trip is normally too fast for that skew to
// show up in a wall-clock assertion, so a per-round delay on ClockSync alone
// (via a server interceptor) stands in for that network cost and makes the
// leak deterministically detectable.
func TestServerMeasuredWindowExcludesClockSync(t *testing.T) {
	const clockSyncDelay = 2 * time.Millisecond
	delayClockSync := func(ctx gorums.ServerContext, in *gorums.Message, next gorums.Handler) (*gorums.Message, error) {
		if in.GetMethod() == "benchkit.Control.ClockSync" {
			time.Sleep(clockSyncDelay)
		}
		return next(ctx, in)
	}
	serverOpts := []gorums.ServerOption{gorums.WithServerInterceptors(delayClockSync)}
	target, stop, err := SetupSymmetricServers(3, serverOpts, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("SetupSymmetricServers: %v", err)
	}
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := AwaitReady(ctx, target); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}

	setup := func(_ benchkit.Options, cc *ConfigContext) func() error {
		return func() error {
			msg := TimedMsg_builder{SendTime: time.Now().UnixNano()}.Build()
			return Multicast(cc, msg).Send()
		}
	}

	run := benchkit.ServerMeasured(target.servers[0].PeerConfig(), setup)
	const duration = 20 * time.Millisecond
	result, err := run(benchkit.Options{Workers: 1, Duration: duration, Interval: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Each of the two clock-sync phases pays clockSyncRounds (50) sequential
	// ClockSync round trips, so if either phase leaked into the window,
	// TotalTime would grow by tens of clockSyncDelay on top of duration. A
	// window correctly bounded to the send phase stays within a few
	// durations' worth of slack for scheduling and the Stop RPC.
	if got, want := time.Duration(result.GetTotalTime()), duration+20*clockSyncDelay; got > want {
		t.Errorf("TotalTime = %v, want <= %v (clock-sync phases leaking into the measurement window?)", got, want)
	}
	var eventDuration time.Duration
	for _, event := range result.GetEvents() {
		if throughput := event.GetThroughput(); throughput != nil {
			eventDuration += time.Duration(throughput.GetDuration())
		}
	}
	if eventDuration == 0 {
		t.Fatal("throughput event duration = 0, want a measured interval")
	}
	if want := duration + 20*clockSyncDelay; eventDuration > want {
		t.Errorf("throughput event duration = %v, want <= %v (clock-sync phases leaking into the event stream?)", eventDuration, want)
	}
}
