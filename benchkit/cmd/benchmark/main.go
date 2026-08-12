package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchkit"
	"github.com/relab/gorums/benchkit/benchmark"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// flags embeds the benchkit standard flag contract (benchmarks, self, remotes,
// workers, payload, rate, time, output, verbose) and adds the
// gorums-specific extras the reference tool exposes.
type flags struct {
	*benchkit.StandardFlags
	maxAsync    int
	server      string
	serverStats bool
	configSize  int
	qSize       int
	sendBuffer  uint
	recvBuffer  uint
	list        bool
	label       string
	compare     string
}

func parseFlags() *flags {
	// The standard contract flags are registered by benchkit so this binary
	// complies with what sweep launches; the extras below are gorums-specific.
	f := &flags{StandardFlags: benchkit.RegisterFlags(flag.CommandLine)}
	flag.IntVar(&f.maxAsync, "max-async", 1000, "Maximum number of async calls that can be in flight at once.")
	flag.StringVar(&f.server, "server", "", "Run a benchmark server on given `address`.")
	flag.BoolVar(&f.serverStats, "server-stats", false, "Show server statistics separately.")
	flag.IntVar(&f.configSize, "config-size", 4, "Size of the configuration to use. In local mode this is the number of in-process servers and must be >= 1. In coordinator mode, values < 1 or greater than the number of remotes use all remotes. Ignored in distributed mode (-self), where every remote is used.")
	flag.IntVar(&f.qSize, "quorum-size", 0, "Number of replies to wait for before completing a quorum call.")
	flag.UintVar(&f.sendBuffer, "send-buffer", 0, "The size of the client's (and server's reverse channel) send buffer.")
	flag.UintVar(&f.recvBuffer, "recv-buffer", 0, "The size of the server's receive buffer.")
	flag.BoolVar(&f.list, "list", false, "List all available benchmarks.")
	flag.StringVar(&f.label, "label", "", "Label for this run, stored in the output file.")
	flag.StringVar(&f.compare, "compare", "", "Compare against results in this `file`.")
	flag.Parse()
	mode, err := normalizeStreamMode(f.StreamMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	f.StreamMode = mode
	return f
}

// normalizeStreamMode validates the -stream-mode flag value and normalizes
// its default: "" and "dual" both resolve to "dual"; "dedup" passes through
// unchanged. Any other value is rejected.
func normalizeStreamMode(mode string) (string, error) {
	switch mode {
	case "", "dual":
		return "dual", nil
	case "dedup":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid -stream-mode %q (want: dual or dedup)", mode)
	}
}

func (f *flags) options() benchkit.Options {
	opts := f.StandardFlags.Options()
	opts.MaxAsync = f.maxAsync
	opts.SendBuffer, opts.RecvBuffer = f.bufferSizes()
	return opts
}

// bufferSizes resolves the -send-buffer and -recv-buffer flags to the actual
// capacities this run uses. A send-buffer flag left at its zero default is
// resolved here to gorums.DefaultSendBufferSize, the same substitution
// gorums.WithSendBufferSize applies internally, so recorded results show what
// actually ran rather than a zero that does not reflect it. Zero is already
// the real receive-buffer default, so it needs no such resolution.
func (f *flags) bufferSizes() (sendBuffer, recvBuffer uint) {
	sendBuffer = f.sendBuffer
	if sendBuffer == 0 {
		sendBuffer = gorums.DefaultSendBufferSize
	}
	return sendBuffer, f.recvBuffer
}

func (f *flags) dialOpts() []gorums.DialOption {
	return []gorums.DialOption{
		gorums.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
		gorums.WithSendBufferSize(f.sendBuffer),
	}
}

// target configures the benchmark target for one of three modes:
// distributed (-self set), local (no -remotes), or coordinator (-remotes set).
// It returns the target and a cleanup function that must be deferred by the
// caller. The mode selection and setup live in benchmark.SetupTarget.
func (f *flags) target(opts *benchkit.Options, dialOpts []gorums.DialOption) (benchmark.BenchTarget, func()) {
	target, cleanup, err := benchmark.SetupTarget(opts, f.Self, f.Remotes, f.configSize, dialOpts...)
	checkf("Failed to set up benchmark target: %v", err)
	return target, cleanup
}

// quorumSize resolves the -quorum-size flag against the configuration size,
// clamping it to numNodes. Unset, it is a majority. The threshold counts the
// local node's in-process reply, so anything below a majority can be satisfied
// without contacting a peer.
func (f *flags) quorumSize(numNodes int) int {
	switch {
	case f.qSize < 1:
		return numNodes/2 + 1
	case f.qSize > numNodes:
		return numNodes
	default:
		return f.qSize
	}
}

func (f *flags) report(results []*benchkit.Result, opts benchkit.Options) {
	benchkit.PrintResults(os.Stdout, results, opts, f.serverStats, f.Self)
	// In distributed mode -self names this node, so it labels the run when
	// -label is unset. The written report and the comparison use the same one.
	label := f.label
	if label == "" && f.Self != "" {
		label = f.Self
	}
	if f.Output != "" {
		checkf("Failed to write results: %v", benchkit.WriteLabeledReport(results, label, f.Output))
	}
	if f.compare != "" {
		checkf("Failed to compare results: %v", benchkit.CompareWithBaseline(f.compare, label, results, os.Stdout))
	}
}

func runServer(addr string, recvSize, sendSize uint) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	lis, err := net.Listen("tcp", addr)
	checkf("Failed to listen on '%s': %v", addr, err)

	srv := benchmark.NewBenchServer(gorums.WithBufferSizes(recvSize, sendSize))
	go func() { checkf("serve failed: %v", srv.Serve(lis)) }()
	benchkit.Logf("Running benchmark server on '%s'\n", addr)

	<-signals
	srv.Stop()
}

func main() {
	f := parseFlags()
	benchkit.SetVerbose(f.Verbose)

	if f.list {
		benchkit.ListBenches(os.Stdout, benchmark.BenchmarkDescriptions())
		return
	}

	stopProfilers, err := benchkit.StartProfilers(f.CPUProfile, f.MemProfile, f.Trace)
	checkf("Failed to start profiling: %v", err)
	defer func() { checkf("Failed to stop profiling: %v", stopProfilers()) }()

	benchkit.ArmFaultInjection(f.FaultKillAfter)

	if f.server != "" {
		runServer(f.server, f.recvBuffer, f.sendBuffer)
		return
	}

	opts := f.options()
	target, cleanup := f.target(&opts, f.dialOpts())
	defer cleanup()

	opts.QuorumSize = f.quorumSize(opts.NumNodes)

	results, err := benchmark.RunBenchmarks(f.Benchmarks, opts, target)
	checkf("Error running benchmarks: %v", err)

	f.report(results, opts)

	// In distributed mode the symmetric topology has no exit barrier: signal
	// peers that this node is done, then race that signal against a grace
	// deadline (deferred cleanup happens once one of the two resolves). See
	// benchmark.ExitGrace and benchmark.AwaitPeersDoneOrGrace.
	if f.Self != "" {
		nodeLabel := f.label
		if nodeLabel == "" {
			nodeLabel = f.Self
		}
		grace := benchmark.ExitGrace(opts.NumNodes)
		benchkit.Logf("[%s %s] Benchmark complete; signaling done, waiting up to %v for peers...\n", time.Now().Format(time.TimeOnly), nodeLabel, grace)
		benchmark.SignalDone(context.Background(), target.Symmetric)
		waitStart := time.Now()
		if benchmark.AwaitPeersDoneOrGrace(context.Background(), target.Symmetric, grace) {
			benchkit.Logf("[%s %s] All peers done after %v; exiting early.\n", time.Now().Format(time.TimeOnly), nodeLabel, time.Since(waitStart))
		} else {
			benchkit.Logf("[%s %s] Grace exhausted after %v; missing done from node(s) %v; exiting anyway.\n",
				time.Now().Format(time.TimeOnly), nodeLabel, time.Since(waitStart), benchmark.MissingDoneSenders(target.Symmetric))
		}
	}
}

func checkf(format string, args ...any) {
	for _, arg := range args {
		if err, _ := arg.(error); err != nil {
			fmt.Fprintf(os.Stderr, format, args...)
			os.Exit(1) // skipcq: RVV-A0003
		}
	}
}
