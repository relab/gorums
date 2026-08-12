package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultOutRoot = "out"

// resolveOutputDir returns the concrete run directory under rootDir.
// Explicit sweep labels name the directory; unlabeled runs keep the timestamped layout.
// Reconnecting to a detached driver run reuses the run directory encoded in -collect.
func resolveOutputDir(rootDir string, now time.Time, sweepLabel string, sweepExplicit bool, collectPath string) string {
	if collectPath != "" {
		return filepath.Join(rootDir, runDirNameFromCollectPath(collectPath))
	}
	if sweepExplicit && sweepLabel != "" {
		return filepath.Join(rootDir, sweepLabel)
	}
	return filepath.Join(rootDir, now.Format("20060102_150405"))
}

// runDirNameFromCollectPath derives the run directory name from the driver's
// detached work directory. The launcher encodes the actual run directory name
// before the unique timestamp suffix, so reconnecting can recover it.
func runDirNameFromCollectPath(collectPath string) string {
	base := filepath.Base(filepath.Clean(collectPath))
	const prefix = "sweep-driver-"
	if !strings.HasPrefix(base, prefix) {
		return base
	}
	base = strings.TrimPrefix(base, prefix)
	if i := strings.LastIndex(base, "-"); i >= 0 {
		return base[:i]
	}
	return base
}

// displayPath returns path relative to the current working directory, so log
// output shows a short, copy-pasteable path (e.g. "out/label" or
// "../data/label") instead of the absolute path that cfg.outDir carries
// internally. It falls back to path unchanged when the working directory is
// unknown or no relative form exists.
func displayPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	return rel
}

// prepareOutputDir creates path and, when it already exists, moves the existing
// directory aside first.
func prepareOutputDir(path string) error {
	if moved, err := rotateExistingOutputDir(path); err != nil {
		return err
	} else if moved != "" {
		log.Printf("existing output directory moved aside: %s -> %s", path, moved)
	}
	return os.MkdirAll(path, 0o755)
}

// rotateExistingOutputDir renames an existing sweep output directory to a
// timestamp-suffixed sibling and returns the new path. If path does not exist,
// it returns an empty string.
func rotateExistingOutputDir(path string) (string, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s exists and is not a directory", path)
	}

	stamp, ok := sweepDirTimestamp(path)
	if !ok {
		stamp = info.ModTime()
	}
	base := path + "-" + stamp.Format("20060102_150405")
	moved := base
	for i := 1; ; i++ {
		if _, err := os.Stat(moved); errors.Is(err, fs.ErrNotExist) {
			break
		}
		moved = fmt.Sprintf("%s-%d", base, i)
	}
	if err := os.Rename(path, moved); err != nil {
		return "", err
	}
	return moved, nil
}

// sweepDirTimestamp returns the earliest manifest timestamp in dir, or the
// directory mtime when no usable manifest exists.
func sweepDirTimestamp(dir string) (time.Time, bool) {
	matches, err := filepath.Glob(filepath.Join(dir, "*"+manifestSuffix))
	if err == nil {
		var (
			best  time.Time
			found bool
		)
		for _, path := range matches {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var m runManifest
			if err := json.Unmarshal(data, &m); err != nil || m.Timestamp == "" {
				continue
			}
			ts, err := time.Parse(time.RFC3339, m.Timestamp)
			if err != nil {
				continue
			}
			if !found || ts.Before(best) {
				best = ts
				found = true
			}
		}
		if found {
			return best, true
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}
