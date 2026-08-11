package benchkit

import (
	"errors"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
)

// StartProfilers starts the profilers selected by non-empty paths — a CPU
// profile, a heap profile, and an execution trace — and returns a stop
// function that finalizes them. A binary built on benchkit wires the standard
// -cpuprofile/-memprofile/-trace flags (see [StandardFlags]) straight into this:
//
//	stop, err := benchkit.StartProfilers(f.CPUProfile, f.MemProfile, f.Trace)
//	...
//	defer stop()
//
// The CPU profile and trace run from this call until stop; the heap profile is
// written once at stop time, after a GC, so it reflects live allocations at the
// end of the run.
func StartProfilers(cpuProfilePath, memProfilePath, tracePath string) (stop func() error, err error) {
	nilFunc := func() error { return nil }

	var (
		cpuProfileStop = nilFunc
		traceStop      = nilFunc
	)

	if cpuProfilePath != "" {
		cpuProfileStop, err = startCPUProfile(cpuProfilePath)
		if err != nil {
			return nil, err
		}
	}

	if tracePath != "" {
		traceStop, err = startTrace(tracePath)
		if err != nil {
			// The CPU profiler, if started above, is still running with its
			// file open; stop it now so this setup failure doesn't leak it.
			return nil, errors.Join(err, cpuProfileStop())
		}
	}

	return func() error {
		// Run every finalizer even if an earlier one errors, so a failing CPU
		// profile stop cannot skip the trace stop or the heap profile.
		return errors.Join(cpuProfileStop(), traceStop(), writeMemProfileIfSet(memProfilePath))
	}, nil
}

// writeMemProfileIfSet writes a heap profile to memProfilePath, or does
// nothing if it is empty.
func writeMemProfileIfSet(memProfilePath string) error {
	if memProfilePath == "" {
		return nil
	}
	return writeMemProfile(memProfilePath)
}

// startCPUProfile starts a CPU profile that will be written to the given path.
// Returns a function to stop the profiler.
func startCPUProfile(cpuProfilePath string) (stop func() error, err error) {
	cpuProfile, err := os.Create(cpuProfilePath)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		// StartCPUProfile failed, so nothing will ever call Close on this
		// file; close it here instead of leaking the descriptor.
		_ = cpuProfile.Close()
		return nil, err
	}
	return func() error {
		pprof.StopCPUProfile()
		return cpuProfile.Close()
	}, nil
}

// writeMemProfile writes a heap profile to the given path.
func writeMemProfile(memProfilePath string) error {
	f, err := os.Create(memProfilePath)
	if err != nil {
		return err
	}
	runtime.GC() // get up-to-date statistics
	if err := pprof.WriteHeapProfile(f); err != nil {
		// WriteHeapProfile failed, so nothing will ever call Close on this
		// file; close it here instead of leaking the descriptor.
		_ = f.Close()
		return err
	}
	return f.Close()
}

// startTrace starts a program trace using the "runtime/trace" package.
// Returns a function to stop the trace.
func startTrace(tracePath string) (stop func() error, err error) {
	traceFile, err := os.Create(tracePath)
	if err != nil {
		return nil, err
	}
	if err := trace.Start(traceFile); err != nil {
		// trace.Start failed, so nothing will ever call Close on this file;
		// close it here instead of leaking the descriptor.
		_ = traceFile.Close()
		return nil, err
	}
	return func() error {
		trace.Stop()
		return traceFile.Close()
	}, nil
}
