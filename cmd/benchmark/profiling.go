package main

import (
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
)

// StartCPUProfile starts a CPU profile that will be written to the given path.
// Returns a function to stop the profiler.
func StartCPUProfile(cpuProfilePath string) (stop func() error, err error) {
	cpuProfile, err := os.Create(cpuProfilePath)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		return nil, err
	}
	return func() error {
		// stop cpu profile
		pprof.StopCPUProfile()
		err = cpuProfile.Close()
		if err != nil {
			return err
		}
		return nil
	}, nil
}

// WriteMemProfile writes a memory profile to the given path.
func WriteMemProfile(memProfilePath string) error {
	f, err := os.Create(memProfilePath)
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
func StartTrace(tracePath string) (stop func() error, err error) {
	traceFile, err := os.Create(tracePath)
	if err != nil {
		return nil, err
	}
	if err := trace.Start(traceFile); err != nil {
		return nil, err
	}
	return func() error {
		// stop trace
		trace.Stop()
		err = traceFile.Close()
		if err != nil {
			return err
		}
		return nil
	}, nil
}

// StartProfilers starts various profilers and returns a function to stop them.
func StartProfilers(cpuProfilePath, memProfilePath, tracePath string) (stopProfile func() error, err error) {
	nilFunc := func() error { return nil }

	var (
		cpuProfileStop = nilFunc
		traceStop      = nilFunc
	)

	if cpuProfilePath != "" {
		cpuProfileStop, err = StartCPUProfile(cpuProfilePath)
		if err != nil {
			return nil, err
		}
	}

	if tracePath != "" {
		traceStop, err = StartTrace(tracePath)
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
		if memProfilePath != "" {
			err = WriteMemProfile(memProfilePath)
		}
		return err
	}, nil
}
