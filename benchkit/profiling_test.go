package benchkit

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStartProfilers verifies that StartProfilers writes a CPU profile, a
// memory profile, and an execution trace to the given paths, and that the
// returned stop function finalizes all three files.
func TestStartProfilers(t *testing.T) {
	dir := t.TempDir()
	cpu := filepath.Join(dir, "cpu.prof")
	mem := filepath.Join(dir, "mem.prof")
	trace := filepath.Join(dir, "trace.out")

	stop, err := StartProfilers(cpu, mem, trace)
	if err != nil {
		t.Fatalf("StartProfilers: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	for _, path := range []string{cpu, mem, trace} {
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("missing profile artifact: %v", err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", filepath.Base(path))
		}
	}
}

// TestStartProfilersDisabled verifies that empty paths disable all profilers:
// no files are created and the stop function is a no-op.
func TestStartProfilersDisabled(t *testing.T) {
	stop, err := StartProfilers("", "", "")
	if err != nil {
		t.Fatalf("StartProfilers: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

// TestStartProfilersStopsCPUProfileOnTraceSetupFailure verifies that a
// trace-setup failure does not leak the already-started CPU profiler: the
// runtime only allows one active CPU profile at a time, so a leaked profiler
// would make every subsequent [StartProfilers] call fail with "cpu profiling
// already in use".
func TestStartProfilersStopsCPUProfileOnTraceSetupFailure(t *testing.T) {
	dir := t.TempDir()
	cpu := filepath.Join(dir, "cpu.prof")
	// A trace path under a nonexistent directory makes os.Create fail inside
	// startTrace, exercising the setup-failure path in StartProfilers.
	badTracePath := filepath.Join(dir, "no-such-dir", "trace.out")

	if _, err := StartProfilers(cpu, "", badTracePath); err == nil {
		t.Fatal("StartProfilers with bad trace path = nil error, want error")
	}

	// If the CPU profiler were left running, this second call would fail
	// with "cpu profiling already in use" instead of succeeding.
	stop, err := StartProfilers(cpu, "", "")
	if err != nil {
		t.Fatalf("StartProfilers after prior failure: %v (CPU profiler was not stopped/cleaned up)", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

// TestStartCPUProfileReturnsErrorWhenAlreadyRunning verifies that
// startCPUProfile surfaces pprof.StartCPUProfile's error when a profile is
// already running, exercising the path where startCPUProfile must close the
// file it just created instead of leaking it before returning that error.
func TestStartCPUProfileReturnsErrorWhenAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.prof")
	stopFirst, err := startCPUProfile(first)
	if err != nil {
		t.Fatalf("startCPUProfile(first): %v", err)
	}
	defer func() {
		if err := stopFirst(); err != nil {
			t.Errorf("stopFirst: %v", err)
		}
	}()

	second := filepath.Join(dir, "second.prof")
	if _, err := startCPUProfile(second); err == nil {
		t.Fatal("startCPUProfile while a profile is already running = nil error, want error")
	}
}
