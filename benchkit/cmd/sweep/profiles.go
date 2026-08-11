package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/pprof/profile"
)

// Profile artifact extensions. The benchmark binary writes these next to its
// result file when sweep passes -cpuprofile/-memprofile (see -collect-profiles),
// and sweep downloads them alongside the result files.
const (
	cpuProfExt = ".cpu.prof"
	memProfExt = ".mem.prof"
)

// mergeCPUProfiles merges every *.cpu.prof in dir into dir/default.pgo, the
// filename the Go toolchain picks up for profile-guided optimization when
// placed in a main package directory. Unreadable profiles abort the merge so a
// corrupt input never silently skews the PGO profile.
func mergeCPUProfiles(dir string) error {
	paths, err := filepath.Glob(filepath.Join(dir, "*"+cpuProfExt))
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no %s files in %s", cpuProfExt, dir)
	}
	profiles := make([]*profile.Profile, len(paths))
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if profiles[i], err = profile.ParseData(data); err != nil {
			return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
		}
	}
	merged, err := profile.Merge(profiles)
	if err != nil {
		return fmt.Errorf("merge %d profiles: %w", len(profiles), err)
	}
	f, err := os.Create(filepath.Join(dir, "default.pgo"))
	if err != nil {
		return err
	}
	if err := merged.Write(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
