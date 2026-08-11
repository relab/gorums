// Package benchmark drives Gorums quorum calls against a configuration of
// server nodes to measure their performance.
//
// It builds on [github.com/relab/gorums/benchkit] to run configurable load
// against local or remote targets, including the symmetric peer-to-peer setup
// where every server also calls the others, and reports latency and throughput.
package benchmark

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/relab/gorums/benchkit"
	"golang.org/x/sync/errgroup"
)

// BenchTarget carries the targets for benchmark execution. Set Config for
// traditional (coordinator→servers) benchmarks and Symmetric for peer-to-peer
// benchmarks; either or both may be set. Benchmarks for unset targets are
// omitted from GetBenchmarks.
type BenchTarget struct {
	Config    Config
	Symmetric *SymmetricTarget
}

// asyncQCFunc issues one async quorum call and returns its future.
type asyncQCFunc func(*ConfigContext, *Echo, int) AsyncEcho

// ErrAsyncQCRampUnsupported is returned by runAsyncQCBenchmark when rate
// ramping (-rate-step/-rate-step-max) is requested; AsyncQuorumCall does not
// support it.
var ErrAsyncQCRampUnsupported = errors.New("rate ramping is not supported by AsyncQuorumCall")

// asyncQCComplete processes the outcome of one async quorum call and reports
// whether the run should continue. [benchkit.ErrRunOver] ends it, recording the
// op neither as a success nor as a failure; any other error is counted via
// [benchkit.Measurement.RecordError] and the run continues, so a saturating
// workload degrades gracefully instead of aborting on the first failure; a nil
// error records elapsed as the round-trip latency.
func asyncQCComplete(err error, elapsed time.Duration, m *benchkit.Measurement) (continueRun bool) {
	switch {
	case errors.Is(err, benchkit.ErrRunOver):
		return false
	case err != nil:
		m.RecordError()
	default:
		m.Stats.AddLatency(elapsed)
	}
	return true
}

// runAsyncQCBenchmark drives the async quorum-call lifecycle: opts.Workers
// dispatchers keep up to MaxAsync calls in flight, so it is a custom
// [benchkit.Bench].Run closure built on benchkit's primitives instead of
// [benchkit.ClientMeasured]. A failed call is counted without aborting the run,
// [benchkit.ErrRunOver] ends it cleanly, and a paced run that falls behind its
// offered-load schedule warns on stderr.
func runAsyncQCBenchmark(opts benchkit.Options, config Config, f asyncQCFunc) (*benchkit.Result, error) {
	if opts.RateStep > 0 || opts.RateStepMax > 0 {
		return nil, fmt.Errorf("benchmark %q: %w", opts.BenchName, ErrAsyncQCRampUnsupported)
	}
	ctx, cancel := benchkit.BenchContext(opts)
	defer cancel()
	cfgCtx := config.Context(ctx)
	msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
	var g errgroup.Group
	var sent atomic.Uint64
	offered := opts.OfferedOps()

	if err := benchkit.StartRemote(cfgCtx, opts); err != nil {
		return nil, err
	}

	m := benchkit.StartMeasurement(opts)
	measureStart := time.Now()
	endTime := measureStart.Add(opts.Duration)
	// gate paces new sends to opts.Rate across all dispatching goroutines; nil
	// when unlimited, so a shared gate replaces a per-worker pacer.
	gate := benchkit.NewRatedGate(opts.Rate, measureStart)
	// inFlight holds one token per dispatched call, so acquiring a token is
	// what bounds concurrency at MaxAsync. A counter compared before dispatch
	// would not: the comparison and the increment are separate steps, and every
	// dispatcher can pass the same comparison before any of them increments.
	inFlight := make(chan struct{}, max(opts.MaxAsync, 1))
	var runOver atomic.Bool
	dispatch := func() error {
		for !time.Now().After(endTime) && ctx.Err() == nil && !runOver.Load() {
			select {
			case inFlight <- struct{}{}:
			case <-ctx.Done():
				return nil
			}
			if !gate.Wait(ctx) {
				<-inFlight
				return nil
			}
			if offered > 0 {
				sent.Add(1)
			}
			start := time.Now()
			fut := f(cfgCtx, msg, opts.QuorumSize)
			// The completion goroutine times the call and does nothing else.
			// Dispatching from here instead would put this goroutine's own work
			// between the call completing and the clock being read, so the
			// harness's scheduling would land inside the measurement.
			g.Go(func() error {
				_, err := fut.Get()
				elapsed := time.Since(start)
				<-inFlight
				if !asyncQCComplete(err, elapsed, m) {
					runOver.Store(true)
				}
				return nil
			})
		}
		return nil
	}

	for range opts.Workers {
		g.Go(dispatch)
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if warning := benchkit.PaceWarning(sent.Load(), offered); warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}

	result := m.Finish()
	if _, err := benchkit.StopRemote(cfgCtx, opts, result); err != nil {
		return nil, err
	}

	return result, nil
}

// asyncSends bounds how many one-way sends a benchmark keeps outstanding, so a
// one-way benchmark can pipeline the way an Async caller does. It is safe for
// concurrent use by the workers sharing one benchmark closure.
type asyncSend interface {
	Wait() error
}

type asyncSends struct {
	handles chan asyncSend
	slots   chan struct{}
}

// newAsyncSends bounds the outstanding sends at depth, the benchmark's
// -max-async. A depth below one is raised to one.
func newAsyncSends(depth int) *asyncSends {
	depth = max(depth, 1)
	return &asyncSends{
		handles: make(chan asyncSend, depth),
		slots:   make(chan struct{}, depth),
	}
}

// dispatch reserves outstanding capacity before invoking send. Once depth
// sends are outstanding, it reaps the oldest to make room. An error belongs to
// that earlier send, so the pending send is not dispatched after the run has
// already failed.
func (a *asyncSends) dispatch(send func() asyncSend) error {
	for {
		select {
		case a.slots <- struct{}{}:
			a.handles <- send()
			return nil
		default:
		}

		oldest := <-a.handles
		err := oldest.Wait()
		<-a.slots
		if err != nil {
			return err
		}
	}
}

// drain collects every send still outstanding and reports the first failure.
// It runs after the send window so the servers observe the whole pipeline.
func (a *asyncSends) drain() error {
	var firstErr error
	for {
		select {
		case h := <-a.handles:
			if err := h.Wait(); err != nil && firstErr == nil {
				firstErr = err
			}
			<-a.slots
		default:
			return firstErr
		}
	}
}

// benchTargetNeeds identifies which BenchTarget field a benchDesc's build
// function requires, so GetBenchmarks can filter benchDescs by what the
// caller's target actually provides.
type benchTargetNeeds int

const (
	needsConfig    benchTargetNeeds = iota // requires a non-nil Config (traditional coordinator→servers)
	needsSymmetric                         // requires a non-nil *SymmetricTarget (peer-to-peer)
)

// benchDesc is the single source of truth for one benchmark: its name and
// description (used by -list via [BenchmarkDescriptions]) and how to build
// its runnable closure (used by [GetBenchmarks]).
type benchDesc struct {
	Name        string
	Description string
	Needs       benchTargetNeeds
	build       func(cfg Config, sym *SymmetricTarget) func(benchkit.Options) (*benchkit.Result, error)
}

// benchDescs lists every known benchmark. Traditional benchmarks (needsConfig)
// receive cfg, which [GetBenchmarks] derives from t.Config or, when unset,
// from the symmetric target's server 0 outbound config; peer-to-peer
// benchmarks (needsSymmetric) receive sym.
var benchDescs = []benchDesc{
	{
		Name:        "QuorumCall",
		Description: "NodeStream based quorum call implementation with FIFO ordering",
		Needs:       needsConfig,
		build: func(cfg Config, _ *SymmetricTarget) func(benchkit.Options) (*benchkit.Result, error) {
			return benchkit.ClientMeasured(cfg, func(opts benchkit.Options, cc *ConfigContext) func() error {
				msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
				if opts.CallTimeout <= 0 {
					return func() error {
						_, err := QuorumCall(cc, msg).Threshold(opts.QuorumSize)
						return err
					}
				}
				// With -call-timeout, each call carries its own deadline so a
				// call stalled behind an unresponsive peer fails with
				// DeadlineExceeded instead of hanging until run end.
				callCfg := cc.Config()
				return func() error {
					callCtx, cancel := context.WithTimeout(cc, opts.CallTimeout)
					defer cancel()
					_, err := QuorumCall(callCfg.Context(callCtx), msg).Threshold(opts.QuorumSize)
					return err
				}
			})
		},
	},
	{
		Name:        "AsyncQuorumCall",
		Description: "NodeStream based async quorum call implementation with FIFO ordering",
		Needs:       needsConfig,
		build: func(cfg Config, _ *SymmetricTarget) func(benchkit.Options) (*benchkit.Result, error) {
			return func(opts benchkit.Options) (*benchkit.Result, error) {
				return runAsyncQCBenchmark(opts, cfg, func(ctx *ConfigContext, in *Echo, quorumSize int) AsyncEcho {
					return QuorumCall(ctx, in).AsyncThreshold(quorumSize)
				})
			}
		},
	},
	{
		Name:        "SlowServer",
		Description: "Quorum Call with a 10ms processing time on the server",
		Needs:       needsConfig,
		build: func(cfg Config, _ *SymmetricTarget) func(benchkit.Options) (*benchkit.Result, error) {
			return benchkit.ClientMeasured(cfg, func(opts benchkit.Options, cc *ConfigContext) func() error {
				msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
				return func() error {
					_, err := SlowServer(cc, msg).Threshold(opts.QuorumSize)
					return err
				}
			})
		},
	},
	{
		Name:        "Multicast",
		Description: "NodeStream based multicast implementation (servers measure latency and throughput)",
		Needs:       needsConfig,
		build: func(cfg Config, _ *SymmetricTarget) func(benchkit.Options) (*benchkit.Result, error) {
			return benchkit.ServerMeasured(cfg, func(opts benchkit.Options, cc *ConfigContext) func() error {
				payload := make([]byte, opts.Payload)
				return func() error {
					msg := TimedMsg_builder{SendTime: time.Now().UnixNano(), Payload: payload}.Build()
					return Multicast(cc, msg).Send()
				}
			})
		},
	},
	{
		Name:        "AsyncMulticast",
		Description: "Multicast pipelined with Async, up to -max-async sends outstanding (servers measure latency and throughput)",
		Needs:       needsConfig,
		build: func(cfg Config, _ *SymmetricTarget) func(benchkit.Options) (*benchkit.Result, error) {
			// outstanding is rebound by setup on every run and drained by the
			// quiesce hook after that run's send window; both run on the run
			// goroutine, in that order.
			var outstanding *asyncSends
			return benchkit.ServerMeasured(cfg,
				func(opts benchkit.Options, cc *ConfigContext) func() error {
					payload := make([]byte, opts.Payload)
					outstanding = newAsyncSends(opts.MaxAsync)
					return func() error {
						msg := TimedMsg_builder{SendTime: time.Now().UnixNano(), Payload: payload}.Build()
						return outstanding.dispatch(func() asyncSend { return Multicast(cc, msg).Async() })
					}
				},
				benchkit.WithQuiesce(func(context.Context) error {
					return outstanding.drain()
				}))
		},
	},
	{
		Name:        "SymmetricQuorumCall",
		Description: "Peer-to-peer quorum call benchmark; each node is both client and server",
		Needs:       needsSymmetric,
		build: func(_ Config, sym *SymmetricTarget) func(benchkit.Options) (*benchkit.Result, error) {
			return func(opts benchkit.Options) (*benchkit.Result, error) {
				return runSymmetricQuorumCall(sym, opts)
			}
		},
	},
	{
		Name:        "SymmetricMulticast",
		Description: "Peer-to-peer multicast benchmark; servers measure one-way latency",
		Needs:       needsSymmetric,
		build: func(_ Config, sym *SymmetricTarget) func(benchkit.Options) (*benchkit.Result, error) {
			return func(opts benchkit.Options) (*benchkit.Result, error) {
				return runSymmetricMulticast(sym, opts)
			}
		},
	},
}

// BenchmarkDescriptions returns name and description for every known
// benchmark, regardless of which targets are available. Used by -list.
func BenchmarkDescriptions() []benchkit.Bench {
	descs := make([]benchkit.Bench, len(benchDescs))
	for i, d := range benchDescs {
		descs[i] = benchkit.Bench{Name: d.Name, Description: d.Description}
	}
	return descs
}

// GetBenchmarks returns runnable benchmarks for the given targets. Traditional
// (needsConfig) benchmarks are included when t.Config is set, or when
// t.Symmetric is a single-process local target, in which case server 0's
// outbound config serves as the Config. They are excluded for a distributed
// (multi-process) symmetric target: every node there runs the same binary, so
// if more than one selected a needsConfig benchmark, each would issue its own
// Control.Start/Stop against the same peer group concurrently, resetting and
// reading every other node's Stats window mid-run. Symmetric benchmarks are
// included whenever t.Symmetric is set, local or distributed.
func GetBenchmarks(t BenchTarget) []benchkit.Bench {
	cfg := t.Config
	if cfg == nil && t.Symmetric != nil && t.Symmetric.selfAddr == "" && len(t.Symmetric.servers) > 0 {
		cfg = t.Symmetric.servers[0].PeerConfig()
	}
	var m []benchkit.Bench
	for _, d := range benchDescs {
		switch d.Needs {
		case needsConfig:
			if cfg == nil {
				continue
			}
		case needsSymmetric:
			if t.Symmetric == nil {
				continue
			}
		}
		m = append(m, benchkit.Bench{
			Name:        d.Name,
			Description: d.Description,
			Run:         d.build(cfg, t.Symmetric),
		})
	}
	return m
}

// RunBenchmarks runs all the benchmarks that match the given regex with the
// given options against the target, delegating selection, the per-benchmark
// metadata, and ordering to the benchkit harness.
func RunBenchmarks(benchRegex *regexp.Regexp, options benchkit.Options, t BenchTarget) ([]*benchkit.Result, error) {
	return benchkit.Run(benchRegex, options, GetBenchmarks(t))
}
