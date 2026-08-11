package benchkit

import "sync/atomic"

// Measurement bundles a run's statistics and time-series ticker so every
// benchmark runner wires up observability the same way, honoring opts.StatsMode
// (the aggregate latency store) and opts.Interval (the time-series event
// stream). Record samples through the exported Stats field during the run, then
// call Finish (client-measured: the Result is built from Stats) or Attach
// (server-measured: the Result is built from server replies).
//
// The lifecycle is StartMeasurement -> run workload -> Finish, Attach, or
// Abandon. There is no warmup phase: measurement runs continuously from t=0
// and the startup transient is trimmed by read-time tools, not here.
type Measurement struct {
	Stats       *Stats  // record samples via Stats.AddLatency / Stats.AddOp
	ticker      *Ticker // owns the event buffer; drives the interval event stream
	initialRate int64
	started     bool
	stopped     bool
	failed      atomic.Uint64
}

// RecordError counts one operation that returned an error. Client-measured
// runners call it instead of aborting so a saturating workload records failed
// operations (surfaced as Result.FailedOps by Finish) rather than terminating
// the run on the first failure.
func (m *Measurement) RecordError() {
	m.failed.Add(1)
}

// newMeasurement creates a dormant measurement whose clock and ticker start
// when [Measurement.start] is called.
func newMeasurement(opts Options) *Measurement {
	s := NewStats(opts.StatsMode)
	return &Measurement{
		Stats:       s,
		ticker:      NewTicker(opts.Interval, s),
		initialRate: int64(opts.startRate()),
	}
}

// StartMeasurement creates the Stats and Ticker for a run from opts, emits the
// START phase marker at the run's starting rate (opts.Rate, or RateStep for a
// ramp with Rate unset), and starts the measurement clock. Call it at t=0,
// immediately before the measurement loop.
func StartMeasurement(opts Options) *Measurement {
	m := newMeasurement(opts)
	m.start()
	return m
}

// start emits the START marker and starts the measurement clock and ticker.
func (m *Measurement) start() {
	if m.started {
		return
	}
	m.started = true
	m.ticker.Start(m.initialRate)
	m.Stats.Start()
}

// stop ends the measurement clock and stops the ticker. The whole-run CV that
// Ticker.Stop returns is recomputed at read time over the trimmed intervals, so
// it is not persisted here.
func (m *Measurement) stop() {
	if !m.started || m.stopped {
		return
	}
	m.stopped = true
	m.Stats.End()
	m.ticker.Stop()
}

// Finish ends the measurement and returns the client-measured Result built from
// the accumulated latency samples, with the time-series events attached. Use it
// for benchmarks where the client times each operation via Stats.AddLatency.
func (m *Measurement) Finish() *Result {
	m.stop()
	r := m.Stats.GetResult()
	r.SetEvents(m.ticker.Events())
	r.SetFailedOps(m.failed.Load())
	setMeasurementMode(r, MeasurementMode_CLIENT_MEASURED)
	return r
}

// Attach ends the measurement and attaches the time-series events to result, a
// Result built elsewhere. Use it for server-measured benchmarks where latency
// comes from server replies rather than the client-side Stats; the client Stats
// then carries only the op count (via Stats.AddOp) for the throughput stream.
func (m *Measurement) Attach(result *Result) {
	m.stop()
	result.SetEvents(m.ticker.Events())
	setMeasurementMode(result, MeasurementMode_SERVER_MEASURED)
}

// Abandon stops the measurement clock and ticker without producing a Result.
// Call it from another package when a workload fails at a point that
// precedes where Finish or Attach would normally run, so the ticker's
// background goroutine and its time.Ticker are not leaked. Callers within
// benchkit itself can call the unexported stop directly.
func (m *Measurement) Abandon() {
	m.stop()
}

// setMeasurementMode records how a Result's latency was produced. Finish and
// Attach are the single source of truth for this: Finish builds the result from
// client Stats (client-measured) while Attach takes a server-built result
// (server-measured). The harness Run preserves the mode when it stamps the rest
// of the RunConfig metadata, so consumers never have to infer it.
func setMeasurementMode(r *Result, mode MeasurementMode) {
	cfg := r.GetConfig()
	if cfg == nil {
		cfg = &RunConfig{}
		r.SetConfig(cfg)
	}
	cfg.SetMeasurementMode(mode)
}
