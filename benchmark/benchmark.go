package benchmark

import (
	context "context"
	"regexp"
	"runtime"
	"sort"
	"sync/atomic"
	"time"

	gorums "github.com/relab/gorums"
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

// Bench is a Benchmark with a name and description
type Bench struct {
	Name        string
	Description string
	runBench    benchFunc
}

type (
	benchFunc  func(Options) (*Result, error)
	qcFunc     func(context.Context, *Echo) gorums.Responses[*Echo]
	serverFunc func(context.Context, *TimedMsg)
)

func runQCBenchmark(opts Options, cfg *gorums.Configuration, quorum int, f qcFunc) (*Result, error) {
	rpcCfg := BenchmarkConfigurationRpc(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg := Echo_builder{Payload: make([]byte, opts.Payload)}.Build()
	s := &Stats{}
	var g errgroup.Group

	for range opts.Concurrent {
		g.Go(func() error {
			warmupEnd := time.Now().Add(opts.Warmup)
			for !time.Now().After(warmupEnd) {
				err := qf(f(ctx, msg), quorum)
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
		err := qf(rpcCfg.StartBenchmark(ctx, &StartRequest{}), quorum)
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
				err := qf(f(ctx, msg), quorum)
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
		memStats, err := stopBenchmarkQF(rpcCfg.StopBenchmark(ctx, &StopRequest{}), cfg.Size())
		if err != nil {
			return nil, err
		}
		result.SetServerStats(memStats)
	}

	return result, nil
}

func runAsyncQCBenchmark(opts Options, cfg *gorums.Configuration, quorum int) (*Result, error) {
	rpcCfg := BenchmarkConfigurationRpc(cfg)
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
			responses := rpcCfg.AsyncQuorumCall(ctx, msg)
			g.Go(func() error {
				err := qf(responses, quorum)
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
		err := qf(rpcCfg.StartBenchmark(ctx, &StartRequest{}), cfg.Size())
		if err != nil {
			return nil, err
		}
	}

	endTime := time.Now().Add(opts.Duration)
	var benchmarkFunc func() error
	benchmarkFunc = func() error {
		for ; !time.Now().After(endTime) && atomic.LoadUint64(&async) < uint64(opts.MaxAsync); atomic.AddUint64(&async, 1) {
			start := time.Now()
			responses := rpcCfg.AsyncQuorumCall(ctx, msg)
			g.Go(func() error {
				err := qf(responses, quorum)
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
		memStats, err := stopBenchmarkQF(BenchmarkConfigurationRpc(cfg).StopBenchmark(ctx, &StopRequest{}), cfg.Size())
		if err != nil {
			return nil, err
		}
		result.SetServerStats(memStats)
	}

	return result, nil
}

func runServerBenchmark(opts Options, cfg *gorums.Configuration, f serverFunc) (*Result, error) {
	rpcCfg := BenchmarkConfigurationRpc(cfg)
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

	err = qf(rpcCfg.StartServerBenchmark(ctx, &StartRequest{}), cfg.Size())
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

	resp, err := stopServerBenchmarkQF(rpcCfg.StopServerBenchmark(ctx, &StopRequest{}), cfg.Size())
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
func GetBenchmarks(cfg *gorums.Configuration, quorum int) []Bench {
	rpcCfg := BenchmarkConfigurationRpc(cfg)
	m := []Bench{
		{
			Name:        "QuorumCall",
			Description: "NodeStream based quorum call implementation with FIFO ordering",
			runBench: func(opts Options) (*Result, error) {
				return runQCBenchmark(opts, cfg, quorum, rpcCfg.QuorumCall)
			},
		},
		{
			Name:        "AsyncQuorumCall",
			Description: "NodeStream based async quorum call implementation with FIFO ordering",
			runBench:    func(opts Options) (*Result, error) { return runAsyncQCBenchmark(opts, cfg, quorum) },
		},
		{
			Name:        "SlowServer",
			Description: "Quorum Call with a 10s processing time on the server",
			runBench: func(opts Options) (*Result, error) {
				return runQCBenchmark(opts, cfg, quorum, rpcCfg.SlowServer)
			},
		},
		{
			Name:        "Multicast",
			Description: "NodeStream based multicast implementation (servers measure latency and throughput)",
			runBench: func(opts Options) (*Result, error) {
				return runServerBenchmark(opts, cfg, func(ctx context.Context, msg *TimedMsg) { rpcCfg.Multicast(ctx, msg) })
			},
		},
	}
	return m
}

// RunBenchmarks runs all the benchmarks that match the given regex with the given options
func RunBenchmarks(benchRegex *regexp.Regexp, options Options, cfg *gorums.Configuration, quorum int) ([]*Result, error) {
	benchmarks := GetBenchmarks(cfg, quorum)
	var results []*Result
	for _, b := range benchmarks {
		if benchRegex.MatchString(b.Name) {
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
