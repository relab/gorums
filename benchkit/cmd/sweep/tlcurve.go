package main

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/relab/gorums/benchkit"
)

// tlSplitRatio bounds the peak-latency dynamic range within one
// throughput-latency figure. When the largest per-curve peak p99 exceeds the
// smallest by more than this factor, the curves are split into scale-band
// groups so a single linear axis never crams widely-differing curves.
const tlSplitRatio = 8.0

// tlPoint is one point on a throughput-latency curve: a rep-averaged
// configuration's achieved throughput against its latency percentiles. group
// is its scale band (see assignTLGroups); curves are drawn one band at a time.
type tlPoint struct {
	benchkit.Dimensions
	throughputKops float64
	p50US          float64
	p50USsd        float64
	p95US          float64
	p99US          float64
	group          int
}

// tlIdent identifies a load-scale family whose curves stay in one scale band.
// Benchmark and stream mode are deliberately excluded: those are the contrasts
// a throughput-latency figure exists to show, so they share a band; payload and
// the buffer capacities all drive latency apart (a large send buffer trades
// bufferbloat-style latency for throughput) and thus drive the banding.
type tlIdent struct{ benchkit.Dimensions }

// tlLoadDims are the dimensions a throughput-latency curve can trace along, in
// display order: a curve raises the offered load and reads off the throughput
// and latency it produced, and these are the two dimensions that raise it.
var tlLoadDims = []string{"workers", "rate"}

// tlLoadDimensions returns the load dimensions a report can draw a curve along:
// those of tlLoadDims the sweep varied, per the dimension value counts.
func tlLoadDimensions(counts map[string]int) []string {
	var loads []string
	for _, load := range tlLoadDims {
		if counts[load] > 1 {
			loads = append(loads, load)
		}
	}
	return loads
}

// tlCurveRows reduces the rep-averaged records to throughput-latency points,
// keeping only configurations that recorded latency, and assigns each a scale
// band. loads names the dimensions a figure traces its curves along; they are
// left out of the band identity, since points differing only in a traced
// dimension are successive points of one curve and must share its band. The
// Typst helper draws one curve per remaining dimension combination within a
// node-count panel, so points without a latency percentile are dropped here.
func tlCurveRows(agg []aggRunRecord, loads []string) []tlPoint {
	mag := map[tlIdent]float64{}
	for _, r := range agg {
		if r.p50US.n == 0 {
			continue
		}
		id := tlIdent{loadScaleDimensions(r.Dimensions, loads)}
		mag[id] = max(mag[id], r.p99US.mean)
	}
	groups := assignTLGroups(mag)

	var out []tlPoint
	for _, r := range agg {
		if r.p50US.n == 0 {
			continue
		}
		out = append(out, tlPoint{
			Dimensions:     r.Dimensions,
			throughputKops: r.throughput.mean / 1e3,
			p50US:          r.p50US.mean,
			p50USsd:        r.p50US.sd,
			p95US:          r.p95US.mean,
			p99US:          r.p99US.mean,
			group:          groups[tlIdent{loadScaleDimensions(r.Dimensions, loads)}],
		})
	}
	return out
}

// assignTLGroups partitions load-scale identities into peak-latency bands. The
// identities are sorted by peak p99 and greedily grouped so the largest peak in
// a band is at most tlSplitRatio times the smallest; a range that already fits
// the ratio yields a single band.
func assignTLGroups(mag map[tlIdent]float64) map[tlIdent]int {
	type entry struct {
		id  tlIdent
		mag float64
	}
	var xs []entry
	for id, m := range mag {
		if m > 0 {
			xs = append(xs, entry{id, m})
		}
	}
	// Sort by peak magnitude; the identity tie-break keeps the mapping
	// deterministic when two identities share a peak.
	slices.SortFunc(xs, func(a, b entry) int {
		return cmp.Or(cmp.Compare(a.mag, b.mag), compareTLIdent(a.id, b.id))
	})
	groups := map[tlIdent]int{}
	if len(xs) == 0 {
		return groups
	}
	g, lo := 1, xs[0].mag
	for _, x := range xs {
		if x.mag/lo > tlSplitRatio {
			g++
			lo = x.mag
		}
		groups[x.id] = g
	}
	return groups
}

func compareTLIdent(a, b tlIdent) int {
	return compareDimensions(a.Dimensions, b.Dimensions)
}

// writeTLCurveCSV writes the throughput-latency points. Columns mirror the
// tidy layout of the other CSVs; p50 carries a spread column for optional error
// bars, and group identifies the scale band.
func writeTLCurveCSV(path string, rows []tlPoint) error {
	header := append(dimensionColumns(),
		[]string{
			"throughput_kops", "p50_us", "p50_us_sd", "p95_us", "p99_us", "group",
		}...)
	return writeCSV(path, header, rows, func(r tlPoint) []string {
		return append(dimensionValues(r.Dimensions),
			[]string{
				formatFloat(r.throughputKops), formatFloat(r.p50US), formatFloat(r.p50USsd),
				formatFloat(r.p95US), formatFloat(r.p99US), strconv.Itoa(r.group),
			}...)
	})
}
