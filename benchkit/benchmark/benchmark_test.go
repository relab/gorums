package benchmark

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
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

func TestBenchmarksTargetRouting(t *testing.T) {
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
			got := benchmarks(tt.t)
			if len(got) != tt.count {
				t.Errorf("got %d benchmarks, want %d", len(got), tt.count)
			}
		})
	}
}

// TestBenchmarksExcludesConfigBenchmarksForDistributedTarget verifies that
// a distributed (multi-process) symmetric target excludes needsConfig
// benchmarks (QuorumCall, Multicast, ...), unlike a local symmetric target.
// Every distributed node runs the same binary; if a needsConfig benchmark
// were exposed, more than one node selecting it would each issue their own
// Control.Start/Stop against the same peer group concurrently, corrupting
// every other node's Stats window mid-run. Only the needsSymmetric
// benchmarks (SymmetricQuorumCall, SymmetricMulticast), designed for
// concurrent per-node execution, are safe here.
func TestBenchmarksExcludesConfigBenchmarksForDistributedTarget(t *testing.T) {
	peers := []string{"127.0.0.1:0", "127.0.0.2:0"}
	target, stop, err := setupRemoteServer(peers[0], peers, nil, gorumstest.InsecureDialOptions(t))
	if err != nil {
		t.Fatalf("setupRemoteServer: %v", err)
	}
	t.Cleanup(stop)

	got := benchmarks(BenchTarget{Symmetric: target})
	for _, b := range got {
		if b.Name == "QuorumCall" || b.Name == "Multicast" || b.Name == "AsyncMulticast" || b.Name == "AsyncQuorumCall" || b.Name == "SlowServer" {
			t.Errorf("benchmarks(distributed target) included needsConfig benchmark %q, want excluded", b.Name)
		}
	}
	const wantCount = 2 // SymmetricQuorumCall, SymmetricMulticast
	if len(got) != wantCount {
		t.Errorf("benchmarks(distributed target) returned %d benchmarks, want %d", len(got), wantCount)
	}
}

// TestBenchmarksMatchesDescriptionsForFullTarget verifies that a target
// exposing both a Config and a SymmetricTarget produces exactly the
// runnable benchmarks BenchmarkDescriptions lists, by name and count: both
// views are derived from the one benchDescs table (see benchmark.go), so
// they cannot drift the way two hand-written lists could.
func TestBenchmarksMatchesDescriptionsForFullTarget(t *testing.T) {
	target := symmetricServers(t, 2)

	// Symmetric alone is enough: benchmarks derives cfg from server 0's
	// outbound config when t.Config is unset, so needsConfig benchmarks are
	// also included.
	got := benchmarks(BenchTarget{Symmetric: target})
	gotNames := make(map[string]bool, len(got))
	for _, b := range got {
		gotNames[b.Name] = true
	}

	wantDescs := BenchmarkDescriptions()
	if len(got) != len(wantDescs) {
		t.Fatalf("benchmarks returned %d benchmarks, want %d (BenchmarkDescriptions)", len(got), len(wantDescs))
	}
	for _, d := range wantDescs {
		if !gotNames[d.Name] {
			t.Errorf("BenchmarkDescriptions lists %q but benchmarks did not return it", d.Name)
		}
	}
}

// TestAsyncQCBoundsInFlight verifies that -max-async bounds the calls actually
// in flight, and that the recorded latency describes the calls rather than the
// harness. Both are checked against the same run: Little's law ties throughput
// and mean latency to the concurrency the bound permits, so a latency inflated
// by the harness's own scheduling shows up as an impossible concurrency.
func TestAsyncQCBoundsInFlight(t *testing.T) {
	target := symmetricServers(t, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
	}

	const maxAsync = 64
	// The harness dispatches a call only after taking a token, and returns the
	// token only after an earlier call completed. So when a call is dispatched,
	// every tracked call that is not yet done holds a token, and the number of
	// such calls cannot exceed maxAsync, whatever the goroutine scheduling.
	var (
		mu          sync.Mutex
		outstanding []AsyncEcho
		peak        int
	)
	opts := benchkit.Options{Workers: 2, MaxAsync: maxAsync, Duration: 2 * time.Second, QuorumSize: 2}
	res, err := runAsyncQCBenchmark(opts, target.servers[0].PeerConfig(),
		func(cc *ConfigContext, in *Echo, quorumSize int) AsyncEcho {
			fut := QuorumCall(cc, in).AsyncThreshold(quorumSize)
			mu.Lock()
			defer mu.Unlock()
			outstanding = slices.DeleteFunc(outstanding, AsyncEcho.Done)
			outstanding = append(outstanding, fut)
			peak = max(peak, len(outstanding))
			return fut
		})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if peak > maxAsync {
		t.Errorf("peak in flight = %d, want <= %d (-max-async)", peak, maxAsync)
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
		throughput, mean, peak, concurrency)
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
	if !errors.Is(err, errAsyncQCRampUnsupported) {
		t.Fatalf("err = %v, want %v", err, errAsyncQCRampUnsupported)
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

// TestAsyncSendsPipelinesAndDrains verifies the two properties AsyncMulticast
// relies on: dispatch keeps at most depth sends outstanding without waiting for
// them, and drain collects the ones the send window left behind so they are not
// lost from the run.
func TestAsyncSendsPipelinesAndDrains(t *testing.T) {
	target := symmetricServers(t, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
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

type failingAsyncSend struct {
	reaping chan<- struct{}
	fail    <-chan struct{}
}

func (s failingAsyncSend) Wait() error {
	close(s.reaping)
	<-s.fail
	return errors.New("send failed")
}

type completedAsyncSend struct{}

func (completedAsyncSend) Wait() error { return nil }

// TestAsyncSendsFailedReapFreesWaitingWorker verifies that a worker waiting
// for capacity can dispatch after another worker's reap fails. The failing
// worker frees its slot without queueing a new send, so the waiting worker
// must take that slot instead of waiting for a send to reap.
func TestAsyncSendsFailedReapFreesWaitingWorker(t *testing.T) {
	outstanding := newAsyncSends(1)
	reaping, fail := make(chan struct{}), make(chan struct{})
	if err := outstanding.dispatch(func() asyncSend { return failingAsyncSend{reaping: reaping, fail: fail} }); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	complete := func() asyncSend { return completedAsyncSend{} }

	reaperErr := make(chan error, 1)
	go func() { reaperErr <- outstanding.dispatch(complete) }()
	<-reaping
	waiterErr := make(chan error, 1)
	go func() { waiterErr <- outstanding.dispatch(complete) }()
	// Let the second worker find the only slot taken and start waiting.
	time.Sleep(20 * time.Millisecond)

	close(fail)
	if err := <-reaperErr; err == nil {
		t.Fatal("reaping dispatch succeeded, want the failed send's error")
	}
	select {
	case err := <-waiterErr:
		if err != nil {
			t.Fatalf("waiting dispatch: %v", err)
		}
	case <-time.After(time.Second):
		// Unblock the stranded worker so the test does not leak it.
		outstanding.slots <- struct{}{}
		outstanding.handles <- completedAsyncSend{}
		<-waiterErr
		t.Fatal("waiting dispatch stayed blocked after the in-flight send failed")
	}
	if err := outstanding.drain(); err != nil {
		t.Fatalf("drain: %v", err)
	}
}

// TestAsyncMulticastBenchmarkRuns verifies that the AsyncMulticast benchmark
// completes a server-measured run and records operations, exercising the
// dispatch and quiesce-drain wiring together.
func TestAsyncMulticastBenchmarkRuns(t *testing.T) {
	target := symmetricServers(t, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
	}

	benches := benchmarks(BenchTarget{Config: target.servers[0].PeerConfig()})
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
	target := symmetricServers(t, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
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
	target := symmetricServers(t, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
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
	target := symmetricServers(t, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
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
	target := symmetricServers(t, 3, gorums.WithServerInterceptors(delayClockSync))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := awaitReady(ctx, target); err != nil {
		t.Fatalf("awaitReady: %v", err)
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
