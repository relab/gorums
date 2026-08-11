package main

import (
	"cmp"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/relab/gorums/benchkit"
)

// reportSubdir is the directory, under a sweep output directory, that holds the
// generated CSVs, report.typ, the copied helper library, and the compiled PDF.
const reportSubdir = "report"

// maxCDFRuns caps how many runs the per-node latency CDF grid draws a panel
// for, so a large sweep does not fill its report with them. Fifteen fills a page
// at three panels per row.
const maxCDFRuns = 15

// maxTimeSeriesRuns caps how many runs get an over-time figure. These are
// per-run traces read one at a time rather than compared side by side, so they
// warrant fewer than the CDF grid's panels.
const maxTimeSeriesRuns = 6

// offsetCDFPoints is the number of quantiles sampled per clock-offset CDF curve.
const offsetCDFPoints = 120

// reportOptions carries the post-hoc filtering choices a report honors.
type reportOptions struct {
	title           string
	includeDegraded bool
	excludeRuns     map[string]bool            // run base names to drop entirely
	excludes        map[string]map[string]bool // dimension/benchmark -> excluded values
}

// reportOptionsFromConfig builds the report filters from the sweep flags.
func reportOptionsFromConfig(cfg *config) reportOptions {
	opts := reportOptions{includeDegraded: cfg.includeDegraded}
	if len(cfg.excludeRuns) > 0 {
		opts.excludeRuns = make(map[string]bool, len(cfg.excludeRuns))
		for _, name := range cfg.excludeRuns {
			opts.excludeRuns[name] = true
		}
	}
	if len(cfg.excludeDims) > 0 {
		opts.excludes = parseExcludeDims(cfg.excludeDims)
	}
	return opts
}

// parseExcludeDims turns DIM=VALUE tokens into a column→values exclusion set,
// warning on tokens that are not in that form.
func parseExcludeDims(tokens []string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, tok := range tokens {
		col, val, ok := strings.Cut(tok, "=")
		if !ok || col == "" || val == "" {
			log.Printf("warning: ignoring -exclude %q (want DIM=VALUE)", tok)
			continue
		}
		if out[col] == nil {
			out[col] = map[string]bool{}
		}
		out[col][val] = true
	}
	return out
}

// autoReport generates the report for a completed run directory as a
// best-effort step: the results are already collected, so a report failure is
// logged, not fatal.
func autoReport(cfg *config) {
	if err := generateReport(cfg.outDir, reportOptionsFromConfig(cfg)); err != nil {
		log.Printf("warning: generate report: %v", err)
	}
}

// generateReport reads a sweep output (or compact-transfer) directory, applies
// the requested filters, writes the derived CSVs and a self-contained
// report.typ under <dir>/report, and best-effort compiles it to PDF when Typst
// is installed. It returns an error only when there is no data to plot or a
// write fails; a missing Typst binary is reported, not fatal.
func generateReport(dir string, opts reportOptions) error {
	log.Printf("  report: loading benchmark data from %s", displayPath(dir))
	runs, cdf, health, err := loadReportData(dir, opts)
	if err != nil {
		return err
	}

	agg := aggregateReps(runs, opts.includeDegraded)
	if len(agg) == 0 {
		return fmt.Errorf("no benchmark data to plot in %s", dir)
	}
	// Name the repetitions that do not belong with their siblings, whatever the
	// sweep's own per-node bounds made of them, so a figure's error bands are
	// never quietly built on one.
	for _, note := range repOutliers(runs, repOutlierSpread) {
		log.Printf("  warning: report: %s", note)
	}

	out := filepath.Join(dir, reportSubdir)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := writeAggRunsCSV(filepath.Join(out, "agg.csv"), agg); err != nil {
		return err
	}

	var in reportInputs

	if cmpRows := pivotComparison(agg, "dual"); cmpRows != nil {
		if err := writeComparisonCSV(filepath.Join(out, "comparison.csv"), cmpRows); err != nil {
			return err
		}
		// The side-by-side table is always useful; which ratio figures the rows
		// can draw is decided per metric and per x-dimension in planFigures.
		in.comparison = cmpRows
	}
	if tl := tlCurveRows(agg, tlLoadDimensions(dimCounts(agg))); len(tl) > 0 {
		if err := writeTLCurveCSV(filepath.Join(out, "tl_curve.csv"), tl); err != nil {
			return err
		}
	}
	// Run labels name only what varies between the sweep's configurations; what
	// they all share sits in the report's experiment line instead.
	varying := varyingDimensions(aggConfigs(agg))
	if len(cdf) > 0 {
		if err := writePlotNodeCDFCSV(filepath.Join(out, "node_cdf.csv"), cdf); err != nil {
			return err
		}
		if nh := nodeHealthRows(health); len(nh) > 0 {
			if err := writeNodeHealthCSV(filepath.Join(out, "node_health.csv"), nh); err != nil {
				return err
			}
			in.nodeHealth = true
		}
		in.cdfRuns = cdfRuns(cdf, varying)
	}
	// Time series are selected independently of the latency CDF: an event stream
	// can carry throughput intervals with no latency samples at all.
	manifests, err := loadRunManifests(dir)
	if err != nil {
		log.Printf("  warning: report: %v", err)
	}
	in.timeSeries = writeTimeSeriesFigures(dir, out, manifests, cdfBases(in.cdfRuns), varying, maxTimeSeriesRuns)
	in.failedTimeSeries = writeFailedTimeSeriesFigures(dir, out, manifests, varying, maxFailedRuns)
	if dg := degradedShareRows(runs); slices.ContainsFunc(dg, func(r degradedShareRecord) bool { return r.degraded > 0 }) {
		if err := writeDegradedShareCSV(filepath.Join(out, "degraded_share.csv"), dg); err != nil {
			return err
		}
		in.degradedShare = true
	}
	// Run-status accounting and clock-offset diagnostics describe the whole
	// sweep directory (every attempted run, the cluster's clock skew), so they
	// intentionally ignore the per-configuration --exclude filters.
	if st, err := runStatusRows(dir); err == nil && anyDegradedOrFailed(st) {
		if err := writeRunStatusCSV(filepath.Join(out, "run_status.csv"), st); err != nil {
			return err
		}
		in.runStatus = true
	}
	if off, err := collectOffsets(filepath.Join(dir, logSubdir)); err == nil && len(off) > 0 {
		if err := writeOffsetsCSV(filepath.Join(out, "offsets.csv"), offsetCDFRows(off, offsetCDFPoints)); err != nil {
			return err
		}
		in.offsets = true
	}

	header := reportHeader{
		title:      cmp.Or(opts.title, "Gorums benchmark report"),
		experiment: experimentSummary(agg, sweepSettingsFromManifests(manifests, dir)),
	}
	specs := planFigures(agg, in)
	typPath := filepath.Join(out, "report.typ")
	if err := writeReportTyp(typPath, header, specs); err != nil {
		return err
	}
	if err := copyReportLib(out); err != nil {
		return err
	}
	artifact := compileReport(typPath)
	log.Printf("  report: %d figure(s) -> %s", len(specs), displayPath(artifact))
	return nil
}

// loadReportData reads the per-rep run records and per-node CDF records for a
// directory. It prefers the compact plotdata.binpb (the normal data present
// after a driver run's compact transfer), falls back to the legacy compact
// plotdata CSVs for output directories collected before plotdata.binpb
// existed, and falls back further to decoding the raw binary result files
// when neither is present (a local run).
func loadReportData(dir string, opts reportOptions) (runs []plotRunRecord, cdf, health []plotNodeCDFRecord, err error) {
	if _, statErr := os.Stat(filepath.Join(dir, plotDataDir, plotDataFile)); statErr == nil {
		pd, err := readPlotData(dir)
		if err != nil {
			return nil, nil, nil, err
		}
		allRuns, allCDF := plotRecordsFromMessage(pd)
		runs = filterRuns(allRuns, opts)
		cdf, health = reduceReportCDF(allCDF, opts, maxCDFRuns)
		return runs, cdf, health, nil
	}

	runsCSV := filepath.Join(dir, plotDataDir, "runs.csv")
	if _, err := os.Stat(runsCSV); err != nil {
		var allCDF []plotNodeCDFRecord
		// The event streams collected here are not retained: the report renders
		// time series from the raw result files still present in a local sweep
		// directory, for the few runs it plans a figure for.
		runs, allCDF, _, err = collectPlotData(dir)
		if err != nil {
			return nil, nil, nil, err
		}
		runs = filterRuns(runs, opts)
		cdf, health = reduceReportCDF(allCDF, opts, maxCDFRuns)
		return runs, cdf, health, nil
	}
	runs, err = readPlotRunsCSV(runsCSV)
	if err != nil {
		return nil, nil, nil, err
	}
	runs = filterRuns(runs, opts)
	cdfCSV := filepath.Join(dir, plotDataDir, "node_cdf.csv")
	if _, err := os.Stat(cdfCSV); err == nil {
		if cdf, health, err = readReportNodeCDFCSV(cdfCSV, opts, maxCDFRuns); err != nil {
			return nil, nil, nil, err
		}
	}
	return runs, cdf, health, nil
}

// aggConfigs returns the configurations behind the rep-averaged records.
func aggConfigs(agg []aggRunRecord) []benchkit.Dimensions {
	configs := make([]benchkit.Dimensions, len(agg))
	for i, r := range agg {
		configs[i] = r.Dimensions
	}
	return configs
}

// cdfRuns returns the runs present in the CDF rows, in first-seen order, each
// with the compact configuration label its panel carries. The rows were already
// reduced to the selected runs upstream (see reportCDFReducer), which is where
// the panel budget is spent.
func cdfRuns(cdf []plotNodeCDFRecord, varying map[string]bool) []cdfRun {
	var runs []cdfRun
	seen := map[string]bool{}
	for _, r := range cdf {
		if seen[r.base] {
			continue
		}
		seen[r.base] = true
		runs = append(runs, cdfRun{base: r.base, title: cmp.Or(configLabel(r.Dimensions, varying), r.base)})
	}
	return runs
}

// cdfBases returns the run bases of the CDF panels, in panel order.
func cdfBases(runs []cdfRun) []string {
	bases := make([]string, len(runs))
	for i, run := range runs {
		bases[i] = run.base
	}
	return bases
}

// sweepSettings are the sweep-wide facts a report's experiment line states.
// Every run of one sweep shares them, so they come from any one manifest; the
// run count is over all of them, whatever each run's outcome.
type sweepSettings struct {
	label    string
	duration string
	trim     string
	runs     int
}

// sweepSettingsFromManifests reads the sweep-wide settings from the run
// manifests, falling back to the output directory's own name for the label so a
// directory whose manifests are absent or unlabeled still names its experiment.
func sweepSettingsFromManifests(manifests []loadedRunManifest, dir string) sweepSettings {
	settings := sweepSettings{runs: len(manifests)}
	if len(manifests) > 0 {
		m := manifests[0].manifest
		settings.label, settings.duration, settings.trim = m.Label, m.Duration, m.Trim
	}
	settings.label = cmp.Or(settings.label, filepath.Base(strings.TrimRight(dir, "/")))
	return settings
}

// writeTimeSeriesFigures generates throughput/latency-over-time CSVs for at
// most limit runs, one per swept configuration, and returns which benchmarks
// got data, for planFigures. dir is the report's source directory (a sweep
// output or compact-transfer directory) and out is the report's own output
// directory (<dir>/report). varying names the dimensions a run's compact label
// must state to identify it.
//
// Candidates come from the run manifests and the event data itself, not from the
// per-node CDF records: per-node CDF data is a latency artifact, while an event
// stream can carry valid throughput intervals with no latency samples at all, so
// a throughput-only benchmark has a trace to draw. Each configuration
// contributes the first of its repetitions with event data, preferring the base
// in preferred (that configuration's per-node CDF base) so both figures describe
// the same run where possible. A run with no event data anywhere contributes
// nothing, which is expected rather than an error: a sweep measured with
// interval reporting off records none. Any other failure (a write error) is
// logged and that run's figures are skipped without failing the report.
func writeTimeSeriesFigures(dir, out string, manifests []loadedRunManifest, preferred []string, varying map[string]bool, limit int) []timeSeriesRunFigures {
	if limit <= 0 {
		return nil
	}
	source := newTimeSeriesSource(dir)

	var figures []timeSeriesRunFigures
	var prevHosts []string
	for _, bases := range timeSeriesCandidates(manifests, preferred) {
		if len(figures) >= limit {
			break
		}
		for _, rm := range bases {
			if benches := source.render(out, rm); len(benches) > 0 {
				figures = append(figures, timeSeriesRunFigures{
					base:        rm.base,
					title:       configLabel(rm.manifest.Dimensions, varying),
					benches:     benches,
					sharesNodes: len(prevHosts) > 0 && slices.Equal(rm.manifest.Hosts, prevHosts),
				})
				prevHosts = rm.manifest.Hosts
				break
			}
		}
	}
	return figures
}

// timeSeriesCandidates groups the runs whose per-node data is intact by
// configuration, so each configuration is offered as an ordered list of
// interchangeable repetitions. The configurations whose base appears in
// preferred come first, in that order, so the figure budget is spent on the same
// runs the per-node CDF grid selected rather than on whichever configurations
// sort first; within a configuration, a base in preferred is moved to the front,
// making it the repetition writeTimeSeriesFigures renders when it has event
// data. The result then alternates stream modes, so a figure cap smaller than the
// candidate list still covers both arms of a comparison. Failed runs are left
// out: they have no aggregate to sit beside, and are rendered by their own report
// section instead (see writeFailedTimeSeriesFigures).
func timeSeriesCandidates(manifests []loadedRunManifest, preferred []string) [][]loadedRunManifest {
	var order []benchkit.Dimensions
	byConfig := map[benchkit.Dimensions][]loadedRunManifest{}
	rank := map[benchkit.Dimensions]int{}
	for _, rm := range manifests {
		if rm.manifest.Status != runStatusSucceeded && rm.manifest.Status != runStatusDegraded {
			continue
		}
		config := rm.manifest.Dimensions
		runs, ok := byConfig[config]
		if !ok {
			order = append(order, config)
			rank[config] = len(preferred)
		}
		if i := slices.Index(preferred, rm.base); i >= 0 {
			runs = slices.Insert(runs, 0, rm)
			rank[config] = min(rank[config], i)
		} else {
			runs = append(runs, rm)
		}
		byConfig[config] = runs
	}
	slices.SortStableFunc(order, func(a, b benchkit.Dimensions) int {
		return cmp.Compare(rank[a], rank[b])
	})
	candidates := make([][]loadedRunManifest, len(order))
	for i, config := range order {
		candidates[i] = byConfig[config]
	}
	return alternateStreamModes(candidates)
}

// alternateStreamModes reorders candidate configurations to take one stream mode
// after another, keeping each mode's own order. A comparison sweep offers far
// more configurations than a report draws figures for, and its modes group
// together in every natural order (a base name sorts them, and so does the
// dimension tuple), which spends the whole cap on one arm. A single-mode sweep is
// returned unchanged.
func alternateStreamModes(candidates [][]loadedRunManifest) [][]loadedRunManifest {
	var modes []string
	byMode := map[string][][]loadedRunManifest{}
	for _, runs := range candidates {
		mode := runs[0].manifest.StreamMode
		if _, ok := byMode[mode]; !ok {
			modes = append(modes, mode)
		}
		byMode[mode] = append(byMode[mode], runs)
	}
	if len(modes) < 2 {
		return candidates
	}
	out := make([][]loadedRunManifest, 0, len(candidates))
	for i := 0; len(out) < len(candidates); i++ {
		for _, mode := range modes {
			if i < len(byMode[mode]) {
				out = append(out, byMode[mode][i])
			}
		}
	}
	return out
}

// maxFailedRuns caps how many failed runs get a time-series figure. A sweep's
// failures are usually one fault repeated, so one representative per
// (configuration, error signature) group is enough to see the shape, and the cap
// keeps a badly broken sweep from filling its report with them.
const maxFailedRuns = 3

// failedRunGroup identifies failed runs that show the same thing: the same
// configuration failing the same way.
type failedRunGroup struct {
	config    benchkit.Dimensions
	signature string
}

// digits matches a run of decimal digits, which errorSignature folds away.
var digits = regexp.MustCompile(`[0-9]+`)

// errorSignature reduces a failed run's cause to a grouping key: the failure
// phase plus its message with digit runs folded to "N", so failures differing
// only in an exit status, a port, or a node index group together.
func errorSignature(m runManifest) string {
	return m.FailurePhase + "|" + digits.ReplaceAllString(m.Error, "N")
}

// writeFailedTimeSeriesFigures renders the time series of failed runs, which
// have no aggregate row and so no place among the per-configuration figures. A
// failed run's throughput trace is the most informative view it has: it shows
// whether its nodes were producing work at all, and when they stopped.
//
// Failed runs are grouped by configuration and error signature and one
// representative of each group is rendered, up to limit figures; whatever the
// cap drops is logged, so the report's failed-runs section never reads as
// complete when it is not. A group whose representative has no event data lets
// the next run in the group stand in.
func writeFailedTimeSeriesFigures(dir, out string, manifests []loadedRunManifest, varying map[string]bool, limit int) []timeSeriesRunFigures {
	if limit <= 0 {
		return nil
	}
	groupSize := map[failedRunGroup]int{}
	for _, rm := range manifests {
		if rm.manifest.Status == runStatusFailed {
			groupSize[failedRunGroup{rm.manifest.Dimensions, errorSignature(rm.manifest)}]++
		}
	}

	source := newTimeSeriesSource(dir)
	rendered := map[failedRunGroup]bool{}
	var figures []timeSeriesRunFigures
	var dropped int
	for _, rm := range manifests {
		if rm.manifest.Status != runStatusFailed {
			continue
		}
		group := failedRunGroup{rm.manifest.Dimensions, errorSignature(rm.manifest)}
		if rendered[group] {
			continue
		}
		if len(figures) >= limit {
			rendered[group] = true // count each dropped group once
			dropped++
			continue
		}
		benches := source.render(out, rm)
		if len(benches) == 0 {
			continue // no event data for this run; try another of its group
		}
		rendered[group] = true
		figures = append(figures, timeSeriesRunFigures{
			base:    rm.base,
			title:   configLabel(rm.manifest.Dimensions, varying),
			benches: benches,
			note:    failedRunNote(rm.manifest, groupSize[group]),
		})
	}
	if dropped > 0 {
		log.Printf("  report: %d further failed-run group(s) not shown (cap %d figures)", dropped, limit)
	}
	return figures
}

// failedRunNote is the caveat printed under a failed run's figures: how the run
// failed, how many nodes are missing from the figure entirely, and how many
// other failed runs this one stands for.
func failedRunNote(m runManifest, groupSize int) string {
	note := fmt.Sprintf("Run failed during %s: %s.", cmp.Or(m.FailurePhase, "the run"), oneLine(m.Error))
	if missing := len(m.MissingFiles); missing > 0 {
		note += fmt.Sprintf(" %d of %d nodes wrote no result file and contribute no line at all, so an absent trace is an absent node, not an idle one.",
			missing, len(m.Files))
	}
	if groupSize > 1 {
		note += fmt.Sprintf(" Representative of %d failed runs with this configuration and error.", groupSize)
	}
	return note
}

// timeSeriesSource renders a run's event streams from whichever form the
// directory holds: the exported plotdata/events.binpb, which a compact transfer
// carries for every run, or the raw per-node result files a sweep output
// directory still has. The exported streams are preferred, since they are
// present for runs whose raw files were left on the driver.
type timeSeriesSource struct {
	dir    string
	events map[string]*benchkit.PlotRunEvents // run base -> its streams; nil when the directory has none
}

// newTimeSeriesSource reads the exported event streams for dir, best effort: an
// unreadable events.binpb is reported and leaves the raw result files as the
// only source.
func newTimeSeriesSource(dir string) *timeSeriesSource {
	source := &timeSeriesSource{dir: dir}
	events, err := readPlotEvents(dir)
	if err != nil {
		log.Printf("  warning: time-series: read %s: %v", plotEventsFile, err)
		return source
	}
	if runs := events.GetRuns(); len(runs) > 0 {
		source.events = make(map[string]*benchkit.PlotRunEvents, len(runs))
		for _, run := range runs {
			source.events[run.GetBase()] = run
		}
	}
	return source
}

// render writes one run's time-series CSVs under <out>/timeseries/<base> and
// returns the benchmarks that got data, or nil when the run has no event data
// or could not be rendered.
func (s *timeSeriesSource) render(out string, rm loadedRunManifest) []string {
	trim, err := parseManifestTrim(rm.manifest.Trim)
	if err != nil {
		log.Printf("  warning: time-series: %s trim %q: %v", rm.base, rm.manifest.Trim, err)
		trim = 0
	}
	outDir := filepath.Join(out, "timeseries", rm.base)
	var benches []string
	if runEvents, ok := s.events[rm.base]; ok {
		benches, err = eventTimeSeries(runEvents, outDir, trim)
	} else {
		files := make([]string, len(rm.manifest.Files))
		for i, f := range rm.manifest.Files {
			files[i] = filepath.Join(s.dir, f)
		}
		benches, err = generateTimeSeries(files, outDir, nil, trim)
	}
	if err != nil {
		log.Printf("  warning: time-series: %s: %v", rm.base, err)
		return nil
	}
	return benches
}

// filterRuns drops runs excluded by base name or by a dimension/benchmark value.
func filterRuns(runs []plotRunRecord, opts reportOptions) []plotRunRecord {
	return slices.DeleteFunc(slices.Clone(runs), func(r plotRunRecord) bool {
		return opts.excludeRuns[r.base] || excludedByDim(opts.excludes, r.Dimensions)
	})
}

// excludedByDim reports whether a record matches any --exclude filter. It
// iterates the (small) filter set rather than building a per-record column map.
func excludedByDim(ex map[string]map[string]bool, dims benchkit.Dimensions) bool {
	for col, vals := range ex {
		dim, ok := findDimension(col)
		if !ok {
			continue
		}
		if vals[dim.value(dims)] {
			return true
		}
	}
	return false
}

// compileReport compiles report.typ to a sibling PDF when the typst binary is
// available, otherwise prints the command to run by hand. It returns the path
// a reader should look at: the compiled PDF on success, or report.typ itself
// when typst is unavailable or compilation fails.
func compileReport(typPath string) string {
	if _, err := exec.LookPath("typst"); err != nil {
		log.Printf("  report: typst not found; compile with: typst compile %q", typPath)
		return typPath
	}
	pdf := strings.TrimSuffix(filepath.Base(typPath), ".typ") + ".pdf"
	cmd := exec.Command("typst", "compile", filepath.Base(typPath), pdf)
	cmd.Dir = filepath.Dir(typPath)
	log.Printf("  report: compiling %s...", displayPath(typPath))
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		log.Printf("  report: typst compile failed: %v\n%s", err, outBytes)
		return typPath
	}
	return filepath.Join(filepath.Dir(typPath), pdf)
}

// ── plotdata CSV readers ─────────────────────────────────────────────────────

// readPlotRunsCSV parses the compact per-rep runs.csv back into plotRunRecords.
// Columns are addressed by header name, so extra or reordered columns are
// tolerated; absent latency fields decode to nil pointers.
func readPlotRunsCSV(path string) ([]plotRunRecord, error) {
	var out []plotRunRecord
	err := forEachCSVRow(path, false, func(r []string, col map[string]int) error {
		out = append(out, plotRunRecord{
			Dimensions: benchkit.Dimensions{
				Benchmark:  field(r, col, "benchmark"),
				Nodes:      atoiOr(field(r, col, "nodes"), 0),
				Workers:    atoiOr(field(r, col, "workers"), 0),
				Payload:    atoiOr(field(r, col, "payload"), 0),
				Rate:       atoiOr(field(r, col, "rate"), 0),
				SendBuffer: atoiOr(field(r, col, "send_buffer"), 0),
				RecvBuffer: atoiOr(field(r, col, "recv_buffer"), 0),
				StreamMode: field(r, col, "stream_mode"),
			},
			base:        field(r, col, "base"),
			label:       field(r, col, "label"),
			status:      field(r, col, "status"),
			rep:         atoiOr(field(r, col, "rep"), 1),
			throughput:  atofOr(field(r, col, "throughput"), 0),
			totalOps:    atouOr(field(r, col, "total_ops"), 0),
			failedOps:   atouOr(field(r, col, "failed_ops"), 0),
			allocsPerOp: atofOr(field(r, col, "allocs_per_op"), 0),
			memPerOp:    atofOr(field(r, col, "mem_per_op"), 0),
			nodesSeen:   atoiOr(field(r, col, "nodes_seen"), 0),
			meanUS:      floatPtr(field(r, col, "mean_us")),
			p50US:       floatPtr(field(r, col, "p50_us")),
			p95US:       floatPtr(field(r, col, "p95_us")),
			p99US:       floatPtr(field(r, col, "p99_us")),
			samples:     uintPtr(field(r, col, "samples")),
		})
		return nil
	})
	return out, err
}

// readReportNodeCDFCSV streams the compact per-node data, retaining one row
// per node for the health heatmap and full CDF points for at most limit runs.
// This bounds report memory and output size even when the source contains
// millions of CDF rows.
func readReportNodeCDFCSV(path string, opts reportOptions, limit int) ([]plotNodeCDFRecord, []plotNodeCDFRecord, error) {
	reducer := newReportCDFReducer(opts, limit)
	err := forEachCSVRow(path, true, func(row []string, col map[string]int) error {
		base := field(row, col, "base")
		dims := benchkit.Dimensions{
			Benchmark:  field(row, col, "benchmark"),
			Nodes:      atoiOr(field(row, col, "nodes"), 0),
			Workers:    atoiOr(field(row, col, "workers"), 0),
			Payload:    atoiOr(field(row, col, "payload"), 0),
			Rate:       atoiOr(field(row, col, "rate"), 0),
			SendBuffer: atoiOr(field(row, col, "send_buffer"), 0),
			RecvBuffer: atoiOr(field(row, col, "recv_buffer"), 0),
			StreamMode: field(row, col, "stream_mode"),
		}
		if reducer.excluded(base, dims) {
			return nil
		}

		node := field(row, col, "node")
		keepCDF, keepHealth := reducer.selectRows(
			base, dims, node,
		)
		if !keepCDF && !keepHealth {
			return nil
		}
		record := plotNodeCDFRecord{
			Dimensions: dims,
			base:       base,
			label:      field(row, col, "label"),
			status:     field(row, col, "status"),
			rep:        atoiOr(field(row, col, "rep"), 1),
			node:       node,
			throughput: atofOr(field(row, col, "throughput"), 0),
			meanUS:     atofOr(field(row, col, "mean_us"), 0),
			p50US:      atofOr(field(row, col, "p50_us"), 0),
			p95US:      atofOr(field(row, col, "p95_us"), 0),
			p99US:      atofOr(field(row, col, "p99_us"), 0),
			samples:    atouOr(field(row, col, "samples"), 0),
			prob:       atofOr(field(row, col, "prob"), 0),
			cdfUS:      atofOr(field(row, col, "cdf_us"), 0),
		}
		reducer.add(record, keepCDF, keepHealth)
		return nil
	})
	reducer.finish()
	return reducer.cdf, reducer.health, err
}

// reportCDFReducer keeps the full CDF of the first repetition of selected
// configurations, up to limit runs, plus one health row per node of every
// eligible run. A configuration is the whole dimension tuple, buffer capacities
// included, so each arm of a buffer sweep contributes its own CDF.
//
// Selection spreads the panels across the sweep: a configuration is taken when it
// is the first to measure some dimension value — a node count, payload, offered
// rate, or stream mode no taken configuration used yet — so the panels do not
// crowd into whichever corner of the sweep sorts first. Since that rule alone
// stops as soon as every value is covered, the remaining panels are filled from
// the configurations it passed over, in run order. Both lists are built in one
// streaming pass, and [reportCDFReducer.finish] drops the rows of the runs that
// did not make the budget, so a source with millions of CDF rows is never held in
// memory.
type reportCDFReducer struct {
	opts        reportOptions
	limit       int
	valuesSeen  map[string]bool
	configsSeen map[benchkit.Dimensions]bool
	spread      []string // bases taken for a dimension value no earlier one took
	filler      []string // bases held to fill the panel budget the spread leaves
	retained    map[string]bool
	healthSeen  map[struct{ base, benchmark, node string }]bool
	cdf         []plotNodeCDFRecord
	health      []plotNodeCDFRecord
}

func newReportCDFReducer(opts reportOptions, limit int) *reportCDFReducer {
	return &reportCDFReducer{
		opts:        opts,
		limit:       limit,
		valuesSeen:  make(map[string]bool),
		configsSeen: make(map[benchkit.Dimensions]bool),
		retained:    make(map[string]bool),
		healthSeen:  make(map[struct{ base, benchmark, node string }]bool),
	}
}

func (r *reportCDFReducer) excluded(base string, dims benchkit.Dimensions) bool {
	return r.opts.excludeRuns[base] || excludedByDim(r.opts.excludes, dims)
}

func (r *reportCDFReducer) selectRows(base string, dims benchkit.Dimensions, node string) (keepCDF, keepHealth bool) {
	if !r.configsSeen[dims] {
		r.configsSeen[dims] = true
		switch {
		case r.spreads(dims):
			if len(r.spread) < r.limit {
				r.spread = append(r.spread, base)
				r.retained[base] = true
			}
		case len(r.filler) < r.limit:
			r.filler = append(r.filler, base)
			r.retained[base] = true
		}
	}
	key := struct{ base, benchmark, node string }{base, dims.Benchmark, node}
	if !r.healthSeen[key] {
		r.healthSeen[key] = true
		keepHealth = true
	}
	return r.retained[base], keepHealth
}

// spreads reports whether a configuration takes a dimension value no already
// taken configuration took, recording its values either way.
func (r *reportCDFReducer) spreads(dims benchkit.Dimensions) bool {
	novel := false
	for _, dim := range dimensionSpecs {
		key := dim.name + "=" + dim.value(dims)
		if !r.valuesSeen[key] {
			r.valuesSeen[key] = true
			novel = true
		}
	}
	return novel
}

// finish drops the retained rows of the runs the panel budget cannot hold: the
// spread runs come first, and the fillers take what budget is left. Rows keep
// their original order, so the panels follow run order rather than the order the
// two lists were built in.
func (r *reportCDFReducer) finish() {
	chosen := make(map[string]bool, r.limit)
	for _, base := range r.spread {
		chosen[base] = true
	}
	for _, base := range r.filler {
		if len(chosen) >= r.limit {
			break
		}
		chosen[base] = true
	}
	r.cdf = slices.DeleteFunc(r.cdf, func(row plotNodeCDFRecord) bool { return !chosen[row.base] })
}

func (r *reportCDFReducer) add(record plotNodeCDFRecord, keepCDF, keepHealth bool) {
	if keepCDF {
		r.cdf = append(r.cdf, record)
	}
	if keepHealth {
		r.health = append(r.health, record)
	}
}

func reduceReportCDF(rows []plotNodeCDFRecord, opts reportOptions, limit int) ([]plotNodeCDFRecord, []plotNodeCDFRecord) {
	reducer := newReportCDFReducer(opts, limit)
	for _, record := range rows {
		if reducer.excluded(record.base, record.Dimensions) {
			continue
		}
		keepCDF, keepHealth := reducer.selectRows(record.base, record.Dimensions, record.node)
		reducer.add(record, keepCDF, keepHealth)
	}
	reducer.finish()
	return reducer.cdf, reducer.health
}

// columnIndex maps each header name to its column position.
func columnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[name] = i
	}
	return idx
}

// field returns the value of the named column, or "" when the column is absent
// or the row is short.
func field(row []string, col map[string]int, name string) string {
	i, ok := col[name]
	if !ok || i >= len(row) {
		return ""
	}
	return row[i]
}

func atoiOr(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

func atofOr(s string, def float64) float64 {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return def
}

func atouOr(s string, def uint64) uint64 {
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v
	}
	return def
}

// floatPtr parses s into a *float64, returning nil for an empty field so an
// absent metric round-trips as nil rather than a spurious zero.
func floatPtr(s string) *float64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func uintPtr(s string) *uint64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}
