package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/relab/gorums/benchkit"
	"google.golang.org/protobuf/proto"
)

const (
	plotDataDir        = "plotdata"
	plotDataFile       = "plotdata.binpb"
	plotEventsFile     = "events.binpb"
	compactTransferDir = "compact-transfer"
	compactMarker      = "compact.collected"
	cdfPoints          = 200
)

type plotRunRecord struct {
	benchkit.Dimensions
	base        string
	label       string
	status      string // succeeded or degraded; consumers exclude degraded from aggregates
	rep         int
	throughput  float64
	totalOps    uint64
	failedOps   uint64
	allocsPerOp float64
	memPerOp    float64
	nodesSeen   int
	// Latency summaries are pointers so an absent distribution (a run that
	// recorded no latency samples) is nil rather than a spurious zero. A zero
	// would be indistinguishable from a real measurement and would pull down
	// rep-averaged means; nil lets aggregation skip the run for these metrics.
	meanUS  *float64
	p50US   *float64
	p95US   *float64
	p99US   *float64
	samples *uint64
}

type plotNodeCDFRecord struct {
	benchkit.Dimensions
	base       string
	label      string
	status     string // succeeded or degraded; degraded rows drive node-health diagnosis
	rep        int
	node       string
	throughput float64
	meanUS     float64
	p50US      float64
	p95US      float64
	p99US      float64
	samples    uint64
	prob       float64
	cdfUS      float64
}

type plotNodeEntry struct {
	benchkit.Dimensions
	node            string
	throughput      float64
	totalOps        uint64
	failedOps       uint64
	allocs          float64
	mem             float64
	latency         *benchkit.LatencyDist
	measurementMode benchkit.MeasurementMode
}

// writeCompactPlotData reduces the binary result files into the compact,
// normalized plotdata.binpb the report generator can render without downloading
// every raw result file, plus the events.binpb beside it holding every run's
// time-series event streams. Failed runs are intentionally excluded from the
// plot data, which has no row for a run without an aggregate; their event
// streams are exported like any other run's, and their .binpb files are copied
// into the compact transfer directory for local diagnosis.
func writeCompactPlotData(outdir string) error {
	runs, nodes, events, err := collectPlotData(outdir)
	if err != nil {
		return err
	}
	dir := filepath.Join(outdir, plotDataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := proto.Marshal(buildPlotData(runs, nodes))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, plotDataFile), data, 0o644); err != nil {
		return err
	}
	return writePlotEvents(dir, events)
}

// writePlotEvents writes the sweep's event streams next to plotdata.binpb,
// removing a stale file when this collection found no events at all, so a
// re-export never leaves an earlier run's streams behind.
func writePlotEvents(dir string, events *benchkit.PlotEvents) error {
	path := filepath.Join(dir, plotEventsFile)
	if len(events.GetRuns()) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := proto.Marshal(events)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// readPlotEvents reads the event streams a prior collection wrote next to
// plotdata.binpb. It returns nil without an error when the file is absent: a
// directory collected before the streams were exported, or a sweep run with
// interval reporting off, has none.
func readPlotEvents(dir string) (*benchkit.PlotEvents, error) {
	data, err := os.ReadFile(filepath.Join(dir, plotDataDir, plotEventsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	events := &benchkit.PlotEvents{}
	if err := proto.Unmarshal(data, events); err != nil {
		return nil, err
	}
	return events, nil
}

// exportPlotCSV regenerates plotdata/runs.csv and plotdata/nodes.csv from a
// collected directory's plotdata.binpb, for humans and agents to grep. This
// is a local, on-demand secondary form: the primary storage and transfer
// format is the compact plotdata.binpb, not these CSVs.
func exportPlotCSV(dir string) error {
	pd, err := readPlotData(dir)
	if err != nil {
		return err
	}
	runs, cdf := plotRecordsFromMessage(pd)
	plotdataDir := filepath.Join(dir, plotDataDir)
	if err := writePlotRunsCSV(filepath.Join(plotdataDir, "runs.csv"), runs); err != nil {
		return err
	}
	return writePlotNodesCSV(filepath.Join(plotdataDir, "nodes.csv"), cdf)
}

// readPlotData reads and decodes the plotdata.binpb a prior collection wrote
// into the sweep output directory dir.
func readPlotData(dir string) (*benchkit.PlotData, error) {
	data, err := os.ReadFile(filepath.Join(dir, plotDataDir, plotDataFile))
	if err != nil {
		return nil, err
	}
	pd := &benchkit.PlotData{}
	if err := proto.Unmarshal(data, pd); err != nil {
		return nil, err
	}
	return pd, nil
}

// buildPlotData normalizes flat run and per-node CDF records into the nested
// PlotData message used for on-disk and cross-network storage: run identity
// is stored once per run and node identity once per node, rather than being
// repeated on every CDF point.
func buildPlotData(runs []plotRunRecord, cdf []plotNodeCDFRecord) *benchkit.PlotData {
	var out []*benchkit.PlotRun
	runIdx := make(map[string]int)
	type benchKey struct{ base, benchmark string }
	benchIdx := make(map[benchKey]int)

	for _, r := range runs {
		i, ok := runIdx[r.base]
		if !ok {
			i = len(out)
			runIdx[r.base] = i
			out = append(out, benchkit.PlotRun_builder{
				Base:   r.base,
				Label:  r.label,
				Status: r.status,
				Rep:    int32(r.rep),
			}.Build())
		}
		run := out[i]
		benchIdx[benchKey{r.base, r.Benchmark}] = len(run.GetBenchmarks())
		run.SetBenchmarks(append(run.GetBenchmarks(), benchkit.PlotBenchmark_builder{
			Config:      benchkit.NewRunConfig(r.Dimensions),
			Throughput:  r.throughput,
			TotalOps:    r.totalOps,
			FailedOps:   r.failedOps,
			AllocsPerOp: r.allocsPerOp,
			MemPerOp:    r.memPerOp,
			NodesSeen:   int32(r.nodesSeen),
			Summary:     runLatencySummary(r.meanUS, r.p50US, r.p95US, r.p99US, r.samples),
		}.Build()))
	}

	type nodeKey struct{ base, benchmark, node string }
	var curKey nodeKey
	var curNode *benchkit.PlotNode
	flushNode := func() {
		if curNode == nil {
			return
		}
		i, ok := benchIdx[benchKey{curKey.base, curKey.benchmark}]
		if !ok {
			return // orphan CDF rows without a matching run row; drop them
		}
		bench := out[runIdx[curKey.base]].GetBenchmarks()[i]
		bench.SetNodes(append(bench.GetNodes(), curNode))
	}
	for _, c := range cdf {
		key := nodeKey{c.base, c.Benchmark, c.node}
		if curNode == nil || key != curKey {
			flushNode()
			curKey, curNode = key, benchkit.PlotNode_builder{
				Node:       c.node,
				Throughput: c.throughput,
				Summary: benchkit.LatencySummary_builder{
					MeanUs: c.meanUS, P50Us: c.p50US, P95Us: c.p95US, P99Us: c.p99US, Samples: c.samples,
				}.Build(),
			}.Build()
		}
		curNode.SetCdfUs(append(curNode.GetCdfUs(), c.cdfUS))
	}
	flushNode()

	return benchkit.PlotData_builder{Runs: out}.Build()
}

// runLatencySummary builds a benchmark-level LatencySummary, returning nil
// when the run recorded no latency samples so an absent distribution stays
// distinguishable from a genuine all-zero measurement.
func runLatencySummary(meanUS, p50US, p95US, p99US *float64, samples *uint64) *benchkit.LatencySummary {
	if samples == nil {
		return nil
	}
	return benchkit.LatencySummary_builder{
		MeanUs: *meanUS, P50Us: *p50US, P95Us: *p95US, P99Us: *p99US, Samples: *samples,
	}.Build()
}

// plotRecordsFromMessage flattens a normalized PlotData message back into the
// per-run and per-node-CDF records the report pipeline consumes, undoing the
// grouping buildPlotData performed and re-deriving each CDF point's
// cumulative probability from its position on the fixed grid.
func plotRecordsFromMessage(pd *benchkit.PlotData) ([]plotRunRecord, []plotNodeCDFRecord) {
	var runs []plotRunRecord
	var cdf []plotNodeCDFRecord
	for _, run := range pd.GetRuns() {
		for _, bench := range run.GetBenchmarks() {
			cfg := bench.GetConfig()
			row := plotRunRecord{
				Dimensions:  cfg.Dimensions(),
				base:        run.GetBase(),
				label:       run.GetLabel(),
				status:      run.GetStatus(),
				rep:         int(run.GetRep()),
				throughput:  bench.GetThroughput(),
				totalOps:    bench.GetTotalOps(),
				failedOps:   bench.GetFailedOps(),
				allocsPerOp: bench.GetAllocsPerOp(),
				memPerOp:    bench.GetMemPerOp(),
				nodesSeen:   int(bench.GetNodesSeen()),
			}
			if s := bench.GetSummary(); s != nil {
				meanUS, p50US, p95US, p99US, samples := s.GetMeanUs(), s.GetP50Us(), s.GetP95Us(), s.GetP99Us(), s.GetSamples()
				row.meanUS, row.p50US, row.p95US, row.p99US, row.samples = &meanUS, &p50US, &p95US, &p99US, &samples
			}
			runs = append(runs, row)

			for _, node := range bench.GetNodes() {
				s := node.GetSummary()
				cdfUS := node.GetCdfUs()
				for i, v := range cdfUS {
					cdf = append(cdf, plotNodeCDFRecord{
						Dimensions: cfg.Dimensions(),
						base:       run.GetBase(),
						label:      run.GetLabel(),
						status:     run.GetStatus(),
						rep:        int(run.GetRep()),
						node:       node.GetNode(),
						throughput: node.GetThroughput(),
						meanUS:     s.GetMeanUs(),
						p50US:      s.GetP50Us(),
						p95US:      s.GetP95Us(),
						p99US:      s.GetP99Us(),
						samples:    s.GetSamples(),
						prob:       cdfProbAt(i, len(cdfUS)),
						cdfUS:      v,
					})
				}
			}
		}
	}
	return runs, cdf
}

// collectPlotData reduces every run in outdir to its plot rows and its event
// streams, in a single pass over the raw per-node result files. Runs whose
// per-node data is not intact contribute event streams only (see
// [reducePlotRun]).
func collectPlotData(outdir string) ([]plotRunRecord, []plotNodeCDFRecord, *benchkit.PlotEvents, error) {
	manifests, err := loadRunManifests(outdir)
	if err != nil {
		return nil, nil, nil, err
	}
	var runRows []plotRunRecord
	var cdfRows []plotNodeCDFRecord
	var eventRuns []*benchkit.PlotRunEvents
	for _, rm := range manifests {
		trim, err := parseManifestTrim(rm.manifest.Trim)
		if err != nil {
			log.Printf("  warning: plotdata: %s trim %q: %v", filepath.Base(rm.path), rm.manifest.Trim, err)
			trim = 0
		}
		runs, nodes, events := reducePlotRun(outdir, rm.base, rm.manifest, trim)
		runRows = append(runRows, runs...)
		cdfRows = append(cdfRows, nodes...)
		if events != nil {
			eventRuns = append(eventRuns, events)
		}
	}
	slices.SortFunc(runRows, func(a, b plotRunRecord) int {
		return cmp.Or(
			strings.Compare(a.base, b.base),
			strings.Compare(a.Benchmark, b.Benchmark),
		)
	})
	slices.SortFunc(cdfRows, func(a, b plotNodeCDFRecord) int {
		return cmp.Or(
			strings.Compare(a.base, b.base),
			strings.Compare(a.Benchmark, b.Benchmark),
			strings.Compare(a.node, b.node),
			cmp.Compare(a.prob, b.prob),
		)
	})
	return runRows, cdfRows, benchkit.PlotEvents_builder{Runs: eventRuns}.Build(), nil
}

type loadedRunManifest struct {
	base     string
	path     string
	manifest runManifest
}

func loadRunManifests(outdir string) ([]loadedRunManifest, error) {
	matches, err := filepath.Glob(filepath.Join(outdir, "*"+manifestSuffix))
	if err != nil {
		return nil, err
	}
	var manifests []loadedRunManifest
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("  warning: plotdata: read %s: %v", filepath.Base(path), err)
			continue
		}
		var m runManifest
		if err := json.Unmarshal(data, &m); err != nil {
			log.Printf("  warning: plotdata: parse %s: %v", filepath.Base(path), err)
			continue
		}
		base := strings.TrimSuffix(filepath.Base(path), manifestSuffix)
		manifests = append(manifests, loadedRunManifest{base: base, path: path, manifest: m})
	}
	slices.SortFunc(manifests, func(a, b loadedRunManifest) int {
		return strings.Compare(a.base, b.base)
	})
	return manifests, nil
}

// parseManifestTrim returns the read-time trim a run was recorded with, or 0
// when the manifest names none.
func parseManifestTrim(trim string) (time.Duration, error) {
	if trim == "" {
		return 0, nil
	}
	return time.ParseDuration(trim)
}

// reducePlotRun decodes one run's per-node result files and reduces them to the
// run's plot rows and its event streams.
//
// A degraded run is reduced like a successful one: its per-node data is intact
// and is what diagnoses the slow node, and its rows carry the status so
// consumers can exclude it from aggregates. A run with any other status has no
// aggregate to plot and yields no rows, but its event streams are collected all
// the same, so its throughput-over-time trace — the most informative view of a
// run that failed part way through — survives into the compact transfer.
//
// A result file that is absent is skipped silently: the nodes that crashed in a
// failed run wrote none, and a compact-transfer directory retains none for a
// successful run. Any other read or decode failure is reported.
func reducePlotRun(outdir, base string, m runManifest, trim time.Duration) ([]plotRunRecord, []plotNodeCDFRecord, *benchkit.PlotRunEvents) {
	plottable := m.Status == runStatusSucceeded || m.Status == runStatusDegraded
	byBench := make(map[string][]plotNodeEntry)
	eventsByBench := make(map[string][]*benchkit.PlotNodeEvents)
	var eventOrder []string
	for _, file := range m.Files {
		path := filepath.Join(outdir, file)
		data, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				log.Printf("  warning: plotdata: %v", err)
			}
			continue
		}
		report, err := benchkit.DecodeReport(data)
		if err != nil {
			log.Printf("  warning: plotdata: parse %s: %v", filepath.Base(path), err)
			continue
		}
		node := report.GetLabel()
		if node == "" {
			node = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		for _, r := range report.GetResults() {
			cfg := r.GetConfig()
			fallback := m.Dimensions
			fallback.StreamMode = cmp.Or(fallback.StreamMode, "dual")
			dims := cfg.DimensionsWithFallback(fallback)
			name := dims.Benchmark
			if name == "" {
				continue
			}
			if events := r.GetEvents(); len(events) > 0 {
				if _, seen := eventsByBench[name]; !seen {
					eventOrder = append(eventOrder, name)
				}
				eventsByBench[name] = append(eventsByBench[name], benchkit.PlotNodeEvents_builder{
					Node: node, Events: events,
				}.Build())
			}
			if !plottable {
				continue
			}
			summary := benchkit.Summarize(r, trim)
			byBench[name] = append(byBench[name], plotNodeEntry{
				Dimensions:      dims,
				node:            node,
				throughput:      summary.Throughput,
				totalOps:        r.GetTotalOps(),
				failedOps:       r.GetFailedOps(),
				allocs:          float64(r.GetAllocsPerOp()),
				mem:             float64(r.GetMemPerOp()),
				latency:         summary.Dist(),
				measurementMode: cfg.GetMeasurementMode(),
			})
		}
	}

	runRows := make([]plotRunRecord, 0, len(byBench))
	var cdfRows []plotNodeCDFRecord
	for _, bench := range slices.Sorted(maps.Keys(byBench)) {
		entries := byBench[bench]
		if len(entries) == 0 {
			continue
		}
		row := aggregatePlotRun(base, m, bench, entries)
		runRows = append(runRows, row)
		for _, entry := range entries {
			cdfRows = append(cdfRows, nodeCDFRows(base, m, bench, entry)...)
		}
	}
	return runRows, cdfRows, runEvents(base, eventOrder, eventsByBench)
}

// runEvents assembles one run's event streams, in first-seen benchmark order,
// or nil when no node of the run recorded an event.
func runEvents(base string, order []string, byBench map[string][]*benchkit.PlotNodeEvents) *benchkit.PlotRunEvents {
	if len(order) == 0 {
		return nil
	}
	benchmarks := make([]*benchkit.PlotBenchmarkEvents, 0, len(order))
	for _, bench := range order {
		benchmarks = append(benchmarks, benchkit.PlotBenchmarkEvents_builder{
			Benchmark: bench, Nodes: byBench[bench],
		}.Build())
	}
	return benchkit.PlotRunEvents_builder{Base: base, Benchmarks: benchmarks}.Build()
}

func aggregatePlotRun(base string, m runManifest, bench string, entries []plotNodeEntry) plotRunRecord {
	dims := entries[0].Dimensions
	row := plotRunRecord{
		Dimensions: dims,
		base:       base,
		label:      manifestLabel(m),
		status:     m.Status,
		rep:        manifestRep(m),
		nodesSeen:  len(entries),
	}
	row.Benchmark = bench
	var allocs, mem float64
	var latency benchkit.LatencyDist
	// Performance signal: when any node is client-measured, sum only those
	// throughputs so a PBFT -client=primary run reports primary client ops/s
	// rather than primary+Σbackup execute rates. Matches mergeResults
	// (summary.go), which drives the printed run summary from the same
	// MeasurementMode; using "has latency data" as a proxy here instead would
	// disagree with that summary for a run whose server-measured backups also
	// record latency samples (server-measured EXACT results do, alongside
	// client-measured ones).
	hasClient := false
	for _, entry := range entries {
		if entry.measurementMode == benchkit.MeasurementMode_CLIENT_MEASURED {
			hasClient = true
			break
		}
	}
	for _, entry := range entries {
		clientMeasured := entry.measurementMode == benchkit.MeasurementMode_CLIENT_MEASURED
		if !hasClient || clientMeasured {
			row.throughput += entry.throughput
		}
		row.totalOps += entry.totalOps
		row.failedOps += entry.failedOps
		allocs += entry.allocs
		mem += entry.mem
		latency.Merge(entry.latency)
	}
	row.allocsPerOp = allocs / float64(len(entries))
	row.memPerOp = mem / float64(len(entries))
	if !latency.Empty() {
		meanUS, p50US, p95US, p99US := latencyStatsUS(&latency)
		row.meanUS, row.p50US, row.p95US, row.p99US = &meanUS, &p50US, &p95US, &p99US
		samples := latency.Count()
		row.samples = &samples
	}
	return row
}

func nodeCDFRows(base string, m runManifest, bench string, entry plotNodeEntry) []plotNodeCDFRecord {
	if entry.latency.Empty() {
		return nil
	}
	meanUS, p50US, p95US, p99US := latencyStatsUS(entry.latency)
	samples := entry.latency.Count()
	cdf := latencyCDFUS(entry.latency)
	rows := make([]plotNodeCDFRecord, len(cdf))
	for i, v := range cdf {
		rows[i] = plotNodeCDFRecord{
			Dimensions: entry.Dimensions,
			base:       base,
			label:      manifestLabel(m),
			status:     m.Status,
			rep:        manifestRep(m),
			node:       entry.node,
			throughput: entry.throughput,
			meanUS:     meanUS,
			p50US:      p50US,
			p95US:      p95US,
			p99US:      p99US,
			samples:    samples,
			prob:       cdfProb(i),
			cdfUS:      v,
		}
		rows[i].Benchmark = bench
	}
	return rows
}

func manifestLabel(m runManifest) string {
	return cmp.Or(m.Label, "run")
}

func manifestRep(m runManifest) int {
	if m.Rep <= 0 {
		return 1
	}
	return m.Rep
}

// latencyStatsUS returns the distribution's mean and p50, p95, and p99
// latencies in microseconds, the unit the plot data and report figures use.
// The caller must have checked that the distribution is not empty.
func latencyStatsUS(latency *benchkit.LatencyDist) (meanUS, p50US, p95US, p99US float64) {
	mean, _ := latency.MeanAndStdDev()
	qs := latency.Quantiles(0.50, 0.95, 0.99)
	return mean / 1e3, qs[0] / 1e3, qs[1] / 1e3, qs[2] / 1e3
}

// medianUS returns the distribution's median latency in microseconds, or 0
// when it holds no samples. Zero stays distinguishable from a real reading
// because a measured median is positive.
func medianUS(latency *benchkit.LatencyDist) float64 {
	if latency.Empty() {
		return 0
	}
	return latency.Quantiles(0.50)[0] / 1e3
}

// latencyCDFUS samples the distribution on the fixed CDF probability grid,
// in microseconds. The caller must have checked that it is not empty.
func latencyCDFUS(latency *benchkit.LatencyDist) []float64 {
	qs := latency.Quantiles(cdfProbs()...)
	out := make([]float64, len(qs))
	for i, q := range qs {
		out[i] = q / 1e3
	}
	return out
}

func cdfProbs() []float64 {
	probs := make([]float64, cdfPoints)
	for i := range probs {
		probs[i] = cdfProb(i)
	}
	return probs
}

func cdfProb(i int) float64 {
	return cdfProbAt(i, cdfPoints)
}

// cdfProbAt returns the cumulative probability of point i in an n-point CDF
// sampled on the fixed, equally spaced grid i/(n-1).
func cdfProbAt(i, n int) float64 {
	if n <= 1 {
		return 0
	}
	return float64(i) / float64(n-1)
}

// plotRunsCSVHeader and plotRunCSVFields are shared by writePlotRunsCSV (the
// on-disk plotdata/runs.csv export) and summaryRows (explain.go's LLM triage
// prompt, which filters the same rows in memory instead of writing a file).
func plotRunsCSVHeader() []string {
	header := append([]string{"base", "label", "status", "rep"}, dimensionColumns()...)
	return append(header,
		"throughput", "total_ops", "failed_ops", "allocs_per_op", "mem_per_op",
		"nodes_seen",
		"mean_us", "p50_us", "p95_us", "p99_us",
		"p50_ms", "p95_ms", "p99_ms", "samples",
	)
}

func plotRunCSVFields(row plotRunRecord) []string {
	rec := append([]string{row.base, row.label, row.status, strconv.Itoa(row.rep)}, dimensionValues(row.Dimensions)...)
	return append(rec,
		formatFloat(row.throughput), strconv.FormatUint(row.totalOps, 10),
		strconv.FormatUint(row.failedOps, 10),
		formatFloat(row.allocsPerOp), formatFloat(row.memPerOp),
		strconv.Itoa(row.nodesSeen),
		formatFloatPtr(row.meanUS), formatFloatPtr(row.p50US),
		formatFloatPtr(row.p95US), formatFloatPtr(row.p99US),
		formatMillisPtr(row.p50US), formatMillisPtr(row.p95US), formatMillisPtr(row.p99US),
		formatUintPtr(row.samples),
	)
}

func writePlotRunsCSV(path string, rows []plotRunRecord) error {
	return writeCSV(path, plotRunsCSVHeader(), rows, plotRunCSVFields)
}

func writePlotNodeCDFCSV(path string, rows []plotNodeCDFRecord) error {
	header := append([]string{"base", "label", "status", "rep"}, dimensionColumns()...)
	header = append(header,
		[]string{"node", "throughput", "mean_us", "p50_us", "p95_us",
			"p99_us", "p50_ms", "p95_ms", "p99_ms", "samples", "prob", "cdf_us",
		}...)
	return writeCSV(path, header, rows, func(row plotNodeCDFRecord) []string {
		rec := append([]string{row.base, row.label, row.status, strconv.Itoa(row.rep)}, dimensionValues(row.Dimensions)...)
		return append(rec,
			[]string{row.node, formatFloat(row.throughput),
				formatFloat(row.meanUS), formatFloat(row.p50US), formatFloat(row.p95US),
				formatFloat(row.p99US), formatFloat(row.p50US / 1e3), formatFloat(row.p95US / 1e3),
				formatFloat(row.p99US / 1e3), strconv.FormatUint(row.samples, 10),
				formatFloat(row.prob), formatFloat(row.cdfUS),
			}...)
	})
}

// plotNodeRow is one node's aggregate latency fields plus its full CDF
// vector, the unit writePlotNodesCSV exports: one row per node rather than
// one row per CDF point. Point i's cumulative probability is implied by its
// position in cdf (i/(len(cdf)-1) on the fixed CDF grid), so it is not stored.
type plotNodeRow struct {
	benchkit.Dimensions
	base, label, status, node               string
	rep                                     int
	throughput, meanUS, p50US, p95US, p99US float64
	samples                                 uint64
	cdf                                     []float64
}

// groupNodeCDFRows collapses per-CDF-point records into one plotNodeRow per
// (base, benchmark, node), assuming matching records are contiguous. This
// holds for records sourced from collectPlotData or plotRecordsFromMessage,
// both of which group by node before flattening to per-point records.
func groupNodeCDFRows(rows []plotNodeCDFRecord) []plotNodeRow {
	var out []plotNodeRow
	for _, r := range rows {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if last.base == r.base && last.Benchmark == r.Benchmark && last.node == r.node {
				last.cdf = append(last.cdf, r.cdfUS)
				continue
			}
		}
		out = append(out, plotNodeRow{
			Dimensions: r.Dimensions,
			base:       r.base, label: r.label, status: r.status, rep: r.rep,
			node:       r.node,
			throughput: r.throughput, meanUS: r.meanUS, p50US: r.p50US, p95US: r.p95US, p99US: r.p99US,
			samples: r.samples, cdf: []float64{r.cdfUS},
		})
	}
	return out
}

// writePlotNodesCSV writes one human- and grep-friendly row per node,
// collapsing each node's CDF into a single space-separated cdf_us column
// instead of one row per CDF point. It is the -export-csv counterpart to the
// compact plotdata.binpb, not part of the report render path.
func writePlotNodesCSV(path string, rows []plotNodeCDFRecord) error {
	header := append([]string{"base", "label", "status", "rep"}, dimensionColumns()...)
	header = append(header, "node", "throughput", "mean_us", "p50_us", "p95_us", "p99_us", "samples", "cdf_us")
	return writeCSV(path, header, groupNodeCDFRows(rows), func(g plotNodeRow) []string {
		us := make([]string, len(g.cdf))
		for i, v := range g.cdf {
			us[i] = formatFloat(v)
		}
		rec := append([]string{g.base, g.label, g.status, strconv.Itoa(g.rep)}, dimensionValues(g.Dimensions)...)
		return append(rec,
			[]string{g.node, formatFloat(g.throughput),
				formatFloat(g.meanUS), formatFloat(g.p50US), formatFloat(g.p95US), formatFloat(g.p99US),
				strconv.FormatUint(g.samples, 10), strings.Join(us, " "),
			}...)
	})
}

func formatFloatPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return formatFloat(*v)
}

func formatMillisPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return formatFloat(*v / 1e3)
}

func formatUintPtr(v *uint64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatUint(*v, 10)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

type compactTransferSummary struct {
	failedResults int
	profiles      int
	eventBytes    int64 // size of the exported events.binpb; 0 when no run recorded events
}

// prepareCompactTransfer creates the small directory the laptop downloads for
// driver runs. It contains the reduced plot data, every run's event streams,
// manifests, logs, and failed-run result files. Successful raw result files
// remain in the driver's work directory.
func prepareCompactTransfer(outdir string, includeProfiles bool) (compactTransferSummary, error) {
	var summary compactTransferSummary
	if err := writeCompactPlotData(outdir); err != nil {
		return summary, err
	}
	dst := filepath.Join(outdir, compactTransferDir)
	if err := os.RemoveAll(dst); err != nil {
		return summary, err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return summary, err
	}
	if _, err := copyGlob(filepath.Join(outdir, "*"+manifestSuffix), dst); err != nil {
		return summary, err
	}
	if err := copyIfExists(filepath.Join(outdir, "sweep.log"), filepath.Join(dst, "sweep.log")); err != nil {
		return summary, err
	}
	if err := copyDirIfExists(filepath.Join(outdir, logSubdir), filepath.Join(dst, logSubdir)); err != nil {
		return summary, err
	}
	if err := copyDir(filepath.Join(outdir, plotDataDir), filepath.Join(dst, plotDataDir)); err != nil {
		return summary, err
	}
	if info, err := os.Stat(filepath.Join(dst, plotDataDir, plotEventsFile)); err == nil {
		summary.eventBytes = info.Size()
	}
	n, err := copyFailedResultFiles(outdir, dst)
	if err != nil {
		return summary, err
	}
	summary.failedResults = n
	if err := copyIfExists(filepath.Join(outdir, "default.pgo"), filepath.Join(dst, "default.pgo")); err != nil {
		return summary, err
	}
	if includeProfiles {
		for _, ext := range []string{cpuProfExt, memProfExt} {
			n, err := copyGlob(filepath.Join(outdir, "*"+ext), dst)
			if err != nil {
				return summary, err
			}
			summary.profiles += n
		}
	}
	return summary, nil
}

// logCompactTransfer reports what a prepared compact transfer holds, so an
// operator sees which of its optional payloads are present before downloading.
func logCompactTransfer(outdir string, summary compactTransferSummary) {
	log.Printf("compact plot data prepared in %s", displayPath(filepath.Join(outdir, compactTransferDir)))
	if summary.failedResults > 0 {
		log.Printf("compact transfer includes %d failed-run result file(s)", summary.failedResults)
	}
	if summary.eventBytes > 0 {
		log.Printf("compact transfer includes %d KiB of event streams for the time-series figures", summary.eventBytes/1024)
	}
	if summary.profiles > 0 {
		log.Printf("compact transfer includes %d profile file(s); profiles may dominate transfer size", summary.profiles)
	}
}

func copyFailedResultFiles(outdir, dst string) (int, error) {
	manifests, err := loadRunManifests(outdir)
	if err != nil {
		return 0, err
	}
	var n int
	for _, rm := range manifests {
		if rm.manifest.Status != runStatusFailed {
			continue
		}
		for _, file := range rm.manifest.Files {
			src := filepath.Join(outdir, file)
			if _, err := os.Stat(src); err != nil {
				continue
			}
			if err := copyFile(src, filepath.Join(dst, file)); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

func copyGlob(pattern, dstDir string) (int, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, err
	}
	for _, src := range matches {
		if err := copyFile(src, filepath.Join(dstDir, filepath.Base(src))); err != nil {
			return 0, err
		}
	}
	return len(matches), nil
}

func copyIfExists(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return copyFile(src, dst)
}

func copyDirIfExists(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return copyDir(src, dst)
}

func copyDir(src, dst string) error {
	return os.CopyFS(dst, os.DirFS(src))
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", src)
	}
	return copyFileMode(src, dst, info.Mode())
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
