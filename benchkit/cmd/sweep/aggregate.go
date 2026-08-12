package main

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/relab/gorums/benchkit"
	"golang.org/x/exp/stats"
)

// aggStat is one metric summarized across the repetitions of a configuration:
// the mean, the sample standard deviation, and the 95% confidence interval
// half-width of the mean (tCritical95(n-1)·sd/√n). n is the number of
// repetitions folded in; n == 0 marks a metric absent from every repetition
// (e.g. latency on a run that recorded none), which the CSV writer emits as
// an empty field.
type aggStat struct {
	mean float64
	sd   float64
	ci95 float64
	n    int
}

// meanSDCI summarizes xs. With fewer than two samples the spread is zero; an
// empty slice yields the zero aggStat (n == 0) rather than dividing by zero.
func meanSDCI(xs []float64) aggStat {
	if len(xs) == 0 {
		return aggStat{}
	}
	mean, sd := stats.MeanAndStdDev(xs) // sd == 0 for a single sample
	ci95 := 0.0
	if len(xs) >= 2 {
		ci95 = tCritical95(len(xs)-1) * sd / math.Sqrt(float64(len(xs)))
	}
	return aggStat{mean: mean, sd: sd, ci95: ci95, n: len(xs)}
}

// tCritical95Table holds the two-tailed 95% Student's t critical value for
// degrees of freedom 1..30 (index 0 unused). Sweep repetition counts are
// typically in this range, where the t distribution's heavier tails matter:
// at df=2 (3 reps) the true multiplier is 4.30, more than double the normal
// distribution's 1.96 that a rep count this small does not justify.
var tCritical95Table = [...]float64{
	0, // unused
	12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228,
	2.201, 2.179, 2.160, 2.145, 2.131, 2.120, 2.110, 2.101, 2.093, 2.086,
	2.080, 2.074, 2.069, 2.064, 2.060, 2.056, 2.052, 2.048, 2.045, 2.042,
}

// tCritical95 returns the two-tailed 95% Student's t critical value for the
// given degrees of freedom. Beyond the table it uses the asymptotic expansion
// around the normal critical value, avoiding an abrupt underestimate at df=31.
func tCritical95(df int) float64 {
	if df >= 1 && df < len(tCritical95Table) {
		return tCritical95Table[df]
	}

	const normalCritical95 = 1.959963984540054
	if df <= 0 {
		return normalCritical95
	}
	v := float64(df)
	z := normalCritical95
	z2 := z * z
	z3 := z2 * z
	z5 := z3 * z2
	z7 := z5 * z2
	return z +
		(z3+z)/(4*v) +
		(5*z5+16*z3+3*z)/(96*v*v) +
		(3*z7+19*z5+17*z3-15*z)/(384*v*v*v)
}

// aggRunRecord is the rep-averaged reduction of the per-rep rows in runs.csv:
// one row per configuration with each metric's mean, spread, and repetition
// counts. It is the primary tidy-long table the generated Typst figures read.
type aggRunRecord struct {
	benchkit.Dimensions
	reps         int // repetitions folded into the means
	repsDegraded int // repetitions flagged degraded for this configuration
	throughput   aggStat
	allocsPerOp  aggStat
	memPerOp     aggStat
	meanUS       aggStat
	p50US        aggStat
	p95US        aggStat
	p99US        aggStat
}

// aggregateReps folds the per-rep records of each configuration into one
// rep-averaged record. Degraded repetitions are counted in repsDegraded and,
// unless includeDegraded is set, excluded from the means (their contaminated
// aggregates would otherwise bias the configuration). A configuration with no
// surviving repetitions is dropped. Records are returned sorted by
// benchmark, nodes, workers, payload, rate, buffer sizes, then stream mode.
func aggregateReps(runs []plotRunRecord, includeDegraded bool) []aggRunRecord {
	type bucket struct {
		thr, allocs, mem      []float64
		meanUS, p50, p95, p99 []float64
		degraded              int
	}
	buckets := make(map[benchkit.Dimensions]*bucket)
	var order []benchkit.Dimensions
	for _, r := range runs {
		key := r.Dimensions
		b := buckets[key]
		if b == nil {
			b = &bucket{}
			buckets[key] = b
			order = append(order, key)
		}
		if r.status == runStatusDegraded {
			b.degraded++
			if !includeDegraded {
				continue
			}
		}
		b.thr = append(b.thr, r.throughput)
		b.allocs = append(b.allocs, r.allocsPerOp)
		b.mem = append(b.mem, r.memPerOp)
		if r.meanUS != nil {
			b.meanUS = append(b.meanUS, *r.meanUS)
		}
		if r.p50US != nil {
			b.p50 = append(b.p50, *r.p50US)
		}
		if r.p95US != nil {
			b.p95 = append(b.p95, *r.p95US)
		}
		if r.p99US != nil {
			b.p99 = append(b.p99, *r.p99US)
		}
	}
	slices.SortFunc(order, compareDimensions)
	out := make([]aggRunRecord, 0, len(order))
	for _, key := range order {
		b := buckets[key]
		if len(b.thr) == 0 {
			continue // only degraded reps, excluded
		}
		out = append(out, aggRunRecord{
			Dimensions: key,
			reps:       len(b.thr), repsDegraded: b.degraded,
			throughput:  meanSDCI(b.thr),
			allocsPerOp: meanSDCI(b.allocs),
			memPerOp:    meanSDCI(b.mem),
			meanUS:      meanSDCI(b.meanUS),
			p50US:       meanSDCI(b.p50),
			p95US:       meanSDCI(b.p95),
			p99US:       meanSDCI(b.p99),
		})
	}
	return out
}

// repOutlierSpread is how far a repetition's throughput may differ from its
// configuration's median, in either direction, before the report names it. Real
// repetitions of a healthy configuration cluster far more tightly than this;
// anything beyond it is a measurement to explain, not a data point to average.
const repOutlierSpread = 1.4

// repOutliers describes every repetition whose throughput differs from its
// configuration's median by more than spread in either direction, in run-base
// order. It is the report's defense in depth behind the sweep's own per-node
// bounds (see degraded.go): a directory collected before those bounds existed,
// or with them disabled, still gets its contaminated repetitions named rather
// than silently averaged in. Repetitions already flagged degraded are left out,
// since they are reported as such, and a configuration with fewer than three
// repetitions is skipped, because with two neither one is the outlier.
func repOutliers(runs []plotRunRecord, spread float64) []string {
	if spread <= 1 {
		return nil
	}
	byConfig := map[benchkit.Dimensions][]plotRunRecord{}
	for _, r := range runs {
		if r.status == runStatusDegraded {
			continue
		}
		byConfig[r.Dimensions] = append(byConfig[r.Dimensions], r)
	}
	var notes []string
	for _, reps := range byConfig {
		if len(reps) < 3 {
			continue
		}
		throughputs := make([]float64, len(reps))
		for i, r := range reps {
			throughputs[i] = r.throughput
		}
		median := stats.Median(slices.Sorted(slices.Values(throughputs)))
		if median <= 0 {
			continue
		}
		for _, r := range reps {
			relative := r.throughput / median
			if relative > spread || relative < 1/spread {
				notes = append(notes, fmt.Sprintf(
					"run %s: %.0f ops/s is %.2fx the median of its %d repetitions",
					r.base, r.throughput, relative, len(reps)))
			}
		}
	}
	slices.Sort(notes)
	return notes
}

// writeAggRunsCSV writes the rep-averaged tidy-long table. Each metric
// contributes value/_sd/_ci95 columns; latency percentiles additionally get
// millisecond mirrors. An absent metric (n == 0) is written as empty fields.
func writeAggRunsCSV(path string, rows []aggRunRecord) error {
	header := append(dimensionColumns(),
		[]string{
			"reps", "reps_degraded",
			"throughput", "throughput_sd", "throughput_ci95",
			"goodput", "goodput_sd", "goodput_ci95",
			"allocs_per_op", "allocs_per_op_sd", "allocs_per_op_ci95",
			"mem_per_op", "mem_per_op_sd", "mem_per_op_ci95",
			"mean_us", "mean_us_sd", "mean_us_ci95",
			"p50_us", "p50_us_sd", "p50_us_ci95",
			"p95_us", "p95_us_sd", "p95_us_ci95",
			"p99_us", "p99_us_sd", "p99_us_ci95",
			"p50_ms", "p50_ms_sd", "p50_ms_ci95",
			"p95_ms", "p95_ms_sd", "p95_ms_ci95",
			"p99_ms", "p99_ms_sd", "p99_ms_ci95",
		}...)
	return writeCSV(path, header, rows, func(r aggRunRecord) []string {
		rec := append(dimensionValues(r.Dimensions),
			[]string{
				strconv.Itoa(r.reps), strconv.Itoa(r.repsDegraded),
			}...)
		rec = append(rec, statCols(r.throughput, 1)...)
		rec = append(rec, statCols(goodputStat(r), 1)...)
		rec = append(rec, statCols(r.allocsPerOp, 1)...)
		rec = append(rec, statCols(r.memPerOp, 1)...)
		rec = append(rec, statCols(r.meanUS, 1)...)
		rec = append(rec, statCols(r.p50US, 1)...)
		rec = append(rec, statCols(r.p95US, 1)...)
		rec = append(rec, statCols(r.p99US, 1)...)
		rec = append(rec, statCols(r.p50US, 1.0/1e3)...)
		rec = append(rec, statCols(r.p95US, 1.0/1e3)...)
		rec = append(rec, statCols(r.p99US, 1.0/1e3)...)
		return rec
	})
}

// comparisonMetrics are the metrics pivoted side by side in the wide
// comparison table. Each carries the scale from its stored aggStat unit
// (throughput as-is; latency percentiles µs→ms) to the emitted column.
var comparisonMetrics = []struct {
	name  string
	get   func(aggRunRecord) aggStat
	scale float64
}{
	{"throughput", func(r aggRunRecord) aggStat { return r.throughput }, 1},
	{"p50_ms", func(r aggRunRecord) aggStat { return r.p50US }, 1.0 / 1e3},
	{"p95_ms", func(r aggRunRecord) aggStat { return r.p95US }, 1.0 / 1e3},
	{"p99_ms", func(r aggRunRecord) aggStat { return r.p99US }, 1.0 / 1e3},
}

// comparisonRecord holds one configuration's rep-averaged records for each
// stream mode present, so the wide table can place the modes side by side and
// derive their ratios.
type comparisonRecord struct {
	benchkit.Dimensions
	baseline string
	perMode  map[string]aggRunRecord
}

// pivotComparison groups the rep-averaged records by configuration (ignoring
// stream mode) so each mode's metrics can be compared side by side. It returns
// nil unless at least two stream modes are present in the data — the wide
// comparison table exists only for a mode-vs-mode study. baseline names the
// denominator mode for ratios; when empty or absent it defaults to "dual" if
// present, else the lexically first mode.
func pivotComparison(agg []aggRunRecord, baseline string) []comparisonRecord {
	modes := make(map[string]bool)
	for _, r := range agg {
		modes[r.StreamMode] = true
	}
	if len(modes) < 2 {
		return nil
	}
	if !modes[baseline] {
		if modes["dual"] {
			baseline = "dual"
		} else {
			baseline = slices.Min(slices.Collect(maps.Keys(modes)))
		}
	}

	byConfig := make(map[benchkit.Dimensions]*comparisonRecord)
	var order []benchkit.Dimensions
	for _, r := range agg {
		key := comparisonDimensions(r.Dimensions)
		c := byConfig[key]
		if c == nil {
			c = &comparisonRecord{
				Dimensions: key, baseline: baseline,
				perMode: make(map[string]aggRunRecord),
			}
			byConfig[key] = c
			order = append(order, key)
		}
		c.perMode[r.StreamMode] = r
	}
	slices.SortFunc(order, compareDimensions)
	out := make([]comparisonRecord, 0, len(order))
	for _, key := range order {
		out = append(out, *byConfig[key])
	}
	return out
}

// ratioStat divides metric x by baseline y, propagating relative uncertainty:
// sd_r = |r|·√((sd_x/x)² + (sd_y/y)²). It reports false when either metric is
// absent or the baseline mean is zero.
func ratioStat(x, y aggStat) (aggStat, bool) {
	if x.n == 0 || y.n == 0 || y.mean == 0 {
		return aggStat{}, false
	}
	r := x.mean / y.mean
	var rel float64
	if x.mean != 0 {
		rel += (x.sd / x.mean) * (x.sd / x.mean)
	}
	rel += (y.sd / y.mean) * (y.sd / y.mean)
	return aggStat{mean: r, sd: math.Abs(r) * math.Sqrt(rel), n: min(x.n, y.n)}, true
}

// writeComparisonCSV writes the wide comparison table: per configuration, each
// metric's value/_sd for every stream mode present across the data, followed
// by the non-baseline/baseline ratio (and its propagated _sd) for the metrics
// of configurations that hold exactly the baseline plus one other mode.
func writeComparisonCSV(path string, rows []comparisonRecord) error {
	modeSet := make(map[string]bool)
	for _, r := range rows {
		for m := range r.perMode {
			modeSet[m] = true
		}
	}
	allModes := slices.Sorted(maps.Keys(modeSet))

	header := append(dimensionColumns("stream_mode"), "baseline", "modes")
	for _, m := range comparisonMetrics {
		for _, mode := range allModes {
			header = append(header, m.name+"_"+mode, m.name+"_"+mode+"_sd")
		}
	}
	for _, m := range comparisonMetrics {
		header = append(header, m.name+"_ratio", m.name+"_ratio_sd")
	}

	return writeCSV(path, header, rows, func(r comparisonRecord) []string {
		present := slices.Sorted(maps.Keys(r.perMode))
		rec := append(dimensionValues(r.Dimensions, "stream_mode"), r.baseline, strings.Join(present, "|"))
		for _, m := range comparisonMetrics {
			for _, mode := range allModes {
				rr, ok := r.perMode[mode]
				if !ok {
					rec = append(rec, "", "")
					continue
				}
				s := m.get(rr)
				if s.n == 0 {
					rec = append(rec, "", "")
					continue
				}
				rec = append(rec, formatFloat(s.mean*m.scale), formatFloat(s.sd*m.scale))
			}
		}
		// Ratio is defined only when the baseline and exactly one other mode
		// are present; more modes leave the comparison ambiguous.
		other, ratioable := soleOtherMode(present, r.baseline)
		for _, m := range comparisonMetrics {
			if !ratioable {
				rec = append(rec, "", "")
				continue
			}
			ratio, ok := ratioStat(m.get(r.perMode[other]), m.get(r.perMode[r.baseline]))
			if !ok {
				rec = append(rec, "", "")
				continue
			}
			rec = append(rec, formatFloat(ratio.mean), formatFloat(ratio.sd))
		}
		return rec
	})
}

// ratioAxisVaries reports whether the paired comparison rows can draw a ratio
// line for one metric against xcol. It mirrors what the ratio-vs figure draws:
// a series is a benchmark plus the dimensions held fixed within a panel (every
// dimension other than xcol and facet, which is empty for a single-panel
// figure), and a series needs a computable ratio at two or more distinct x
// values before anything but the parity line appears. A metric absent from
// either mode, or a configuration without exactly the baseline and one other
// mode, has no ratio and contributes no point.
func ratioAxisVaries(rows []comparisonRecord, metric func(aggRunRecord) aggStat, xcol, facet string) bool {
	// The facet is part of the series identity: each panel holds its own lines.
	seriesDims := append([]string{facet}, slices.DeleteFunc(slices.Clone(dimOrder), func(d string) bool {
		return d == xcol || d == facet
	})...)
	xvalues := map[string]map[string]bool{}
	for _, r := range rows {
		other, ok := soleOtherMode(slices.Sorted(maps.Keys(r.perMode)), r.baseline)
		if !ok {
			continue
		}
		if _, ok := ratioStat(metric(r.perMode[other]), metric(r.perMode[r.baseline])); !ok {
			continue
		}
		parts := make([]string, 0, len(seriesDims)+1)
		parts = append(parts, r.Benchmark)
		for _, d := range seriesDims {
			parts = append(parts, dimensionValue(r.Dimensions, d))
		}
		key := strings.Join(parts, "|")
		if xvalues[key] == nil {
			xvalues[key] = map[string]bool{}
		}
		xvalues[key][dimensionValue(r.Dimensions, xcol)] = true
		if len(xvalues[key]) > 1 {
			return true
		}
	}
	return false
}

// soleOtherMode returns the single non-baseline mode when present holds exactly
// the baseline and one other mode; otherwise it reports false.
func soleOtherMode(present []string, baseline string) (string, bool) {
	if len(present) != 2 || !slices.Contains(present, baseline) {
		return "", false
	}
	for _, m := range present {
		if m != baseline {
			return m, true
		}
	}
	return "", false
}

// goodputStat derives cluster byte throughput (throughput × payload) for one
// configuration. The spread scales linearly with the constant payload, so the
// sample sd and CI carry over multiplied by the payload.
func goodputStat(r aggRunRecord) aggStat {
	p := float64(r.Payload)
	return aggStat{
		mean: r.throughput.mean * p,
		sd:   r.throughput.sd * p,
		ci95: r.throughput.ci95 * p,
		n:    r.throughput.n,
	}
}

// statCols renders one aggStat as its value/_sd/_ci95 fields, scaling every
// component by scale (e.g. 1/1000 to convert µs to ms). An absent metric
// (n == 0) becomes three empty fields.
func statCols(s aggStat, scale float64) []string {
	if s.n == 0 {
		return []string{"", "", ""}
	}
	return []string{
		formatFloat(s.mean * scale),
		formatFloat(s.sd * scale),
		formatFloat(s.ci95 * scale),
	}
}
