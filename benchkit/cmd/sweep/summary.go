package main

import (
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/relab/gorums/benchkit"
)

// benchSummary accumulates one benchmark's results across all nodes of a run.
type benchSummary struct {
	throughput float64              // summed across nodes (cluster aggregate)
	latency    benchkit.LatencyDist // merged across nodes, from raw samples or histograms
	nodes      int                  // number of nodes that contributed
	cvSum      float64              // sum of per-node throughput CV values (averaged at print time)
	cvCount    int                  // number of nodes that reported a valid CV
}

// printRunSummary loads the per-node result files for a completed run and prints
// an aggregated table: throughput summed across nodes, latency percentiles
// recomputed from the merged samples (or from the merged histograms for HDR
// runs). Missing or unreadable files are skipped with a warning so a partial
// run still reports what it collected.
func printRunSummary(outdir, base string, nodes []nodeAssignment, trim time.Duration) {
	byBench := make(map[string]*benchSummary)
	for _, node := range nodes {
		path := filepath.Join(outdir, resultFilename(base, node, resultExt))
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("  warning: summary: %v", err)
			continue
		}
		if err := parseBinaryResultFile(data, byBench, trim); err != nil {
			log.Printf("  warning: summary: parse %s: %v", filepath.Base(path), err)
		}
	}
	if len(byBench) == 0 {
		return
	}

	tw := tabwriter.NewWriter(log.Writer(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  BENCHMARK\tTHROUGHPUT\tCV\tMEAN\tSTDDEV\tp50\tp95\tp99\tNODES\tSAMPLES")
	for _, name := range slices.Sorted(maps.Keys(byBench)) {
		s := byBench[name]
		cvStr := "-"
		if s.cvCount > 0 {
			cvStr = fmt.Sprintf("%.1f%%", 100*s.cvSum/float64(s.cvCount))
		}
		// Latency columns come from the merged raw samples when any node
		// contributed them, and from the merged histograms (HDR runs; whole-run,
		// since the histogram has no time dimension) otherwise.
		meanStr, stddevStr, p50, p95, p99 := "n/a", "n/a", "n/a", "n/a", "n/a"
		var samples uint64
		if !s.latency.Empty() {
			mean, stddev := s.latency.MeanAndStdDev()
			qs := s.latency.Quantiles(0.50, 0.95, 0.99)
			meanStr, stddevStr = fmtDur(int64(mean)), fmtDur(int64(stddev))
			p50, p95, p99 = fmtDur(int64(qs[0])), fmtDur(int64(qs[1])), fmtDur(int64(qs[2]))
			samples = s.latency.Count()
		}
		fmt.Fprintf(tw, "  %s\t%.0f ops/s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			name, s.throughput, cvStr, meanStr, stddevStr, p50, p95, p99,
			s.nodes, samples)
	}
	tw.Flush()
}

// parseBinaryResultFile decodes a binary result file and merges its contents
// into byBench.
func parseBinaryResultFile(data []byte, byBench map[string]*benchSummary, trim time.Duration) error {
	res, err := benchkit.DecodeReport(data)
	if err != nil {
		return err
	}
	mergeResults(res, byBench, trim)
	return nil
}

// mergeResults merges the summary fields (name, throughput, latencies or
// histogram) of every Result in res into byBench, trimming intervals and
// samples recorded before trim (see [benchkit.Summarize]). Other schema fields
// are ignored, and additive schema changes are tolerated by protobuf's wire
// compatibility.
//
// Throughput is summed only from client-measured results when any exist in
// this report, so a PBFT primary-client run contributes primary ops/s rather
// than primary+Σbackup execute rates. When every result is server-measured,
// thruputs are still summed (symmetric multi-node clients).
func mergeResults(res *benchkit.Report, byBench map[string]*benchSummary, trim time.Duration) {
	results := res.GetResults()
	hasClient := false
	for _, r := range results {
		if r.GetConfig().GetMeasurementMode() == benchkit.MeasurementMode_CLIENT_MEASURED {
			hasClient = true
			break
		}
	}
	for _, r := range results {
		name := r.GetConfig().GetName()
		if name == "" {
			continue
		}
		s := byBench[name]
		if s == nil {
			s = &benchSummary{}
			byBench[name] = s
		}
		node := benchkit.Summarize(r, trim)
		clientMeasured := r.GetConfig().GetMeasurementMode() == benchkit.MeasurementMode_CLIENT_MEASURED
		if !hasClient || clientMeasured {
			s.throughput += node.Throughput
		}
		s.nodes++
		s.latency.Merge(node.Dist())
		if node.CVValid {
			s.cvSum += node.CV
			s.cvCount++
		}
	}
}

func fmtDur(ns int64) string {
	return time.Duration(ns).String()
}
