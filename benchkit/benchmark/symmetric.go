package benchmark

import (
	"cmp"
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
)

// peerConfig contains the peers driven by this process in a symmetric run.
type peerConfig = []*benchkit.Peer

// newBenchPeer returns a [benchkit.Peer] for srv with the gorums workload
// server registered on it, recording into the Control of the peer. Call it
// before srv serves.
func newBenchPeer(srv *gorums.Server) *benchkit.Peer {
	p := benchkit.NewPeer(srv)
	RegisterBenchmarkServer(srv, &workloadServer{ctrl: p.Control()})
	return p
}

// setupSymmetricServers creates n local Gorums servers with the benchkit Control
// plane and the gorums workload server registered and serving. It returns the peers,
// a stop function, and any error.
func setupSymmetricServers(n int, serverOpts []gorums.ServerOption, dialOpts ...gorums.DialOption) (peerConfig, func(), error) {
	servers, stop, err := gorums.NewLocalServers(
		n,
		gorums.WithLocalServerOptions(serverOpts...),
		gorums.WithLocalDialOptions(dialOpts...),
	)
	if err != nil {
		return nil, nil, err
	}
	peers := make(peerConfig, n)
	for i, srv := range servers {
		peers[i] = newBenchPeer(srv)
	}
	for _, srv := range servers {
		go func() { _ = srv.ListenAndServe() }()
	}
	return peers, stop, nil
}

// awaitReady waits for the mesh to complete. The first peer's check covers
// the whole mesh.
func awaitReady(ctx context.Context, peers peerConfig) error {
	return peers[0].AwaitReady(ctx)
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
func quorumRunOver(peers peerConfig, quorumSize int) bool {
	for _, p := range peers {
		if runComplete(p.Server().PeerConfig().Size(), p.Control().DoneCount(), quorumSize) {
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
func anyPeerFinished(peers peerConfig) bool {
	for _, p := range peers {
		if p.Control().DoneCount() > 0 {
			return true
		}
	}
	return false
}

// estimateAllOffsets runs benchkit.EstimateOffsets from each server to its outbound peers.
// The returned offset maps follow the order of peers.
func estimateAllOffsets(ctx context.Context, peers peerConfig) ([]map[uint32]int64, error) {
	offsets := make([]map[uint32]int64, len(peers))
	for i, p := range peers {
		off, err := benchkit.EstimateOffsets(ctx, p.Server().PeerConfig())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		offsets[i] = off
	}
	return offsets, nil
}

func flushSymmetricOutbound(ctx context.Context, peers peerConfig) error {
	for _, p := range peers {
		if err := p.FlushOutbound(ctx); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// runSymmetricQuorumCall benchmarks quorum call round-trip latency and
// throughput in a symmetric (peer-to-peer) topology. Each server issues
// QuorumCall RPCs to its outbound peers; the client side measures latency.
// quorumSize defaults to a majority of the outbound peer count.
func runSymmetricQuorumCall(ctx context.Context, peers peerConfig, opts benchkit.Options) (*benchkit.Result, error) {
	ctx, cancel := benchkit.BenchContext(ctx, opts)
	defer cancel()

	// Yield each server's outbound configuration so MeasureLatency drives one
	// concurrent send target per server in a single measurement phase.
	configs := func(yield func(Config) bool) {
		for _, p := range peers {
			if !yield(p.Server().PeerConfig()) {
				return
			}
		}
	}
	setup := func(ctx context.Context, cfg Config, opts benchkit.Options) func() error {
		client := NewBenchmarkClient(cfg)
		msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
		// Honor the configured quorum size, matching the coordinator QuorumCall
		// benchmark; fall back to a majority of the outbound peers when unset.
		quorumSize := cmp.Or(opts.QuorumSize, cfg.Size()/2+1)
		call := func(ctx context.Context) error {
			_, err := client.QuorumCall(ctx, msg).Threshold(quorumSize)
			if err != nil && quorumRunOver(peers, quorumSize) {
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
			return func() error { return call(ctx) }
		}
		// With -call-timeout, each call carries its own deadline so a call
		// stalled behind an unresponsive peer fails with DeadlineExceeded
		// instead of hanging until run end. The branch is taken here at setup
		// so the default path stays free of per-op timers.
		return func() error {
			callCtx, cancelCall := context.WithTimeout(ctx, opts.CallTimeout)
			defer cancelCall()
			return call(callCtx)
		}
	}
	return benchkit.MeasureLatency(ctx, opts, configs, setup)
}

// runSymmetricMulticast benchmarks multicast throughput and server-side
// one-way latency in a symmetric topology. Each server sends Multicast RPCs
// to its outbound peers; the server side measures latency via TimedMsg.SendTime.
// Throughput is reported as total sends per second across all servers.
func runSymmetricMulticast(ctx context.Context, peers peerConfig, opts benchkit.Options) (*benchkit.Result, error) {
	ctx, cancel := benchkit.BenchContext(ctx, opts)
	defer cancel()

	payload := make([]byte, opts.Payload)
	// The receiving server buckets samples per sender, using the sender ID
	// that gorums reports, and corrects them by that sender's estimated clock
	// offset (see [benchkit.EstimateOffsets] and
	// [benchkit.Stats.GetResultCorrected]).
	newMsg := func() *TimedMsg {
		return TimedMsg_builder{SendTime: time.Now().UnixNano(), Payload: payload}.Build()
	}

	// resetServerStats clears and restarts every local server's counters so the
	// next phase measures from zero; used before the connection flush and again
	// at the start of the measurement window. The stats mode carries opts.StatsMode
	// so a symmetric server-measured run honors -stats-mode like the coordinator
	// path does via the Start RPC.
	resetServerStats := func() {
		for _, p := range peers {
			p.Control().Reset(opts.StatsMode)
		}
	}

	resetServerStats()
	if err := flushSymmetricOutbound(ctx, peers); err != nil && !anyPeerFinished(peers) {
		return nil, err
	}

	var totalSent atomic.Int64
	var elapsed time.Duration

	// One send target per server; the receiving server buckets and
	// clock-corrects samples by the sender's node ID.
	sends := make([]func() error, len(peers))
	for i, p := range peers {
		client := NewBenchmarkClient(p.Server().PeerConfig())
		sends[i] = func() error {
			if err := client.Multicast(ctx, newMsg()).Async().Wait(); err != nil {
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
	latencyStats := make([]*benchkit.Stats, len(peers))
	for i, p := range peers {
		latencyStats[i] = p.Control().Stats()
	}
	m.TrackLatency(latencyStats...)

	estimate := func() ([]map[uint32]int64, error) {
		off, err := estimateAllOffsets(ctx, peers)
		if err != nil && anyPeerFinished(peers) {
			// Peers finished and exited before this straggler could sync clocks;
			// the incomplete offset set is the expected end of the run, not a
			// fault. estimateAllOffsets returns nil on error, so hand back one
			// empty offset map per server to keep buildReplies' per-server
			// indexing in bounds; AverageOffsets tolerates the empty side, so
			// the samples are left uncorrected rather than failing the run.
			return make([]map[uint32]int64, len(peers)), nil
		}
		return off, err
	}
	measure := func() error {
		// Reset server stats so the measurement phase counts from zero.
		resetServerStats()
		startTime := time.Now()
		if err := window(); err != nil && !anyPeerFinished(peers) {
			return err
		}
		elapsed = time.Since(startTime)
		if err := flushSymmetricOutbound(ctx, peers); err != nil && !anyPeerFinished(peers) {
			return err
		}
		return nil
	}
	buildReplies := func(before, after []map[uint32]int64) (map[uint32]*benchkit.Result, error) {
		for _, p := range peers {
			p.Control().Stats().End()
		}
		// Aggregate server-side latency samples across all local servers,
		// correcting each sample by its sender's averaged clock offset.
		replies := make(map[uint32]*benchkit.Result, len(peers))
		for i, p := range peers {
			benchkit.LogOffsets(p.String(), before[i], after[i])
			offsets := benchkit.AverageOffsets(before[i], after[i])
			replies[uint32(i+1)] = p.Control().Stats().GetResultCorrected(offsets)
		}
		return replies, nil
	}
	r, err := benchkit.RunOffsetCorrected(estimate, measure, buildReplies)
	if err != nil {
		m.Abandon()
		return nil, err
	}
	if err := m.Attach(r); err != nil {
		return nil, err
	}

	// Override TotalOps and Throughput with client-side send counts, which
	// are the authoritative measure of work done. Server receives are N times
	// sends in this topology, so server TotalOps would overcount.
	n := uint64(totalSent.Load())
	r.SetClientTotals(n, elapsed)
	r.SetPerOpMemoryFromServerStats(n)
	return r, nil
}
