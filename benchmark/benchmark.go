package benchmark

import (
	context "context"
	"regexp"
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
}

type benchFunc func(Options) (*Stats, error)
type qcFunc func(context.Context, *Echo) (*Echo, error)
type asyncQCFunc func(context.Context, *Echo) *FutureEcho

func runQCBenchmark(opts Options, f qcFunc) (*Stats, error) {
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
				end := time.Now()
				s.AddLatency(end.Sub(start))
			}
			return nil
		})
	}

	err = g.Wait()
	s.End()

	if err != nil {
		return nil, err
	}
	return s, nil
}

func runAsyncQCBenchmark(opts Options, f asyncQCFunc) (*Stats, error) {
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
				end := time.Now()
				atomic.AddUint64(&async, ^uint64(0))
				s.AddLatency(end.Sub(start))
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
	return s, nil
}

func mapBenchmarks(cfg *Configuration) map[string]benchFunc {
	m := map[string]benchFunc{
		"UnorderedQC": func(opts Options) (*Stats, error) {
			return runQCBenchmark(opts, func(ctx context.Context, msg *Echo) (*Echo, error) {
				return cfg.UnorderedQC(ctx, msg)
			})
		},

		"OrderedQC": func(opts Options) (*Stats, error) { return runQCBenchmark(opts, cfg.OrderedQC) },

		"UnorderedAsync": func(opts Options) (*Stats, error) {
			return runAsyncQCBenchmark(opts, func(ctx context.Context, msg *Echo) *FutureEcho {
				return cfg.UnorderedAsync(ctx, msg)
			})
		},

		"OrderedAsync": func(opts Options) (*Stats, error) { return runAsyncQCBenchmark(opts, cfg.OrderedAsync) },

		"UnorderedSlowServer": func(opts Options) (*Stats, error) {
			return runQCBenchmark(opts, func(ctx context.Context, msg *Echo) (*Echo, error) {
				return cfg.UnorderedSlowServer(ctx, msg)
			})
		},

		"OrderedSlowServer": func(opts Options) (*Stats, error) { return runQCBenchmark(opts, cfg.OrderedSlowServer) },
	}
	return m
}

// RunBenchmarks runs all the benchmarks that match the given regex with the given options
func RunBenchmarks(benchRegex *regexp.Regexp, options Options, manager *Manager) ([]Result, error) {
	nodeIDs := manager.NodeIDs()
	cfg, err := manager.NewConfiguration(nodeIDs[:options.NumNodes], &QSpec{QSize: options.QuorumSize})
	if err != nil {
		return nil, err
	}
	benchmarks := mapBenchmarks(cfg)
	var results []Result
	for b, f := range benchmarks {
		if benchRegex.MatchString(b) {
			stats, err := f(options)
			if err != nil {
				return nil, err
			}
			result := stats.GetResult()
			result.Name = b
			results = append(results, result)
		}
	}
	return results, nil
}
