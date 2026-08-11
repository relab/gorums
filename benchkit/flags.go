package benchkit

import (
	"flag"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// StandardFlags holds the CLI flag contract that every sweep-driven benchmark
// binary must accept (doc/benchkit.html, sections 9 and 11).
// A binary built on benchkit registers exactly this set via RegisterFlags, so
// it complies with the contract automatically; sweep launches it without knowing
// what the workload does.
type StandardFlags struct {
	Benchmarks     *regexp.Regexp // -benchmarks: regexp selecting benchmarks to run
	Self           string         // -self: this node's listen address; non-empty triggers distributed mode
	Remotes        []string       // -remotes: comma-separated peer addresses
	Workers        int            // -workers: concurrent worker goroutines
	Payload        int            // -payload: request/response payload size in bytes
	Rate           int            // -rate: target sends/sec per node; 0 = unlimited (saturating)
	Duration       time.Duration  // -time: measurement duration
	Output         string         // -output: result file path
	Verbose        bool           // -verbose: log connection progress
	StatsMode      StatsMode      // -stats-mode: aggregate latency backing store
	Interval       time.Duration  // -interval: ticker interval for per-interval metrics; 0 = disabled
	RateStep       int            // -rate-step: rate increment per ramp step; 0 = disabled
	RateStepMax    int            // -rate-step-max: maximum target rate for ramp; 0 = disabled
	StreamMode     string         // -stream-mode: symmetric stream topology (dual or dedup)
	CPUProfile     string         // -cpuprofile: CPU profile output path; empty = disabled
	MemProfile     string         // -memprofile: heap profile output path; empty = disabled
	Trace          string         // -trace: execution trace output path; empty = disabled
	FaultKillAfter time.Duration  // -fault-kill-after: exit cleanly after this duration; 0 = disabled
	CallTimeout    time.Duration  // -call-timeout: per-call deadline for quorum-call workloads; 0 = disabled
}

// RegisterFlags registers the standard flag contract on fs and returns a pointer
// to the values, populated after fs.Parse. A protocol binary calls this, adds any
// protocol-specific flags to the same FlagSet, parses, and passes Options() to
// Run. The default selector matches every benchmark.
func RegisterFlags(fs *flag.FlagSet) *StandardFlags {
	f := &StandardFlags{Benchmarks: regexp.MustCompile(".*")}
	fs.Func("benchmarks", "A `regexp` matching the benchmarks to run.", func(v string) (err error) {
		f.Benchmarks, err = regexp.Compile(v)
		return
	})
	fs.Func("remotes", "A comma-separated `list` of remote addresses to connect to.", func(v string) error {
		f.Remotes = strings.Split(v, ",")
		return nil
	})
	fs.IntVar(&f.Workers, "workers", 1, "Number of goroutines that can make calls concurrently.")
	fs.IntVar(&f.Payload, "payload", 0, "Size of the payload in request and response messages (in bytes).")
	fs.IntVar(&f.Rate, "rate", 0, "Target sends per second per node; 0 means unlimited (saturating).")
	fs.DurationVar(&f.Duration, "time", 1*time.Second, "The duration of each benchmark.")
	fs.StringVar(&f.Output, "output", "", "Write results to this `file`.")
	fs.StringVar(&f.Self, "self", "", "This node's listen `address`; triggers distributed mode.")
	fs.BoolVar(&f.Verbose, "verbose", false, "Log connection progress in distributed mode.")
	fs.DurationVar(&f.Interval, "interval", 500*time.Millisecond, "Ticker interval for per-interval metrics; 0 = disabled.")
	fs.IntVar(&f.RateStep, "rate-step", 0, "Rate increment per ramp step (ops/s); 0 = disabled (no ramp).")
	fs.IntVar(&f.RateStepMax, "rate-step-max", 0, "Maximum target rate for ramp (ops/s); 0 = disabled (no ramp).")
	fs.StringVar(&f.StreamMode, "stream-mode", "dual", "Symmetric stream topology: dual or dedup.")
	fs.StringVar(&f.CPUProfile, "cpuprofile", "", "A `file` to write cpu profile to.")
	fs.StringVar(&f.MemProfile, "memprofile", "", "A `file` to write memory profile to.")
	fs.StringVar(&f.Trace, "trace", "", "A `file` to write trace to.")
	fs.DurationVar(&f.FaultKillAfter, "fault-kill-after", 0, "Fault injection: exit cleanly after this duration; 0 = disabled (see ArmFaultInjection).")
	fs.DurationVar(&f.CallTimeout, "call-timeout", 0, "Per-call deadline for quorum-call workloads; a call stalled behind an unresponsive peer fails with DeadlineExceeded instead of hanging until run end. 0 = disabled.")
	fs.Func("stats-mode", "Aggregate latency backing store: `exact` (default) or hdr.", func(v string) error {
		switch v {
		case "exact":
			f.StatsMode = StatsMode_EXACT
		case "hdr":
			f.StatsMode = StatsMode_HDR
		default:
			return fmt.Errorf("unknown stats mode %q (want: exact or hdr)", v)
		}
		return nil
	})
	return f
}

// Options builds the run Options from the standard flags. Fields outside the CLI
// contract (NumNodes, Remote, QuorumSize, MaxAsync) are left zero for the caller
// to set from the run topology.
func (f *StandardFlags) Options() Options {
	return Options{
		Workers:     f.Workers,
		Payload:     f.Payload,
		Rate:        f.Rate,
		Duration:    f.Duration,
		StatsMode:   f.StatsMode,
		Interval:    f.Interval,
		RateStep:    f.RateStep,
		RateStepMax: f.RateStepMax,
		StreamMode:  f.StreamMode,
		CallTimeout: f.CallTimeout,
	}
}
