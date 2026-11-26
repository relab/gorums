package main

import (
	"maps"
	"slices"
	"strconv"
)

// runStatusRecord tallies run outcomes for one node count. completed counts the
// runs that produced usable measurements (succeeded plus degraded); a degraded
// run kept intact per-node data even though one node fell below the health
// threshold, so it still contributes to performance aggregates. skipped counts
// the runs whose binary rejected the combination of flag values.
type runStatusRecord struct {
	nodes     int
	total     int
	succeeded int
	degraded  int
	failed    int
	skipped   int
	completed int
}

// runStatusRows tallies the run manifests under outdir by node count. It
// captures the completion accounting that performance figures alone omit: how
// many repetitions of each cluster size finished cleanly, degraded, failed, or
// were skipped.
func runStatusRows(outdir string) ([]runStatusRecord, error) {
	manifests, err := loadRunManifests(outdir)
	if err != nil {
		return nil, err
	}
	byNodes := make(map[int]*runStatusRecord)
	for _, rm := range manifests {
		n := rm.manifest.Nodes
		rec := byNodes[n]
		if rec == nil {
			rec = &runStatusRecord{nodes: n}
			byNodes[n] = rec
		}
		rec.total++
		switch rm.manifest.Status {
		case runStatusSucceeded:
			rec.succeeded++
			rec.completed++
		case runStatusDegraded:
			rec.degraded++
			rec.completed++
		case runStatusFailed:
			rec.failed++
		case runStatusSkipped:
			rec.skipped++
		}
	}
	out := make([]runStatusRecord, 0, len(byNodes))
	for _, n := range slices.Sorted(maps.Keys(byNodes)) {
		out = append(out, *byNodes[n])
	}
	return out, nil
}

// writeRunStatusCSV writes the per-node-count run outcome tallies.
func writeRunStatusCSV(path string, rows []runStatusRecord) error {
	return writeCSV(path,
		[]string{"nodes", "total", "succeeded", "degraded", "failed", "skipped", "completed"},
		rows, func(r runStatusRecord) []string {
			return []string{
				strconv.Itoa(r.nodes), strconv.Itoa(r.total), strconv.Itoa(r.succeeded),
				strconv.Itoa(r.degraded), strconv.Itoa(r.failed), strconv.Itoa(r.skipped),
				strconv.Itoa(r.completed),
			}
		})
}

// anyUnsuccessful reports whether any run degraded, failed, or was skipped, so
// the report includes the run-status figure only when outcomes are worth showing.
func anyUnsuccessful(rows []runStatusRecord) bool {
	return slices.ContainsFunc(rows, func(r runStatusRecord) bool {
		return r.degraded > 0 || r.failed > 0 || r.skipped > 0
	})
}
