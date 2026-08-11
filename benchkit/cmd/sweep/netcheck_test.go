package main

import (
	"math"
	"strings"
	"testing"
)

// TestParsePingLoss verifies extraction of the packet-loss percentage from
// Linux ping summary output, including fractional percentages and the error
// on unrecognized output.
func TestParsePingLoss(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    float64
		wantErr bool
	}{
		{
			name: "no loss",
			out: `--- 152.94.162.19 ping statistics ---
20 packets transmitted, 20 received, 0% packet loss, time 1918ms
rtt min/avg/max/mdev = 0.139/0.193/0.240/0.013 ms`,
			want: 0,
		},
		{
			name: "heavy loss",
			out: `--- 152.94.162.26 ping statistics ---
50 packets transmitted, 36 received, 28% packet loss, time 10191ms`,
			want: 28,
		},
		{
			name: "fractional loss",
			out:  "100 packets transmitted, 99 received, 1.5% packet loss, time 9912ms",
			want: 1.5,
		},
		{
			name:    "unrecognized output",
			out:     "ping: unknown host bb99",
			wantErr: true,
		},
		{
			name:    "empty",
			out:     "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePingLoss(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("loss = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRingTargets verifies that each host is assigned its ring successor's
// peer address, covering every host's link in both directions across the
// ring, and that a single host has nothing to check.
func TestRingTargets(t *testing.T) {
	hosts := []hostAssignment{
		{alias: "bb2"},
		{alias: "bb3", peerHost: "152.94.162.13"},
		{alias: "bb4"},
	}
	got := ringTargets(hosts)
	want := map[string]string{
		"bb2": "152.94.162.13", // bb3's advertised peer address
		"bb3": "bb4",           // no peerHost -> alias
		"bb4": "bb2",           // ring wraps
	}
	if len(got) != len(want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
	for from, to := range want {
		if got[from] != to {
			t.Errorf("target[%q] = %q, want %q", from, got[from], to)
		}
	}

	if got := ringTargets(hosts[:1]); len(got) != 0 {
		t.Errorf("single host targets = %v, want none", got)
	}
}

// TestNetcheckFailure verifies the abort decision: loss at or above the limit
// on any link fails the check with every lossy link named, while loss below
// the limit (or no loss) passes.
func TestNetcheckFailure(t *testing.T) {
	losses := []linkLoss{
		{from: "bb2", target: "bb16", pct: 28},
		{from: "bb16", target: "bb17", pct: 24},
		{from: "bb3", target: "bb4", pct: 1},
	}
	err := netcheckFailure(losses, 5)
	if err == nil {
		t.Fatal("expected failure for links at 28% and 24% loss")
	}
	for _, want := range []string{"bb2", "bb16", "28", "24", "-netcheck=false"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q\ngot: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "bb3 -> bb4") {
		t.Errorf("sub-limit link should not be listed as a failure\ngot: %v", err)
	}

	if err := netcheckFailure(losses[2:], 5); err != nil {
		t.Errorf("1%% loss should pass at a 5%% limit, got: %v", err)
	}
	if err := netcheckFailure(nil, 5); err != nil {
		t.Errorf("no loss should pass, got: %v", err)
	}
}

// TestPingCommand verifies the probe command shape: fixed count and interval,
// a deadline so a black-holed target cannot stall the check, quiet output,
// and a shell-quoted target.
func TestPingCommand(t *testing.T) {
	cmd := pingCommand("152.94.162.26")
	for _, want := range []string{"ping", "-c 20", "-i 0.1", "-w 8", "-q", "'152.94.162.26'"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command missing %q\ngot: %s", want, cmd)
		}
	}
}
