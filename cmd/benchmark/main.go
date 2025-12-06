package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/benchmark"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type regexpFlag struct {
	val *regexp.Regexp
}

func (f *regexpFlag) String() string {
	if f.val == nil {
		return ""
	}
	return fmt.Sprintf("'%s'", f.val.String())
}

func (f *regexpFlag) Set(v string) (err error) {
	f.val, err = regexp.Compile(v)
	return
}

func (f *regexpFlag) Get() *regexp.Regexp {
	return f.val
}

type listFlag struct {
	val []string
}

func (f *listFlag) String() string {
	return strings.Join(f.val, ",")
}

func (f *listFlag) Set(v string) error {
	f.val = strings.Split(v, ",")
	return nil
}

func (f *listFlag) Get() []string {
	return f.val
}

func listBenchmarks() {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	benchmarks := benchmark.GetBenchmarks(nil)
	for _, b := range benchmarks {
		fmt.Fprintf(tw, "%s:\t%s\n", b.Name, b.Description)
	}
	tw.Flush()
}

func runServer(server string, serverBuffer uint) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	lis, err := net.Listen("tcp", server)
	checkf("Failed to listen on '%s': %v", server, err)

	srv := benchmark.NewBenchServer(gorums.WithReceiveBufferSize(serverBuffer))
	go func() { checkf("serve failed: %v", srv.Serve(lis)) }()

	fmt.Printf("Running benchmark server on '%s'\n", server)

	<-signals
	srv.Stop()
}

func printResults(results []*benchmark.Result, options benchmark.Options, serverStats bool) {
	resultWriter := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	fmt.Fprint(resultWriter, "Benchmark\tThroughput\tLatency\tStd.dev\tClient")
	if !serverStats || !options.Remote {
		fmt.Fprint(resultWriter, "+Servers\t\t")
	} else if serverStats {
		fmt.Fprint(resultWriter, "\t\t")
		for i := 1; i <= options.NumNodes; i++ {
			fmt.Fprintf(resultWriter, "Server %d\t\t", i)
		}
	}
	fmt.Fprintln(resultWriter)
	for _, r := range results {
		if !serverStats && options.Remote {
			for _, s := range r.GetServerStats() {
				r.SetMemPerOp(r.GetMemPerOp() + s.GetMemory()/r.GetTotalOps())
				r.SetAllocsPerOp(r.GetAllocsPerOp() + s.GetAllocs()/r.GetTotalOps())
			}
		}
		fmt.Fprint(resultWriter, r.Format())
		if serverStats && options.Remote {
			for _, s := range r.GetServerStats() {
				fmt.Fprintf(resultWriter, "%d B/op\t%d allocs/op\t", s.GetMemory()/r.GetTotalOps(), s.GetAllocs()/r.GetTotalOps())
			}
		}
		fmt.Fprintln(resultWriter)
	}
	resultWriter.Flush()
}

func main() {
	var (
		benchmarksFlag = regexpFlag{val: regexp.MustCompile(".*")}
		remotesFlag    = listFlag{}
		warmupFlag     = flag.Duration("warmup", 100*time.Millisecond, "Warmup duration.")
		benchTimeFlag  = flag.Duration("time", 1*time.Second, "The duration of each benchmark.")
		traceFile      = flag.String("trace", "", "A `file` to write trace to.")
		cpuprofile     = flag.String("cpuprofile", "", "A `file` to write cpu profile to.")
		memprofile     = flag.String("memprofile", "", "A `file` to write memory profile to.")
		payload        = flag.Int("payload", 0, "Size of the payload in request and response messages (in bytes).")
		concurrent     = flag.Int("concurrent", 1, "Number of goroutines that can make calls concurrently.")
		maxAsync       = flag.Int("max-async", 1000, "Maximum number of async calls that can be in flight at once.")
		server         = flag.String("server", "", "Run a benchmark server on given `address`.")
		serverStats    = flag.Bool("server-stats", false, "Show server statistics separately")
		cfgSize        = flag.Int("config-size", 4, "Size of the configuration to use. If < 1, all nodes will be used.")
		qSize          = flag.Int("quorum-size", 0, "Number of replies to wait for before completing a quorum call.")
		sendBuffer     = flag.Uint("send-buffer", 0, "The size of the send buffer.")
		serverBuffer   = flag.Uint("server-buffer", 0, "The size of the server buffers.")
		list           = flag.Bool("list", false, "List all available benchmarks")
	)
	flag.Var(&benchmarksFlag, "benchmarks", "A `regexp` matching the benchmarks to run.")
	flag.Var(&remotesFlag, "remotes", "A comma separated `list` of remote addresses to connect to.")
	flag.Parse()

	benchReg := benchmarksFlag.Get()
	remotes := remotesFlag.Get()

	if *list {
		listBenchmarks()
		return
	}

	stopProfilers, err := StartProfilers(*cpuprofile, *memprofile, *traceFile)
	checkf("Failed to start profiling: %v", err)
	defer func() {
		checkf("Failed to stop profiling: %v", stopProfilers())
	}()

	if *server != "" {
		runServer(*server, *serverBuffer)
		return
	}

	var options benchmark.Options
	options.Concurrent = *concurrent
	options.MaxAsync = *maxAsync
	options.Payload = *payload
	options.Warmup = *warmupFlag
	options.Duration = *benchTimeFlag
	options.Remote = true

	// start local servers if needed
	if len(remotes) < 1 {
		options.Remote = false
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		remotes = benchmark.StartLocalServers(ctx, *cfgSize, gorums.WithReceiveBufferSize(*serverBuffer))
	}

	numNodes := len(remotes)
	if *cfgSize < 1 || *cfgSize > numNodes {
		options.NumNodes = numNodes
	} else {
		options.NumNodes = *cfgSize
	}

	// find a valid value for QuorumSize
	switch {
	case options.NumNodes == 1:
		options.QuorumSize = 1
	case *qSize < 1:
		options.QuorumSize = options.NumNodes / 2 // the default value
	case *qSize > options.NumNodes:
		options.QuorumSize = options.NumNodes
	default:
		options.QuorumSize = *qSize
	}

	mgrOpts := []gorums.ManagerOption{
		gorums.WithDialOptions(
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		),
		gorums.WithSendBufferSize(*sendBuffer),
	}

	mgr := gorums.NewManager(mgrOpts...)
	defer mgr.Close()

	cfg, err := gorums.NewConfiguration(mgr, gorums.WithNodeList(remotes[:options.NumNodes]))
	checkf("Failed to create configuration: %v", err)

	results, err := benchmark.RunBenchmarks(benchReg, options, cfg)
	checkf("Error running benchmarks: %v", err)

	printResults(results, options, *serverStats)
}

func checkf(format string, args ...any) {
	for _, arg := range args {
		if err, _ := arg.(error); err != nil {
			fmt.Fprintf(os.Stderr, format, args...)
			os.Exit(1) // skipcq: RVV-A0003
		}
	}
}
