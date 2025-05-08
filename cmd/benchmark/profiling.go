package main

import (
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"

	"github.com/relab/gorums/benchmark"
)

// StartCPUProfile starts a CPU profile that will be written to the given path.
// Returns a function to stop the profiler.
func StartCPUProfile(cpuProfileFile string) (stop func() error, err error) {
	cpuProfile, err := os.Create(cpuProfileFile)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		return nil, err
	}
	return func() error {
		pprof.StopCPUProfile()
		err = cpuProfile.Close()
		if err != nil {
			return err
		}
		return nil
	}, nil
}

// WriteMemProfile writes a memory profile to the given path.
func WriteMemProfile(memProfileFile string) error {
	f, err := os.Create(memProfileFile)
	if err != nil {
		return err
	}
	runtime.GC() // get up-to-date statistics
	if err := pprof.WriteHeapProfile(f); err != nil {
		return err
	}
	err = f.Close()
	return err
}

// StartTrace starts a program trace using the "runtime/trace" package.
// Returns a function to stop the trace.
func StartTrace(traceFile string) (stop func() error, err error) {
	f, err := os.Create(traceFile)
	if err != nil {
		return nil, err
	}
	if err := trace.Start(f); err != nil {
		return nil, err
	}
	return func() error {
		trace.Stop()
		err = f.Close()
		if err != nil {
			return err
		}
		return nil
	}, nil
}

// StartProfilers starts various profilers and returns a function to stop them.
func StartProfilers(options benchmark.Options) (stopProfile func() error, err error) {
	nilFunc := func() error { return nil }

	var (
		cpuProfileStop = nilFunc
		traceStop      = nilFunc
	)

	if options.CpuProfileFile != "" {
		cpuProfileStop, err = StartCPUProfile(options.CpuProfileFile)
		if err != nil {
			return nil, err
		}
	}

	if options.TraceFile != "" {
		traceStop, err = StartTrace(options.TraceFile)
		if err != nil {
			return nil, err
		}
	}

	return func() error {
		err := cpuProfileStop()
		if err != nil {
			return err
		}
		err = traceStop()
		if err != nil {
			return err
		}
		if options.MemProfileFile != "" {
			err = WriteMemProfile(options.MemProfileFile)
		}
		return err
	}, nil
}
