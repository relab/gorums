package main

import (
	"cmp"
	"maps"
	"slices"
	"strconv"

	"github.com/relab/gorums/benchkit"
)

// Stat names of the named-metric columns, in column order. A column is the
// metric name, a dot, and the stat name, such as "encode.p50_us"; the dot
// keeps the columns apart from the dimension columns, whose names never hold
// one.
var (
	seriesStatNames  = []string{"mean_us", "p50_us", "p95_us", "p99_us"}
	counterStatNames = []string{"sum", "max"}
)

// metricColumn identifies the column group of one named metric in agg.csv and
// nodes.csv: the metric name and whether it is a series or a counter.
type metricColumn struct {
	name   string
	series bool
}

// metricColumnOf returns the column group of m.
func metricColumnOf(m *benchkit.MetricSummary) metricColumn {
	return metricColumn{name: m.GetName(), series: m.GetCounter() == nil}
}

// statNames returns the stat names of the column group, in column order.
func (c metricColumn) statNames() []string {
	if c.series {
		return seriesStatNames
	}
	return counterStatNames
}

// header returns the column names of the column group, each stat name
// followed by the given suffixes, such as "_sd" and "_ci95".
func (c metricColumn) header(suffixes ...string) []string {
	var out []string
	for _, stat := range c.statNames() {
		col := c.name + "." + stat
		out = append(out, col)
		for _, suffix := range suffixes {
			out = append(out, col+suffix)
		}
	}
	return out
}

// sortedMetricColumns returns the column groups of seen, sorted by metric
// name, series before counters.
func sortedMetricColumns(seen map[metricColumn]bool) []metricColumn {
	return slices.SortedFunc(maps.Keys(seen), func(a, b metricColumn) int {
		if c := cmp.Compare(a.name, b.name); c != 0 {
			return c
		}
		switch {
		case a.series == b.series:
			return 0
		case a.series:
			return -1
		default:
			return 1
		}
	})
}

// metricValues returns the values of m in the order of the stat names of its
// column group: the mean and percentiles of a series in microseconds, or the
// sum and max of a counter. It returns nil for a series without samples, whose
// statistics are not meaningful.
func metricValues(m *benchkit.MetricSummary) []float64 {
	if c := m.GetCounter(); c != nil {
		return []float64{float64(c.GetSum()), float64(c.GetMax())}
	}
	d := m.GetDistribution()
	if d.GetSamples() == 0 {
		return nil
	}
	return []float64{d.GetMeanUs(), d.GetP50Us(), d.GetP95Us(), d.GetP99Us()}
}

// metricFields returns the values of one node's m as CSV fields, in the order
// of the stat names of its column group: the sum and max of a counter as exact
// integers, or the values of a series that [metricValues] returns. It returns
// nil for a series without samples.
func metricFields(m *benchkit.MetricSummary) []string {
	if c := m.GetCounter(); c != nil {
		return []string{strconv.FormatUint(c.GetSum(), 10), strconv.FormatUint(c.GetMax(), 10)}
	}
	values := metricValues(m)
	if values == nil {
		return nil
	}
	fields := make([]string, len(values))
	for i, v := range values {
		fields[i] = formatFloat(v)
	}
	return fields
}
