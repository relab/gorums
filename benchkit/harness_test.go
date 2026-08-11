package benchkit

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
)

// TestRunPhase exercises the generalized RunPhase skeleton: error propagation,
// the no-op boundary conditions (zero workers, past deadline), and the
// multi-doOp fan-out where each doOp gets its own pool of workers.
func TestRunPhase(t *testing.T) {
	errBoom := errors.New("boom")

	tests := []struct {
		name    string
		numG    int
		numOps  int  // number of doOps to fan out across
		past    bool // endTime already elapsed before the phase starts
		failing bool // doOps return errBoom on first call
		wantErr error
		// wantInvoked is true when at least one doOp must have run.
		wantInvoked bool
	}{
		{name: "PastDeadlineRunsNothing", numG: 4, numOps: 1, past: true, wantErr: nil, wantInvoked: false},
		{name: "ZeroWorkersRunsNothing", numG: 0, numOps: 3, wantErr: nil, wantInvoked: false},
		{name: "SingleDoOpRuns", numG: 1, numOps: 1, wantErr: nil, wantInvoked: true},
		{name: "FanOutRunsEveryDoOp", numG: 2, numOps: 3, wantErr: nil, wantInvoked: true},
		{name: "ErrorPropagates", numG: 1, numOps: 1, failing: true, wantErr: errBoom, wantInvoked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// invoked is a bit vector: bit i is set once doOp i has run.
			// allBits is the value with every doOp's bit set.
			var invoked atomic.Int32
			allBits := int32(1)<<tt.numOps - 1
			doOps := make([]func() error, tt.numOps)

			// The workers run unpaced in a tight loop; under GOMAXPROCS=1 they
			// are not guaranteed to all be scheduled within a short window, so
			// cancel the phase once every doOp has run at least once (bounded
			// by the generous ceiling below). cancel is idempotent, so calling
			// it from every doOp once all bits are set is harmless.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			for i := range doOps {
				bit := int32(1) << i
				doOps[i] = func() error {
					if invoked.Or(bit)|bit == allBits {
						cancel()
					}
					if tt.failing {
						return errBoom
					}
					return nil
				}
			}

			start := time.Now()
			// A generous ceiling; cancel above ends passing cases promptly, so
			// this only bounds how long a genuinely stuck doOp is waited for.
			endTime := start.Add(2 * time.Second)
			if tt.past {
				endTime = start.Add(-time.Millisecond)
			}
			err := RunPhase(ctx, tt.numG, 0, start, endTime, doOps...)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RunPhase err = %v, want %v", err, tt.wantErr)
			}

			got := invoked.Load()
			if anyRan := got != 0; anyRan != tt.wantInvoked {
				t.Errorf("any doOp invoked = %v, want %v", anyRan, tt.wantInvoked)
			}
			// On the non-failing path every doOp must have run, since ctx is
			// only cancelled once all have.
			if tt.wantInvoked && !tt.failing && got != allBits {
				t.Errorf("not every doOp was invoked: bits=%03b want=%03b", got, allBits)
			}
		})
	}
}

// TestRunPhaseStopsOnContextCancel verifies that cancellation promptly stops
// closed-loop workers.
func TestRunPhaseStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var ops atomic.Int64
	start := time.Now()
	endTime := start.Add(2 * time.Second)
	err := RunPhase(ctx, 4, 0, start, endTime, func() error {
		if ops.Add(1) == 1 {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunPhase err = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("RunPhase returned after %v; want prompt stop on cancel, not spin to endTime", elapsed)
	}
	if n := ops.Load(); n > 1000 {
		t.Errorf("ops after cancel = %d, want a handful (workers must stop, not spin)", n)
	}
}

// TestRunPhaseRunOverStopsWorker verifies that a doOp returning ErrRunOver
// stops its worker cleanly: the phase ends promptly without treating the
// sentinel as a fault, so RunPhase returns nil.
func TestRunPhaseRunOverStopsWorker(t *testing.T) {
	var ops atomic.Int64
	start := time.Now()
	err := RunPhase(context.Background(), 2, 0, start, start.Add(2*time.Second), func() error {
		ops.Add(1)
		return ErrRunOver
	})
	if err != nil {
		t.Fatalf("RunPhase err = %v, want nil (ErrRunOver is a clean stop, not a fault)", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("RunPhase returned after %v; want prompt stop on ErrRunOver", elapsed)
	}
	// Each of the 2 workers stops on its first ErrRunOver.
	if n := ops.Load(); n != 2 {
		t.Errorf("ops = %d, want 2 (one per worker before stopping)", n)
	}
}

// TestMeasureLatencyRunOverNotRecorded verifies that ErrRunOver stops its
// worker without changing the success or failure counts.
func TestMeasureLatencyRunOverNotRecorded(t *testing.T) {
	opts := Options{Workers: 2, Duration: 2 * time.Second}
	var calls atomic.Int64
	setup := func(_ Options, _ *gorums.ConfigContext) func() error {
		return func() error {
			if calls.Add(1) > 3 {
				return ErrRunOver
			}
			return nil
		}
	}
	start := time.Now()
	configs := slices.Values([]gorums.Config{gorumstest.NoDialedConfig(t)})
	result, err := MeasureLatency(context.Background(), opts, configs, setup)
	if err != nil {
		t.Fatalf("MeasureLatency err = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("MeasureLatency returned after %v; want prompt stop on ErrRunOver", elapsed)
	}
	if got := result.GetTotalOps(); got != 3 {
		t.Errorf("TotalOps = %d, want 3 (only pre-run-over ops recorded)", got)
	}
	if got := result.GetFailedOps(); got != 0 {
		t.Errorf("FailedOps = %d, want 0 (run over is not a failure)", got)
	}
}

// TestRunOffsetCorrected verifies the estimate→measure→estimate→aggregate
// sequencing and that an error at any stage short-circuits the remaining
// stages, leaving later stages uncalled.
func TestRunOffsetCorrected(t *testing.T) {
	errEst := errors.New("estimate failed")
	errMeasure := errors.New("measure failed")
	errReplies := errors.New("buildReplies failed")

	// validReplies yields a single non-empty reply so AggregateServerResults
	// succeeds on the happy path.
	validReplies := func() map[uint32]*Result {
		return map[uint32]*Result{1: Result_builder{TotalOps: 4, Latencies: []int64{1, 2, 3, 4}}.Build()}
	}

	tests := []struct {
		name string
		// failAtEstimate: 0 never, 1 first call, 2 second call.
		failAtEstimate int
		failMeasure    bool
		failReplies    bool
		emptyReplies   bool // buildReplies returns an empty map → aggregate ErrIncomplete
		wantErr        error
		wantEstimate   int // expected estimate() call count
		wantMeasure    int // expected measure() call count
		wantReplies    int // expected buildReplies() call count
		wantTotalOps   uint64
	}{
		{name: "HappyPath", wantErr: nil, wantEstimate: 2, wantMeasure: 1, wantReplies: 1, wantTotalOps: 4},
		{name: "EstimateBeforeErrors", failAtEstimate: 1, wantErr: errEst, wantEstimate: 1, wantMeasure: 0, wantReplies: 0},
		{name: "MeasureErrors", failMeasure: true, wantErr: errMeasure, wantEstimate: 1, wantMeasure: 1, wantReplies: 0},
		{name: "EstimateAfterErrors", failAtEstimate: 2, wantErr: errEst, wantEstimate: 2, wantMeasure: 1, wantReplies: 0},
		{name: "BuildRepliesErrors", failReplies: true, wantErr: errReplies, wantEstimate: 2, wantMeasure: 1, wantReplies: 1},
		{name: "EmptyRepliesIncomplete", emptyReplies: true, wantErr: gorums.ErrIncomplete, wantEstimate: 2, wantMeasure: 1, wantReplies: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var estimateCalls, measureCalls, repliesCalls int
			estimate := func() (int, error) {
				estimateCalls++
				if tt.failAtEstimate == estimateCalls {
					return 0, errEst
				}
				return estimateCalls, nil
			}
			measure := func() error {
				measureCalls++
				if tt.failMeasure {
					return errMeasure
				}
				return nil
			}
			buildReplies := func(before, after int) (map[uint32]*Result, error) {
				repliesCalls++
				if tt.failReplies {
					return nil, errReplies
				}
				if tt.emptyReplies {
					return nil, nil
				}
				return validReplies(), nil
			}

			r, err := RunOffsetCorrected(estimate, measure, buildReplies)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("RunOffsetCorrected err = %v, want %v", err, tt.wantErr)
			}
			if estimateCalls != tt.wantEstimate {
				t.Errorf("estimate calls = %d, want %d", estimateCalls, tt.wantEstimate)
			}
			if measureCalls != tt.wantMeasure {
				t.Errorf("measure calls = %d, want %d", measureCalls, tt.wantMeasure)
			}
			if repliesCalls != tt.wantReplies {
				t.Errorf("buildReplies calls = %d, want %d", repliesCalls, tt.wantReplies)
			}
			if tt.wantErr == nil && r.GetTotalOps() != tt.wantTotalOps {
				t.Errorf("TotalOps = %d, want %d", r.GetTotalOps(), tt.wantTotalOps)
			}
		})
	}
}

// TestRunMeasureSinglePhase verifies that runMeasure runs a single phase when
// rate ramp options are zero.
func TestRunMeasureSinglePhase(t *testing.T) {
	var ops atomic.Int64
	s := NewStats(StatsMode_EXACT)
	tk := NewTicker(0, s)
	tk.Start(0)
	opts := Options{Workers: 2, Rate: 0, Duration: 100 * time.Millisecond}
	if err := runMeasure(context.Background(), tk, opts, func() error {
		ops.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("runMeasure: %v", err)
	}
	tk.Stop()
	if ops.Load() == 0 {
		t.Error("runMeasure: no ops executed in single-phase mode")
	}
}

// TestOptionsStartRate verifies the offered rate of the first measurement
// phase: opts.Rate normally, but RateStep when ramping is enabled and Rate is
// unset, so a ramped run climbs from the first step instead of starting
// unlimited and dropping.
func TestOptionsStartRate(t *testing.T) {
	ramp := Options{RateStep: 50, RateStepMax: 200}
	rampWithRate := ramp
	rampWithRate.Rate = 100

	tests := []struct {
		name string
		opts Options
		want int
	}{
		{"NoRampUnlimitedStaysUnlimited", Options{}, 0},
		{"NoRampUsesRate", Options{Rate: 7}, 7},
		{"RampWithRateKeepsRate", rampWithRate, 100},
		{"RampUnlimitedStartsAtRateStep", ramp, 50},
		{"PartialRampStaysUnlimited", Options{RateStep: 50}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.startRate(); got != tt.want {
				t.Errorf("startRate() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestRunMeasureRampUnlimitedStartsAtRateStep verifies that a ramp configured
// with Rate=0 starts at RateStep and climbs to RateStepMax: the START marker
// carries RateStep and the RATE_STEP markers carry increasing rates above it.
func TestRunMeasureRampUnlimitedStartsAtRateStep(t *testing.T) {
	opts := Options{
		Workers:     1,
		Rate:        0, // unlimited; ramp must override the start rate
		Duration:    300 * time.Millisecond,
		Interval:    time.Hour, // capture phase markers without background ticks
		RateStep:    50,
		RateStepMax: 200,
	}
	m := StartMeasurement(opts)
	if err := runMeasure(context.Background(), m.ticker, opts, func() error { return nil }); err != nil {
		t.Fatalf("runMeasure: %v", err)
	}
	m.ticker.Stop()

	events := m.ticker.Events()
	if ph := events[0].GetPhase(); ph == nil || ph.GetPhase() != PhaseMarker_START || ph.GetRate() != 50 {
		t.Errorf("events[0] = %v, want START with rate 50", events[0])
	}
	var rates []int64
	for _, ev := range events {
		if ph := ev.GetPhase(); ph != nil && ph.GetPhase() == PhaseMarker_RATE_STEP {
			rates = append(rates, ph.GetRate())
		}
	}
	// 4 levels (50, 100, 150, 200) over 300ms → 3 transitions.
	if len(rates) != 3 || rates[0] != 100 || rates[1] != 150 || rates[2] != 200 {
		t.Errorf("RATE_STEP rates = %v, want [100 150 200]", rates)
	}
}

// TestOptionsOfferedOps verifies the number of sends the offered-load schedule
// asks of one send target: rate × duration, summed per level when ramping, and
// 0 for an unlimited (closed-loop) run.
func TestOptionsOfferedOps(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want float64
	}{
		{"Unlimited", Options{Duration: time.Second}, 0},
		{"Paced", Options{Rate: 100, Duration: 2 * time.Second}, 200},
		// Levels 100, 200, 300 × 1s each.
		{"Ramp", Options{RateStep: 100, RateStepMax: 300, Duration: 3 * time.Second}, 600},
		// Levels 200, 300, 400 × 1s each.
		{"RampWithStartRate", Options{Rate: 200, RateStep: 100, RateStepMax: 400, Duration: 3 * time.Second}, 900},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.OfferedOps(); got != tt.want {
				t.Errorf("OfferedOps() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPaceWarning verifies that a warning is produced exactly when a paced run
// attempted markedly fewer sends than its offered-load schedule asked for, and
// that the message names the worker-sizing fix.
func TestPaceWarning(t *testing.T) {
	tests := []struct {
		name     string
		sent     uint64
		offered  float64
		wantWarn bool
	}{
		{"OnSchedule", 100, 100, false},
		{"WithinTolerance", 96, 100, false},
		{"Behind", 50, 100, true},
		{"JustBelowTolerance", 94, 100, true},
		{"NoSchedule", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := PaceWarning(tt.sent, tt.offered)
			if got := msg != ""; got != tt.wantWarn {
				t.Fatalf("PaceWarning(%d, %v) = %q, want warning: %v", tt.sent, tt.offered, msg, tt.wantWarn)
			}
			if tt.wantWarn && !strings.Contains(msg, "-workers") {
				t.Errorf("warning %q does not mention -workers", msg)
			}
		})
	}
}

// TestOptionsRampSteps verifies the number of offered-load levels derived from
// the ramp options: one per RateStep increment from the start rate up to and
// including RateStepMax, with a partial final increment still counting as a
// level.
func TestOptionsRampSteps(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want int
	}{
		{"ExactSpan", Options{RateStep: 50, RateStepMax: 200}, 4},                 // 50,100,150,200
		{"WithStartRate", Options{Rate: 100, RateStep: 100, RateStepMax: 300}, 3}, // 100,200,300
		{"PartialLastStep", Options{RateStep: 100, RateStepMax: 250}, 3},          // 100,200,250
		{"StartAtMax", Options{Rate: 200, RateStep: 50, RateStepMax: 200}, 1},     // 200
		{"StartAboveMax", Options{Rate: 500, RateStep: 50, RateStepMax: 200}, 1},  // 500
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.rampSteps(); got != tt.want {
				t.Errorf("rampSteps() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestRunMeasureRampEmitsRateStepEvents verifies that runMeasure emits the
// expected number of RATE_STEP phase markers when ramping is configured.
func TestRunMeasureRampEmitsRateStepEvents(t *testing.T) {
	s := NewStats(StatsMode_EXACT)
	// Interval longer than the run: the buffer captures the synchronously emitted
	// phase markers (START, RATE_STEP, STOP) without any background ticks firing.
	tk := NewTicker(time.Hour, s)

	tk.Start(100) // start marker at rate 100

	opts := Options{
		Workers:     1,
		Rate:        100,
		Duration:    300 * time.Millisecond,
		RateStep:    100,
		RateStepMax: 300,
	}
	var opsCount atomic.Int64
	if err := runMeasure(context.Background(), tk, opts, func() error {
		opsCount.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("runMeasure: %v", err)
	}
	tk.Stop() // STOP

	// Count RATE_STEP phase markers: 3 levels (100, 200, 300) → 2 transitions.
	var rateSteps int
	for _, ev := range tk.Events() {
		if ph := ev.GetPhase(); ph != nil && ph.GetPhase() == PhaseMarker_RATE_STEP {
			rateSteps++
		}
	}
	if rateSteps != 2 {
		t.Errorf("rateSteps = %d, want 2", rateSteps)
	}
	if opsCount.Load() == 0 {
		t.Error("no ops executed during ramp")
	}
}

// validOptions returns a minimal Options value that passes Validate, so each
// test case below only needs to override the field under test.
func validOptions() Options {
	return Options{Workers: 1, Duration: time.Second}
}

// TestOptionsValidate verifies the generic option constraints [Run] enforces
// before executing any benchmark: bad flag values must fail fast with a
// specific message instead of panicking or silently producing a zero-work
// run.
func TestOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{"Valid", validOptions(), false},
		{"ZeroWorkers", func() Options { o := validOptions(); o.Workers = 0; return o }(), true},
		{"NegativeWorkers", func() Options { o := validOptions(); o.Workers = -1; return o }(), true},
		{"ZeroDuration", func() Options { o := validOptions(); o.Duration = 0; return o }(), true},
		{"NegativeDuration", func() Options { o := validOptions(); o.Duration = -time.Second; return o }(), true},
		{"NegativePayload", func() Options { o := validOptions(); o.Payload = -1; return o }(), true},
		{"NegativeRate", func() Options { o := validOptions(); o.Rate = -1; return o }(), true},
		{"NegativeInterval", func() Options { o := validOptions(); o.Interval = -time.Second; return o }(), true},
		{"NegativeCallTimeout", func() Options { o := validOptions(); o.CallTimeout = -time.Second; return o }(), true},
		{"NegativeRateStep", func() Options { o := validOptions(); o.RateStep = -1; return o }(), true},
		{"NegativeRateStepMax", func() Options { o := validOptions(); o.RateStepMax = -1; return o }(), true},
		{"RateStepWithoutMax", func() Options { o := validOptions(); o.RateStep = 10; return o }(), true},
		{"RateStepMaxWithoutStep", func() Options { o := validOptions(); o.RateStepMax = 10; return o }(), true},
		{"RateStepMaxBelowStep", func() Options { o := validOptions(); o.RateStep = 100; o.RateStepMax = 50; return o }(), true},
		{"ValidRamp", func() Options { o := validOptions(); o.RateStep = 50; o.RateStepMax = 200; return o }(), false},
		{"EmptyStreamMode", func() Options { o := validOptions(); o.StreamMode = ""; return o }(), false},
		{"DualStreamMode", func() Options { o := validOptions(); o.StreamMode = "dual"; return o }(), false},
		{"DedupStreamMode", func() Options { o := validOptions(); o.StreamMode = "dedup"; return o }(), false},
		{"InvalidStreamMode", func() Options { o := validOptions(); o.StreamMode = "bogus"; return o }(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRunRejectsInvalidOptions verifies that Run refuses to execute any
// benchmark when opts fails Validate.
func TestRunRejectsInvalidOptions(t *testing.T) {
	opts := validOptions()
	opts.Workers = 0
	var ran bool
	benches := []Bench{{Name: "Bench", Run: func(Options) (*Result, error) {
		ran = true
		return &Result{}, nil
	}}}
	_, err := Run(regexp.MustCompile(".*"), opts, benches)
	if err == nil {
		t.Fatal("Run with invalid options = nil error, want error")
	}
	if ran {
		t.Error("Run invoked the benchmark despite invalid options")
	}
}

// TestRunNoMatchingBenchmark verifies that a selector matching no benchmark
// fails instead of returning an empty, successful result set — a selector
// typo should not look like a clean zero-benchmark run.
func TestRunNoMatchingBenchmark(t *testing.T) {
	benches := []Bench{{Name: "QuorumCall", Run: func(Options) (*Result, error) {
		return &Result{}, nil
	}}}
	_, err := Run(regexp.MustCompile("^NoSuchBenchmark$"), validOptions(), benches)
	if err == nil {
		t.Fatal("Run with no matching benchmark = nil error, want error")
	}
}

// TestRunStampsFullConfig verifies that [Run] stamps every semantic option
// onto the result's [RunConfig], including QuorumSize, MaxAsync, RateStep,
// RateStepMax, and CallTimeout: two runs with different quorum size or
// rate-ramp settings must not serialize identical configs.
func TestRunStampsFullConfig(t *testing.T) {
	opts := Options{
		Workers:     2,
		Duration:    time.Second,
		Payload:     16,
		Rate:        100,
		Interval:    50 * time.Millisecond,
		QuorumSize:  3,
		MaxAsync:    500,
		RateStep:    50,
		RateStepMax: 200,
		CallTimeout: 20 * time.Millisecond,
		StatsMode:   StatsMode_HDR,
		StreamMode:  "dedup",
		NumNodes:    4,
		Remote:      true,
	}
	benches := []Bench{{Name: "Bench", Run: func(Options) (*Result, error) { return &Result{}, nil }}}
	results, err := Run(regexp.MustCompile(".*"), opts, benches)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	cfg := results[0].GetConfig()
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"NumNodes", cfg.GetNumNodes(), int32(4)},
		{"Mode", cfg.GetMode(), "remote"},
		{"Workers", cfg.GetWorkers(), int32(2)},
		{"Payload", cfg.GetPayload(), int32(16)},
		{"Rate", cfg.GetRate(), int64(100)},
		{"Interval", cfg.GetInterval(), int64(50 * time.Millisecond)},
		{"QuorumSize", cfg.GetQuorumSize(), int32(3)},
		{"MaxAsync", cfg.GetMaxAsync(), int32(500)},
		{"RateStep", cfg.GetRateStep(), int64(50)},
		{"RateStepMax", cfg.GetRateStepMax(), int64(200)},
		{"CallTimeout", cfg.GetCallTimeout(), int64(20 * time.Millisecond)},
		{"StatsMode", cfg.GetStatsMode(), StatsMode_HDR},
		{"StreamMode", cfg.GetStreamMode(), "dedup"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("RunConfig.%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestClientMeasuredCountsErrorsWithoutAborting verifies that client-measured
// operations count errors while allowing the run to complete.
func TestClientMeasuredCountsErrorsWithoutAborting(t *testing.T) {
	var calls atomic.Int64
	run := ClientMeasured(gorumstest.NoDialedConfig(t),
		func(_ Options, _ *gorums.ConfigContext) func() error {
			return func() error {
				// Fail every other op; persistent errors must not abort.
				if calls.Add(1)%2 == 0 {
					return errors.New("quorum call failed")
				}
				return nil
			}
		})
	result, err := run(Options{Workers: 1, Duration: 30 * time.Millisecond})
	if err != nil {
		t.Fatalf("run aborted on op error: %v", err)
	}
	if result.GetTotalOps() == 0 {
		t.Error("TotalOps = 0, want > 0 (successful ops must still be recorded)")
	}
	if result.GetFailedOps() == 0 {
		t.Error("FailedOps = 0, want > 0 (op errors must be counted)")
	}
}

// TestClientMeasuredQuiesce verifies that the WithQuiesce hook runs after the
// measurement window closes (every op already recorded) and that a quiesce
// error fails the run.
func TestClientMeasuredQuiesce(t *testing.T) {
	var opsDone atomic.Int64
	var opsAtQuiesce int64
	quiesceCalls := 0
	run := ClientMeasured(gorumstest.NoDialedConfig(t),
		func(_ Options, _ *gorums.ConfigContext) func() error {
			return func() error {
				opsDone.Add(1)
				return nil
			}
		},
		WithQuiesce(func(context.Context) error {
			quiesceCalls++
			opsAtQuiesce = opsDone.Load()
			return nil
		}))

	result, err := run(Options{Workers: 2, Duration: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if quiesceCalls != 1 {
		t.Fatalf("quiesce calls = %d, want 1", quiesceCalls)
	}
	if opsAtQuiesce != opsDone.Load() {
		t.Errorf("ops at quiesce = %d, want %d (window must be closed before quiesce)",
			opsAtQuiesce, opsDone.Load())
	}
	if result.GetTotalOps() == 0 {
		t.Error("TotalOps = 0, want > 0")
	}

	errDrain := errors.New("drain failed")
	failing := ClientMeasured(gorumstest.NoDialedConfig(t),
		func(_ Options, _ *gorums.ConfigContext) func() error {
			return func() error { return nil }
		},
		WithQuiesce(func(context.Context) error { return errDrain }))
	if _, err := failing(Options{Workers: 1, Duration: time.Millisecond}); !errors.Is(err, errDrain) {
		t.Errorf("run with failing quiesce = %v, want %v", err, errDrain)
	}
}

// TestClientMeasuredVerify verifies the WithVerify hook: in a local run there
// are no remote Stop replies, so the hook receives a nil map, and a non-nil
// verify error fails the run.
func TestClientMeasuredVerify(t *testing.T) {
	verifyCalls := 0
	var gotReplies map[uint32]*Result
	setup := func(_ Options, _ *gorums.ConfigContext) func() error {
		return func() error { return nil }
	}
	run := ClientMeasured(gorumstest.NoDialedConfig(t), setup,
		WithVerify(func(replies map[uint32]*Result) error {
			verifyCalls++
			gotReplies = replies
			return nil
		}))
	if _, err := run(Options{Workers: 1, Duration: time.Millisecond}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if verifyCalls != 1 {
		t.Fatalf("verify calls = %d, want 1", verifyCalls)
	}
	if gotReplies != nil {
		t.Errorf("verify replies = %v, want nil in local mode", gotReplies)
	}

	errVerify := errors.New("ops diverged")
	failing := ClientMeasured(gorumstest.NoDialedConfig(t), setup,
		WithVerify(func(map[uint32]*Result) error { return errVerify }))
	if _, err := failing(Options{Workers: 1, Duration: time.Millisecond}); !errors.Is(err, errVerify) {
		t.Errorf("run with failing verify = %v, want %v", err, errVerify)
	}
}

// TestOptionsServerOptions verifies which server options a run's configuration
// implies. These carry the stream topology and the buffer capacities to the
// server; BufferSizesOption is unconditional, so only StreamDedupOption varies
// the count.
func TestOptionsServerOptions(t *testing.T) {
	tests := []struct {
		name      string
		opts      Options
		wantCount int
		wantDedup bool
	}{
		{
			name:      "NothingConfigured",
			wantCount: 1,
		},
		{
			name:      "DedupOnly",
			opts:      Options{StreamMode: "dedup"},
			wantCount: 2, wantDedup: true,
		},
		{
			// Zero is a real receive-buffer size, and still produces an option.
			name:      "RecvBufferZero",
			opts:      Options{RecvBuffer: 0},
			wantCount: 1,
		},
		{
			name:      "DedupAndBuffers",
			opts:      Options{StreamMode: "dedup", SendBuffer: 256, RecvBuffer: 16},
			wantCount: 2, wantDedup: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(tt.opts.ServerOptions()); got != tt.wantCount {
				t.Errorf("ServerOptions() length = %d, want %d", got, tt.wantCount)
			}
			if got := tt.opts.StreamDedupOption() != nil; got != tt.wantDedup {
				t.Errorf("StreamDedupOption() non-nil = %v, want %v", got, tt.wantDedup)
			}
			if tt.opts.BufferSizesOption() == nil {
				t.Error("BufferSizesOption() = nil, want non-nil")
			}
		})
	}
}
