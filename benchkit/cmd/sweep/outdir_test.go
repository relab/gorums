package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveOutputDir(t *testing.T) {
	now := time.Date(2026, 6, 27, 13, 2, 18, 0, time.UTC)
	tests := []struct {
		name          string
		rootDir       string
		sweepLabel    string
		sweepExplicit bool
		collectPath   string
		want          string
	}{
		{
			name:          "explicit sweep label",
			rootDir:       "results",
			sweepLabel:    "e1",
			sweepExplicit: true,
			want:          filepath.Join("results", "e1"),
		},
		{
			name:       "timestamped run",
			rootDir:    "results",
			sweepLabel: "run",
			want:       filepath.Join("results", "20260627_130218"),
		},
		{
			name:        "reconnect path",
			rootDir:     "results",
			collectPath: "/tmp/sweep-driver-e3-tlcurve-20260627_130218",
			want:        filepath.Join("results", "e3-tlcurve"),
		},
		{
			name:        "reconnect path with timestamp label",
			rootDir:     "results",
			collectPath: "/tmp/sweep-driver-20260627_130218-20260627_130219",
			want:        filepath.Join("results", "20260627_130218"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveOutputDir(tt.rootDir, now, tt.sweepLabel, tt.sweepExplicit, tt.collectPath)
			if got != tt.want {
				t.Fatalf("resolveOutputDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRotateExistingOutputDirUsesManifestTimestamp(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "e1")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	older := time.Date(2026, 6, 27, 12, 3, 4, 0, time.UTC)
	newer := older.Add(5 * time.Minute)
	writeTestManifest(t, path, "first", newer)
	writeTestManifest(t, path, "second", older)

	rotated, err := rotateExistingOutputDir(path)
	if err != nil {
		t.Fatalf("rotateExistingOutputDir: %v", err)
	}
	want := filepath.Join(root, "e1-"+older.Format("20060102_150405"))
	if rotated != want {
		t.Fatalf("rotated = %q, want %q", rotated, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original path still exists: %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("rotated path missing: %v", err)
	}
}

func TestRotateExistingOutputDirUsesDirModTimeFallback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "e2")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stamp := time.Date(2026, 6, 27, 12, 34, 56, 0, time.Local)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	rotated, err := rotateExistingOutputDir(path)
	if err != nil {
		t.Fatalf("rotateExistingOutputDir: %v", err)
	}
	want := filepath.Join(root, "e2-"+stamp.Format("20060102_150405"))
	if rotated != want {
		t.Fatalf("rotated = %q, want %q", rotated, want)
	}
}

func writeTestManifest(t *testing.T, dir, base string, ts time.Time) {
	t.Helper()
	m := runManifest{Timestamp: ts.Format(time.RFC3339)}
	data, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	path := filepath.Join(dir, base+manifestSuffix)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestDisplayPath(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "under cwd",
			path: filepath.Join(cwd, "out", "dedup-recheck"),
			want: filepath.Join("out", "dedup-recheck"),
		},
		{
			name: "cwd itself",
			path: cwd,
			want: ".",
		},
		{
			name: "outside cwd",
			path: filepath.Join(filepath.Dir(cwd), "elsewhere"),
			want: filepath.Join("..", "elsewhere"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayPath(tt.path); got != tt.want {
				t.Fatalf("displayPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
