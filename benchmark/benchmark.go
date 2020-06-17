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
	Concurrent int           // Number of concurrent calls
	Duration   time.Duration // Duration of benchmark
	MaxAsync   int           // Max async calls at once
	NumNodes   int           // Number of nodes to include in configuration
	Payload    int           // Size of message payload
	QuorumSize int           // Number of messages to wait for
	Warmup     time.Duration // Warmup time
	Remote     bool          // Whether the servers are remote (true) or local (false)
}

type Bench struct {
	Name        string
	Description string
	runBench    benchFunc
}

type benchFunc func(Options) (*Result, error)
type qcFunc func(context.Context, *Echo) (*Echo, error)
type asyncQCFunc func(context.Context, *Echo) *FutureEcho
type serverFunc func(*TimedMsg) error

func runQCBenchmark(opts Options, cfg *Configuration, f qcFunc) (*Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg := &Echo{Payload: make([]byte, opts.Payload)}
	s := &Stats{}
	var g errgroup.Group

	for n := 0; n < opts.Concurrent; n++ {
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
	for n := 0; n < opts.Concurrent; n++ {
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
		result.ServerStats = memStats.MemoryStats
	}

	return result, nil
}

func runAsyncQCBenchmark(opts Options, cfg *Configuration, f asyncQCFunc) (*Result, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg := &Echo{Payload: make([]byte, opts.Payload)}
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
				warmupFunc()
				return nil
			})
		}
		return nil
	}

	for n := 0; n < opts.Concurrent; n++ {
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
				benchmarkFunc()
				return nil
			})
		}
		return nil
	}

	s.Start()
	for n := 0; n < opts.Concurrent; n++ {
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
		result.ServerStats = memStats.MemoryStats
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

	benchmarkFunc := func(stopTime time.Time) error {
		for !time.Now().After(stopTime) {
			msg := &TimedMsg{SendTime: time.Now().UnixNano(), Payload: payload}
			err := f(msg)
			if err != nil {
				return err
			}
		}
		return nil
	}

	warmupEnd := time.Now().Add(opts.Warmup)
	for n := 0; n < opts.Concurrent; n++ {
		g.Go(func() error { return benchmarkFunc(warmupEnd) })
	}
	g.Wait()

	_, err := cfg.StartServerBenchmark(ctx, &StartRequest{})
	if err != nil {
		return nil, err
	}

	runtime.ReadMemStats(&start)
	endTime := time.Now().Add(opts.Duration)
	for n := 0; n < opts.Concurrent; n++ {
		g.Go(func() error { return benchmarkFunc(endTime) })
	}
	g.Wait()
	runtime.ReadMemStats(&end)

	resp, err := cfg.StopServerBenchmark(ctx, &StopRequest{})
	if err != nil {
		return nil, err
	}

	clientAllocs := (end.Mallocs - start.Mallocs) / resp.TotalOps
	clientMem := (end.TotalAlloc - start.TotalAlloc) / resp.TotalOps

	resp.AllocsPerOp = clientAllocs
	resp.MemPerOp = clientMem
	return resp, nil
}

func GetBenchmarks(cfg *Configuration) []Bench {
	m := []Bench{
		{
			Name:        "UnorderedQC",
			Description: "Unary RPC based quorum call implementation without FIFO ordering",
			runBench: func(opts Options) (*Result, error) {
				return runQCBenchmark(opts, cfg, func(ctx context.Context, msg *Echo) (*Echo, error) {
					return cfg.UnorderedQC(ctx, msg)
				})
			},
		},
		{
			Name:        "OrderedQC",
			Description: "NodeStream based quorum call implementation with FIFO ordering",
			runBench:    func(opts Options) (*Result, error) { return runQCBenchmark(opts, cfg, cfg.OrderedQC) },
		},
		{
			Name:        "ConcurrentQC",
			Description: "NodeStream based quorum call implementation with concurrent handlers and no FIFO ordering",
			runBench:    func(opts Options) (*Result, error) { return runQCBenchmark(opts, cfg, cfg.ConcurrentQC) },
		},
		{
			Name:        "UnorderedAsync",
			Description: "Unary RPC based async quorum call implementation without FIFO ordering",
			runBench: func(opts Options) (*Result, error) {
				return runAsyncQCBenchmark(opts, cfg, func(ctx context.Context, msg *Echo) *FutureEcho {
					return cfg.UnorderedAsync(ctx, msg)
				})
			},
		},
		{
			Name:        "OrderedAsync",
			Description: "NodeStream based async quorum call implementation with FIFO ordering",
			runBench:    func(opts Options) (*Result, error) { return runAsyncQCBenchmark(opts, cfg, cfg.OrderedAsync) },
		},
		{
			Name:        "ConcurrentAsync",
			Description: "NodeStream based async quorum call implementation with concurrent handlers and no FIFO ordering",
			runBench:    func(opts Options) (*Result, error) { return runAsyncQCBenchmark(opts, cfg, cfg.ConcurrentAsync) },
		},
		{
			Name:        "UnorderedSlowServer",
			Description: "UnorderedQC with a 10ms processing time on the server",
			runBench: func(opts Options) (*Result, error) {
				return runQCBenchmark(opts, cfg, func(ctx context.Context, msg *Echo) (*Echo, error) {
					return cfg.UnorderedSlowServer(ctx, msg)
				})
			},
		},
		{
			Name:        "OrderedSlowServer",
			Description: "OrderedQC with a 10s processing time on the server",
			runBench:    func(opts Options) (*Result, error) { return runQCBenchmark(opts, cfg, cfg.OrderedSlowServer) },
		},
		{
			Name:        "ConcurrentSlowServer",
			Description: "ConcurrentQC with a 10s processing time on the server",
			runBench:    func(opts Options) (*Result, error) { return runQCBenchmark(opts, cfg, cfg.ConcurrentSlowServer) },
		},
		{
			Name:        "Multicast",
			Description: "NodeStream based multicast implementation (servers measure latency and throughput)",
			runBench:    func(opts Options) (*Result, error) { return runServerBenchmark(opts, cfg, cfg.Multicast) },
		},
		{
			Name:        "ConcurrentMulticast",
			Description: "NodeStream based multicast implementation with concurrent handlers (servers measuer latency and throughput)",
			runBench: func(opts Options) (*Result, error) {
				return runServerBenchmark(opts, cfg, cfg.ConcurrentMulticast)
			},
		},
	}
	return m
}

// RunBenchmarks runs all the benchmarks that match the given regex with the given options
func RunBenchmarks(benchRegex *regexp.Regexp, options Options, manager *Manager) ([]*Result, error) {
	nodeIDs := manager.NodeIDs()
	cfg, err := manager.NewConfiguration(nodeIDs[:options.NumNodes], &QSpec{QSize: options.QuorumSize, CfgSize: options.NumNodes})
	if err != nil {
		return nil, err
	}
	benchmarks := GetBenchmarks(cfg)
	var results []*Result
	for _, b := range benchmarks {
		if benchRegex.MatchString(b.Name) {
			result, err := b.runBench(options)
			if err != nil {
				return nil, err
			}
			result.Name = b.Name
			i := sort.Search(len(results), func(i int) bool {
				return results[i].Name >= result.Name
			})
			results = append(results, nil)
			copy(results[i+1:], results[i:])
			results[i] = result
		}
	}
	return results, nil
}
