package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/relab/gorums/benchmark"
	"google.golang.org/grpc"
)

var (
	traceFile  = flag.String("trace", "", "File to write trace to.")
	cpuprofile = flag.String("cpuprofile", "", "File to write cpu profile to.")
	memprofile = flag.String("memprofile", "", "File to write memory profile to.")
	remotes    = flag.String("remotes", "", "List of remote servers to connect to.")
	benchmarks = flag.String("benchmarks", "", "List of benchmarks to run.")
	warmup     = flag.Int("warmup", 100, "Number of requests to send as warmup.")
	benchTime  = flag.String("time", "10s", "How long to run each benchmark")
	payload    = flag.Int("payload", 0, "Size of the payload in request and response messages (in bytes).")
)

type benchFunc func(context.Context, *benchmark.Echo, ...grpc.CallOption) (*benchmark.Echo, error)

var allBenchmarks map[string]benchFunc

func buildBenchmarksMap(cfg *benchmark.Configuration) {
	allBenchmarks = make(map[string]benchFunc)
	allBenchmarks["OrderedQC"] = cfg.OrderedQC
	allBenchmarks["UnorderedQC"] = cfg.UnorderedQC
}

func runAllBenchmarks() {
	duration, err := time.ParseDuration(*benchTime)
	if err != nil {
		log.Fatalf("Failed to parse 'time': %v\n", err)
	}

	var mgrOpts = []benchmark.ManagerOption{
		benchmark.WithGrpcDialOptions(grpc.WithBlock(), grpc.WithInsecure()),
		benchmark.WithDialTimeout(10 * time.Second),
	}

	if trace.IsEnabled() {
		mgrOpts = append(mgrOpts, benchmark.WithTracing())
	}

	if len(*remotes) < 1 {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		startLocalServers(ctx, 4)
	}

	mgr, err := benchmark.NewManager(strings.Split(*remotes, ","), mgrOpts...)
	if err != nil {
		log.Fatalf("Failed to create manager: %v\n", err)
	}
	defer mgr.Close()

	numNodes, _ := mgr.Size()
	cfg, err := mgr.NewConfiguration(mgr.NodeIDs(), &benchmark.QSpec{QSize: numNodes / 2})

	buildBenchmarksMap(cfg)

	msg := &benchmark.Echo{Payload: make([]byte, *payload)}

	resultWriter := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	fmt.Fprintln(resultWriter, "Benchmark\tThrougput\tLatency\tStd.dev\tMemory Usage\t Memory Allocs\t")

	for _, b := range strings.Split(*benchmarks, ",") {
		if f, ok := allBenchmarks[b]; ok {
			fmt.Fprintf(resultWriter, "%s-%d\t", b, runtime.NumCPU())
			result := runBenchmark(f, msg, duration).GetResult()
			fmt.Fprintf(resultWriter, "%s\n", result)
		}
	}
	resultWriter.Flush()
}

func runBenchmark(f benchFunc, msg *benchmark.Echo, duration time.Duration) *benchmark.Stats {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &benchmark.Stats{}

	for i := 0; i < *warmup; i++ {
		_, err := f(ctx, msg)
		if err != nil {
			log.Fatalf("An error occurred during benchmarking: %v\n", err)
		}
	}

	endTime := time.Now().Add(duration)
	s.Start()
	for !time.Now().After(endTime) {
		start := time.Now()
		_, err := f(ctx, msg)
		if err != nil {
			log.Fatalf("An error occurred during benchmarking: %v\n", err)
		}
		end := time.Now()
		s.AddLatency(end.Sub(start))
	}
	s.End()

	return s
}

func startLocalServers(ctx context.Context, n int) {
	var ports []string
	basePort := 40000
	var servers []*benchmark.Server
	for p := basePort; p < basePort+n; p++ {
		port := fmt.Sprintf(":%d", p)
		ports = append(ports, port)
		lis, err := net.Listen("tcp", port)
		if err != nil {
			log.Fatalf("Failed to start local server: %v\n", err)
		}
		srv := benchmark.NewServer()
		servers = append(servers, srv)
		go srv.Serve(lis)
	}
	*remotes = strings.Join(ports, ",")
	go func() {
		<-ctx.Done()
		for _, srv := range servers {
			srv.Stop()
		}
	}()
}

func main() {
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

	runAllBenchmarks()
}
