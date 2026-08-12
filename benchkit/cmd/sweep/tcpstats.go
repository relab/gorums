package main

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/relab/iago"
)

// Per-run TCP forensics: the kernel's TCP counters are snapshotted on every
// host before and after each run, and the deltas are recorded in the run
// manifest. A node behind a lossy link produces no error — it just runs
// slowly with its retransmission counters spinning — so when a run is flagged
// degraded, its manifest already carries the evidence pointing at the sick
// host (and the -explain triage sees it in the manifest it is handed). The
// counters are host-wide, so unrelated background traffic contributes noise,
// but a loss-induced degradation dwarfs it by orders of magnitude.

// tcpCounterAllowlist names the /proc/net/snmp and /proc/net/netstat counters
// worth recording, as "<Prefix>.<Name>" keys. Together they distinguish loss
// severity: FastRetrans is recovery without stalling, Timeouts and
// SlowStartRetrans mean the connection stalled for a full RTO (the signature
// of a bad link: throughput pinned near the 200 ms minimum RTO), SynRetrans
// is loss during connection setup, and AbortOnTimeout is connections given up.
var tcpCounterAllowlist = map[string]bool{
	"Tcp.RetransSegs":            true,
	"TcpExt.TCPTimeouts":         true,
	"TcpExt.TCPLostRetransmit":   true,
	"TcpExt.TCPFastRetrans":      true,
	"TcpExt.TCPSlowStartRetrans": true,
	"TcpExt.TCPSynRetrans":       true,
	"TcpExt.TCPAbortOnTimeout":   true,
}

// tcpStatsCommand reads both counter files in one exec; their formats are
// identical (paired header/value lines per prefix) so the outputs concatenate.
const tcpStatsCommand = "cat /proc/net/snmp /proc/net/netstat"

// parseProcNetCounters parses the paired header/value line format of
// /proc/net/snmp and /proc/net/netstat ("Tcp: A B C" followed by
// "Tcp: 1 2 3") and returns the manifest allowlisted counters as
// "<Prefix>.<Name>" keys. Malformed lines are skipped.
func parseProcNetCounters(out string) map[string]uint64 {
	return parseAllowedCounters(out, tcpCounterAllowlist)
}

// parseAllowedCounters parses the paired header/value line format of
// /proc/net/snmp and /proc/net/netstat and returns the counters named in allow
// as "<Prefix>.<Name>" keys. Malformed lines are skipped. Callers pass their own
// allowlist so the manifest forensics and the -check diagnostics can select
// different counters from the same input.
func parseAllowedCounters(out string, allow map[string]bool) map[string]uint64 {
	counters := make(map[string]uint64)
	headers := make(map[string][]string) // prefix -> column names, awaiting the value line
	for line := range strings.Lines(out) {
		prefix, rest, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		names, seen := headers[prefix]
		if !seen {
			headers[prefix] = fields
			continue
		}
		delete(headers, prefix)
		if len(fields) != len(names) {
			continue
		}
		for i, name := range names {
			key := prefix + "." + name
			if !allow[key] {
				continue
			}
			if v, err := strconv.ParseUint(fields[i], 10, 64); err == nil {
				counters[key] = v
			}
		}
	}
	return counters
}

// diffCounters returns after-before per counter, keeping only counters that
// advanced. A counter absent from before counts from zero (first sight); a
// counter that went backwards (host rebooted mid-sweep) is dropped rather
// than underflowed. Returns nil when nothing advanced.
func diffCounters(before, after map[string]uint64) map[string]uint64 {
	var deltas map[string]uint64
	for key, a := range after {
		b := before[key]
		if a <= b {
			continue
		}
		if deltas == nil {
			deltas = make(map[string]uint64)
		}
		deltas[key] = a - b
	}
	return deltas
}

// captureTCPStats snapshots the TCP counters of every host in g, keyed by
// host alias. Hosts that fail to answer are simply absent: the snapshot is
// best-effort forensics and must never fail a run.
func captureTCPStats(g iago.Group) map[string]map[string]uint64 {
	snap, _ := iago.Collect(withTimeout(g, 30*time.Second), "tcp stats", func(ctx context.Context, host iago.Host) (map[string]uint64, error) {
		out, err := iago.Output(ctx, host, tcpStatsCommand)
		if err != nil {
			return nil, err
		}
		return parseProcNetCounters(out), nil
	})
	maps.DeleteFunc(snap, func(_ string, counters map[string]uint64) bool {
		return len(counters) == 0
	})
	return snap
}

// diffTCPStats returns the per-host counter deltas between two snapshots,
// omitting hosts with no advanced counters. Returns nil when no host has any.
func diffTCPStats(before, after map[string]map[string]uint64) map[string]map[string]uint64 {
	var stats map[string]map[string]uint64
	for host, a := range after {
		deltas := diffCounters(before[host], a)
		if len(deltas) == 0 {
			continue
		}
		if stats == nil {
			stats = make(map[string]map[string]uint64)
		}
		stats[host] = deltas
	}
	return stats
}

// degradedTCPSummary renders the TCP counter deltas of a degraded node's host
// for the console warning, or "" when none were captured. nodeAddr is the
// node's host:port label; the stats are keyed by bare host alias.
func degradedTCPSummary(nodeAddr string, tcpStats map[string]map[string]uint64) string {
	host := hostFromAddr(nodeAddr)
	counters := tcpStats[host]
	if len(counters) == 0 {
		return ""
	}
	parts := make([]string, 0, len(counters))
	for _, key := range slices.Sorted(maps.Keys(counters)) {
		parts = append(parts, fmt.Sprintf("%s=%d", strings.TrimPrefix(key, "TcpExt."), counters[key]))
	}
	return "; tcp: " + strings.Join(parts, " ")
}
