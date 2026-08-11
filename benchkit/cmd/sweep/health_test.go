package main

import (
	"strings"
	"testing"
)

// TestHealthProbeHosts verifies the host-selection logic that decides which
// hosts get post-incident probed for a given run outcome: the hosts of nodes
// with a missing result file for a failed run, the flagged hosts for a
// degraded run (worst-first, as findDegradedNodes orders them), and none for
// a successful run.
func TestHealthProbeHosts(t *testing.T) {
	base := "e1_Q_N3_W1_P0"
	nodes := []nodeAssignment{
		{host: "bb2", port: 9000},
		{host: "bb3", port: 9000},
		{host: "bb4", port: 9000},
	}

	tests := []struct {
		name string
		o    runOutcome
		want []string
	}{
		{
			name: "failed run probes hosts with missing result files",
			o: runOutcome{
				status:       runStatusFailed,
				missingFiles: []string{resultFilename(base, nodes[2], resultExt)},
			},
			want: []string{"bb4"},
		},
		{
			name: "failed run with multiple missing files dedups by host and preserves node order",
			o: runOutcome{
				status: runStatusFailed,
				missingFiles: []string{
					resultFilename(base, nodes[2], resultExt),
					resultFilename(base, nodes[0], resultExt),
				},
			},
			want: []string{"bb2", "bb4"},
		},
		{
			name: "failed run with no missing files (e.g. collection-phase failure) probes nothing",
			o:    runOutcome{status: runStatusFailed},
			want: nil,
		},
		{
			name: "degraded run probes flagged hosts worst-first",
			o: runOutcome{
				status: runStatusDegraded,
				degraded: []degradedNode{
					{Host: "bb4:9000", Relative: 0.05},
					{Host: "bb3:9000", Relative: 0.3},
				},
			},
			want: []string{"bb4", "bb3"},
		},
		{
			name: "degraded run dedups hosts sharing multiple nodes",
			o: runOutcome{
				status: runStatusDegraded,
				degraded: []degradedNode{
					{Host: "bb4:9000", Relative: 0.05},
					{Host: "bb4:9001", Relative: 0.1},
				},
			},
			want: []string{"bb4"},
		},
		{
			name: "succeeded run probes nothing",
			o:    runOutcome{status: runStatusSucceeded},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := healthProbeHosts(tt.o, base, nodes)
			if len(got) != len(tt.want) {
				t.Fatalf("hosts = %v, want %v", got, tt.want)
			}
			for i, h := range got {
				if h != tt.want[i] {
					t.Errorf("hosts[%d] = %q, want %q", i, h, tt.want[i])
				}
			}
		})
	}
}

// TestHealthyPingTargets verifies that the ping-ring bonus check targets up
// to n hosts not in the implicated set, in allHosts order, and that an
// implicated host is never chosen as its own health check target.
func TestHealthyPingTargets(t *testing.T) {
	allHosts := []hostAssignment{
		{alias: "bb2"},
		{alias: "bb3", peerHost: "152.94.162.13"},
		{alias: "bb4"},
		{alias: "bb5"},
	}

	got := healthyPingTargets(allHosts, map[string]bool{"bb2": true}, 2)
	want := []string{"152.94.162.13", "bb4"} // bb3's peer address, then bb4
	if len(got) != len(want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
	for i, target := range want {
		if got[i] != target {
			t.Errorf("targets[%d] = %q, want %q", i, got[i], target)
		}
	}

	if got := healthyPingTargets(allHosts, map[string]bool{"bb2": true, "bb3": true, "bb4": true, "bb5": true}, 2); got != nil {
		t.Errorf("all hosts implicated: targets = %v, want none", got)
	}

	if got := healthyPingTargets(allHosts, nil, 1); len(got) != 1 {
		t.Errorf("n=1: targets = %v, want exactly 1", got)
	}
}

// TestHealthProbeCommand verifies the ping-ring bonus command is tighter
// (fewer pings, shorter deadline) than netcheck's preflight probe, since it
// runs mid-sweep and must stay cheap, and that the target is shell-quoted.
func TestHealthProbeCommand(t *testing.T) {
	cmd := healthProbeCommand("152.94.162.26")
	for _, want := range []string{"ping", "-c 5", "-w 2", "-q", "'152.94.162.26'"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command missing %q\ngot: %s", want, cmd)
		}
	}
}
