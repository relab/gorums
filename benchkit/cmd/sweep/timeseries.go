package main

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/relab/gorums/benchkit"
)

// This file collects a run's time-series event streams (Result.events) and
// hands them to [benchkit.WriteTimeSeriesCSVs], which renders the
// throughput-over-time, latency-over-time, and saturation-curve CSVs the report
// draws its figures from. The streams reach sweep two ways — embedded in raw
// result files, or exported into a run's events.binpb — and both end in the
// same renderer.

// generateTimeSeries reads resultFiles (binary result files, all belonging to
// one run), groups each Result's embedded event stream by benchmark name, and
// renders per benchmark matching sel a throughput-over-time, latency-over-time,
// and saturation-curve CSV into outDir. It returns the benchmark names it wrote
// data for, in first-seen order, so the caller can plan one report figure per
// name. Multi-node rows stay distinguishable via the node column (the Report
// label, falling back to the filename stem). Trimming and the no-data skip are
// [benchkit.WriteTimeSeriesCSVs]'s contract.
//
// A file that cannot be read or decoded is skipped rather than failing the
// whole call, and an absent file is skipped without a warning: resultFiles may
// include a manifest's raw result paths from a compact-transfer directory,
// where successful runs' raw files were intentionally not retained (see
// prepareCompactTransfer), so a missing file here is an expected, not
// exceptional, outcome. If no file could be read at all, it returns (nil, nil)
// rather than an error, matching that expected "nothing to show" outcome; a
// nil sel matches every benchmark.
func generateTimeSeries(resultFiles []string, outDir string, sel *regexp.Regexp, trim time.Duration) ([]string, error) {
	byBench := make(map[string][]benchkit.TimeSeriesNode)
	var order []string
	var decoded int
	for _, f := range resultFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			// An absent file is the expected outcome for a compact-transfer
			// directory, which retains no raw result file for a successful
			// run, so it is skipped silently; a large sweep would otherwise
			// warn once per node of every run on each replot. Any other read
			// failure is a real problem and is reported.
			if !errors.Is(err, fs.ErrNotExist) {
				log.Printf("  warning: time-series: read %s: %v", filepath.Base(f), err)
			}
			continue
		}
		report, err := benchkit.DecodeReport(data)
		if err != nil {
			log.Printf("  warning: time-series: decode %s: %v", filepath.Base(f), err)
			continue
		}
		decoded++
		stem := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		for _, r := range report.GetResults() {
			bench := cmp.Or(r.GetConfig().GetName(), stem)
			if sel != nil && !sel.MatchString(bench) {
				continue
			}
			if _, seen := byBench[bench]; !seen {
				order = append(order, bench)
			}
			byBench[bench] = append(byBench[bench], benchkit.TimeSeriesNode{
				Node:   cmp.Or(report.GetLabel(), stem),
				Events: r.GetEvents(),
			})
		}
	}
	if decoded == 0 {
		return nil, nil
	}
	if len(byBench) == 0 {
		// Files were read but held no result matching sel: a mistyped
		// selector rather than an absent run, so report it instead of silently
		// writing nothing.
		return nil, fmt.Errorf("no benchmark in %d result file(s) matched the selector", decoded)
	}
	groups := make([]benchkit.TimeSeriesGroup, len(order))
	for i, bench := range order {
		groups[i] = benchkit.TimeSeriesGroup{Benchmark: bench, Nodes: byBench[bench]}
	}
	return benchkit.WriteTimeSeriesCSVs(outDir, groups, trim)
}

// eventTimeSeries renders one run's exported event streams (see
// [readPlotEvents]) the same way [generateTimeSeries] renders the streams
// embedded in raw result files, so a compact-transfer directory needs no raw
// data to draw a time series.
func eventTimeSeries(runEvents *benchkit.PlotRunEvents, outDir string, trim time.Duration) ([]string, error) {
	benchmarks := runEvents.GetBenchmarks()
	groups := make([]benchkit.TimeSeriesGroup, 0, len(benchmarks))
	for _, bench := range benchmarks {
		group := benchkit.TimeSeriesGroup{Benchmark: bench.GetBenchmark()}
		for _, node := range bench.GetNodes() {
			group.Nodes = append(group.Nodes, benchkit.TimeSeriesNode{
				Node: node.GetNode(), Events: node.GetEvents(),
			})
		}
		groups = append(groups, group)
	}
	if len(groups) == 0 {
		return nil, nil
	}
	return benchkit.WriteTimeSeriesCSVs(outDir, groups, trim)
}
