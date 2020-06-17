package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/relab/gorums/benchmark"
	"google.golang.org/grpc"
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

type durationFlag struct {
	val time.Duration
}

func (f *durationFlag) String() string {
	return f.val.String()
}

func (f *durationFlag) Set(v string) (err error) {
	f.val, err = time.ParseDuration(v)
	return err
}

func (f *durationFlag) Get() time.Duration {
	return f.val
}

func main() {
	var (
		benchmarksFlag = regexpFlag{val: regexp.MustCompile(".*")}
		remotesFlag    = listFlag{}
		warmupFlag     = durationFlag{val: 100 * time.Millisecond}
		benchTimeFlag  = durationFlag{val: 1 * time.Second}
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
	)
	flag.Var(&benchmarksFlag, "benchmarks", "A `regexp` matching the benchmarks to run.")
	flag.Var(&remotesFlag, "remotes", "A comma separated `list` of remote addresses to connect to.")
	flag.Var(&warmupFlag, "warmup", "Warmup `duration`.")
	flag.Var(&benchTimeFlag, "time", "The `duration` of each benchmark.")
	flag.Parse()

	benchReg := benchmarksFlag.Get()
	remotes := remotesFlag.Get()
	warmup := warmupFlag.Get()
	benchTime := benchTimeFlag.Get()

	// set up profiling and tracing
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("Could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("Could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	if *traceFile != "" {
		// TODO: not sure if enabling gRPC tracing is appropriate here
		grpc.EnableTracing = true
		f, err := os.Create(*traceFile)
		if err != nil {
			log.Fatal("Could not create trace file: ", err)
		}
		defer f.Close()
		if err := trace.Start(f); err != nil {
			log.Fatal("Failed to start trace: ", err)
		}
		defer trace.Stop()
	}

	defer func() {
		if *memprofile != "" {
			f, err := os.Create(*memprofile)
			if err != nil {
				log.Fatal("could not create memory profile: ", err)
			}
			defer f.Close()
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Fatal("could not write memory profile: ", err)
			}
		}
	}()

	if *server != "" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

		lis, err := net.Listen("tcp", *server)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to listen on '%s': %v\n", *server, err)
			os.Exit(1)
		}
		srv := benchmark.NewServer(benchmark.WithServerBufferSize(*serverBuffer))
		go srv.Serve(lis)

		fmt.Printf("Running benchmark server on '%s'\n", *server)

		<-signals
		srv.Stop()
		return
	}

	remote := true
	if len(remotes) < 1 {
		remote = false
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		remotes = benchmark.StartLocalServers(ctx, *cfgSize, benchmark.WithServerBufferSize(*serverBuffer))
	}

	var mgrOpts = []benchmark.ManagerOption{
		benchmark.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
		benchmark.WithDialTimeout(10 * time.Second),
		benchmark.WithSendBufferSize(*sendBuffer),
	}

	if trace.IsEnabled() {
		mgrOpts = append(mgrOpts, benchmark.WithTracing())
	}

	mgr, err := benchmark.NewManager(remotes, mgrOpts...)
	if err != nil {
		log.Fatalf("Failed to create manager: %v\n", err)
	}
	defer mgr.Close()

	var options benchmark.Options
	options.Concurrent = *concurrent
	options.MaxAsync = *maxAsync
	options.Payload = *payload
	options.Warmup = warmup
	options.Duration = benchTime
	options.Remote = remote

	numNodes, _ := mgr.Size()
	if *cfgSize < 1 || *cfgSize > numNodes {
		options.NumNodes = numNodes
	} else {
		options.NumNodes = *cfgSize
	}

	if options.NumNodes == 1 {
		options.QuorumSize = 1
	} else if *qSize < 1 {
		options.QuorumSize = options.NumNodes / 2
	} else if *qSize > options.NumNodes {
		options.QuorumSize = options.NumNodes
	} else {
		options.QuorumSize = *qSize
	}

	results, err := benchmark.RunBenchmarks(benchReg, options, mgr)
	if err != nil {
		log.Fatalf("Error running benchmarks: %v\n", err)
	}

	resultWriter := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	fmt.Fprint(resultWriter, "Benchmark\tThrougput\tLatency\tStd.dev\tClient")
	if !*serverStats || !remote {
		fmt.Fprint(resultWriter, "+Servers\t\t")
	} else if *serverStats {
		fmt.Fprint(resultWriter, "\t\t")
		for i := 1; i <= options.NumNodes; i++ {
			fmt.Fprintf(resultWriter, "Server %d\t\t", i)
		}
	}
	fmt.Fprintln(resultWriter)
	for _, r := range results {
		if !*serverStats && remote {
			for _, s := range r.ServerStats {
				r.MemPerOp += s.Memory / r.TotalOps
				r.AllocsPerOp += s.Allocs / r.TotalOps
			}
		}
		fmt.Fprint(resultWriter, r.Format())
		if *serverStats && remote {
			for _, s := range r.ServerStats {
				fmt.Fprintf(resultWriter, "%d B/op\t%d allocs/op\t", s.Memory/r.TotalOps, s.Allocs/r.TotalOps)
			}
		}
		fmt.Fprintln(resultWriter)
	}
	resultWriter.Flush()
}
