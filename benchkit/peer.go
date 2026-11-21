package benchkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"runtime/pprof"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/relab/gorums"
)

// Peer is one gorums server of a peer-to-peer benchmark run, together with
// the [Control] service registered on it. Every peer of the run calls every
// other peer, and the Control service of each peer answers the readiness and
// Done calls of the others.
type Peer struct {
	srv  *gorums.Server
	ctrl *Control
}

// NewPeer returns a Peer for srv, which must have peers ([gorums.WithPeers])
// and must not serve yet. NewPeer registers a new [Control] on srv and arms its
// Done tracking for the peers of srv.
func NewPeer(srv *gorums.Server) *Peer {
	ctrl := NewControl()
	RegisterControlServer(srv, ctrl)
	ctrl.ArmDone(srv.PeerConfig().Size())
	return &Peer{srv: srv, ctrl: ctrl}
}

// ListenPeer returns the Peer for node self of a distributed run over the
// addresses in remotes, which must include self. Every node of the run sorts
// remotes in the same way, so node i of the sorted list has node ID i+1 on
// every machine. The server listens on the port of self on every interface,
// with the server options of opts, and dials its peers with dialOpts. It does
// not serve yet.
func ListenPeer(self string, remotes []string, opts Options, dialOpts ...gorums.DialOption) (*Peer, error) {
	sorted := slices.Clone(remotes)
	slices.Sort(sorted)
	idx := slices.Index(sorted, self)
	if idx < 0 {
		return nil, fmt.Errorf("self address %q not found in peer list", self)
	}
	// Listen on the wildcard address rather than self: listening on a hostname
	// binds the single IP the local resolver returns for it, and hosts that
	// follow the Debian convention resolve their own name to the loopback
	// address 127.0.1.1, which no peer can reach.
	_, port, err := net.SplitHostPort(self)
	if err != nil {
		return nil, fmt.Errorf("invalid self address %q: %w", self, err)
	}
	srv := gorums.NewServer(append([]gorums.ServerOption{
		gorums.WithAddr(net.JoinHostPort("", port)),
		gorums.WithPeers(uint32(idx+1), gorums.WithNodeList(sorted), dialOpts...),
	}, opts.ServerOptions()...)...)
	return NewPeer(srv), nil
}

// Server returns the gorums server of p.
func (p *Peer) Server() *gorums.Server { return p.srv }

// Control returns the [Control] service registered on the server of p.
func (p *Peer) Control() *Control { return p.ctrl }

// String describes p as "node ID (address)", with the address of p in its
// peer list.
func (p *Peer) String() string {
	return fmt.Sprintf("node %d (%s)", p.srv.NodeID(), p.selfAddr())
}

// selfAddr returns the address of p in its peer list.
func (p *Peer) selfAddr() string {
	id := p.srv.NodeID()
	for _, node := range p.srv.PeerConfig() {
		if node.ID() == id {
			return node.Address()
		}
	}
	return p.srv.Addr()
}

// AwaitReady returns nil once every peer of p reports a connected stream to
// each of its peers, as [AwaitMesh] checks it. The server of p must serve, and
// every peer must run a [Control] service. A peer that does not answer fails
// the check within the stall bound of [AwaitMesh]. On an error other than a
// canceled ctx, AwaitReady also writes a network self-diagnosis of this host to
// stderr: whether the listener is reachable at the address of p, and a
// goroutine dump.
func (p *Peer) AwaitReady(ctx context.Context) error {
	if err := AwaitMesh(ctx, p.srv.PeerConfig()); err != nil {
		if !errors.Is(err, context.Canceled) {
			diagnoseStall(diagWriter, p.srv, p.selfAddr())
		}
		return fmt.Errorf("%s: peers not ready: %w", p, err)
	}
	Logf("[ready] %s: listening on %s, every peer connected\n", p, p.srv.Addr())
	return nil
}

// FlushOutbound returns nil once every peer of p has answered a Control call
// from p. A peer handles the requests on one stream in order, so its answer
// means that it has handled every one-way request that p sent it earlier. The
// error names each peer with a recorded stream or call error.
func (p *Peer) FlushOutbound(ctx context.Context) error {
	out := p.srv.PeerConfig()
	_, err := NewControlClient(out).PeerView(ctx, &PeerViewRequest{}).Threshold(out.Size())
	if err != nil {
		if unresponsive := unresponsivePeers(out); unresponsive != "" {
			return fmt.Errorf("%w; unresponsive peers: %s", err, unresponsive)
		}
	}
	return err
}

// unresponsivePeers names the nodes of cfg with a recorded stream or call
// error, formatted as "node ID (address): error". It returns "" when every
// node is healthy.
func unresponsivePeers(cfg Config) string {
	var details []string
	for _, node := range cfg {
		if node.LastErr() != nil {
			details = append(details, node.Detail())
		}
	}
	return strings.Join(details, ", ")
}

// SignalDone tells every peer of p that p has finished its own benchmark work
// and will send no further calls. A peer waits for these signals in
// [Peer.AwaitPeersDone]. The signal is advisory, so SignalDone logs a send
// error and returns.
//
// SignalDone returns at the latest after [ExitGrace] of the peer count, the
// time a peer waits for the signal. It returns once the signal is queued on
// every stream. [Peer.Stop] keeps the streams sending while the peers end
// theirs, so a queued signal normally reaches each peer before the process
// exits.
func (p *Peer) SignalDone(ctx context.Context) {
	out := p.srv.PeerConfig()
	ctx, cancel := context.WithTimeout(ctx, ExitGrace(out.Size()))
	defer cancel()
	if err := NewControlClient(out).Done(ctx, &DoneRequest{}).Async().Wait(); err != nil {
		Logf("Done signal from %s: %v\n", p, err)
	}
}

// AwaitPeersDone returns nil once every peer of p has signaled Done (see
// [Peer.SignalDone]). When ctx ends first, it returns an error that names the
// peers without a signal and wraps ctx.Err(). A peer sends its Done signal
// after its own calls on the same stream, so a nil return means that p has
// received every request that its peers sent before their signals.
func (p *Peer) AwaitPeersDone(ctx context.Context) error {
	select {
	case <-p.ctrl.DoneCh():
		return nil
	case <-ctx.Done():
		return fmt.Errorf("no Done signal from node(s) %v: %w", p.ctrl.MissingDone(), ctx.Err())
	}
}

// Stop stops the server of p so that a one-way request queued just before the
// stop, such as the Done signal of [Peer.SignalDone], normally reaches its
// peer before the process exits. Stop returns within one second.
func (p *Peer) Stop() {
	stopFlushed(p.srv, stopFlushBound)
}

// readyTimeout bounds the readiness check of [Peer.Run]. A large run that
// still makes progress needs this much time; a dead peer fails the check
// within the stall bound of [AwaitMesh].
const readyTimeout = 2 * time.Minute

// Workload holds the hooks of one workload for [Peer.Run].
type Workload struct {
	// Setup registers the services of the workload on the server of the peer
	// and returns its benchmarks. Setup runs before the server serves.
	Setup func(*Peer) ([]Bench, error)
	// Final, if set, runs after every peer has signaled Done, or after the exit
	// grace period, so it observes the requests of the peers on this server.
	// It can change the results before [Peer.Run] returns them.
	Final func(*Peer, []*Result) error
}

// Run runs the whole lifecycle of p for one benchmark run and returns the
// results of the benchmarks of w that match sel. Run calls w.Setup, serves,
// waits until the mesh of the run is complete (see [Peer.AwaitReady]), and runs
// the benchmarks with [Run]. It then signals Done, waits for the Done signals
// of its peers up to [ExitGrace] of the peer count, calls w.Final, and stops
// the server (see [Peer.Stop]). Run stops the server on every return, and
// returns at the first error of a step. When ctx ends during the wait for the
// Done signals, Run returns an error that wraps ctx.Err() and skips w.Final.
func (p *Peer) Run(ctx context.Context, sel *regexp.Regexp, w Workload, opts Options) ([]*Result, error) {
	defer p.Stop()
	benches, err := w.Setup(p)
	if err != nil {
		return nil, fmt.Errorf("%s: setup: %w", p, err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- p.srv.ListenAndServe() }()

	readyCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	err = p.AwaitReady(readyCtx)
	cancel()
	if err != nil {
		select {
		case serr := <-serveErr:
			return nil, errors.Join(err, fmt.Errorf("%s: serve: %w", p, serr))
		default:
			return nil, err
		}
	}

	results, err := Run(ctx, sel, benches, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}

	grace := ExitGrace(p.srv.PeerConfig().Size())
	Logf("[%s %s] benchmarks complete; signaling Done, waiting up to %v for peers\n", time.Now().Format(time.TimeOnly), p, grace)
	p.SignalDone(ctx)
	start := time.Now()
	doneCtx, cancel := context.WithTimeout(ctx, grace)
	err = p.AwaitPeersDone(doneCtx)
	cancel()
	switch {
	case err != nil && ctx.Err() != nil:
		return nil, fmt.Errorf("%s: %w", p, err)
	case err != nil:
		Logf("[%s %s] exit grace ended after %v: %v\n", time.Now().Format(time.TimeOnly), p, time.Since(start), err)
	default:
		Logf("[%s %s] all peers done after %v\n", time.Now().Format(time.TimeOnly), p, time.Since(start))
	}

	if w.Final != nil {
		if err := w.Final(p, results); err != nil {
			return nil, fmt.Errorf("%s: final: %w", p, err)
		}
	}
	return results, nil
}

// RunLocal runs n peers in this process, each with [Peer.Run], and returns
// the results of each peer, indexed by node ID - 1. The servers come from
// [gorums.NewLocalServers], with the server options of opts and then localOpts.
// n must be at least 2. RunLocal returns the errors of all peers, each naming
// its peer. An error in one peer cancels the context of the others.
func RunLocal(ctx context.Context, n int, sel *regexp.Regexp, w Workload, opts Options, localOpts ...gorums.LocalServerOption) ([][]*Result, error) {
	if n < 2 {
		return nil, fmt.Errorf("RunLocal needs at least 2 peers, got %d", n)
	}
	servers, stop, err := gorums.NewLocalServers(n,
		append([]gorums.LocalServerOption{gorums.WithLocalServerOptions(opts.ServerOptions()...)}, localOpts...)...)
	if err != nil {
		return nil, err
	}
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([][]*Result, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i, srv := range servers {
		p := NewPeer(srv)
		wg.Go(func() {
			results[i], errs[i] = p.Run(ctx, sel, w, opts)
			if errs[i] != nil {
				cancel()
			}
		})
	}
	wg.Wait()
	return results, errors.Join(errs...)
}

// stopFlushBound caps how long [Peer.Stop] waits for a graceful stop.
const stopFlushBound = time.Second

// stopFlushed stops srv so that a one-way request queued just before the stop
// reaches its peer. [gorums.Server.GracefulStop] first waits until the peers
// have ended the streams that they dialed to srv. The streams that srv dialed
// keep sending during that wait, which normally gives a request queued before
// the stop the time to leave the queue. GracefulStop then drains the streams
// that srv dialed, and a request still queued at that point fails with
// [gorums.ErrNodeClosed]. [gorums.Server.Stop] discards every queued request
// at once, so the last node to finish would exit without its peers seeing its
// Done, and they would wait out their grace period.
//
// A graceful stop also waits for running handlers and for slow peers, so after
// bound stopFlushed calls [gorums.Server.Stop] and returns without waiting for
// the graceful stop to end: Stop does not end in-process handlers that ignore
// their context, and the graceful stop waits for them. stopFlushed always ends
// with Stop, which also closes a listener that the server never served.
func stopFlushed(srv *gorums.Server, bound time.Duration) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.GracefulStop()
	}()
	select {
	case <-done:
	case <-time.After(bound):
	}
	srv.Stop()
}

// ExitGrace returns how long a peer of a run with numNodes nodes waits, after
// its own benchmark work, for the Done signals of its peers. Most runs finish
// within milliseconds of each other, so the signals arrive long before this
// bound. The bound matters when a signal is lost or the whole run is slow. It
// grows with the run size, because the skew between the nodes' start times
// grows with it, and it is at most 20 seconds.
func ExitGrace(numNodes int) time.Duration {
	const (
		base     = 3 * time.Second
		perNode  = 300 * time.Millisecond
		maxGrace = 20 * time.Second
	)
	return min(base+time.Duration(numNodes)*perNode, maxGrace)
}

// diagWriter receives the self-diagnosis of [Peer.AwaitReady]. It is a
// variable so tests can capture the diagnosis.
var diagWriter io.Writer = os.Stderr

// diagnoseStall writes a network self-diagnosis to w when the readiness check
// of [Peer.AwaitReady] fails. This process still runs on the affected host,
// so it can tell the known error classes apart:
//
//   - self-dial fails and the bound address differs from the advertised
//     address: the listener is bound to the wrong interface, because the host
//     resolves its own name to an unexpected address, such as a loopback
//     address.
//   - self-dial fails with matching addresses: a local firewall refuses new
//     connections, or the listener died.
//   - self-dial succeeds: the listener works from this host, so the blockage
//     is between the peers and this host (network filtering, peer-side state).
//
// The goroutine dump at the end shows whether the readiness calls stalled
// inside gorums rather than on the network.
func diagnoseStall(w io.Writer, srv *gorums.Server, selfAddr string) {
	fmt.Fprintf(w, "[diag] readiness stall: listener bound to %s; self peer-list address %s\n", srv.Addr(), selfAddr)
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
