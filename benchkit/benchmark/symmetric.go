package benchmark

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
)

// SymmetricTarget bundles the gorums servers and their registered control
// planes for symmetric (peer-to-peer) benchmarks.
type SymmetricTarget struct {
	servers  []*gorums.Server
	controls []*benchkit.Control
	numPeers int // cluster size (including self); sizes the exit grace period
	labels   []string
	selfAddr string // distributed mode only: this node's address in the peer list; enables the probe-stall self-diagnosis
}

// SetupSymmetricServers creates n local Gorums servers with the benchkit Control
// plane and the gorums workload server registered and serving. Returns a
// SymmetricTarget, a stop function, and any error.
func SetupSymmetricServers(n int, serverOpts []gorums.ServerOption, dialOpts ...gorums.DialOption) (*SymmetricTarget, func(), error) {
	servers, stop, err := gorums.NewLocalServers(
		n,
		gorums.WithLocalServerOptions(serverOpts...),
		gorums.WithLocalDialOptions(dialOpts...),
	)
	if err != nil {
		return nil, nil, err
	}
	controls := make([]*benchkit.Control, n)
	labels := make([]string, n)
	for i, srv := range servers {
		controls[i] = attachBenchServer(srv)
		labels[i] = fmt.Sprintf("server %d (%s)", i+1, srv.Addr())
	}
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}
	return &SymmetricTarget{servers: servers, controls: controls, numPeers: n, labels: labels}, stop, nil
}

// SetupRemoteServer creates a single Gorums server for distributed
// benchmarking. selfAddr must appear in peerAddrs; the slice is sorted to
// assign stable node IDs (1..N) across all machines.
//
// Exit synchronization is not handled here: after a node finishes its own
// benchmark it must keep its listener open for a short grace period (see
// ExitGrace) before exiting, so that slower peers can complete their final
// cross-node RPCs without hitting a closed listener. The caller is responsible
// for that linger; see cmd/benchmark.
func SetupRemoteServer(selfAddr string, peerAddrs []string, serverOpts []gorums.ServerOption, dialOpts ...gorums.DialOption) (*SymmetricTarget, func(), error) {
	sorted := slices.Clone(peerAddrs)
	slices.Sort(sorted)
	idx := slices.Index(sorted, selfAddr)
	if idx < 0 {
		return nil, nil, fmt.Errorf("self address %q not found in peer list", selfAddr)
	}
	myID := uint32(idx + 1)
	peerList := gorums.WithNodeList(sorted)

	// Listen on the wildcard address rather than selfAddr: listening on a
	// hostname binds the single IP the local resolver returns for it, and
	// hosts following the Debian convention resolve their own name to the
	// loopback address 127.0.1.1, leaving the listener unreachable for all
	// peers (see doc/benchkit-troubleshooting.html). selfAddr is still used
	// for node identity and ID assignment above.
	_, port, err := net.SplitHostPort(selfAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid self address %q: %w", selfAddr, err)
	}
	srv := gorums.NewServer(append([]gorums.ServerOption{
		gorums.WithAddr(net.JoinHostPort("", port)),
		gorums.WithPeers(myID, peerList, dialOpts...),
	}, serverOpts...)...)

	ctrl := attachBenchServer(srv)
	ctrl.ArmDone(len(peerAddrs)) // same count used for numPeers below
	label := fmt.Sprintf("node %d (%s)", myID, selfAddr)

	go func() { _ = srv.ListenAndServe() }()
	benchkit.Logf("[%s] listener bound to %s\n", label, srv.Addr())
	t := &SymmetricTarget{
		servers:  []*gorums.Server{srv},
		controls: []*benchkit.Control{ctrl},
		numPeers: len(peerAddrs),
		labels:   []string{label},
		selfAddr: selfAddr,
	}
	return t, func() {
		srv.Stop()
		benchkit.Logf("[%s %s] server stopped\n", time.Now().Format(time.TimeOnly), label)
	}, nil
}

// ExitGrace returns the fallback ceiling a distributed-mode replica waits,
// after finishing its own benchmark, for its peers to also finish before it
// gives up and exits anyway.
//
// The symmetric topology has no exit barrier: a node multicasts an advisory
// Done signal (see [SignalDone]) and races it against this ceiling (see
// [AwaitPeersDoneOrGrace]), exiting as soon as every peer has signaled or this
// timeout elapses, whichever comes first. Because most runs finish within
// milliseconds of each other and exit via the Done signal long before the
// ceiling is reached, this is a worst-case bound, not the expected wait — it
// only matters when Done itself doesn't arrive (e.g. a peer's inbound
// channel is broken, the same class of issue that made the previous Done
// barrier unreliable) or the whole cluster is unusually slow. The bound is
// the inter-node completion skew, which is dominated by mesh-formation/
// release skew in AwaitReady and therefore grows with cluster size; the
// trailing round-trips are sub-second and absorbed by the base term. The
// result is clamped so a very large cluster does not linger excessively.
func ExitGrace(numNodes int) time.Duration {
	const (
		base     = 3 * time.Second
		perNode  = 300 * time.Millisecond
		maxGrace = 20 * time.Second
	)
	return min(base+time.Duration(numNodes)*perNode, maxGrace)
}

// SignalDone notifies every outbound peer that this node has finished its
// own benchmark work and trailing flush and will issue no further calls. The
// send is best-effort: per-peer errors are discarded, since a dropped or failed
// notification only costs the receiving peer its fast exit via
// AwaitPeersDoneOrGrace, never correctness, because that wait always falls
// back to its own grace deadline.
//
// The whole notification is bounded by a deadline. Done is a one-way Multicast
// whose enqueue waits for send-queue space until the request context is done
// (the one-way path in internal/stream.(*Channel).Enqueue). Callers pass a
// deadline-free context (cmd/benchmark passes context.Background), so without a
// bound a full or backpressured send queue during the teardown broadcast traps
// this call — and hence the caller — forever; this deadlocked node teardown on
// the cluster. The bound mirrors the peers' own wait (ExitGrace over the
// cluster size): never spend longer announcing Done than a peer will wait to
// hear it. Wait blocks until each send completes or the deadline fires,
// keeping the deferred cancel from aborting a send still in flight.
func SignalDone(ctx context.Context, t *SymmetricTarget) {
	ctx, cancel := context.WithTimeout(ctx, ExitGrace(t.numPeers))
	defer cancel()
	for i, srv := range t.servers {
		out := srv.PeerConfig()
		if out.Size() == 0 {
			continue
		}
		req := benchkit.DoneRequest_builder{SenderId: t.controls[i].SelfID()}.Build()
		// Report rather than return: Done is advisory, and a peer that already
		// finished and exited makes a failed send the expected outcome, not a
		// fault. Logging it still distinguishes that from a run where every
		// send failed and no peer was ever told.
		if err := benchkit.Done(out.Context(ctx), req).Send(); err != nil {
			benchkit.Logf("Done signal from node %d: %v\n", t.controls[i].SelfID(), err)
		}
	}
}

// AwaitPeersDoneOrGrace blocks until every peer has signaled Done or grace
// elapses, whichever is first, using one shared deadline across all of t's
// local servers. It returns true only if every server's peers all signaled
// Done before the deadline; false means at least one server fell back to
// the grace timeout. It never errors: the timeout is a safe, expected
// fallback, not a failure.
func AwaitPeersDoneOrGrace(ctx context.Context, t *SymmetricTarget, grace time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	for _, ctrl := range t.controls {
		ch := ctrl.DoneCh()
		if ch == nil {
			continue
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

// MissingDoneSenders returns the peer node IDs across all of t's local
// servers that have not yet signaled Done, for diagnostics when
// AwaitPeersDoneOrGrace falls back to its timeout.
func MissingDoneSenders(t *SymmetricTarget) []uint32 {
	var missing []uint32
	for _, ctrl := range t.controls {
		missing = append(missing, ctrl.MissingDone()...)
	}
	return missing
}

// runComplete reports whether so many of outSize outbound peers have finished
// (done) that a phase needing `need` still-live peers can no longer succeed. It
// is the pure decision behind quorumRunOver: false until at least one peer has
// signaled Done, so a failure before any peer finishes (a genuine startup or
// mid-run fault) is never mistaken for the expected end of the run.
func runComplete(outSize, done, need int) bool {
	return done > 0 && outSize-done < need
}

// quorumRunOver reports whether enough outbound peers have signaled Done that a
// quorum call needing quorumSize replies can no longer form a quorum from the
// live peers. When true, an incomplete/connection-refused quorum call is the
// expected end of the run (peers finished and closed their listeners), not a
// fault, so a straggler treats it as a clean stop rather than a failure.
func quorumRunOver(t *SymmetricTarget, quorumSize int) bool {
	for i, srv := range t.servers {
		if runComplete(srv.PeerConfig().Size(), t.controls[i].DoneCount(), quorumSize) {
			return true
		}
	}
	return false
}

// anyPeerFinished reports whether any outbound peer has signaled Done. The
// all-peers phases (offset estimation and the trailing flush) require every
// peer, a requirement no live-peer set can meet once a peer exits, so a single
// Done marks their failure as the expected end of the run. It is false until a
// peer finishes, so startup failures before anyone is Done are not masked.
func anyPeerFinished(t *SymmetricTarget) bool {
	for i := range t.servers {
		if t.controls[i].DoneCount() > 0 {
			return true
		}
	}
	return false
}

// readyStallTimeout bounds how long the outbound probe in AwaitReady may go
// without any peer responding. A peer that died during startup (e.g. because
// its listen port was taken) never responds, so waiting out the rest of the
// readiness deadline only delays the inevitable failure; each response resets
// the timer, so a large mesh that is still forming keeps the probe alive. It
// is a variable so tests can shorten it.
var readyStallTimeout = 20 * time.Second

// probeAttemptTimeout bounds one per-peer echo attempt in the outbound probe
// of awaitReady. A reply that is silently lost (e.g. to stream churn during
// setup) costs one attempt round rather than the caller's whole
// readiness deadline; the next round re-sends a fresh echo over whatever
// stream is then current. It is a variable so tests can shorten it.
var probeAttemptTimeout = 2 * time.Second

// probeLogf emits outbound-probe progress, notably the per-round straggler
// list. It is a variable so tests can capture and assert on the straggler
// log; production code logs via benchkit.Logf.
var probeLogf = benchkit.Logf

// AwaitReady waits until every server in t is ready to run the benchmark: it
// validates round-trip connectivity to every outbound peer of every server,
// sending each peer echoes until it responds, so every outbound connection is
// established before the benchmark starts (gRPC dials lazily and may still be
// in backoff after setup). See probeOutbound for the per-peer retry/stall
// semantics. In distributed mode a probe failure additionally writes a
// network self-diagnosis (see diagnoseProbeStall).
//
// In dedup mode, call this after [gorums.Server.WaitForAll] so every shared
// stream is live before the probe exercises it.
func AwaitReady(ctx context.Context, t *SymmetricTarget) error {
	for i, srv := range t.servers {
		label := t.label(i)
		if err := probeOutbound(ctx, srv, label); err != nil {
			if t.selfAddr != "" {
				diagnoseProbeStall(diagWriter, srv, t.selfAddr)
			}
			return fmt.Errorf("%s: outbound peers not ready: %w", label, err)
		}
		benchkit.Logf("[ready] %s: outbound ready\n", label)
	}
	return nil
}

// probeOutbound validates round-trip connectivity to every outbound peer of
// srv before the benchmark starts. Each peer that has not yet responded is
// probed in parallel with a single-node echo call under its own
// probeAttemptTimeout deadline, so a silently lost reply costs one round, not
// the caller's whole readiness deadline (the all-or-nothing Threshold(N)
// probe it replaces blocked on one missing reply with "incomplete call
// (errors: 0)" until ctx expired). Responders leave the pending set; the
// remaining stragglers are logged by node ID each round. The probe fails
// early, naming the pending peers, when no peer has responded for
// readyStallTimeout: with nobody making progress, waiting out the rest of the
// deadline only delays the inevitable failure.
func probeOutbound(ctx context.Context, srv *gorums.Server, label string) error {
	pending := srv.PeerConfig()
	msg := Echo_builder{}.Build()
	probeLogf("[ready] %s: probing %d outbound connections...\n", label, pending.Size())
	lastResponse := time.Now()
	for pending.Size() > 0 {
		attemptCtx, cancel := context.WithTimeout(ctx, probeAttemptTimeout)
		var mu sync.Mutex
		var responded []uint32
		var wg sync.WaitGroup
		for _, node := range pending {
			wg.Go(func() {
				if _, err := QuorumCall(gorums.Config{node}.Context(attemptCtx), msg).Threshold(1); err == nil {
					mu.Lock()
					responded = append(responded, node.ID())
					mu.Unlock()
				}
			})
		}
		wg.Wait()
		cancel()
		if len(responded) > 0 {
			lastResponse = time.Now()
		}
		pending = pending.Remove(responded...)
		if pending.Size() == 0 {
			break
		}
		probeLogf("[ready] %s: outbound probe: %d peers pending: %s\n", label, pending.Size(), pendingPeerDetails(pending))
		if time.Since(lastResponse) >= readyStallTimeout {
			return fmt.Errorf("no outbound peer responded for %v; pending: %s", readyStallTimeout, pendingPeerDetails(pending))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("pending: %s: %w", pendingPeerDetails(pending), ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil
}

// nodeDetail formats node as "node ID (address)", appending its last recorded
// error when one exists, so peer-naming diagnostics identify both the peer
// and its known cause without cross-referencing logs.
func nodeDetail(node *gorums.Node) string {
	detail := fmt.Sprintf("node %d (%s)", node.ID(), node.Address())
	if err := node.LastErr(); err != nil {
		detail += fmt.Sprintf(": %v", err)
	}
	return detail
}

// pendingPeerDetails names each peer still pending in the outbound probe.
func pendingPeerDetails(pending gorums.Config) string {
	details := make([]string, 0, len(pending))
	for _, node := range pending {
		details = append(details, nodeDetail(node))
	}
	return strings.Join(details, ", ")
}

// diagWriter receives the probe-stall self-diagnosis. It is a variable so
// tests can capture and assert on the diagnosis instead of spamming stderr.
var diagWriter io.Writer = os.Stderr

// diagnoseProbeStall writes a network self-diagnosis to w when the outbound
// readiness probe fails in distributed mode. At that point this process is
// still alive on the affected host, which makes it ideally placed to
// discriminate the known failure classes (see doc/benchkit-troubleshooting.html):
//
//   - self-dial fails and the bound address differs from the advertised
//     address: the listener is bound to the wrong interface because the
//     host resolved its own name unexpectedly (e.g. to a loopback address).
//   - self-dial fails with matching addresses: a local firewall is refusing
//     new connections, or the listener died.
//   - self-dial succeeds: the listener works from this host, so the blockage
//     is between the peers and this host (network filtering, peer-side state).
//
// The goroutine dump at the end shows whether the probe's own calls stalled
// inside gorums rather than on the network, which would indicate a gorums
// bug rather than a host or network condition.
func diagnoseProbeStall(w io.Writer, srv *gorums.Server, selfAddr string) {
	fmt.Fprintf(w, "[diag] probe stall: listener bound to %s; self peer-list address %s\n", srv.Addr(), selfAddr)
	tcpAddr, err := net.ResolveTCPAddr("tcp", selfAddr)
	if err != nil {
		fmt.Fprintf(w, "[diag] cannot resolve self address %s: %v\n", selfAddr, err)
		return
	}
	advertised := tcpAddr.String()
	if tcpAddr.IP.IsLoopback() {
		fmt.Fprintf(w, "[diag] note: %s resolves to a loopback address on this host; remote peers dialing the host's real IP cannot reach a loopback-bound listener\n", selfAddr)
	}
	conn, err := net.DialTimeout("tcp", advertised, 2*time.Second)
	if err != nil {
		fmt.Fprintf(w, "[diag] self-dial %s failed: %v (listener unreachable on advertised address: wrong bind or local firewall)\n", advertised, err)
	} else {
		_ = conn.Close()
		fmt.Fprintf(w, "[diag] self-dial %s ok (listener reachable from this host; inbound blockage is between peers and this host)\n", advertised)
	}
	fmt.Fprintf(w, "[diag] goroutine dump:\n")
	_ = pprof.Lookup("goroutine").WriteTo(w, 2)
}

func (t *SymmetricTarget) label(i int) string {
	if i >= 0 && i < len(t.labels) && t.labels[i] != "" {
		return t.labels[i]
	}
	return fmt.Sprintf("server %d", i+1)
}

// estimateAllOffsets runs benchkit.EstimateOffsets from every server in t to its
// outbound peers, returning one offset map per server, indexed to match
// t.servers (and t.controls).
func estimateAllOffsets(ctx context.Context, t *SymmetricTarget) ([]map[uint32]int64, error) {
	offsets := make([]map[uint32]int64, len(t.servers))
	for i, srv := range t.servers {
		off, err := benchkit.EstimateOffsets(ctx, srv.PeerConfig())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.label(i), err)
		}
		offsets[i] = off
	}
	return offsets, nil
}

func flushSymmetricOutbound(ctx context.Context, t *SymmetricTarget) error {
	for i, srv := range t.servers {
		if err := flushOutbound(ctx, srv); err != nil {
			return fmt.Errorf("%s: %w", t.label(i), err)
		}
	}
	return nil
}

func flushOutbound(ctx context.Context, srv *gorums.Server) error {
	out := srv.PeerConfig()
	if out == nil {
		return fmt.Errorf("missing outbound configuration")
	}
	n := out.Size()
	if n == 0 {
		return gorums.ErrIncomplete
	}
	_, err := QuorumCall(out.Context(ctx), Echo_builder{}.Build()).Threshold(n)
	if err != nil {
		// A flush failure means a peer stopped responding mid-run (e.g. it
		// exited before the grace period elapsed). Name the unresponsive peers
		// so the cause is identifiable without cross-referencing logs.
		if unresponsive := unresponsiveOutbound(srv); unresponsive != "" {
			return fmt.Errorf("%w; unresponsive peers: %s", err, unresponsive)
		}
	}
	return err
}

// unresponsiveOutbound names the outbound peers with a recorded dial/call error,
// formatted as "node ID (address): error". It returns "" when every peer is
// healthy, so callers can include it only when there is something to report.
func unresponsiveOutbound(srv *gorums.Server) string {
	out := srv.PeerConfig()
	if out == nil {
		return ""
	}
	details := make([]string, 0, out.Size())
	for _, node := range out {
		if err := node.LastErr(); err != nil {
			details = append(details, nodeDetail(node))
		}
	}
	return strings.Join(details, ", ")
}

// runSymmetricQuorumCall benchmarks quorum call round-trip latency and
// throughput in a symmetric (peer-to-peer) topology. Each server issues
// QuorumCall RPCs to its outbound peers; the client side measures latency.
// quorumSize defaults to a majority of the outbound peer count.
func runSymmetricQuorumCall(t *SymmetricTarget, opts benchkit.Options) (*benchkit.Result, error) {
	ctx, cancel := benchkit.BenchContext(opts)
	defer cancel()

	// Yield each server's outbound configuration so MeasureLatency drives one
	// concurrent send target per server in a single measurement phase.
	configs := func(yield func(gorums.Config) bool) {
		for _, srv := range t.servers {
			if !yield(srv.PeerConfig()) {
				return
			}
		}
	}
	setup := func(opts benchkit.Options, cc *gorums.ConfigContext) func() error {
		msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
		// Honor the configured quorum size, matching the coordinator QuorumCall
		// benchmark; fall back to a majority of the outbound peers when unset.
		quorumSize := cmp.Or(opts.QuorumSize, cc.Config().Size()/2+1)
		call := func(cc *gorums.ConfigContext) error {
			_, err := QuorumCall(cc, msg).Threshold(quorumSize)
			if err != nil && quorumRunOver(t, quorumSize) {
				// Peers finished and closed their listeners; the incomplete
				// quorum is the expected end of the run, not a fault. Cancel the
				// window so every worker stops promptly, and return ErrRunOver
				// so this op is recorded neither as a success nor as a failure
				// and MeasureLatency returns the samples gathered so far
				// instead of failing.
				cancel()
				return benchkit.ErrRunOver
			}
			return err
		}
		if opts.CallTimeout <= 0 {
			return func() error { return call(cc) }
		}
		// With -call-timeout, each call carries its own deadline so a call
		// stalled behind an unresponsive peer fails with DeadlineExceeded
		// instead of hanging until run end. The branch is taken here at setup
		// so the default path stays free of per-op timers.
		cfg := cc.Config()
		return func() error {
			callCtx, cancelCall := context.WithTimeout(cc, opts.CallTimeout)
			defer cancelCall()
			return call(cfg.Context(callCtx))
		}
	}
	return benchkit.MeasureLatency(ctx, opts, configs, setup)
}

// runSymmetricMulticast benchmarks multicast throughput and server-side
// one-way latency in a symmetric topology. Each server sends Multicast RPCs
// to its outbound peers; the server side measures latency via TimedMsg.SendTime.
// Throughput is reported as total sends per second across all servers.
func runSymmetricMulticast(t *SymmetricTarget, opts benchkit.Options) (*benchkit.Result, error) {
	ctx, cancel := benchkit.BenchContext(opts)
	defer cancel()

	payload := make([]byte, opts.Payload)
	// Each sender tags the message with its own node ID so the receiving server
	// can bucket samples per sender and correct them by that sender's estimated
	// clock offset (see [benchkit.EstimateOffsets] and
	// [benchkit.Stats.GetResultCorrected]).
	newMsg := func(senderID uint32) *TimedMsg {
		return TimedMsg_builder{SendTime: time.Now().UnixNano(), SenderId: senderID, Payload: payload}.Build()
	}

	// resetServerStats clears and restarts every local server's counters so the
	// next phase measures from zero; used before the connection flush and again
	// at the start of the measurement window. The stats mode carries opts.StatsMode
	// so a symmetric server-measured run honors -stats-mode like the coordinator
	// path does via the Start RPC.
	resetServerStats := func() {
		for _, ctrl := range t.controls {
			ctrl.Reset(opts.StatsMode)
		}
	}

	resetServerStats()
	if err := flushSymmetricOutbound(ctx, t); err != nil && !anyPeerFinished(t) {
		return nil, err
	}

	var totalSent atomic.Int64
	var elapsed time.Duration

	// One send target per server; each tags its message with its own node ID so
	// the receiving server can bucket and clock-correct samples per sender.
	sends := make([]func() error, len(t.servers))
	for i, srv := range t.servers {
		cfgCtx := srv.PeerConfig().Context(ctx)
		senderID := t.controls[i].SelfID()
		sends[i] = func() error {
			if err := Multicast(cfgCtx, newMsg(senderID)).Send(); err != nil {
				return err
			}
			totalSent.Add(1)
			return nil
		}
	}

	// Latency is measured server-side; the client-side Measurement carries only
	// the op count (via Stats.AddOp) so the ticker can emit the throughput
	// time-series, and honors opts.Interval / opts.StatsMode like the other
	// runners. Going through MeasureOneWay also gives the symmetric multicast the
	// rate-ramp and ticker rate-step support the coordinator runners already have.
	m, window := benchkit.MeasureOneWay(ctx, opts, sends...)

	estimate := func() ([]map[uint32]int64, error) {
		off, err := estimateAllOffsets(ctx, t)
		if err != nil && anyPeerFinished(t) {
			// Peers finished and exited before this straggler could sync clocks;
			// the incomplete offset set is the expected end of the run, not a
			// fault. estimateAllOffsets returns nil on error, so hand back one
			// empty offset map per server to keep buildReplies' per-server
			// indexing in bounds; AverageOffsets tolerates the empty side, so
			// the samples are left uncorrected rather than failing the run.
			return make([]map[uint32]int64, len(t.servers)), nil
		}
		return off, err
	}
	measure := func() error {
		// Reset server stats so the measurement phase counts from zero.
		resetServerStats()
		startTime := time.Now()
		if err := window(); err != nil && !anyPeerFinished(t) {
			return err
		}
		elapsed = time.Since(startTime)
		if err := flushSymmetricOutbound(ctx, t); err != nil && !anyPeerFinished(t) {
			return err
		}
		return nil
	}
	buildReplies := func(before, after []map[uint32]int64) (map[uint32]*benchkit.Result, error) {
		for _, ctrl := range t.controls {
			ctrl.Stats().End()
		}
		// Aggregate server-side latency samples across all local servers,
		// correcting each sample by its sender's averaged clock offset.
		replies := make(map[uint32]*benchkit.Result, len(t.controls))
		for i, ctrl := range t.controls {
			benchkit.LogOffsets(t.label(i), before[i], after[i])
			offsets := benchkit.AverageOffsets(before[i], after[i])
			replies[uint32(i+1)] = ctrl.Stats().GetResultCorrected(offsets)
		}
		return replies, nil
	}
	r, err := benchkit.RunOffsetCorrected(estimate, measure, buildReplies)
	if err != nil {
		m.Abandon()
		return nil, err
	}
	m.Attach(r)

	// Override TotalOps and Throughput with client-side send counts, which
	// are the authoritative measure of work done. Server receives are N times
	// sends in this topology, so server TotalOps would overcount.
	n := uint64(totalSent.Load())
	r.SetTotalOps(n)
	r.SetTotalTime(int64(elapsed))
	if n > 0 {
		r.SetThroughput(float64(n) / elapsed.Seconds())
	}
	return r, nil
}
