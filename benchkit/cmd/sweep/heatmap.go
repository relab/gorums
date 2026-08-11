package main

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/relab/gorums/benchkit"
	"golang.org/x/exp/stats"
)

// nodeHealthRecord summarizes one host's throughput relative to its run median
// across repetitions and configurations with the same benchmark, node count,
// and stream mode. A healthy cluster is uniform near 1.0; a host behind a slow
// link or a faulty host shows as a low value, so the heatmap exposes it at a
// glance without producing one column per repetition.
type nodeHealthRecord struct {
	benchkit.Dimensions
	host       string
	throughput float64
	rel        float64
	runs       int
}

// nodeHealthRows first reduces the per-node CDF rows to one throughput per node
// per run, then divides by that run's median node throughput. It finally takes
// the median of those relative values for each benchmark/node-count/mode/host
// combination. A run whose median is zero is treated as uniform (rel 1.0)
// rather than producing NaN. Rows are sorted by benchmark, mode, node count,
// then host in natural order (bb2 before bb10).
func nodeHealthRows(cdf []plotNodeCDFRecord) []nodeHealthRecord {
	type key struct{ base, benchmark, node string }
	first := make(map[key]plotNodeCDFRecord)
	var order []key
	for _, r := range cdf {
		k := key{r.base, r.Benchmark, r.node}
		if _, ok := first[k]; ok {
			continue
		}
		first[k] = r
		order = append(order, k)
	}
	type runKey struct{ base, benchmark string }
	byRun := make(map[runKey][]float64)
	for _, k := range order {
		rec := first[k]
		rk := runKey{k.base, k.benchmark}
		byRun[rk] = append(byRun[rk], rec.throughput)
	}
	medians := make(map[runKey]float64, len(byRun))
	for k, xs := range byRun {
		if len(xs) > 0 {
			medians[k] = stats.Median(xs)
		}
	}

	type summaryKey struct {
		benchkit.Dimensions
		host string
	}
	type summary struct {
		throughput []float64
		rel        []float64
	}
	summaries := make(map[summaryKey]*summary)
	for _, k := range order {
		rec := first[k]
		rk := runKey{rec.base, rec.Benchmark}
		if len(byRun[rk]) < 2 {
			continue
		}
		sk := summaryKey{
			Dimensions: nodeHealthDimensions(rec.Dimensions),
			host:       hostFromAddr(rec.node),
		}
		s := summaries[sk]
		if s == nil {
			s = &summary{}
			summaries[sk] = s
		}
		s.throughput = append(s.throughput, rec.throughput)
		if m := medians[rk]; m > 0 {
			s.rel = append(s.rel, rec.throughput/m)
		} else {
			s.rel = append(s.rel, 1.0)
		}
	}

	out := make([]nodeHealthRecord, 0, len(summaries))
	for key, s := range summaries {
		out = append(out, nodeHealthRecord{
			Dimensions: key.Dimensions,
			host:       key.host,
			throughput: stats.Median(s.throughput),
			rel:        stats.Median(s.rel),
			runs:       len(s.rel),
		})
	}
	slices.SortFunc(out, func(a, b nodeHealthRecord) int {
		return cmp.Or(
			compareDimensions(a.Dimensions, b.Dimensions),
			compareHost(a.host, b.host),
		)
	})
	return out
}

// compareHost orders hosts naturally so a numeric suffix sorts by value
// (bb2 before bb10) rather than lexically.
func compareHost(a, b string) int {
	ap, an := splitTrailingNum(a)
	bp, bn := splitTrailingNum(b)
	if ap != bp {
		return cmp.Compare(ap, bp)
	}
	return cmp.Compare(an, bn)
}

// splitTrailingNum splits a host into its non-numeric prefix and trailing
// integer (0 when absent), e.g. "bb10" -> ("bb", 10).
func splitTrailingNum(s string) (string, int) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return s, 0
	}
	n, _ := strconv.Atoi(s[i:])
	return s[:i], n
}

// writeNodeHealthCSV writes the per-host relative-throughput summaries. The col
// field is the compact configuration label the heatmap uses as its column axis,
// naming only the dimensions that vary across the rows.
func writeNodeHealthCSV(path string, rows []nodeHealthRecord) error {
	configs := make([]benchkit.Dimensions, len(rows))
	for i, r := range rows {
		configs[i] = r.Dimensions
	}
	varying := varyingDimensions(configs)
	return writeCSV(path,
		append(dimensionColumns("workers", "payload", "rate", "send_buffer", "recv_buffer"),
			"col", "host", "throughput", "rel", "runs"),
		rows, func(r nodeHealthRecord) []string {
			return append(dimensionValues(r.Dimensions, "workers", "payload", "rate", "send_buffer", "recv_buffer"),
				[]string{cmp.Or(configLabel(r.Dimensions, varying), "all"),
					r.host, formatFloat(r.throughput), formatFloat(r.rel), strconv.Itoa(r.runs),
				}...)
		})
}

// degradedShareRecord is the fraction of a configuration's repetitions flagged
// degraded, kept per stream mode: systematic degradation concentrated in one
// mode is itself a benchkit.
type degradedShareRecord struct {
	benchkit.Dimensions
	total    int
	degraded int
	share    float64
}

// degradedShareRows counts, per configuration, how many repetitions were
// flagged degraded out of all that completed (succeeded plus degraded), so a
// heatmap can show where degradation concentrates. It works from the per-rep
// records because a configuration whose every repetition degraded is dropped
// from the rep-averaged table yet still belongs in this diagnostic.
func degradedShareRows(runs []plotRunRecord) []degradedShareRecord {
	counts := make(map[benchkit.Dimensions]*degradedShareRecord)
	for _, r := range runs {
		k := r.Dimensions
		c := counts[k]
		if c == nil {
			c = &degradedShareRecord{Dimensions: r.Dimensions}
			counts[k] = c
		}
		c.total++
		if r.status == runStatusDegraded {
			c.degraded++
		}
	}
	out := make([]degradedShareRecord, 0, len(counts))
	for _, c := range counts {
		if c.total > 0 {
			c.share = float64(c.degraded) / float64(c.total)
		}
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b degradedShareRecord) int {
		return compareDimensions(a.Dimensions, b.Dimensions)
	})
	return out
}

// degradedShareRowDims are the dimensions the degraded-share heatmap puts on
// its row axis: the cluster size and the stream mode, whose horizontal labels
// stay readable. Every other varying dimension goes on the column axis, which
// spreads one long label per configuration over two axes instead of one.
var degradedShareRowDims = map[string]bool{"benchmark": true, "nodes": true, "stream_mode": true}

// writeDegradedShareCSV writes the degraded-fraction rows with the compact row
// and column labels the heatmap uses as its axes. Both name only the dimensions
// that vary across the rows, so a sweep that held the worker count and the
// buffer capacities fixed does not repeat them in every label.
func writeDegradedShareCSV(path string, rows []degradedShareRecord) error {
	configs := make([]benchkit.Dimensions, len(rows))
	for i, r := range rows {
		configs[i] = r.Dimensions
	}
	varying := varyingDimensions(configs)
	rowDims, colDims := map[string]bool{}, map[string]bool{}
	for name := range varying {
		if degradedShareRowDims[name] {
			rowDims[name] = true
		} else {
			colDims[name] = true
		}
	}
	return writeCSV(path,
		append(dimensionColumns(), "row", "col", "total", "degraded", "share"),
		rows, func(r degradedShareRecord) []string {
			return append(dimensionValues(r.Dimensions),
				[]string{
					cmp.Or(configLabel(r.Dimensions, rowDims), "all"),
					cmp.Or(configLabel(r.Dimensions, colDims), "all"),
					strconv.Itoa(r.total), strconv.Itoa(r.degraded), formatFloat(r.share),
				}...)
		})
}
