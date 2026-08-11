package benchkit

import (
	"fmt"
	"io"
	"unicode/utf8"
)

// PrintResults renders the standard result table for one node's results to w:
// one row per benchmark with the Result.Row columns (throughput, latency
// mean/stddev/percentiles, per-op memory). When nodeLabel is non-empty, it is
// prepended as a Node column and the table is followed by a blank line, so
// interleaved multi-node output stays attributable. In remote runs the
// per-server memory stats are folded into the B/op and allocs/op columns
// unless serverStats is set, which instead appends separate per-server
// columns.
func PrintResults(w io.Writer, results []*Result, opts Options, serverStats bool, nodeLabel string) {
	headers := make([]string, 0, 9)
	if nodeLabel != "" {
		headers = append(headers, "Node")
	}
	headers = append(headers, "Benchmark", "Throughput", "Latency", "Std.dev", "p50", "p95", "p99")
	if !serverStats || !opts.Remote {
		headers = append(headers, "B/op", "allocs/op")
	} else {
		headers = append(headers, "Client B/op", "Client allocs/op")
		for i := 1; i <= opts.NumNodes; i++ {
			headers = append(headers, fmt.Sprintf("Server %d B/op", i), fmt.Sprintf("Server %d allocs/op", i))
		}
	}

	rows := make([][]string, 0, len(results))
	for _, r := range results {
		row := make([]string, 0, len(headers))
		if nodeLabel != "" {
			row = append(row, nodeLabel)
		}
		row = append(row, r.Row()...)
		if !serverStats && opts.Remote {
			// Add each server's per-op memory into the B/op and allocs/op columns
			// (Row's last two cells). Update the row strings only, never r:
			// callers print before persisting r, so mutating it here would write
			// these display-only combined totals to the output file.
			memPerOp, allocsPerOp := r.GetMemPerOp(), r.GetAllocsPerOp()
			for mem, allocs := range r.ServerMemPerOp() {
				memPerOp += mem
				allocsPerOp += allocs
			}
			row[len(row)-2] = fmt.Sprintf("%d B/op", memPerOp)
			row[len(row)-1] = fmt.Sprintf("%d allocs/op", allocsPerOp)
		}
		if serverStats && opts.Remote {
			for mem, allocs := range r.ServerMemPerOp() {
				row = append(row,
					fmt.Sprintf("%d B/op", mem),
					fmt.Sprintf("%d allocs/op", allocs),
				)
			}
		}
		rows = append(rows, row)
	}
	leftAligned := 1
	if nodeLabel != "" {
		leftAligned = 2
	}
	printTable(w, headers, rows, leftAligned)
	if nodeLabel != "" {
		fmt.Fprintln(w)
	}
}

// printTable renders headers and rows with columns padded to their widest
// cell; the first leftAligned columns are left-aligned (names, labels) and the
// rest right-aligned (metrics), so decimal points line up.
func printTable(w io.Writer, headers []string, rows [][]string, leftAligned int) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}

	printRow := func(row []string) {
		for i := range headers {
			if i > 0 {
				fmt.Fprint(w, "    ")
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if i < leftAligned {
				fmt.Fprintf(w, "%-*s", widths[i], cell)
				continue
			}
			fmt.Fprintf(w, "%*s", widths[i], cell)
		}
		fmt.Fprintln(w)
	}

	printRow(headers)
	for _, row := range rows {
		printRow(row)
	}
}
