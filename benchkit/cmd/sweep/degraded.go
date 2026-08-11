package main

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/relab/gorums/benchkit"
	"golang.org/x/exp/stats"
)

// Degradation detection: a run whose nodes all completed can still be
// worthless when one node's measurement does not belong with its peers'. After
// each successful run the sweep compares every node against the run median in
// three ways and marks the run "degraded" when any of them trips:
//
//   - Throughput far below the median (-degraded-below, default 0.5): the slow
//     node — a lossy network link, a throttled CPU, a sick disk. It drags the
//     cluster aggregate down and inflates tail latency without producing any
//     error, so nothing else in the pipeline notices.
//   - Throughput far above the median (-degraded-above, default 2): a node
//     cannot complete several times its peers' work in a symmetric benchmark,
//     so this is the signature of operations recorded without a network round
//     trip. One such node inflated a config aggregate to 14x its median and
//     destroyed the error bands of two figures before this check existed.
//   - Median latency far below the run median (-degraded-latency-below, default
//     0.2): the same anomaly seen in the latency distribution, and independent
//     evidence for it. A quorum call "completing" in a fraction of the time its
//     peers need did not do the round trip the benchmark measures.
//
// Every bound is median-relative because healthy runs on real clusters already
// show ±30% node skew — an absolute or mean-relative bound would either flag
// everything or let a single extreme outlier drag the reference down with it.

// Reasons a node was flagged, as recorded in the run manifest.
const (
	degradedSlow             = "throughput below the run median"
	degradedExcessThroughput = "throughput above the run median"
	degradedFastLatency      = "median latency below the run median"
)

// degradedNode records one node whose measurement fell outside a degraded
// threshold, as stored in the run manifest.
type degradedNode struct {
	Host       string  `json:"host"`               // host:port label of the flagged node
	Reason     string  `json:"reason,omitempty"`   // which bound the node crossed
	Throughput float64 `json:"throughput"`         // ops/s over the kept window
	Relative   float64 `json:"relative_to_median"` // the flagged metric / the run median of that metric
}

// String describes a flagged node for the sweep log.
func (d degradedNode) String() string {
	return fmt.Sprintf("%s: %s, at %.0f%% of it (%.0f ops/s)", d.Host, d.Reason, 100*d.Relative, d.Throughput)
}

// nodeMeasurement is one node's health signal over the kept window: the
// throughput it reported and its median latency in microseconds, which is 0
// when the node recorded no latency at all.
type nodeMeasurement struct {
	throughput float64
	p50US      float64
}

// degradationBounds are the median-relative limits a node's measurement must
// respect. A non-positive bound disables that check.
type degradationBounds struct {
	below        float64 // minimum throughput, as a fraction of the run median
	above        float64 // maximum throughput, as a multiple of the run median
	latencyBelow float64 // minimum median latency, as a fraction of the run median
}

// enabled reports whether any bound is in force.
func (b degradationBounds) enabled() bool {
	return b.below > 0 || b.above > 0 || b.latencyBelow > 0
}

// degradedBounds returns the degradation bounds the sweep flags configured.
func (cfg *config) degradedBounds() degradationBounds {
	return degradationBounds{
		below:        cfg.degradedBelow,
		above:        cfg.degradedAbove,
		latencyBelow: cfg.degradedLatencyBelow,
	}
}

// collectNodeMeasurements loads each node's collected result file for a run and
// returns its health signal (throughput summed across the node's results and its
// median latency, both trimmed like the run summary) keyed by the node's host
// label. A run measures one benchmark, so the first result carrying latency data
// supplies the median. Missing or unreadable files are skipped without a
// warning: collection coverage is tracked by countResultFiles, and the caller
// runs only after a successful collection.
//
// When a run mixes client-measured and server-measured results (e.g. PBFT
// -client=primary: primary has client RTT thruput, backups have execute
// thruput), only the server-measured nodes are used for degradation if at
// least two exist — that isolates replica execute-lag health from the
// primary's client performance signal. If there are fewer than two
// server-measured nodes, all nodes are compared (legacy multi-client).
func collectNodeMeasurements(outdir, base string, nodes []nodeAssignment, trim time.Duration) map[string]nodeMeasurement {
	type nodeEntry struct {
		host        string
		measurement nodeMeasurement
		server      bool // true if any result is SERVER_MEASURED
		client      bool
	}
	var list []nodeEntry
	for _, node := range nodes {
		data, err := os.ReadFile(filepath.Join(outdir, resultFilename(base, node, resultExt)))
		if err != nil {
			continue
		}
		report, err := benchkit.DecodeReport(data)
		if err != nil {
			continue
		}
		entry := nodeEntry{host: node.hostAddr()}
		for _, r := range report.GetResults() {
			summary := benchkit.Summarize(r, trim)
			entry.measurement.throughput += summary.Throughput
			if entry.measurement.p50US == 0 {
				entry.measurement.p50US = medianUS(summary.Dist())
			}
			switch r.GetConfig().GetMeasurementMode() {
			case benchkit.MeasurementMode_SERVER_MEASURED:
				entry.server = true
			case benchkit.MeasurementMode_CLIENT_MEASURED:
				entry.client = true
			}
		}
		list = append(list, entry)
	}

	serverOnly := 0
	for _, n := range list {
		if n.server && !n.client {
			serverOnly++
		}
	}
	// Prefer pure server-measured nodes for health when the run has both roles
	// (primary client + backup execute thruput).
	useServerOnly := serverOnly >= 2
	measurements := make(map[string]nodeMeasurement, len(list))
	for _, n := range list {
		if useServerOnly && !(n.server && !n.client) {
			continue
		}
		measurements[n.host] = n.measurement
	}
	return measurements
}

// findDegradedNodes returns the nodes whose measurement falls outside bounds,
// most extreme first. Each check needs at least two nodes carrying its metric
// and a positive run median: a run that produced no throughput at all is a
// failure, not a degradation, and one that recorded no latency has no latency
// median to judge against. A node is reported once, by the first bound it
// crosses in the order the bounds are declared.
func findDegradedNodes(nodes map[string]nodeMeasurement, bounds degradationBounds) []degradedNode {
	if !bounds.enabled() || len(nodes) < 2 {
		return nil
	}
	throughputs := make(map[string]float64, len(nodes))
	latencies := make(map[string]float64, len(nodes))
	for host, m := range nodes {
		throughputs[host] = m.throughput
		if m.p50US > 0 {
			latencies[host] = m.p50US
		}
	}
	tputMedian := runMedian(throughputs)
	latencyMedian := runMedian(latencies)

	var flagged []degradedNode
	for host, m := range nodes {
		var reason string
		relative := 0.0
		switch {
		case tputMedian > 0 && bounds.below > 0 && m.throughput < bounds.below*tputMedian:
			reason, relative = degradedSlow, m.throughput/tputMedian
		case tputMedian > 0 && bounds.above > 0 && m.throughput > bounds.above*tputMedian:
			reason, relative = degradedExcessThroughput, m.throughput/tputMedian
		case latencyMedian > 0 && bounds.latencyBelow > 0 && m.p50US > 0 && m.p50US < bounds.latencyBelow*latencyMedian:
			reason, relative = degradedFastLatency, m.p50US/latencyMedian
		default:
			continue
		}
		flagged = append(flagged, degradedNode{
			Host:       host,
			Reason:     reason,
			Throughput: m.throughput,
			Relative:   relative,
		})
	}
	// Most extreme first: the furthest from parity in either direction.
	slices.SortFunc(flagged, func(a, b degradedNode) int {
		return cmp.Or(
			cmp.Compare(deviation(b.Relative), deviation(a.Relative)),
			cmp.Compare(a.Host, b.Host),
		)
	})
	return flagged
}

// runMedian returns the median of the values, or 0 when fewer than two nodes
// carry the metric, which leaves no peer group to compare against.
func runMedian(values map[string]float64) float64 {
	if len(values) < 2 {
		return 0
	}
	return stats.Median(slices.Sorted(maps.Values(values)))
}

// deviation measures how far a median-relative ratio is from parity, so a node
// at 12x and one at 1/12 of the median rank equally extreme.
func deviation(relative float64) float64 {
	if relative <= 0 {
		return math.Inf(1)
	}
	return math.Abs(math.Log(relative))
}
