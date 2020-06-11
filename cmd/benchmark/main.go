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

func main() {
	var (
		traceFile   = flag.String("trace", "", "File to write trace to.")
		cpuprofile  = flag.String("cpuprofile", "", "File to write cpu profile to.")
		memprofile  = flag.String("memprofile", "", "File to write memory profile to.")
		remotes     = flag.String("remotes", "", "List of remote servers to connect to.")
		benchmarks  = flag.String("benchmarks", ".*", "List of benchmarks to run.")
		warmup      = flag.String("warmup", "100ms", "How long a warmup should last.")
		benchTime   = flag.String("time", "1s", "How long to run each benchmark")
		payload     = flag.Int("payload", 0, "Size of the payload in request and response messages (in bytes).")
		concurrent  = flag.Int("concurrent", 1, "Number of goroutines that can make calls concurrently.")
		maxAsync    = flag.Int("max-async", 1000, "Maximum number of async calls that can be in flight at once.")
		server      = flag.String("server", "", "Run a benchmark server on given address.")
		serverStats = flag.Bool("server-stats", false, "Show server statistics separately")
	)
	flag.Parse()

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
		signals := make(chan os.Signal)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

		lis, err := net.Listen("tcp", *server)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to listen on '%s': %v\n", *server, err)
			os.Exit(1)
		}
		srv := benchmark.NewServer()
		go srv.Serve(lis)

		fmt.Printf("Running benchmark server on '%s'\n", *server)

		<-signals
		srv.Stop()
		return
	}

	remote := true
	if len(*remotes) < 1 {
		remote = false
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ports := benchmark.StartLocalServers(ctx, 4)
		*remotes = strings.Join(ports, ",")
	}

	var mgrOpts = []benchmark.ManagerOption{
		benchmark.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
		benchmark.WithDialTimeout(10 * time.Second),
	}

	if trace.IsEnabled() {
		mgrOpts = append(mgrOpts, benchmark.WithTracing())
	}

	mgr, err := benchmark.NewManager(strings.Split(*remotes, ","), mgrOpts...)
	if err != nil {
		log.Fatalf("Failed to create manager: %v\n", err)
	}
	defer mgr.Close()

	var options benchmark.Options
	options.Concurrent = *concurrent
	options.MaxAsync = *maxAsync
	options.NumNodes, _ = mgr.Size()
	options.QuorumSize = options.NumNodes / 2
	options.Payload = *payload
	options.Warmup, err = time.ParseDuration(*warmup)
	options.Remote = remote

	if err != nil {
		log.Fatalf("Failed to parse 'warmup': %v\n", err)
	}
	options.Duration, err = time.ParseDuration(*benchTime)
	if err != nil {
		log.Fatalf("Failed to parse 'time': %v\n", err)
	}

	benchReg, err := regexp.Compile(*benchmarks)
	if err != nil {
		log.Fatalf("Could not parse regular expression: %v\n", err)
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
		for i := 1; i <= len(strings.Split(*remotes, ",")); i++ {
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
