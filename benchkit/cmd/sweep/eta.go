package main

import (
	"fmt"
	"strings"
	"time"
)

// perRunOverhead is the rough per-run cost outside the measurement window
// (-time): killing lingering processes, checking that ports are free, launching
// the nodes, the AwaitReady handshake and clock sync, the post-run exit grace
// (which scales with the node count and dominates this term), and downloading
// the result files. -trim does not appear here: it only drops warmup samples
// when summarizing and never extends a run's wall-clock. This constant forms
// the upper bound of the static estimate; actual first-run and elapsed timings
// never recalibrate the displayed range.
const perRunOverhead = 15 * time.Second

// sweepFactorBreakdown renders the multiplicative factors behind the run count,
// e.g. "n:3 × workers:3 × payload:3 × stream:2 × reps:10", so the up-front
// estimate shows how the sweep parameters produce the total. Factors that
// contribute a single value are omitted to keep the line readable; the result
// is "" when every factor is 1 (a single run).
func sweepFactorBreakdown(sc sweepConfig) string {
	streamModes := max(len(sc.streamModes), 1) // empty defaults to a single mode (dual)
	factors := []struct {
		name string
		n    int
	}{
		{"n", len(sc.numNodes)},
		{"workers", len(sc.workers)},
		{"payload", len(sc.payloads)},
		{"rate", len(sc.rates)},
		{"send-buffer", len(sc.sendBuffers)},
		{"recv-buffer", len(sc.recvBuffers)},
		{"bench", len(sc.benchmarks)},
		{"stream", streamModes},
		{"reps", max(sc.reps, 1)},
	}
	var parts []string
	for _, f := range factors {
		if f.n > 1 {
			parts = append(parts, fmt.Sprintf("%s:%d", f.name, f.n))
		}
	}
	return strings.Join(parts, " × ")
}

func sweepForecast(now time.Time, completed, total int, duration time.Duration) (earliest, latest time.Duration, earliestFinish, latestFinish time.Time) {
	left := max(total-completed, 0)
	earliest = duration * time.Duration(left)
	latest = (duration + perRunOverhead) * time.Duration(left)
	return earliest, latest, now.Add(earliest), now.Add(latest)
}

// sweepEstimateLine renders the up-front "estimated sweep time" line shown
// before the first run completes, from the run count (the product of the
// swept parameters) and the per-run wall-clock (-time plus perRunOverhead).
// Shared by the local run path and the driver launcher so a -detach run,
// which never streams the driven sweep's own log back to the laptop, still
// reports the estimate before the launcher disconnects.
func sweepEstimateLine(sc sweepConfig, duration time.Duration) string {
	total := countRuns(sc)
	if total == 0 {
		return ""
	}
	earliest, latest, earliestFinish, latestFinish := sweepForecast(time.Now(), 0, total, duration)
	breakdown := sweepFactorBreakdown(sc)
	if breakdown != "" {
		breakdown = " (" + breakdown + ")"
	}
	return fmt.Sprintf("estimated sweep time: %d run(s)%s: %s–%s; earliest–latest finish %s–%s (static estimate, %s measurement + up to %s overhead/run)",
		total, breakdown, formatETA(earliest), formatETA(latest),
		earliestFinish.Format("15:04 MST"), latestFinish.Format("15:04 MST"),
		formatETA(duration), formatETA(perRunOverhead))
}

func sweepProgressLine(now time.Time, duration time.Duration, completed, total int) string {
	earliest, latest, earliestFinish, latestFinish := sweepForecast(now, completed, total, duration)
	return fmt.Sprintf("  %s–%s remaining, finish %s–%s (%d/%d done; static estimate)",
		formatETA(earliest), formatETA(latest),
		earliestFinish.Format("15:04 MST"), latestFinish.Format("15:04 MST"),
		completed, total)
}

// formatETA renders d as a compact human duration for progress output:
// "1h05m" when at least an hour, "12m" when at least a minute, and "45s"
// otherwise. A negative duration is clamped to zero.
func formatETA(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", max(int(d.Round(time.Second).Seconds()), 0))
	}
	d = d.Round(time.Minute)
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
