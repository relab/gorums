package main

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// dimOrder is the sweep dimensions a figure can vary along or facet by, in a
// stable display order.
var dimOrder = func() []string {
	var names []string
	for _, dim := range dimensionSpecs {
		if dim.name != "benchmark" && dim.name != "stream_mode" {
			names = append(names, dim.name)
		}
	}
	return names
}()

// dimLabel is the x-axis label for each sweep dimension.
var dimLabel = func() map[string]string {
	labels := make(map[string]string, len(dimensionSpecs))
	for _, dim := range dimensionSpecs {
		labels[dim.name] = dim.label
	}
	return labels
}()

// dimValue reads one sweep dimension from a record as a number.
func dimValue(r aggRunRecord, dim string) int {
	value, _ := strconv.Atoi(dimensionValue(r.Dimensions, dim))
	return value
}

// dimCounts returns the number of distinct values each sweep dimension takes.
func dimCounts(agg []aggRunRecord) map[string]int {
	seen := map[string]map[int]bool{}
	for _, d := range dimOrder {
		seen[d] = map[int]bool{}
	}
	for _, r := range agg {
		for _, d := range dimOrder {
			seen[d][dimValue(r, d)] = true
		}
	}
	out := map[string]int{}
	for d, s := range seen {
		out[d] = len(s)
	}
	return out
}

// facetFor picks the dimension to facet a figure's panels by when its x-axis is
// xcol: the fewest-valued OTHER dimension that varies, chosen only when two or
// more other dimensions vary (a single varying dimension stays a set of lines
// in one panel). Ties break by dimOrder. Empty means a single panel.
func facetFor(counts map[string]int, xcol string) string {
	best := ""
	varying := 0
	for _, d := range dimOrder {
		if d == xcol || counts[d] <= 1 {
			continue
		}
		varying++
		if best == "" || counts[d] < counts[best] {
			best = d
		}
	}
	if varying < 2 {
		return ""
	}
	return best
}

// figureKind selects which library function renders a figureSpec.
type figureKind int

const (
	kindMetricVs      figureKind = iota // metric-vs against a sweep dimension (agg.csv)
	kindTLCurve                         // throughput-latency curve (tl_curve.csv)
	kindPerNodeCDF                      // per-node latency CDF grid, one panel per run (node_cdf.csv)
	kindRatio                           // dedup/dual metric ratio vs a sweep dimension (comparison.csv)
	kindNodeHealth                      // per-node throughput heatmap (node_health.csv)
	kindDegradedShare                   // degraded-fraction heatmap (degraded_share.csv)
	kindOffsetCDF                       // clock offset/drift CDF (offsets.csv)
	kindRunStatus                       // run-outcome table (run_status.csv)
	kindTimeSeries                      // throughput/latency/saturation over time for one run's benchmark (timeseries/<base>/<bench>_*.csv)
)

// Report sections group the figures that answer one kind of question, so a
// reader can tell the sweep's headline results from its diagnostics. Each
// section starts a new page.
const (
	scalingSection    = "Scaling"
	loadCurveSection  = "Load curves"
	comparisonSection = "Stream-mode comparison"
	healthSection     = "Cluster health"
	cdfSection        = "Per-node latency"
	timeSeriesSection = "Within-run time series"
	failedRunsSection = "Failed runs"
)

// sectionNotes is what each section says about its figures before the first one:
// what they show, and — where the report has to choose which runs to draw — how
// it chose them, which no individual figure is in a position to state.
var sectionNotes = map[string]string{
	scalingSection: "How each metric responds to one swept dimension, rep-averaged with the 95% confidence " +
		"interval of the mean shaded. Panels facet the fewest-valued other dimension that varies; the legend " +
		"names the dimensions that separate the series within a panel.",
	loadCurveSection: "Median latency against the throughput each level of offered load achieved, one panel per " +
		"cluster size, with the p95-p99 tail shaded above the line. Curves whose peak latencies span more than " +
		"a factor of eight are split into scale bands, so one linear axis never crams widely differing curves.",
	comparisonSection: "Each metric of the non-baseline stream mode divided by the baseline's, against a dashed " +
		"parity line: above 1.0 the non-baseline mode is larger. Only configurations measured in both modes " +
		"contribute.",
	healthSection: "Whether the cluster, rather than the code under test, explains a measurement.",
	cdfSection: "One panel per run, one curve per node, shaded light to dark in node order: a node whose latency " +
		"distribution does not belong with its peers' separates from the bundle. Runs are chosen to spread " +
		"across the sweep — a configuration earns a panel when it is the first to measure some dimension value — " +
		"and the remaining panels are filled in run order.",
	timeSeriesSection: "One run per configuration, taken in the order the per-node latency panels selected them " +
		"and alternating stream modes so both arms of a comparison are covered. Every node of the run draws one " +
		"line; consecutive figures over the same nodes share the legend of the first. A run that ramped its " +
		"offered rate gets a third panel tracing achieved throughput against it.",
	failedRunsSection: "A failed run has no aggregate row, so its throughput trace stands here instead: it shows " +
		"whether its nodes were producing work at all, and when they stopped. One representative per " +
		"configuration and error signature is drawn.",
}

// figureSpec is one planned figure: which library call to emit and with what
// axes. yscale maps the CSV column unit to the display unit named by ylabel.
// section names the heading it sits under; note is a caveat printed under this
// figure.
type figureSpec struct {
	kind    figureKind
	slug    string
	heading string
	section string
	note    string
	dataCSV string
	// runScoped marks a figure whose heading already names the run or
	// configuration it draws, so the sweep-wide subject is not appended to it.
	runScoped bool
	// metric-vs fields
	xcol            string
	ycol            string
	bandCol         string
	ylabel          string
	yscale          float64
	facet           string
	payloadPositive bool // restrict to payload>0 rows (goodput)
	// tl-curve fields
	group int
	load  string // the dimension the curve traces along (see tlLoadDims)
	// per-node-cdf fields
	runs []cdfRun
	// time-series fields
	base  string
	bench string // benchmark name within the run named by base
	// sharesNodes marks a figure drawn over the same nodes, in the same order,
	// as the preceding figure of its section: its node colors are those of that
	// figure's legend, so it draws none of its own.
	sharesNodes bool
}

// cdfRun is one run drawn in the per-node latency CDF grid: the run base its
// rows are selected by, and the compact configuration label its panel carries.
type cdfRun struct {
	base  string
	title string
}

// timeSeriesRunFigures names the benchmarks one run base has time-series data
// for, produced by generateTimeSeries. title is the run's compact configuration
// label, which identifies its figures without repeating the run base in a
// heading. sharesNodes marks a run over the same nodes as the previously selected
// one, whose figures then share that run's legend. note carries the caveat a
// failed run's figures print (how it failed, which nodes are absent from the
// figure).
type timeSeriesRunFigures struct {
	base        string
	title       string
	benches     []string
	sharesNodes bool
	note        string
}

// reportInputs names the auxiliary datasets a report can draw beyond the
// tidy-long aggregate, so planFigures includes a figure only when its data was
// produced. cdfRuns lists the runs with per-node CDF data, one panel each;
// timeSeries names one run per configuration whose raw event data could be
// rendered — the configuration's cdfRuns entry when its raw per-node result
// files are still present, otherwise another repetition of the same
// configuration that has them (see writeTimeSeriesFigures). failedTimeSeries
// names the failed runs that get their own report section, which the
// per-configuration figures have no place for (see
// writeFailedTimeSeriesFigures). comparison holds the side-by-side mode rows
// behind comparison.csv, which planFigures consults per metric and per
// x-dimension rather than treating as one sweep-wide flag.
type reportInputs struct {
	cdfRuns          []cdfRun
	timeSeries       []timeSeriesRunFigures
	failedTimeSeries []timeSeriesRunFigures

	comparison    []comparisonRecord
	nodeHealth    bool
	degradedShare bool
	offsets       bool
	runStatus     bool
}

// ratioFigureMetrics are the metrics a ratio figure can draw, each naming its
// ratio column in comparison.csv, the figure's slug and heading stems, its
// y-axis label, and the aggStat the availability check reads.
var ratioFigureMetrics = []struct {
	ratioCol string
	slug     string
	heading  string
	ylabel   string
	get      func(aggRunRecord) aggStat
}{
	{
		ratioCol: "throughput_ratio", slug: "throughput_ratio", heading: "Throughput",
		ylabel: "throughput ratio", get: func(r aggRunRecord) aggStat { return r.throughput },
	},
	{
		ratioCol: "p50_ms_ratio", slug: "latency_ratio", heading: "Median-latency",
		ylabel: "p50 ratio", get: func(r aggRunRecord) aggStat { return r.p50US },
	},
}

// planFigures decides which figures the data supports: a metric-vs-dimension
// figure is planned only when that dimension varies. For each such dimension it
// plans aggregate throughput and median latency; goodput is planned against
// payload and nodes when a non-zero payload varies; per-operation cost against
// nodes when nodes vary. A throughput-latency curve is planned per scale band
// when the worker count varies. The remaining figures are planned when their
// dataset is present per in.
func planFigures(agg []aggRunRecord, in reportInputs) []figureSpec {
	counts := dimCounts(agg)
	var specs []figureSpec

	// A throughput-only sweep (no latency samples or histogram recorded by
	// any run) has no data for a latency figure to show.
	hasLatency := slices.ContainsFunc(agg, func(r aggRunRecord) bool { return r.p50US.n > 0 })
	for _, x := range dimOrder {
		if counts[x] <= 1 {
			continue
		}
		facet := facetFor(counts, x)
		specs = append(specs, figureSpec{
			slug: "throughput_vs_" + x, section: scalingSection,
			heading: "Aggregate throughput vs. " + x,
			dataCSV: "agg.csv", xcol: x, ycol: "throughput", bandCol: "throughput_ci95",
			ylabel: "kops/s", yscale: 1.0 / 1e3, facet: facet,
		})
		if hasLatency {
			specs = append(specs, figureSpec{
				slug: "latency_vs_" + x, section: scalingSection,
				heading: "Median latency vs. " + x,
				dataCSV: "agg.csv", xcol: x, ycol: "p50_ms", bandCol: "p50_ms_ci95",
				ylabel: "p50 (ms)", yscale: 1.0, facet: facet,
			})
		}
	}

	// Goodput (throughput × payload) when a non-zero payload varies.
	if counts["payload"] > 1 && slices.ContainsFunc(agg, func(r aggRunRecord) bool { return r.Payload > 0 }) {
		specs = append(specs, figureSpec{
			slug: "goodput_vs_payload", section: scalingSection,
			heading: "Cluster byte throughput vs. payload",
			dataCSV: "agg.csv", xcol: "payload", ycol: "goodput", bandCol: "goodput_ci95",
			ylabel: "MB/s", yscale: 1.0 / 1e6, facet: facetFor(counts, "payload"),
			payloadPositive: true,
		})
		if counts["nodes"] > 1 {
			specs = append(specs, figureSpec{
				slug: "goodput_vs_nodes", section: scalingSection,
				heading: "Cluster byte throughput vs. nodes",
				dataCSV: "agg.csv", xcol: "nodes", ycol: "goodput", bandCol: "goodput_ci95",
				ylabel: "MB/s", yscale: 1.0 / 1e6, facet: facetFor(counts, "nodes"),
				payloadPositive: true,
			})
		}
	}

	// Per-operation cost vs cluster size.
	if counts["nodes"] > 1 {
		specs = append(specs,
			figureSpec{
				slug: "mem_per_op_vs_nodes", section: scalingSection,
				heading: "Heap bytes per op vs. nodes",
				dataCSV: "agg.csv", xcol: "nodes", ycol: "mem_per_op", bandCol: "mem_per_op_ci95",
				ylabel: "bytes/op", yscale: 1.0, facet: facetFor(counts, "nodes"),
			},
			figureSpec{
				slug: "allocs_per_op_vs_nodes", section: scalingSection,
				heading: "Allocations per op vs. nodes",
				dataCSV: "agg.csv", xcol: "nodes", ycol: "allocs_per_op", bandCol: "allocs_per_op_ci95",
				ylabel: "allocs/op", yscale: 1.0, facet: facetFor(counts, "nodes"),
			},
		)
	}

	// Throughput-latency curve, one figure per load dimension the sweep varied
	// and per scale band within it. Either load dimension yields a curve, so a
	// rate sweep at a fixed worker count gets one just as a worker sweep does.
	loads := tlLoadDimensions(counts)
	for _, load := range loads {
		bands := tlGroups(agg, loads)
		for _, g := range bands {
			slug := "tl_curve_" + load
			heading := "Aggregate throughput vs. latency over " + load
			if len(bands) > 1 {
				slug = fmt.Sprintf("%s_%d", slug, g)
				heading = fmt.Sprintf("%s (scale band %d/%d)", heading, g, len(bands))
			}
			specs = append(specs, figureSpec{
				kind: kindTLCurve, slug: slug, section: loadCurveSection, heading: heading,
				dataCSV: "tl_curve.csv", group: g, load: load,
			})
		}
	}

	// dedup/dual (or non-baseline/baseline) comparison figures, one per metric
	// per varying dimension, mirroring the metric-vs loop above: a comparison
	// swept over any dimension (not just workers) gets a ratio figure against
	// that dimension. Each figure is planned only when the paired comparison
	// rows can actually draw it — the metric present in both modes, and one
	// series varying along x — since ratio-vs drops a single-point series and
	// would render nothing but the parity line.
	for _, x := range dimOrder {
		if counts[x] <= 1 {
			continue
		}
		facet := facetFor(counts, x)
		for _, m := range ratioFigureMetrics {
			if !ratioAxisVaries(in.comparison, m.get, x, facet) {
				continue
			}
			specs = append(specs, figureSpec{
				kind: kindRatio, slug: m.slug + "_vs_" + x, section: comparisonSection,
				heading: m.heading + " ratio vs. " + x + " (non-baseline / baseline)",
				dataCSV: "comparison.csv", xcol: x, ycol: m.ratioCol, ylabel: m.ylabel, facet: facet,
			})
		}
	}

	// Diagnostics.
	if in.nodeHealth {
		specs = append(specs, figureSpec{
			kind: kindNodeHealth, slug: "node_health", section: healthSection,
			heading: "Per-node throughput relative to run median",
			dataCSV: "node_health.csv", note: nodeHealthNote,
		})
	}
	if in.degradedShare {
		specs = append(specs, figureSpec{
			kind: kindDegradedShare, slug: "degraded_share", section: healthSection,
			heading: "Degraded-repetition fraction",
			dataCSV: "degraded_share.csv", note: degradedShareNote,
		})
	}
	if in.offsets {
		specs = append(specs, figureSpec{
			kind: kindOffsetCDF, slug: "clock_offsets", section: healthSection,
			heading: "Clock offset and residual drift",
			dataCSV: "offsets.csv",
		})
	}
	if in.runStatus {
		specs = append(specs, figureSpec{
			kind: kindRunStatus, slug: "run_status", section: healthSection,
			heading: "Run outcomes per node count",
			dataCSV: "run_status.csv",
		})
	}

	// Per-node latency CDF: one figure whose panels are the runs the caller
	// selected, so a page shows a grid of runs rather than one run per page.
	if len(in.cdfRuns) > 0 {
		specs = append(specs, figureSpec{
			kind: kindPerNodeCDF, slug: "per_node_cdf", section: cdfSection,
			heading: "Per-node latency CDF",
			dataCSV: "node_cdf.csv", runs: in.cdfRuns,
		})
	}

	// Throughput, latency, and (for a run that ramped its offered rate) the
	// saturation curve over time, one figure per (run, benchmark) with raw event
	// data.
	for _, ts := range in.timeSeries {
		specs = append(specs, timeSeriesFigures(ts, "time_series", timeSeriesSection)...)
	}

	// Failed runs, last and under their own heading: a failed run has no
	// aggregate row, so its trace cannot sit in the per-configuration structure
	// above. Each carries the note stating how it failed and which nodes are
	// absent from the figure.
	for _, ts := range in.failedTimeSeries {
		specs = append(specs, timeSeriesFigures(ts, "failed_time_series", failedRunsSection)...)
	}

	if subject := figureSubject(agg); subject != "" {
		for i, s := range specs {
			if !s.runScoped {
				specs[i].heading = s.heading + " — " + subject
			}
		}
	}
	return specs
}

// timeSeriesFigures plans the over-time figures for one run, one per benchmark
// it recorded event data for, with slugs under the given stem and under the
// given section. Each figure's heading names the run by its compact
// configuration label rather than its base, which the figure's data note carries
// instead.
func timeSeriesFigures(ts timeSeriesRunFigures, slugStem, section string) []figureSpec {
	specs := make([]figureSpec, 0, len(ts.benches))
	for _, bench := range ts.benches {
		heading := "Throughput and latency over time"
		if ts.title != "" {
			heading += " — " + ts.title
		}
		specs = append(specs, figureSpec{
			kind: kindTimeSeries, section: section,
			slug:      slugStem + "_" + ts.base + "_" + bench,
			heading:   heading,
			runScoped: ts.title != "",
			note:      ts.note,
			dataCSV:   timeSeriesDataNote(ts.base, bench),
			base:      ts.base, bench: bench,
			sharesNodes: ts.sharesNodes,
		})
	}
	return specs
}

// figureSubject names the categorical identity every figure of a report shares:
// the single benchmark the sweep measured and, when it compared none, the single
// stream mode. It belongs in the section headings, since a legend that repeats
// it on every entry spends the figure's width on what does not distinguish one
// series from another. It is empty when the sweep varied both, which the legends
// then carry.
func figureSubject(agg []aggRunRecord) string {
	var parts []string
	for _, dim := range []string{"benchmark", "stream_mode"} {
		values := map[string]bool{}
		for _, r := range agg {
			values[dimensionValue(r.Dimensions, dim)] = true
		}
		if len(values) != 1 {
			continue
		}
		for value := range values {
			if value != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.Join(parts, ", ")
}

// Notes printed under the diagnostic figures, which show a distribution over
// runs rather than a measured metric and are read wrong without them.
const (
	nodeHealthNote = "Each cell is a host's median throughput across the repetitions of one configuration, " +
		"divided by that run's median across hosts: a uniform cluster is green everywhere near 1.0, " +
		"and a host behind a slow link or a throttled CPU stands out low. " +
		"Grey means the host took no part in that configuration."
	degradedShareNote = "Each cell is the fraction of one configuration's repetitions the sweep flagged degraded " +
		"(a node whose throughput or latency did not belong with its peers'; see the run-outcome table). " +
		"Green is zero and red is every repetition."
)

// timeSeriesCSVPaths returns the throughput, latency, and saturation-curve
// CSV paths (relative to the report directory) generateTimeSeries wrote for
// one run's benchmark.
func timeSeriesCSVPaths(base, bench string) (tput, lat, sat string) {
	dir := filepath.Join("timeseries", base)
	return filepath.Join(dir, bench+"_throughput.csv"),
		filepath.Join(dir, bench+"_latency.csv"),
		filepath.Join(dir, bench+"_saturation.csv")
}

// timeSeriesDataNote names the three CSVs one over-time figure reads, in the
// brace form a shell uses, so the note identifies the run without printing its
// directory three times.
func timeSeriesDataNote(base, bench string) string {
	return filepath.Join("timeseries", base, bench) + "_{throughput,latency,saturation}.csv"
}

// tlGroups returns the distinct scale-band group numbers present in the
// throughput-latency points, in ascending order.
func tlGroups(agg []aggRunRecord, loads []string) []int {
	seen := map[int]bool{}
	for _, p := range tlCurveRows(agg, loads) {
		seen[p.group] = true
	}
	return slices.Sorted(maps.Keys(seen))
}

// reportHeader is what a report says about itself before its first figure: the
// title, and the one-line description of the experiment behind the data. The
// experiment line is where the sweep's identity and its fixed configuration
// live, so no figure heading has to repeat them.
type reportHeader struct {
	title      string
	experiment string
}

// experimentSummary describes the sweep behind a report in one line: the label
// that names it, every dimension it measured with the values it took, and the
// sweep-wide settings from its manifests.
func experimentSummary(agg []aggRunRecord, settings sweepSettings) string {
	var parts []string
	if settings.label != "" {
		parts = append(parts, settings.label)
	}
	for _, dim := range dimensionSpecs {
		values := dimensionSpread(agg, dim)
		if len(values) == 0 {
			continue
		}
		switch dim.name {
		case "benchmark":
			parts = append(parts, strings.Join(values, ", "))
		case "stream_mode":
			parts = append(parts, "stream mode "+strings.Join(values, ", "))
		default:
			parts = append(parts, strings.ReplaceAll(dim.name, "_", " ")+" "+strings.Join(values, ", "))
		}
	}
	scale := fmt.Sprintf("%d configurations", len(agg))
	if settings.runs > 0 {
		scale += fmt.Sprintf(", %d runs", settings.runs)
	}
	parts = append(parts, scale)
	if settings.duration != "" {
		run := settings.duration + " per run"
		if settings.trim != "" {
			run += ", " + settings.trim + " trim"
		}
		parts = append(parts, run)
	}
	return strings.Join(parts, "; ")
}

// dimensionSpread returns the distinct values a dimension took across the
// aggregate, numeric dimensions in ascending order and the rest alphabetically.
// A dimension left at its unset marker (0) yields nothing: the sweep did not
// configure it, so it describes no part of the experiment.
func dimensionSpread(agg []aggRunRecord, dim dimensionSpec) []string {
	seen := map[string]bool{}
	for _, r := range agg {
		if value := dim.value(r.Dimensions); value != "" && value != "0" {
			seen[value] = true
		}
	}
	values := slices.Collect(maps.Keys(seen))
	slices.SortFunc(values, func(a, b string) int {
		if dim.tag == "" {
			return strings.Compare(a, b)
		}
		return cmp.Compare(atoiOr(a, 0), atoiOr(b, 0))
	})
	return values
}

// reportPreamble styles the generated report: a large centered title, section
// headings that each open a page, and figure headings under them.
const reportPreamble = `#import "gorumsplot.typ": *
#set page(paper: "a4", margin: 2cm)
#set text(size: 10pt)
#show heading.where(level: 1): it => align(center, block(below: 0.7em, text(size: 20pt, weight: "bold", it.body)))
#show heading.where(level: 2): it => block(above: 0.4em, below: 0.8em, text(size: 14pt, weight: "bold", it.body))
#show heading.where(level: 3): it => block(above: 1.1em, below: 0.6em, text(size: 11pt, weight: "bold", it.body))
`

// writeReportTyp emits a self-contained report.typ that imports the copied
// helper library, loads the CSVs, and renders each planned figure under its own
// heading, grouped into sections that each start a page. header names the report
// and the experiment behind it.
func writeReportTyp(path string, header reportHeader, specs []figureSpec) error {
	var b strings.Builder
	fmt.Fprint(&b, reportPreamble)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "= #text(%q)\n\n", header.title)
	if header.experiment != "" {
		fmt.Fprintf(&b, "#align(center)[#emph[#text(%q)]]\n\n", header.experiment)
	}
	// Load only the CSVs the planned figures reference.
	loaded := map[string]bool{}
	for _, s := range specs {
		v := csvVar[s.dataCSV]
		if v == "" || loaded[v] {
			continue
		}
		loaded[v] = true
		fmt.Fprintf(&b, "#let %s = csv(%q, row-type: dictionary)\n", v, s.dataCSV)
	}
	fmt.Fprintln(&b)

	if len(specs) == 0 {
		fmt.Fprintln(&b, "_No sweep dimension varies; nothing to plot._")
	}
	section := ""
	for i, s := range specs {
		if s.section != section {
			section = s.section
			// Each section opens a page, so a section's figures are read
			// together and none is split across a page boundary that a
			// preceding section's length happened to fall on. The break is weak,
			// so the first section stays on the title page.
			if i > 0 {
				fmt.Fprintln(&b, "#pagebreak(weak: true)")
			}
			fmt.Fprintf(&b, "== #text(%q)\n\n", section)
			if note := sectionNotes[section]; note != "" {
				fmt.Fprintf(&b, "#emph[#text(%q)]\n\n", note)
			}
		}
		fmt.Fprintf(&b, "=== #text(%q)\n\n", s.heading)
		call := figureCall(s)
		if s.kind == kindRunStatus {
			fmt.Fprintln(&b, "#"+call)
		} else {
			fmt.Fprintln(&b, "#fitwidth("+call+")")
		}
		if s.note != "" {
			fmt.Fprintf(&b, "\n#emph[#text(%q)]\n", s.note)
		}
		fmt.Fprintf(&b, "\n#emph[Data: `%s`.]\n\n", s.dataCSV)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// csvVar maps a CSV filename to the Typst binding the report loads it into. A
// filename with no entry yields the empty string, and its figure is skipped.
var csvVar = map[string]string{
	"agg.csv":            "agg",
	"tl_curve.csv":       "tl",
	"node_cdf.csv":       "cdf",
	"comparison.csv":     "cmp",
	"node_health.csv":    "nh",
	"degraded_share.csv": "dg",
	"offsets.csv":        "off",
	"run_status.csv":     "st",
}

// figureCall renders the Typst call that draws a figureSpec.
func figureCall(s figureSpec) string {
	switch s.kind {
	case kindTLCurve:
		return fmt.Sprintf("tl-curve(tl, group: %d, load: %q)", s.group, s.load)
	case kindPerNodeCDF:
		var runs strings.Builder
		for _, run := range s.runs {
			fmt.Fprintf(&runs, "(base: %q, title: %q), ", run.base, run.title)
		}
		return fmt.Sprintf("per-node-cdf(cdf, (%s))", runs.String())
	case kindRatio:
		facet, facetLabel := "none", "none"
		if s.facet != "" {
			facet = fmt.Sprintf("%q", s.facet)
			// A plain Typst string, not a content block: the template
			// concatenates it with "= " + value via the string "+" operator.
			facetLabel = fmt.Sprintf("%q", dimLabel[s.facet])
		}
		return fmt.Sprintf(
			`ratio-vs(cmp, %q, xcol: %q, xlabel: [%s], ylabel: [%s], facet: %s, facet-label: %s)`,
			s.ycol, s.xcol, dimLabel[s.xcol], s.ylabel, facet, facetLabel,
		)
	case kindNodeHealth:
		return `heatmap(nh, xcol: "col", ycol: "host", valuecol: "rel", ` +
			`label: [host throughput / run median])`
	case kindDegradedShare:
		return `heatmap(dg, xcol: "col", ycol: "row", valuecol: "share", vmax: 1.0, reverse: true, ` +
			`label: [degraded fraction])`
	case kindOffsetCDF:
		return "offset-cdf(off)"
	case kindRunStatus:
		return "run-status-table(st)"
	case kindTimeSeries:
		tput, lat, sat := timeSeriesCSVPaths(s.base, s.bench)
		legend := ""
		if s.sharesNodes {
			legend = ", legend: false"
		}
		return fmt.Sprintf(
			"time-series(csv(%q, row-type: dictionary), csv(%q, row-type: dictionary), "+
				"sat: csv(%q, row-type: dictionary)%s)",
			tput, lat, sat, legend,
		)
	default:
		return metricVsCall(s)
	}
}

// metricVsCall renders the Typst call that draws a figureSpec via metric-vs.
func metricVsCall(s figureSpec) string {
	data := "agg"
	if s.payloadPositive {
		data = `agg.filter(r => int(r.payload) > 0)`
	}
	facet, facetLabel := "none", "none"
	if s.facet != "" {
		facet = fmt.Sprintf("%q", s.facet)
		// A plain Typst string, not a content block: the template
		// concatenates it with "= " + value via the string "+" operator.
		facetLabel = fmt.Sprintf("%q", dimLabel[s.facet])
	}
	return fmt.Sprintf(
		`metric-vs(%s, xcol: %q, ycol: %q, band-col: %q, ylabel: [%s], xlabel: [%s], yscale: %s, facet: %s, facet-label: %s)`,
		data, s.xcol, s.ycol, s.bandCol,
		s.ylabel, dimLabel[s.xcol], formatFloat(s.yscale), facet, facetLabel,
	)
}
