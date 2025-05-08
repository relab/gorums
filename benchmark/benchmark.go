package benchmark

import (
	context "context"
	"regexp"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// Options controls different options for the benchmarks
type Options struct {
	// benchmark options
	Concurrent  int            // Number of concurrent calls
	Duration    time.Duration  // Duration of benchmark
	MaxAsync    int            // Max async calls at once
	Payload     int            // Size of message payload
	QuorumSize  int            // Number of messages to wait for
	Warmup      time.Duration  // Warmup time
	BenchRexExp *regexp.Regexp // Regular expression deciding which benchmarks to run, leave empty to run all benchmarks
	Remote      bool           // Whether the servers are remote (true) or local (false)
	ServerStats bool           // Show server statistics separately

	// client options
	SendBuffer uint     // The size of the out buffer channel to each node
	NumNodes   int      // Number of nodes to include in configuration
	Remotes    []string // list of servers to connect to

	// server options
	Server       string // The listener address
	ServerBuffer uint   // The size of the out buffer channel

	// profiling options
	CpuProfileFile string // cpu profile file name
	MemProfileFile string // memory profile file name
	TraceFile      string // execution trace file name

	// help options
	List bool // list all different benchmarks
}

// Bench is a Benchmark with a name and description
type Bench struct {
	Name        string
	Description string
	runBench    benchFunc
}

type (
	benchFunc   func(Options) (*Result, error)
	qcFunc      func(context.Context, *Echo) (*Echo, error)
	asyncQCFunc func(context.Context, *Echo) *AsyncEcho
	serverFunc  func(context.Context, *TimedMsg)
)

func runQCBenchmark(opts Options, cfg *Configuration, f qcFunc) (*Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
	s := &Stats{}
	var g errgroup.Group

	for range opts.Concurrent {
		g.Go(func() error {
			warmupEnd := time.Now().Add(opts.Warmup)
			for !time.Now().After(warmupEnd) {
				_, err := f(ctx, msg)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		return nil, err
	}

	if opts.Remote {
		_, err := cfg.StartBenchmark(ctx, &StartRequest{})
		if err != nil {
			return nil, err
		}
	}

	s.Start()
	for range opts.Concurrent {
		g.Go(func() error {
			endTime := time.Now().Add(opts.Duration)
			for !time.Now().After(endTime) {
				start := time.Now()
				_, err := f(ctx, msg)
				if err != nil {
					return err
				}
				s.AddLatency(time.Since(start))
			}
			return nil
		})
	}

	err = g.Wait()
	s.End()
	if err != nil {
		return nil, err
	}

	result := s.GetResult()
	if opts.Remote {
		memStats, err := cfg.StopBenchmark(ctx, &StopRequest{})
		if err != nil {
			return nil, err
		}
		result.SetServerStats(memStats.GetMemoryStats())
	}

	return result, nil
}

func runAsyncQCBenchmark(opts Options, cfg *Configuration, f asyncQCFunc) (*Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
	s := &Stats{}
	var g errgroup.Group

	warmupEnd := time.Now().Add(opts.Warmup)
	var async uint64

	var warmupFunc func() error
	warmupFunc = func() error {
		for ; !time.Now().After(warmupEnd) && atomic.LoadUint64(&async) < uint64(opts.MaxAsync); atomic.AddUint64(&async, 1) {
			fut := f(ctx, msg)
			g.Go(func() error {
				_, err := fut.Get()
				if err != nil {
					return err
				}
				atomic.AddUint64(&async, ^uint64(0))
				_ = warmupFunc()
				return nil
			})
		}
		return nil
	}

	for range opts.Concurrent {
		g.Go(warmupFunc)
	}
	err := g.Wait()
	if err != nil {
		return nil, err
	}

	if opts.Remote {
		_, err := cfg.StartBenchmark(ctx, &StartRequest{})
		if err != nil {
			return nil, err
		}
	}

	endTime := time.Now().Add(opts.Duration)
	var benchmarkFunc func() error
	benchmarkFunc = func() error {
		for ; !time.Now().After(endTime) && atomic.LoadUint64(&async) < uint64(opts.MaxAsync); atomic.AddUint64(&async, 1) {
			start := time.Now()
			fut := f(ctx, msg)
			g.Go(func() error {
				_, err := fut.Get()
				if err != nil {
					return err
				}
				s.AddLatency(time.Since(start))
				atomic.AddUint64(&async, ^uint64(0))
				_ = benchmarkFunc()
				return nil
			})
		}
		return nil
	}

	s.Start()
	for range opts.Concurrent {
		g.Go(benchmarkFunc)
	}
	err = g.Wait()
	s.End()
	if err != nil {
		return nil, err
	}

	result := s.GetResult()
	if opts.Remote {
		memStats, err := cfg.StopBenchmark(ctx, &StopRequest{})
		if err != nil {
			return nil, err
		}
		result.SetServerStats(memStats.GetMemoryStats())
	}

	return result, nil
}

func runServerBenchmark(opts Options, cfg *Configuration, f serverFunc) (*Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	payload := make([]byte, opts.Payload)
	var g errgroup.Group
	var start runtime.MemStats
	var end runtime.MemStats

	benchmarkFunc := func(stopTime time.Time) {
		for !time.Now().After(stopTime) {
			msg := TimedMsg_builder{SendTime: time.Now().UnixNano(), Payload: payload}.Build()
			f(ctx, msg)
		}
	}

	warmupEnd := time.Now().Add(opts.Warmup)
	for range opts.Concurrent {
		go benchmarkFunc(warmupEnd)
	}
	err := g.Wait()
	if err != nil {
		return nil, err
	}

	_, err = cfg.StartServerBenchmark(ctx, &StartRequest{})
	if err != nil {
		return nil, err
	}

	runtime.ReadMemStats(&start)
	endTime := time.Now().Add(opts.Duration)
	for range opts.Concurrent {
		benchmarkFunc(endTime)
	}
	err = g.Wait()
	if err != nil {
		return nil, err
	}
	runtime.ReadMemStats(&end)

	resp, err := cfg.StopServerBenchmark(ctx, &StopRequest{})
	if err != nil {
		return nil, err
	}

	clientAllocs := (end.Mallocs - start.Mallocs) / resp.GetTotalOps()
	clientMem := (end.TotalAlloc - start.TotalAlloc) / resp.GetTotalOps()

	resp.SetAllocsPerOp(clientAllocs)
	resp.SetMemPerOp(clientMem)
	return resp, nil
}

// GetBenchmarks returns a list of Benchmarks that can be performed on the configuration
func GetBenchmarks(cfg *Configuration) []Bench {
	m := []Bench{
		{
			Name:        "QuorumCall",
			Description: "NodeStream based quorum call implementation with FIFO ordering",
			runBench:    func(opts Options) (*Result, error) { return runQCBenchmark(opts, cfg, cfg.QuorumCall) },
		},
		{
			Name:        "AsyncQuorumCall",
			Description: "NodeStream based async quorum call implementation with FIFO ordering",
			runBench:    func(opts Options) (*Result, error) { return runAsyncQCBenchmark(opts, cfg, cfg.AsyncQuorumCall) },
		},
		{
			Name:        "SlowServer",
			Description: "Quorum Call with a 10s processing time on the server",
			runBench:    func(opts Options) (*Result, error) { return runQCBenchmark(opts, cfg, cfg.SlowServer) },
		},
		{
			Name:        "Multicast",
			Description: "NodeStream based multicast implementation (servers measure latency and throughput)",
			runBench: func(opts Options) (*Result, error) {
				return runServerBenchmark(opts, cfg, func(ctx context.Context, msg *TimedMsg) { cfg.Multicast(ctx, msg) })
			},
		},
	}
	return m
}

// RunBenchmarks runs all the benchmarks that match the given regex with the given options
func RunBenchmarks(options Options, cfg *Configuration) ([]*Result, error) {
	benchmarks := GetBenchmarks(cfg)
	var results []*Result
	for _, b := range benchmarks {
		if options.BenchRexExp.MatchString(b.Name) {
			result, err := b.runBench(options)
			if err != nil {
				return nil, err
			}
			result.SetName(b.Name)
			i := sort.Search(len(results), func(i int) bool {
				return results[i].GetName() >= result.GetName()
			})
			results = append(results, nil)
			copy(results[i+1:], results[i:])
			results[i] = result
		}
	}
	return results, nil
}
