package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/pprof/profile"
)

// testCPUProfile builds a minimal valid CPU profile with one sample carrying
// the given cpu-nanoseconds value.
func testCPUProfile(t *testing.T, path string, value int64) {
	t.Helper()
	fn := &profile.Function{ID: 1, Name: "main.work", SystemName: "main.work", Filename: "work.go"}
	loc := &profile.Location{ID: 1, Line: []profile.Line{{Function: fn, Line: 1}}}
	p := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "samples", Unit: "count"},
			{Type: "cpu", Unit: "nanoseconds"},
		},
		Sample:     []*profile.Sample{{Location: []*profile.Location{loc}, Value: []int64{1, value}}},
		Location:   []*profile.Location{loc},
		Function:   []*profile.Function{fn},
		PeriodType: &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Period:     10_000_000,
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := p.Write(f); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

// TestMergeCPUProfiles verifies that mergeCPUProfiles merges every *.cpu.prof
// in the directory into a valid default.pgo whose sample values are the sum of
// the inputs.
func TestMergeCPUProfiles(t *testing.T) {
	dir := t.TempDir()
	testCPUProfile(t, filepath.Join(dir, "run_Q_N2_bb1_9000.cpu.prof"), 100)
	testCPUProfile(t, filepath.Join(dir, "run_Q_N2_bb2_9000.cpu.prof"), 250)

	if err := mergeCPUProfiles(dir); err != nil {
		t.Fatalf("mergeCPUProfiles: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "default.pgo"))
	if err != nil {
		t.Fatalf("read default.pgo: %v", err)
	}
	merged, err := profile.ParseData(data)
	if err != nil {
		t.Fatalf("parse default.pgo: %v", err)
	}
	var totalCPU int64
	for _, s := range merged.Sample {
		totalCPU += s.Value[1]
	}
	if totalCPU != 350 {
		t.Errorf("merged cpu value = %d, want 350", totalCPU)
	}
}

// TestMergeCPUProfilesNoInputs verifies that a directory without CPU profiles
// yields an error rather than an empty default.pgo.
func TestMergeCPUProfilesNoInputs(t *testing.T) {
	if err := mergeCPUProfiles(t.TempDir()); err == nil {
		t.Error("mergeCPUProfiles(empty dir) = nil error, want error")
	}
}
