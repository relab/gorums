package main

import (
	"bufio"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// offsetLineRE matches a per-run clock-offset diagnostic line, e.g.
//
//	[offsets node 15 (10.0.0.1:9000)] peer 2: before=-297µs after=-301µs drift=-3µs
//
// before is the raw skew the correction removes; drift is the residual it
// cannot. Both carry a Go-style duration unit.
// The magnitudes are single-unit Go durations (µs is U+00B5, as time.Duration
// prints). Clock skew is sub-second in practice, but m/h are accepted so a
// pathologically desynced peer is recorded rather than silently dropped.
var offsetLineRE = regexp.MustCompile(
	`\[offsets node (\d+) \([^)]*\)\] peer (\d+): ` +
		`before=(-?[\d.]+)(ns|us|µs|ms|s|m|h) ` +
		`after=-?[\d.]+(?:ns|us|µs|ms|s|m|h) ` +
		`drift=(-?[\d.]+)(ns|us|µs|ms|s|m|h)`)

var unitToUS = map[string]float64{
	"ns": 1e-3, "us": 1, "µs": 1, "ms": 1e3, "s": 1e6, "m": 60e6, "h": 3600e6,
}

// nodeCountRE extracts the run's node count from a log filename (…_N28_…).
var nodeCountRE = regexp.MustCompile(`_N(\d+)_`)

// offsetSample is one cross-machine clock observation: the absolute skew and
// residual drift, in microseconds, tagged with the run's node count.
type offsetSample struct {
	nodeCount int
	offsetUS  float64
	driftUS   float64
}

// collectOffsets parses every *.log under logDir for clock-offset diagnostic
// lines, returning one sample per cross-machine peer observation. Self/loopback
// peers (node == peer) are skipped: they carry no cross-machine skew.
func collectOffsets(logDir string) ([]offsetSample, error) {
	logs, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		return nil, err
	}
	var samples []offsetSample
	for _, path := range logs {
		nodeCount := 0
		if m := nodeCountRE.FindStringSubmatch(filepath.Base(path)); m != nil {
			nodeCount, _ = strconv.Atoi(m[1])
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.Contains(line, "[offsets") {
				continue
			}
			m := offsetLineRE.FindStringSubmatch(line)
			if m == nil || m[1] == m[2] {
				continue
			}
			samples = append(samples, offsetSample{
				nodeCount: nodeCount,
				offsetUS:  absUS(m[3], m[4]),
				driftUS:   absUS(m[5], m[6]),
			})
		}
		closeErr := f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	return samples, nil
}

// absUS parses a signed duration magnitude with unit into absolute microseconds.
func absUS(value, unit string) float64 {
	v, _ := strconv.ParseFloat(value, 64)
	if v < 0 {
		v = -v
	}
	return v * unitToUS[unit]
}

// offsetCDFRecord is one point of an empirical CDF of a clock metric.
type offsetCDFRecord struct {
	metric  string // "offset" or "drift"
	group   string // "all" or "N<count>"
	valueUS float64
	cdf     float64
}

// offsetCDFRows builds empirical CDFs of the absolute offset and drift, overall
// ("all") and per node count ("N<count>"), each sampled at points+1 quantiles.
// Groups with no samples are omitted; the rows are ordered metric, group,
// then ascending value.
func offsetCDFRows(samples []offsetSample, points int) []offsetCDFRecord {
	offsets := map[string][]float64{}
	drifts := map[string][]float64{}
	add := func(group string, s offsetSample) {
		offsets[group] = append(offsets[group], s.offsetUS)
		drifts[group] = append(drifts[group], s.driftUS)
	}
	for _, s := range samples {
		add("all", s)
		if s.nodeCount > 0 {
			add("N"+strconv.Itoa(s.nodeCount), s)
		}
	}

	groups := offsetGroupOrder(samples)
	var out []offsetCDFRecord
	for _, m := range []struct {
		name string
		data map[string][]float64
	}{{"offset", offsets}, {"drift", drifts}} {
		for _, g := range groups {
			out = append(out, cdfPointsFor(m.name, g, m.data[g], points)...)
		}
	}
	return out
}

// offsetGroupOrder returns "all" followed by each distinct node count present,
// ascending.
func offsetGroupOrder(samples []offsetSample) []string {
	seen := map[int]bool{}
	for _, s := range samples {
		if s.nodeCount > 0 {
			seen[s.nodeCount] = true
		}
	}
	groups := []string{"all"}
	for _, c := range slices.Sorted(maps.Keys(seen)) {
		groups = append(groups, "N"+strconv.Itoa(c))
	}
	return groups
}

// cdfPointsFor samples the empirical CDF of xs at points+1 evenly spaced
// quantiles.
func cdfPointsFor(metric, group string, xs []float64, points int) []offsetCDFRecord {
	if len(xs) == 0 {
		return nil
	}
	s := slices.Sorted(slices.Values(xs))
	n := len(s)
	out := make([]offsetCDFRecord, 0, points+1)
	for i := 0; i <= points; i++ {
		q := float64(i) / float64(points)
		k := min(int(q*float64(n)), n-1)
		out = append(out, offsetCDFRecord{
			metric:  metric,
			group:   group,
			valueUS: s[k],
			cdf:     float64(k+1) / float64(n),
		})
	}
	return out
}

// writeOffsetsCSV writes the clock-offset CDF rows.
func writeOffsetsCSV(path string, rows []offsetCDFRecord) error {
	return writeCSV(path,
		[]string{"metric", "group", "value_us", "cdf"},
		rows, func(r offsetCDFRecord) []string {
			return []string{r.metric, r.group, formatFloat(r.valueUS), formatFloat(r.cdf)}
		})
}
