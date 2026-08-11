package main

import (
	"testing"
)

// procNetSample is a trimmed concatenation of /proc/net/snmp and
// /proc/net/netstat as captured by one tcpstats snapshot.
const procNetSample = `Ip: Forwarding DefaultTTL InReceives
Ip: 2 64 1088116699
Tcp: ActiveOpens PassiveOpens RetransSegs OutSegs
Tcp: 100 200 24075589 1191999548
TcpExt: SyncookiesSent TCPTimeouts TCPLostRetransmit TCPFastRetrans TCPSlowStartRetrans TCPSynRetrans TCPAbortOnTimeout
TcpExt: 0 904393 216310 409750 22761701 555 1912
IpExt: InNoRoutes InMcastPkts
IpExt: 0 15795806
`

// TestParseProcNetCounters verifies parsing of the paired header/value lines
// of /proc/net/snmp and /proc/net/netstat into prefixed counter keys, keeping
// only the allowlisted TCP-health counters.
func TestParseProcNetCounters(t *testing.T) {
	got := parseProcNetCounters(procNetSample)
	want := map[string]uint64{
		"Tcp.RetransSegs":            24075589,
		"TcpExt.TCPTimeouts":         904393,
		"TcpExt.TCPLostRetransmit":   216310,
		"TcpExt.TCPFastRetrans":      409750,
		"TcpExt.TCPSlowStartRetrans": 22761701,
		"TcpExt.TCPSynRetrans":       555,
		"TcpExt.TCPAbortOnTimeout":   1912,
	}
	if len(got) != len(want) {
		t.Fatalf("counters = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("counter[%q] = %d, want %d", k, got[k], v)
		}
	}
	if _, ok := got["Tcp.ActiveOpens"]; ok {
		t.Error("non-allowlisted counter Tcp.ActiveOpens should be dropped")
	}
}

// TestParseProcNetCountersMalformed verifies that malformed input (mismatched
// header/value counts, garbage lines) yields no counters rather than a panic.
func TestParseProcNetCountersMalformed(t *testing.T) {
	for _, in := range []string{
		"",
		"garbage without colon\n",
		"Tcp: RetransSegs OutSegs\nTcp: 1\n", // value line shorter than header
		"Tcp: RetransSegs\nUdp: 5\n",         // prefixes do not pair up
	} {
		if got := parseProcNetCounters(in); len(got) != 0 {
			t.Errorf("parseProcNetCounters(%q) = %v, want empty", in, got)
		}
	}
}

// TestDiffCounters verifies that only counters that advanced during the run
// are reported, and that a missing or rewound counter (a reboot mid-sweep) is
// dropped rather than underflowing.
func TestDiffCounters(t *testing.T) {
	before := map[string]uint64{
		"Tcp.RetransSegs":      1000,
		"TcpExt.TCPTimeouts":   50,
		"TcpExt.TCPSynRetrans": 7,
	}
	after := map[string]uint64{
		"Tcp.RetransSegs":       1500, // advanced
		"TcpExt.TCPTimeouts":    50,   // unchanged -> dropped
		"TcpExt.TCPSynRetrans":  3,    // rewound (reboot) -> dropped
		"TcpExt.TCPFastRetrans": 42,   // appeared -> counted from zero
	}
	got := diffCounters(before, after)
	want := map[string]uint64{
		"Tcp.RetransSegs":       500,
		"TcpExt.TCPFastRetrans": 42,
	}
	if len(got) != len(want) {
		t.Fatalf("deltas = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("delta[%q] = %d, want %d", k, got[k], v)
		}
	}
	if diffCounters(before, before) != nil {
		t.Error("identical snapshots should yield nil deltas")
	}
}

// TestDiffTCPStats verifies the per-host wrapper: hosts with no advanced
// counters are omitted, and no-change snapshots yield nil.
func TestDiffTCPStats(t *testing.T) {
	before := map[string]map[string]uint64{
		"bb2":  {"TcpExt.TCPTimeouts": 10},
		"bb16": {"TcpExt.TCPTimeouts": 100},
	}
	after := map[string]map[string]uint64{
		"bb2":  {"TcpExt.TCPTimeouts": 10},   // unchanged -> omitted
		"bb16": {"TcpExt.TCPTimeouts": 5000}, // spinning
	}
	got := diffTCPStats(before, after)
	if len(got) != 1 || got["bb16"]["TcpExt.TCPTimeouts"] != 4900 {
		t.Fatalf("deltas = %v, want bb16 TCPTimeouts=4900 only", got)
	}
	if diffTCPStats(before, before) != nil {
		t.Error("identical snapshots should yield nil")
	}
}

// TestDegradedTCPSummary verifies the console rendering of a degraded node's
// TCP deltas: keyed by bare host alias from the node's host:port label,
// sorted counters with the TcpExt prefix stripped, empty when nothing was
// captured.
func TestDegradedTCPSummary(t *testing.T) {
	stats := map[string]map[string]uint64{
		"bb16": {"TcpExt.TCPTimeouts": 4900, "Tcp.RetransSegs": 120000},
	}
	got := degradedTCPSummary("bb16:9000", stats)
	want := "; tcp: Tcp.RetransSegs=120000 TCPTimeouts=4900"
	if got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	if got := degradedTCPSummary("bb2:9000", stats); got != "" {
		t.Errorf("summary for host without stats = %q, want empty", got)
	}
	if got := degradedTCPSummary("bb16:9000", nil); got != "" {
		t.Errorf("summary with no stats = %q, want empty", got)
	}
}
