package main

import (
	"io"
	"log"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseDiag(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   diagFields
	}{
		{
			name: "FullOutput",
			output: "EPOCH=1716800000.500000000\nHOST=bb1\nKERNEL=Linux 5.15.0\nCPUS=16\nLOAD=0.42\n" +
				"PROCS=2\nPORTSBUSY= 9000 9001\nEPOCH_END=1716800000.560000000\n",
			want: diagFields{
				hostname:  "bb1",
				kernel:    "Linux 5.15.0",
				cpus:      "16",
				load:      "0.42",
				epoch:     1716800000.5,
				epochEnd:  1716800000.56,
				procs:     2,
				portsBusy: []string{"9000", "9001"},
			},
		},
		{
			name:   "NoBusyPortsNoProcs",
			output: "HOST=bb2\nPROCS=0\nPORTSBUSY=\n",
			want:   diagFields{hostname: "bb2", procs: 0, portsBusy: nil},
		},
		{
			name:   "UnknownKeysIgnored",
			output: "HOST=bb3\nMYSTERY=42\nCPUS=8\n",
			want:   diagFields{hostname: "bb3", cpus: "8"},
		},
		{
			name:   "MalformedLinesSkipped",
			output: "garbage line without equals\nHOST=bb4\n\n",
			want:   diagFields{hostname: "bb4"},
		},
		{
			name:   "BadNumbersZeroed",
			output: "PROCS=notanumber\nEPOCH=alsobad\n",
			want:   diagFields{procs: 0, epoch: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDiag(tt.output)
			if got.hostname != tt.want.hostname ||
				got.kernel != tt.want.kernel ||
				got.cpus != tt.want.cpus ||
				got.load != tt.want.load ||
				got.epoch != tt.want.epoch ||
				got.epochEnd != tt.want.epochEnd ||
				got.procs != tt.want.procs ||
				!slices.Equal(got.portsBusy, tt.want.portsBusy) {
				t.Errorf("parseDiag()\n got = %+v\nwant = %+v", got, tt.want)
			}
		})
	}
}

func TestSkewRTT(t *testing.T) {
	base := time.Unix(1716800000, 0)
	tests := []struct {
		name              string
		epoch, epochEnd   float64
		before, after     time.Time
		wantSkew, wantRTT time.Duration
	}{
		{
			// Remote clock +500ms ahead; 2ms each network leg, 6ms on-host script.
			// The remote reads (t2,t3) bracket the script symmetrically, so the NTP
			// offset recovers 500ms and the delay recovers the 4ms round-trip,
			// excluding the on-host time — unlike a single midpoint sample.
			name:     "RemoteAhead",
			epoch:    1716800000.492,
			epochEnd: 1716800000.498,
			before:   base.Add(-10 * time.Millisecond),
			after:    base,
			wantSkew: 500 * time.Millisecond,
			wantRTT:  4 * time.Millisecond,
		},
		{
			// Remote clock -250ms behind, same path shape.
			name:     "RemoteBehind",
			epoch:    1716799999.742,
			epochEnd: 1716799999.748,
			before:   base.Add(-10 * time.Millisecond),
			after:    base,
			wantSkew: -250 * time.Millisecond,
			wantRTT:  4 * time.Millisecond,
		},
		{
			// Only the start epoch present (older host): fall back to the biased
			// midpoint estimate and report the raw wall-clock span as the rtt.
			name:     "SingleEpochFallback",
			epoch:    1716800000.25,
			epochEnd: 0,
			before:   base.Add(-4 * time.Millisecond),
			after:    base.Add(4 * time.Millisecond),
			wantSkew: 250 * time.Millisecond,
			wantRTT:  8 * time.Millisecond,
		},
		{
			name:     "NoEpochIsZero",
			epoch:    0,
			epochEnd: 0,
			before:   base,
			after:    base.Add(20 * time.Millisecond),
			wantSkew: 0,
			wantRTT:  20 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := diagFields{epoch: tt.epoch, epochEnd: tt.epochEnd}
			// Round to the nearest millisecond: the float64 epochs carry sub-µs
			// imprecision that formatSkew also rounds away.
			gotSkew, gotRTT := f.skewRTT(tt.before, tt.after)
			if gotSkew.Round(time.Millisecond) != tt.wantSkew || gotRTT.Round(time.Millisecond) != tt.wantRTT {
				t.Errorf("skewRTT(%v, %v) = (%v, %v), want (%v, %v)",
					tt.before, tt.after, gotSkew, gotRTT, tt.wantSkew, tt.wantRTT)
			}
		})
	}
}

func TestDiagCommand(t *testing.T) {
	prog := newRemoteProgram("")
	cmd := diagCommand(9000, prog, "/local")
	// The probe must cover numProbePorts consecutive ports from the base.
	for _, want := range []string{"9000", "9001", "9002", "9003"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("diagCommand(9000) missing port %q in:\n%s", want, cmd)
		}
	}
	// The lingering-process probe must use the program's bracketed pgrep pattern.
	if !strings.Contains(cmd, prog.pgrep()) {
		t.Errorf("diagCommand(9000) missing pgrep pattern %q in:\n%s", prog.pgrep(), cmd)
	}
	// A literal %s.%N for date must survive Sprintf (no stray format verbs).
	if !strings.Contains(cmd, "date +%s.%N") {
		t.Errorf("diagCommand(9000) missing 'date +%%s.%%N' in:\n%s", cmd)
	}
	// The remote clock must be sampled at both ends, with EPOCH before EPOCH_END,
	// so skewRTT can bracket the script's on-host duration (see skewRTT).
	start, end := strings.Index(cmd, "EPOCH="), strings.Index(cmd, "EPOCH_END=")
	if start < 0 || end < 0 || start >= end {
		t.Errorf("diagCommand(9000) must read EPOCH before EPOCH_END; got indices %d, %d in:\n%s", start, end, cmd)
	}
	// The raw /proc counter dump must be appended so the check can report
	// retransmit health from the same output.
	if !strings.Contains(cmd, tcpStatsCommand) {
		t.Errorf("diagCommand(9000) missing tcp stats command %q in:\n%s", tcpStatsCommand, cmd)
	}
	if strings.Contains(cmd, "%!") {
		t.Errorf("diagCommand(9000) has a Printf formatting error:\n%s", cmd)
	}
}

// TestCheckTCPAllowlistFromDiagOutput verifies the check-specific counters are
// parsed from output that mixes the KEY=VALUE diag lines with the appended raw
// /proc counter dump, and that the KEY=VALUE lines are ignored by the parser.
func TestCheckTCPAllowlistFromDiagOutput(t *testing.T) {
	out := "HOST=bb9\nKERNEL=Linux 5.15.0\nPORTSBUSY=\n" + procNetSample
	got := parseAllowedCounters(out, checkTCPAllowlist)
	want := map[string]uint64{
		"Tcp.RetransSegs":      24075589,
		"Tcp.OutSegs":          1191999548,
		"TcpExt.TCPSynRetrans": 555,
	}
	if len(got) != len(want) {
		t.Fatalf("counters = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("counter[%q] = %d, want %d", k, got[k], v)
		}
	}
}

func TestFormatRetrans(t *testing.T) {
	tests := []struct {
		name                          string
		retransSegs, outSegs, synRetx uint64
		wantPct, wantSyn              string
	}{
		{name: "NoData", retransSegs: 0, outSegs: 0, synRetx: 0, wantPct: "-", wantSyn: "-"},
		{name: "Healthy", retransSegs: 1000, outSegs: 1_000_000, synRetx: 0, wantPct: "0.10%", wantSyn: "0"},
		{name: "Sick", retransSegs: 24075589, outSegs: 1191999548, synRetx: 555, wantPct: "2.02%", wantSyn: "555"},
		{name: "ZeroRetransRealData", retransSegs: 0, outSegs: 500, synRetx: 0, wantPct: "0.00%", wantSyn: "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPct, gotSyn := formatRetrans(tt.retransSegs, tt.outSegs, tt.synRetx)
			if gotPct != tt.wantPct || gotSyn != tt.wantSyn {
				t.Errorf("formatRetrans(%d, %d, %d) = (%q, %q), want (%q, %q)",
					tt.retransSegs, tt.outSegs, tt.synRetx, gotPct, gotSyn, tt.wantPct, tt.wantSyn)
			}
		})
	}
}

func TestFailureDiagCommand(t *testing.T) {
	prog := newRemoteProgram("")
	cmd := failureDiagCommand([]string{"9000", "9001"}, prog)
	// Each node port must be probed for listeners and connections.
	for _, want := range []string{"9000", "9001"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("failureDiagCommand missing port %q in:\n%s", want, cmd)
		}
	}
	// The process probe must use the program's bracketed pgrep pattern.
	if !strings.Contains(cmd, prog.pgrep()) {
		t.Errorf("failureDiagCommand missing pgrep pattern %q in:\n%s", prog.pgrep(), cmd)
	}
	// Socket state must cover both listeners and all connections (time-wait).
	for _, want := range []string{"ss -ltnp", "ss -tan", "loadavg", "fd_limit_soft"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("failureDiagCommand missing probe %q in:\n%s", want, cmd)
		}
	}
	if strings.Contains(cmd, "%!") {
		t.Errorf("failureDiagCommand has a Printf formatting error:\n%s", cmd)
	}
}

// TestDiagLocal runs the local self-probe path used for the driver host,
// exercising the same command shape diagCommand emits (KEY=VALUE lines followed
// by the raw /proc counter dump) so the parsed result matches an SSH probe.
func TestDiagLocal(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	t.Run("Reachable", func(t *testing.T) {
		// A stand-in for diagCommand's output: identifying KEY=VALUE lines plus
		// the /proc counter dump the check reads for retransmit health. The dump
		// is emitted through a heredoc so its newlines survive into the output.
		cmd := "echo HOST=driverhost\n" +
			"echo KERNEL='Linux 6.8.0'\n" +
			"echo CPUS=12\n" +
			"echo LOAD=0.50\n" +
			"echo PROCS=0\n" +
			"echo PORTSBUSY=\n" +
			"cat <<'PROCNET'\n" + procNetSample + "\nPROCNET\n"
		d := diagLocal("bb1", cmd)
		if !d.reachable {
			t.Fatalf("diagLocal reachable = false, errMsg = %q", d.errMsg)
		}
		if d.alias != "bb1" || d.hostname != "driverhost" || d.cpus != "12" {
			t.Errorf("diagLocal parsed = %+v", d.diagFields)
		}
		if d.outSegs != 1191999548 || d.retransSegs != 24075589 || d.synRetrans != 555 {
			t.Errorf("diagLocal tcp counters = retrans %d out %d syn %d",
				d.retransSegs, d.outSegs, d.synRetrans)
		}
	})
	t.Run("CommandFailure", func(t *testing.T) {
		d := diagLocal("bb1", "exit 3")
		if d.reachable {
			t.Errorf("diagLocal reachable = true for a failing command")
		}
		if d.errMsg == "" {
			t.Errorf("diagLocal errMsg empty for a failing command")
		}
	})
}

func TestFormatSkew(t *testing.T) {
	tests := []struct {
		name string
		skew time.Duration
		rtt  time.Duration
		want string
	}{
		{name: "PositiveSkew", skew: 12 * time.Millisecond, rtt: 6 * time.Millisecond, want: "+12ms (±3ms)"},
		{name: "NegativeSkew", skew: -1500 * time.Millisecond, rtt: 20 * time.Millisecond, want: "-1500ms (±10ms)"},
		{name: "ZeroSkew", skew: 0, rtt: 0, want: "+0ms (±0ms)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSkew(tt.skew, tt.rtt); got != tt.want {
				t.Errorf("formatSkew(%v, %v) = %q, want %q", tt.skew, tt.rtt, got, tt.want)
			}
		})
	}
}

// TestPrintDiagTableReturnsUnreachableCount verifies that printDiagTable
// reports how many hosts were unreachable, the signal checkHosts uses to
// fail -check loudly instead of always reporting success regardless of the
// table's contents.
func TestPrintDiagTableReturnsUnreachableCount(t *testing.T) {
	defer log.SetOutput(log.Writer())
	log.SetOutput(io.Discard)

	tests := []struct {
		name    string
		results map[string]hostDiag
		want    int
	}{
		{"AllReachable", map[string]hostDiag{
			"bb1": {alias: "bb1", reachable: true, diagFields: diagFields{storage: "ok"}},
			"bb2": {alias: "bb2", reachable: true, diagFields: diagFields{storage: "ok"}},
		}, 0},
		{"SomeUnreachable", map[string]hostDiag{
			"bb1": {alias: "bb1", reachable: true, diagFields: diagFields{storage: "ok"}},
			"bb2": {alias: "bb2", reachable: false, errMsg: "dial timeout"},
			"bb3": {alias: "bb3", reachable: false, errMsg: "connection refused"},
		}, 2},
		{"Empty", map[string]hostDiag{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := printDiagTable(tt.results); got != tt.want {
				t.Errorf("printDiagTable() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestCheckHostsFailsOnUnreachableHost verifies that -check's underlying
// checkHosts returns an error when any host is unreachable, instead of
// always returning nil regardless of what the table shows: -check exiting 0
// on unreachable hosts meant scripted preflights (and the driver-routed
// check's own exit-status branch) could never gate on the check.
func TestCheckHostsFailsOnUnreachableHost(t *testing.T) {
	defer log.SetOutput(log.Writer())
	log.SetOutput(io.Discard)

	// An alias with no matching SSH config entry fails to dial, landing in
	// group.DialErrors rather than aborting the whole check.
	err := checkHosts([]string{"no-such-sweep-test-host.invalid"}, "", 9000, newRemoteProgram(""), "", "/tmp")
	if err == nil {
		t.Error("checkHosts(unreachable host) = nil error, want an error")
	}
}
