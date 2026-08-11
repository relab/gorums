package benchkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/relab/gorums"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

// memSnapshot captures heap allocation counters for computing client-side
// per-op memory stats across a measurement window.
type memSnapshot struct {
	ms runtime.MemStats
}

// read records the current heap allocation counters.
func (m *memSnapshot) read() { runtime.ReadMemStats(&m.ms) }

// delta returns the allocations and bytes allocated per operation between m and
// end, or zero when totalOps is zero.
func (m memSnapshot) delta(end memSnapshot, totalOps uint64) (allocsPerOp, memPerOp uint64) {
	if totalOps == 0 {
		return 0, 0
	}
	return (end.ms.Mallocs - m.ms.Mallocs) / totalOps,
		(end.ms.TotalAlloc - m.ms.TotalAlloc) / totalOps
}

// benchSlack bounds the scheduling and shutdown slack on top of the run
// duration for a single benchmark invocation. A hung RPC is thus surfaced as a
// timeout rather than a hang.
const benchSlack = 30 * time.Second

// Options controls the parameters shared by every benchmark the harness runs.
type Options struct {
	Workers    int           // Number of concurrent worker goroutines
	Duration   time.Duration // Duration of benchmark
	MaxAsync   int           // Max async calls at once
	NumNodes   int           // Number of nodes to include in configuration
	Payload    int           // Size of message payload
	QuorumSize int           // Number of messages to wait for
	Rate       int           // Target sends per second per node; 0 means unlimited (saturate)
	Remote     bool          // Whether the servers are remote (true) or local (false)
	StatsMode  StatsMode     // Aggregate latency backing store; 0 = StatsMode_EXACT (default)
	Interval   time.Duration // Ticker interval for per-interval metrics; 0 = disabled
	BenchName  string        // Benchmark name, stamped by Run before invoking Bench.Run
	RateStep   int           // Rate increment per step during the ramp; 0 = disabled
	// RateStepMax is the maximum target rate during the ramp; 0 = disabled. The
	// duration of each ramp step is derived: Duration divided evenly across the
	// rate levels (rampSteps), so a ramp always spans exactly Duration and ends
	// exactly at RateStepMax.
	RateStepMax int
	StreamMode  string // Symmetric stream topology: "dual" (default) or "dedup"
	// CallTimeout is the per-call deadline for quorum-call workloads; 0 =
	// disabled. With a deadline, a call stalled behind an unresponsive peer
	// fails the run with DeadlineExceeded — an attributable error in the
	// manifest — instead of silently zeroing the node's throughput until the
	// run-scoped context expires.
	CallTimeout time.Duration
	// SendBuffer and RecvBuffer record the buffer capacities the run is
	// configured with, so results differing only by buffer size stay
	// distinguishable. Zero selects Gorums' own default for that buffer, the
	// same substitution [gorums.WithSendBufferSize] and [gorums.WithBufferSizes]
	// apply internally.
	SendBuffer uint
	RecvBuffer uint
}

// rampEnabled reports whether rate ramping is active: both ramp options
// (RateStep, RateStepMax) must be set.
func (o Options) rampEnabled() bool {
	return o.RateStep > 0 && o.RateStepMax > 0
}

// rampSteps returns the number of offered-load levels in a ramp: one per
// RateStep increment from the start rate up to and including RateStepMax, where
// a partial final increment still counts as a level. A start rate at or above
// RateStepMax yields a single level.
func (o Options) rampSteps() int {
	span := o.RateStepMax - o.startRate()
	if span <= 0 {
		return 1
	}
	return (span+o.RateStep-1)/o.RateStep + 1
}

// startRate returns the offered rate of the first measurement phase: Rate
// normally, but RateStep when ramping is enabled and Rate is unset, so a
// ramped run climbs from the first step instead of starting unlimited and
// dropping at the first transition.
func (o Options) startRate() int {
	if o.rampEnabled() && o.Rate <= 0 {
		return o.RateStep
	}
	return o.Rate
}

// StreamDedupOption returns the [gorums.WithStreamDedup] server option when the
// stream mode requests deduplication, or nil otherwise.
func (o Options) StreamDedupOption() gorums.ServerOption {
	if o.StreamMode == "dedup" {
		return gorums.WithStreamDedup()
	}
	return nil
}

// BufferSizesOption returns the [gorums.WithBufferSizes] server option carrying
// the run's configured buffer capacities.
func (o Options) BufferSizesOption() gorums.ServerOption {
	return gorums.WithBufferSizes(o.RecvBuffer, o.SendBuffer)
}

// ServerOptions returns the server options this run's configuration implies,
// with any that do not apply omitted.
func (o Options) ServerOptions() []gorums.ServerOption {
	var opts []gorums.ServerOption
	for _, opt := range []gorums.ServerOption{o.StreamDedupOption(), o.BufferSizesOption()} {
		if opt != nil {
			opts = append(opts, opt)
		}
	}
	return opts
}

// Validate checks the generic constraints every benchkit binary shares,
// independent of workload topology; topology-specific checks (e.g. node
// counts) belong to the caller. [Run] calls this before executing any
// benchmark.
func (o Options) Validate() error {
	switch {
	case o.Workers < 1:
		return fmt.Errorf("workers must be >= 1, got %d", o.Workers)
	case o.Duration <= 0:
		return fmt.Errorf("duration must be > 0, got %v", o.Duration)
	case o.Payload < 0:
		return fmt.Errorf("payload must be >= 0, got %d", o.Payload)
	case o.Rate < 0:
		return fmt.Errorf("rate must be >= 0, got %d", o.Rate)
	case o.Interval < 0:
		return fmt.Errorf("interval must be >= 0, got %v", o.Interval)
	case o.CallTimeout < 0:
		return fmt.Errorf("call timeout must be >= 0, got %v", o.CallTimeout)
	case o.RateStep < 0:
		return fmt.Errorf("rate step must be >= 0, got %d", o.RateStep)
	case o.RateStepMax < 0:
		return fmt.Errorf("rate step max must be >= 0, got %d", o.RateStepMax)
	case (o.RateStep > 0) != (o.RateStepMax > 0):
		return fmt.Errorf("rate-step and rate-step-max must both be set or both be zero, got rate-step=%d rate-step-max=%d", o.RateStep, o.RateStepMax)
	case o.rampEnabled() && o.RateStepMax < o.RateStep:
		return fmt.Errorf("rate-step-max (%d) must be >= rate-step (%d)", o.RateStepMax, o.RateStep)
	}
	switch o.StreamMode {
	case "", "dual", "dedup":
	default:
		return fmt.Errorf("invalid stream mode %q (want: dual or dedup)", o.StreamMode)
	}
	return nil
}

// Bench is a named, runnable benchmark. Run performs one measured campaign with
// the given options and returns this node's Result. The harness (Run) stamps the
// shared run metadata (name, node count, mode, duration, …) onto the returned
// Result, so a Bench.Run closure only fills in the measured fields.
type Bench struct {
	Name        string
	Description string
	Run         func(Options) (*Result, error)
}

// ListBenches writes a name/description listing of benches to w, one per
// line, aligned by tab.
func ListBenches(w io.Writer, benches []Bench) {
	tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	for _, b := range benches {
		fmt.Fprintf(tw, "%s:\t%s\n", b.Name, b.Description)
	}
	tw.Flush()
}

// BenchContext returns a context bounded by the benchmark's run duration plus a
// fixed slack, so a stuck RPC fails the benchmark instead of blocking
// indefinitely.
func BenchContext(opts Options) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opts.Duration+benchSlack)
}

// CollectReplies drains a ResponseSeq into a map keyed by node ID. It returns
// the joined errors observed from any node so that a single failing server is
// not silently dropped (as CollectAll does by storing the zero value) and so
// the caller sees every failure, not just the first.
func CollectReplies[T proto.Message](seq gorums.ResponseSeq[T]) (map[uint32]T, error) {
	replies := make(map[uint32]T)
	var errs []error
	for r := range seq {
		if r.Err != nil {
			errs = append(errs, fmt.Errorf("node %d: %w", r.NodeID, r.Err))
			continue
		}
		replies[r.NodeID] = r.Value
	}
	return replies, errors.Join(errs...)
}

// ErrRunOver is returned by a workload op to signal that the measurement run is
// over (e.g. a straggler's peers have already finished and closed their
// listeners): the worker that receives it stops cleanly, and the op is recorded
// neither as a success nor as a failure. RunPhase treats it as a clean stop
// rather than a fault, so MeasureLatency returns the samples gathered before
// the run ended instead of failing.
var ErrRunOver = errors.New("benchmark run over")

// RunPhase launches numG goroutines per doOp, each calling its doOp in a tight
// loop from start until endTime or until ctx is cancelled, collecting the first
// non-nil error. When rate > 0, the goroutines for a single doOp are paced to a
// combined rate sends per second (each worker takes an equal, staggered share);
// rate <= 0 saturates. A doOp returning ErrRunOver stops its worker cleanly
// without failing the phase. Passing more than one doOp fans the phase out
// across several independent send targets that run concurrently in one shared
// errgroup, each with its own pool of numG paced workers; this is how the
// symmetric runners drive every system at once. It is the shared measurement
// skeleton used by the benchmark runners.
func RunPhase(ctx context.Context, numG, rate int, start, endTime time.Time, doOps ...func() error) error {
	var g errgroup.Group
	for _, doOp := range doOps {
		for w := range numG {
			g.Go(func() error {
				p := NewPacer(rate, numG, w, start)
				// The loop checks ctx itself: a closed-loop (nil-pacer) worker
				// never blocks in Wait, so cancellation would otherwise go
				// unnoticed and the worker would spin until endTime.
				for time.Now().Before(endTime) && ctx.Err() == nil {
					if !p.Wait(ctx) {
						return nil
					}
					if err := doOp(); err != nil {
						if errors.Is(err, ErrRunOver) {
							return nil
						}
						return err
					}
				}
				return nil
			})
		}
	}
	return g.Wait()
}

// RunOffsetCorrected runs the server-measured-latency sequence: estimate peer
// clock offsets, run the measurement send, estimate offsets again, then build
// the per-node corrected results and aggregate them into one cluster-wide
// Result.
//
// estimate samples the per-peer clock offsets and is called once before and once
// after the measurement; the offset type O differs between callers (a single
// offset map coordinator-side, one map per system in symmetric mode), so it is a
// type parameter. measure brackets the send window tightly between the two
// estimate calls: it starts the run, runs the send loop, and stops the run,
// so neither client memory accounting nor the server-measured throughput
// window includes clock-offset estimation time. buildReplies is the
// correction step: it turns the before/after offsets and the replies measure
// collected into per-node corrected Results.
func RunOffsetCorrected[O any](
	estimate func() (O, error),
	measure func() error,
	buildReplies func(before, after O) (map[uint32]*Result, error),
) (*Result, error) {
	// Estimate offsets just before the measurement window; servers measure
	// one-way latency against their own clock, so samples carry this offset.
	before, err := estimate()
	if err != nil {
		return nil, err
	}
	if err := measure(); err != nil {
		return nil, err
	}
	// Estimate offsets again after the window; averaging before and after
	// tolerates small clock drift over the run.
	after, err := estimate()
	if err != nil {
		return nil, err
	}
	replies, err := buildReplies(before, after)
	if err != nil {
		return nil, err
	}
	return AggregateServerResults(replies)
}

// OfferedOps returns the number of sends the offered-load schedule asks of one
// send target over the run: rate × duration, summed per level when ramping. It
// returns 0 for an unlimited (closed-loop) run, which has no schedule.
func (o Options) OfferedOps() float64 {
	if o.rampEnabled() {
		numSteps := o.rampSteps()
		stepSec := (o.Duration / time.Duration(numSteps)).Seconds()
		rate := o.startRate()
		var total float64
		for range numSteps {
			total += float64(rate) * stepSec
			rate = min(rate+o.RateStep, o.RateStepMax)
		}
		return total
	}
	if o.Rate <= 0 {
		return 0
	}
	return float64(o.Rate) * o.Duration.Seconds()
}

// paceTolerance is the fraction of the offered sends below which a paced run
// is reported as having fallen behind its open-loop schedule.
const paceTolerance = 0.95

// PaceWarning returns a warning when a paced run attempted markedly fewer
// sends than its offered-load schedule asked for, and "" otherwise. Falling
// behind means the workers could not sustain the offered rate — sustaining
// rate R at per-op latency L needs roughly R × L in-flight ops — so the run
// degraded toward closed-loop saturation while the recorded rate markers
// still claim the offered load.
func PaceWarning(sent uint64, offered float64) string {
	if offered <= 0 || float64(sent) >= paceTolerance*offered {
		return ""
	}
	return fmt.Sprintf("warning: offered rate not sustained: %d of %.0f scheduled sends (%.0f%%); "+
		"raise -workers (sustaining a rate needs about rate x latency in-flight ops) or lower the rate",
		sent, offered, 100*float64(sent)/offered)
}

// runMeasure executes the measurement phase, applying rate ramping if both
// ramp options (RateStep, RateStepMax) are set. When the run is paced, it
// counts the attempted sends and warns on stderr when the run fell behind the
// offered-load schedule (see [PaceWarning]); the count is one atomic add per
// send, taken only on the paced path, where each send already pays a timer
// wait. The unlimited (closed-loop) path is untouched.
func runMeasure(ctx context.Context, ticker *Ticker, opts Options, doOps ...func() error) error {
	offered := opts.OfferedOps() * float64(len(doOps))
	if offered <= 0 {
		return runSchedule(ctx, ticker, opts, doOps...)
	}
	var sent atomic.Uint64
	counted := make([]func() error, len(doOps))
	for i, doOp := range doOps {
		counted[i] = func() error {
			sent.Add(1)
			return doOp()
		}
	}
	if err := runSchedule(ctx, ticker, opts, counted...); err != nil {
		return err
	}
	if msg := PaceWarning(sent.Load(), offered); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	return nil
}

// runSchedule drives the offered-load schedule. It emits a RateStep event on
// the ticker before each rate transition. When ramping is disabled, the entire
// Duration runs at opts.Rate. A ramp starts at opts.startRate (RateStep when
// Rate is unset), climbs by RateStep per level up to RateStepMax, and divides
// Duration evenly across the levels, so it always spans exactly Duration and
// ends exactly at RateStepMax. Passing more than one doOp fans the phase out
// across independent send targets, as the symmetric runners do (see [RunPhase]).
func runSchedule(ctx context.Context, ticker *Ticker, opts Options, doOps ...func() error) error {
	if !opts.rampEnabled() {
		start := time.Now()
		return RunPhase(ctx, opts.Workers, opts.Rate, start, start.Add(opts.Duration), doOps...)
	}
	// Rate-ramp mode: one phase per offered-load level.
	numSteps := opts.rampSteps()
	stepDur := opts.Duration / time.Duration(numSteps)
	currentRate := opts.startRate()
	for i := range numSteps {
		dur := stepDur
		if i == numSteps-1 {
			// The last level absorbs the integer-division remainder.
			dur = opts.Duration - time.Duration(i)*stepDur
		}
		stepStart := time.Now()
		if err := RunPhase(ctx, opts.Workers, currentRate, stepStart, stepStart.Add(dur), doOps...); err != nil {
			return err
		}
		if i < numSteps-1 {
			currentRate = min(currentRate+opts.RateStep, opts.RateStepMax)
			ticker.RateStep(int64(currentRate))
		}
	}
	return nil
}

// StartRemote resets the remote servers' op counters and Stats baselines at the
// start of a client-measured run, so the Stop reply observes only the work done
// in the measurement window. It is a no-op in local mode, where the in-process
// servers share the client's heap and their memory stats cannot be separated.
func StartRemote(cc *gorums.ConfigContext, opts Options) error {
	if !opts.Remote {
		return nil
	}
	_, err := Start(cc, StartRequest_builder{StatsMode: opts.StatsMode}.Build()).All()
	return err
}

// StopRemote collects each remote server's per-op memory stats via
// [Control.Stop] and appends them to result. It returns the per-server Stop
// replies keyed by node ID so callers can verify them (see [WithVerify]). It
// is the counterpart to [StartRemote] and is likewise a no-op in local mode,
// where it returns a nil map.
func StopRemote(cc *gorums.ConfigContext, opts Options, result *Result) (map[uint32]*Result, error) {
	if !opts.Remote {
		return nil, nil
	}
	replies, err := CollectReplies(Stop(cc, &StopRequest{}).Results())
	if err != nil {
		return nil, err
	}
	AppendServerStats(result, replies)
	return replies, nil
}

// MeasureLatency runs the client-measured measurement window over one or more
// configurations and returns the Result built from the accumulated latency
// samples. setup is invoked once per yielded configuration (outside the loop) to
// build the request message and bind the context; the returned closure is the
// tight per-operation send, which MeasureLatency times and records via
// [Stats.AddLatency]. Yielding a single configuration drives the coordinator
// case (one send target); yielding one configuration per peer system drives
// the symmetric case (every system sends concurrently in one shared phase).
//
// MeasureLatency owns only the measurement window; any control-plane
// lifecycle ([StartRemote]/[StopRemote], local server resets) is the
// caller's concern.
func MeasureLatency(ctx context.Context, opts Options, configs iter.Seq[gorums.Config], setup func(opts Options, cc *gorums.ConfigContext) func() error) (*Result, error) {
	m := StartMeasurement(opts)
	var doOps []func() error
	for cfg := range configs {
		op := setup(opts, cfg.Context(ctx))
		doOps = append(doOps, func() error {
			start := time.Now()
			err := op()
			if err != nil {
				if errors.Is(err, ErrRunOver) {
					// The workload declared the run over: pass the sentinel
					// through to RunPhase so this worker stops, recording the
					// op neither as a success nor as a failure.
					return err
				}
				// Count the failed op and continue: a saturating workload can
				// see failed quorum calls, and aborting the run on the first
				// one turns graceful degradation into a crash. FailedOps records
				// them; only latencies of successful ops feed TotalOps and the
				// latency distribution.
				m.RecordError()
				return nil
			}
			m.Stats.AddLatency(time.Since(start))
			return nil
		})
	}
	if err := runMeasure(ctx, m.ticker, opts, doOps...); err != nil {
		m.stop()
		return nil, err
	}
	return m.Finish(), nil
}

// MeasureOneWay builds the client-side send window for a server-measured run. It
// wraps each one-way send into a doOp that records a client op on success (the
// op count feeds only the throughput time-series; latency is measured server
// side) and returns the Measurement together with a window closure that starts
// the measurement, runs the paced phase via runMeasure, and stops the
// measurement. Server-measured runners therefore get rate ramping and ticker
// rate-step events without counting work outside the send window. Passing more
// than one send fans the window out across independent targets, as the
// symmetric multicast runner does.
//
// MeasureOneWay owns only the send window: the caller feeds window into
// RunOffsetCorrected as the measure step and Attaches the resulting Result to
// the returned Measurement. Any pre/post-window work (server resets, client
// memory snapshots, outbound flushes) stays with the caller.
func MeasureOneWay(ctx context.Context, opts Options, sends ...func() error) (*Measurement, func() error) {
	m := newMeasurement(opts)
	doOps := make([]func() error, len(sends))
	for i, send := range sends {
		doOps[i] = func() error {
			if err := send(); err != nil {
				return err
			}
			m.Stats.AddOp()
			return nil
		}
	}
	window := func() error {
		m.start()
		defer m.stop()
		return runMeasure(ctx, m.ticker, opts, doOps...)
	}
	return m, window
}

// LifecycleOption customizes the reusable lifecycles built by ClientMeasured
// and ServerMeasured with optional hooks.
type LifecycleOption func(*lifecycleHooks)

// lifecycleHooks holds the optional hooks of one lifecycle. The zero value
// disables every hook.
type lifecycleHooks struct {
	quiesce func(context.Context) error
	verify  func(map[uint32]*Result) error
}

// newLifecycleHooks applies opts to a zero hook set.
func newLifecycleHooks(opts []LifecycleOption) lifecycleHooks {
	var h lifecycleHooks
	for _, o := range opts {
		o(&h)
	}
	return h
}

// runQuiesce invokes the quiesce hook when one is registered.
func (h lifecycleHooks) runQuiesce(ctx context.Context) error {
	if h.quiesce == nil {
		return nil
	}
	return h.quiesce(ctx)
}

// WithQuiesce registers a drain hook invoked after the measurement window
// closes and before Control.Stop collects the server-side statistics, letting
// a benchmark drain in-flight operations so the boundary measurement observes
// them. The context is the run's BenchContext; a non-nil error fails the run.
func WithQuiesce(f func(context.Context) error) LifecycleOption {
	return func(h *lifecycleHooks) { h.quiesce = f }
}

// runVerify invokes the verify hook when one is registered.
func (h lifecycleHooks) runVerify(replies map[uint32]*Result) error {
	if h.verify == nil {
		return nil
	}
	return h.verify(replies)
}

// WithVerify registers a correctness check over the per-server Control.Stop
// replies, keyed by node ID, invoked after the measurement completes. A
// non-nil error fails the run, so no result file is written for a run that
// does not verify (e.g. reject when per-server TotalOps diverge beyond a
// tolerance). In a local client-measured run there are no remote replies and
// the hook receives a nil map.
func WithVerify(f func(map[uint32]*Result) error) LifecycleOption {
	return func(h *lifecycleHooks) { h.verify = f }
}

// ClientMeasured builds a [Bench].Run closure for the standard
// client-measured lifecycle: (in remote mode) [Control.Start], a paced
// measurement window timed on the client via [Stats] from t=0, and finally
// [Control.Stop] to collect each server's memory stats. The protocol author
// supplies setup, which binds a configuration context and returns the tight
// per-operation closure; building the request message and binding the
// context happen once, outside the measurement loop. Optional lifecycle
// hooks (e.g. [WithQuiesce]) run between the window and Control.Stop.
func ClientMeasured(cfg gorums.Config, setup func(opts Options, cc *gorums.ConfigContext) func() error, lifecycleOpts ...LifecycleOption) func(Options) (*Result, error) {
	hooks := newLifecycleHooks(lifecycleOpts)
	return func(opts Options) (*Result, error) {
		ctx, cancel := BenchContext(opts)
		defer cancel()
		cc := cfg.Context(ctx)
		if err := StartRemote(cc, opts); err != nil {
			return nil, err
		}
		result, err := MeasureLatency(ctx, opts, slices.Values([]gorums.Config{cfg}), setup)
		if err != nil {
			return nil, err
		}
		if err := hooks.runQuiesce(ctx); err != nil {
			return nil, err
		}
		replies, err := StopRemote(cc, opts, result)
		if err != nil {
			return nil, err
		}
		if err := hooks.runVerify(replies); err != nil {
			return nil, err
		}
		return result, nil
	}
}

// ServerMeasured builds a [Bench].Run closure for the standard
// server-measured lifecycle: [Control.Start], a paced one-way send window
// from t=0 with clock-offset correction around it, and [Control.Stop] to
// collect each server's latency samples. The protocol author supplies setup,
// which binds a configuration context and returns the tight per-send closure
// (it stamps and sends one one-way message). A failed send aborts the run and
// is not counted as an operation, as in [ClientMeasured]. Optional lifecycle
// hooks (e.g. [WithQuiesce]) run between the send window and Control.Stop,
// where a drain ensures the servers observe every in-flight one-way send.
func ServerMeasured(cfg gorums.Config, setup func(opts Options, cc *gorums.ConfigContext) func() error, lifecycleOpts ...LifecycleOption) func(Options) (*Result, error) {
	hooks := newLifecycleHooks(lifecycleOpts)
	return func(opts Options) (*Result, error) {
		ctx, cancel := BenchContext(opts)
		defer cancel()
		cc := cfg.Context(ctx)
		send := setup(opts, cc)
		var startMem, endMem memSnapshot
		var replies map[uint32]*Result

		m, window := MeasureOneWay(ctx, opts, send)
		measure := func() error {
			// Start, the memory snapshots, and Stop are issued here, inside the
			// window estimate() brackets, so the two clock-sync phases (50
			// sequential RPC rounds each) never inflate client per-op memory
			// accounting or the server-measured throughput window.
			if _, err := Start(cc, StartRequest_builder{StatsMode: opts.StatsMode}.Build()).All(); err != nil {
				return err
			}
			startMem.read()
			if err := window(); err != nil {
				return err
			}
			if err := hooks.runQuiesce(ctx); err != nil {
				return err
			}
			endMem.read()
			var err error
			replies, err = CollectReplies(Stop(cc, &StopRequest{}).Results())
			return err
		}
		estimate := func() (map[uint32]int64, error) {
			return EstimateOffsets(ctx, cfg)
		}
		buildReplies := func(before, after map[uint32]int64) (map[uint32]*Result, error) {
			LogOffsets("servers", before, after)
			offsets := AverageOffsets(before, after)
			for id, reply := range replies {
				CorrectLatencies(reply, offsets[id])
			}
			// Verify the corrected per-server replies: exactly what the
			// aggregation step below will combine.
			if err := hooks.runVerify(replies); err != nil {
				return nil, err
			}
			return replies, nil
		}
		resp, err := RunOffsetCorrected(estimate, measure, buildReplies)
		if err != nil {
			m.stop()
			return nil, err
		}
		m.Attach(resp)

		// Divide the client-side memory delta by the client's own send count;
		// resp.GetTotalOps() aggregates over all servers (N x the sends for
		// multicast) and would under-report the per-send client cost.
		clientAllocs, clientMem := startMem.delta(endMem, m.Stats.Ops())
		resp.SetAllocsPerOp(clientAllocs)
		resp.SetMemPerOp(clientMem)
		return resp, nil
	}
}

// Run selects the benchmarks whose name matches sel, runs each with opts, stamps
// the shared run metadata onto each Result, and returns the results sorted by
// benchmark name. The mode metadata ("local"/"remote") is derived from
// opts.Remote. Run rejects invalid options (see [Options.Validate]) and a
// selector matching no benchmark, instead of returning an empty successful
// result set.
func Run(sel *regexp.Regexp, opts Options, benches []Bench) ([]*Result, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}
	if opts.StreamMode == "" {
		opts.StreamMode = "dual"
	}
	mode := "local"
	if opts.Remote {
		mode = "remote"
	}
	var matched bool
	var results []*Result
	for _, b := range benches {
		if !sel.MatchString(b.Name) {
			continue
		}
		matched = true
		opts.BenchName = b.Name
		result, err := b.Run(opts)
		if err != nil {
			return nil, err
		}
		// The runner stamped the measurement mode via Finish/Attach; preserve it
		// while filling in the rest of the metadata from opts.
		result.SetConfig(RunConfig_builder{
			Name:            b.Name,
			NumNodes:        int32(opts.NumNodes),
			Mode:            mode,
			Duration:        int64(opts.Duration),
			Workers:         int32(opts.Workers),
			Payload:         int32(opts.Payload),
			Rate:            int64(opts.Rate),
			Interval:        int64(opts.Interval),
			MeasurementMode: result.GetConfig().GetMeasurementMode(),
			StatsMode:       opts.StatsMode,
			StreamMode:      opts.StreamMode,
			QuorumSize:      int32(opts.QuorumSize),
			MaxAsync:        int32(opts.MaxAsync),
			RateStep:        int64(opts.RateStep),
			RateStepMax:     int64(opts.RateStepMax),
			CallTimeout:     int64(opts.CallTimeout),
			SendBuffer:      int32(opts.SendBuffer),
			RecvBuffer:      int32(opts.RecvBuffer),
		}.Build())
		i := sort.Search(len(results), func(i int) bool {
			return results[i].GetConfig().GetName() >= result.GetConfig().GetName()
		})
		results = append(results, nil)
		copy(results[i+1:], results[i:])
		results[i] = result
	}
	if !matched {
		return nil, fmt.Errorf("no benchmarks match %q", sel)
	}
	return results, nil
}
