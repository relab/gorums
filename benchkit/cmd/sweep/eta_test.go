package main

import (
	"strings"
	"testing"
	"time"
)

func TestSweepForecast(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	const duration = 10 * time.Second
	tests := []struct {
		name                   string
		completed, total       int
		wantEarliest, wantLast time.Duration
	}{
		{
			name: "AllRuns", completed: 0, total: 10,
			wantEarliest: 100 * time.Second, wantLast: 250 * time.Second,
		},
		{
			name: "RemainingRuns", completed: 2, total: 10,
			wantEarliest: 80 * time.Second, wantLast: 200 * time.Second,
		},
		{
			name: "AllComplete", completed: 10, total: 10,
		},
		{
			name: "OvershootClampsToZero", completed: 11, total: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			earliest, latest, earliestFinish, latestFinish := sweepForecast(now, tt.completed, tt.total, duration)
			if earliest != tt.wantEarliest || latest != tt.wantLast {
				t.Errorf("range = %v-%v, want %v-%v", earliest, latest, tt.wantEarliest, tt.wantLast)
			}
			if !earliestFinish.Equal(now.Add(tt.wantEarliest)) || !latestFinish.Equal(now.Add(tt.wantLast)) {
				t.Errorf("finish range = %v-%v", earliestFinish, latestFinish)
			}
		})
	}
}

func TestSweepFactorBreakdown(t *testing.T) {
	tests := []struct {
		name string
		sc   sweepConfig
		want string
	}{
		{
			// The dedup-eval-v3 example: n:3 × workers:3 × payload:3 × stream:2 × reps:10.
			name: "MultiFactor",
			sc: sweepConfig{
				numNodes:    []int{9, 15, 29},
				workers:     []int{8, 16, 32},
				payloads:    []int{1024, 4096, 16384},
				rates:       []int{0},
				benchmarks:  []string{"SymmetricQuorumCall"},
				streamModes: []string{"dual", "dedup"},
				reps:        10,
			},
			want: "n:3 × workers:3 × payload:3 × stream:2 × reps:10",
		},
		{
			// Every factor is a single value: no breakdown to show.
			name: "SingleRun",
			sc: sweepConfig{
				numNodes: []int{9}, workers: []int{1}, payloads: []int{0},
				rates: []int{0}, benchmarks: []string{"X"}, streamModes: []string{"dual"}, reps: 1,
			},
			want: "",
		},
		{
			// An empty streamModes defaults to a single mode, so it is omitted.
			name: "EmptyStreamModesOmitted",
			sc: sweepConfig{
				numNodes: []int{3, 5}, workers: []int{1}, payloads: []int{0},
				rates: []int{0}, benchmarks: []string{"X"}, streamModes: nil, reps: 1,
			},
			want: "n:2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sweepFactorBreakdown(tt.sc); got != tt.want {
				t.Errorf("sweepFactorBreakdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSweepEstimateLine(t *testing.T) {
	t.Run("EmptySweepYieldsNoLine", func(t *testing.T) {
		if got := sweepEstimateLine(sweepConfig{}, 20*time.Second); got != "" {
			t.Errorf("sweepEstimateLine() = %q, want empty", got)
		}
	})

	t.Run("ReportsRunCountAndBreakdown", func(t *testing.T) {
		sc := sweepConfig{
			numNodes:    []int{9, 15},
			workers:     []int{8},
			payloads:    []int{1024},
			rates:       []int{0},
			benchmarks:  []string{"SymmetricQuorumCall"},
			streamModes: []string{"dual"},
			reps:        1,
		}
		got := sweepEstimateLine(sc, 20*time.Second)
		if !strings.HasPrefix(got, "estimated sweep time: 2 run(s) (n:2): 40s–") {
			t.Errorf("sweepEstimateLine() = %q", got)
		}
	})
}

func TestFormatETA(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "Seconds", d: 45 * time.Second, want: "45s"},
		{name: "SubMinuteRoundsToSeconds", d: 59500 * time.Millisecond, want: "60s"},
		{name: "Minutes", d: 12 * time.Minute, want: "12m"},
		{name: "MinutesRounded", d: 12*time.Minute + 20*time.Second, want: "12m"},
		{name: "Hours", d: time.Hour + 5*time.Minute, want: "1h05m"},
		{name: "HoursZeroPadMinutes", d: 2 * time.Hour, want: "2h00m"},
		{name: "Negative", d: -5 * time.Second, want: "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatETA(tt.d); got != tt.want {
				t.Errorf("formatETA(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
