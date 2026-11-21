// Package benchmark drives Gorums quorum calls against a configuration of
// server nodes to measure their performance.
//
// It builds on [benchkit] to run configurable load
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

// Target supplies inputs for local or coordinator benchmark execution.
// Config enables traditional benchmarks. Symmetric holds the local peers
// for symmetric benchmarks and supplies the first peer's outbound config
// for traditional benchmarks when Config is nil.
type Target struct {
	Config    Config
	Symmetric []*benchkit.Peer
}

// asyncQCFunc starts one QuorumCall on the configuration of client and returns
// a function that waits for quorumSize replies and returns the call's error.
type asyncQCFunc func(ctx context.Context, client BenchmarkClient, in *Echo, quorumSize int) (wait func() error)

// asyncThreshold is the [asyncQCFunc] of the AsyncQuorumCall benchmark. It
// dispatches the call with [gorums.Call.Async], so the requests are sent before
// it returns, and the returned function collects the replies with
// [gorums.Responses.Threshold].
func asyncThreshold(ctx context.Context, client BenchmarkClient, in *Echo, quorumSize int) func() error {
	responses := client.QuorumCall(ctx, in).Async()
	return func() error {
		_, err := responses.Threshold(quorumSize)
		return err
	}
}

// errAsyncQCRampUnsupported is returned by runAsyncQCBenchmark when rate
// ramping (-rate-step/-rate-step-max) is requested; AsyncQuorumCall does not
// support it.
var errAsyncQCRampUnsupported = errors.New("rate ramping is not supported by AsyncQuorumCall")

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
func runAsyncQCBenchmark(ctx context.Context, opts benchkit.Options, config Config, f asyncQCFunc) (*benchkit.Result, error) {
	if opts.RateStep > 0 || opts.RateStepMax > 0 {
		return nil, fmt.Errorf("benchmark %q: %w", opts.BenchName, errAsyncQCRampUnsupported)
	}
	ctx, cancel := benchkit.BenchContext(ctx, opts)
	defer cancel()
	client := NewBenchmarkClient(config)
	msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
	var g errgroup.Group
	var sent atomic.Uint64
	offered := opts.OfferedOps()

	if err := benchkit.StartRemote(ctx, config, opts); err != nil {
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
			wait := f(ctx, client, msg, opts.QuorumSize)
			// The completion goroutine times the call and does nothing else.
			// Dispatching from here instead would put this goroutine's own work
			// between the call completing and the clock being read, so the
			// harness's scheduling would land inside the measurement.
			g.Go(func() error {
				err := wait()
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
	if warning := benchkit.PaceWarning("", sent.Load(), offered); warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}

	result := m.Finish()
	if _, err := benchkit.StopRemote(ctx, config, opts, result); err != nil {
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
// sends are outstanding, it reaps the oldest to make room, or takes the slot
// another worker's reap frees first. An error belongs to that earlier send, so
// the pending send is not dispatched after the run has already failed.
func (a *asyncSends) dispatch(send func() asyncSend) error {
	for {
		select {
		case a.slots <- struct{}{}:
			a.handles <- send()
			return nil
		default:
		}

		// Every send may already be held by other reaping workers. A reap
		// that fails frees its slot without queueing a new send, so wait for
		// that slot as well as for a send to reap.
		select {
		case a.slots <- struct{}{}:
			a.handles <- send()
			return nil
		case oldest := <-a.handles:
			err := oldest.Wait()
			<-a.slots
			if err != nil {
				return err
			}
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

type runFunc = func(context.Context, benchkit.Options) (*benchkit.Result, error)

// benchDesc defines a benchmark's listing metadata and builds its runner
// from a target of type T.
type benchDesc[T any] struct {
	Name        string
	Description string
	build       func(T) runFunc
}

var configBenchDescs = []benchDesc[Config]{
	{
		Name:        "QuorumCall",
		Description: "NodeStream based quorum call implementation with FIFO ordering",
		build: func(cfg Config) runFunc {
			return benchkit.ClientMeasured(cfg, func(ctx context.Context, cfg Config, opts benchkit.Options) func() error {
				client := NewBenchmarkClient(cfg)
				msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
				if opts.CallTimeout <= 0 {
					return func() error {
						_, err := client.QuorumCall(ctx, msg).Threshold(opts.QuorumSize)
						return err
					}
				}
				// With -call-timeout, each call carries its own deadline so a
				// call stalled behind an unresponsive peer fails with
				// DeadlineExceeded instead of hanging until run end.
				return func() error {
					callCtx, cancel := context.WithTimeout(ctx, opts.CallTimeout)
					defer cancel()
					_, err := client.QuorumCall(callCtx, msg).Threshold(opts.QuorumSize)
					return err
				}
			})
		},
	},
	{
		Name:        "AsyncQuorumCall",
		Description: "NodeStream based async quorum call implementation with FIFO ordering",
		build: func(cfg Config) runFunc {
			return func(ctx context.Context, opts benchkit.Options) (*benchkit.Result, error) {
				return runAsyncQCBenchmark(ctx, opts, cfg, asyncThreshold)
			}
		},
	},
	{
		Name:        "SlowServer",
		Description: "Quorum Call with a 10ms processing time on the server",
		build: func(cfg Config) runFunc {
			return benchkit.ClientMeasured(cfg, func(ctx context.Context, cfg Config, opts benchkit.Options) func() error {
				client := NewBenchmarkClient(cfg)
				msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
				return func() error {
					_, err := client.SlowServer(ctx, msg).Threshold(opts.QuorumSize)
					return err
				}
			})
		},
	},
	{
		Name:        "Multicast",
		Description: "NodeStream based multicast implementation (servers measure latency and throughput)",
		build: func(cfg Config) runFunc {
			return benchkit.ServerMeasured(cfg, func(ctx context.Context, cfg Config, opts benchkit.Options) func() error {
				client := NewBenchmarkClient(cfg)
				payload := make([]byte, opts.Payload)
				return func() error {
					msg := TimedMsg_builder{SendTime: time.Now().UnixNano(), Payload: payload}.Build()
					return client.Multicast(ctx, msg).Async().Wait()
				}
			})
		},
	},
	{
		Name:        "AsyncMulticast",
		Description: "Multicast pipelined with Async, up to -max-async sends outstanding (servers measure latency and throughput)",
		build: func(cfg Config) runFunc {
			// outstanding is rebound by setup on every run and drained by the
			// quiesce hook after that run's send window; both run on the run
			// goroutine, in that order.
			var outstanding *asyncSends
			return benchkit.ServerMeasured(cfg,
				func(ctx context.Context, cfg Config, opts benchkit.Options) func() error {
					client := NewBenchmarkClient(cfg)
					payload := make([]byte, opts.Payload)
					outstanding = newAsyncSends(opts.MaxAsync)
					return func() error {
						msg := TimedMsg_builder{SendTime: time.Now().UnixNano(), Payload: payload}.Build()
						return outstanding.dispatch(func() asyncSend { return client.Multicast(ctx, msg).Async() })
					}
				},
				benchkit.WithQuiesce(func(context.Context) error {
					return outstanding.drain()
				}))
		},
	},
}

var symmetricBenchDescs = []benchDesc[peerConfig]{
	{
		Name:        "SymmetricQuorumCall",
		Description: "Peer-to-peer quorum call benchmark; each node is both client and server",
		build: func(peers peerConfig) runFunc {
			return func(ctx context.Context, opts benchkit.Options) (*benchkit.Result, error) {
				return runSymmetricQuorumCall(ctx, peers, opts)
			}
		},
	},
	{
		Name:        "SymmetricMulticast",
		Description: "Peer-to-peer multicast benchmark; servers measure one-way latency",
		build: func(peers peerConfig) runFunc {
			return func(ctx context.Context, opts benchkit.Options) (*benchkit.Result, error) {
				return runSymmetricMulticast(ctx, peers, opts)
			}
		},
	},
}

func (d benchDesc[T]) description() benchkit.Bench {
	return benchkit.Bench{Name: d.Name, Description: d.Description}
}

func describeBenchmarks[T any](descs []benchDesc[T]) []benchkit.Bench {
	benches := make([]benchkit.Bench, len(descs))
	for i, d := range descs {
		benches[i] = d.description()
	}
	return benches
}

func bindBenchmarks[T any](descs []benchDesc[T], target T) []benchkit.Bench {
	benches := make([]benchkit.Bench, len(descs))
	for i, d := range descs {
		benches[i] = d.description()
		benches[i].Run = d.build(target)
	}
	return benches
}

// Descriptions returns the name and description of every benchmark in run order.
func Descriptions() []benchkit.Bench {
	return append(describeBenchmarks(configBenchDescs), describeBenchmarks(symmetricBenchDescs)...)
}

// DistributedDescriptions returns the names and descriptions of the symmetric
// benchmarks available to each peer in a distributed run.
func DistributedDescriptions() []benchkit.Bench {
	return describeBenchmarks(symmetricBenchDescs)
}

func benchmarks(t Target) []benchkit.Bench {
	cfg := t.Config
	if cfg == nil && len(t.Symmetric) > 0 {
		cfg = t.Symmetric[0].Server().PeerConfig()
	}
	var benches []benchkit.Bench
	if cfg != nil {
		benches = bindBenchmarks(configBenchDescs, cfg)
	}
	if len(t.Symmetric) > 0 {
		benches = append(benches, bindBenchmarks(symmetricBenchDescs, t.Symmetric)...)
	}
	return benches
}

// Run runs all the benchmarks that match the given regex with the
// given options against the target, delegating selection, the per-benchmark
// metadata, and ordering to the benchkit harness (see [benchkit.Run]).
func Run(ctx context.Context, benchRegex *regexp.Regexp, t Target, opts benchkit.Options) ([]*benchkit.Result, error) {
	return benchkit.Run(ctx, benchRegex, benchmarks(t), opts)
}
