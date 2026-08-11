package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/relab/iago"
)

// Preflight network check: before deploying anything, every host pings its
// ring successor and the sweep aborts when any link shows packet loss at or
// above netcheckMaxLossPct. A lossy link does not fail a benchmark run — TCP
// retransmits through it — but it silently destroys the measurement (a node
// behind a ~25% loss link runs at a few percent of its peers' throughput,
// pinned at the TCP minimum RTO). The ring covers every host's link in both
// directions (it pings out once and is pinged once) at the cost of one probe
// per host, all run in parallel, so the check adds only a few seconds.

const (
	// netcheckPings and netcheckIntervalS size one probe: 20 pings at 100 ms
	// spacing take ~2 s and resolve loss to 5% granularity, coarse but ample
	// for the ≥5% abort threshold.
	netcheckPings     = 20
	netcheckIntervalS = "0.1"
	// netcheckDeadlineS caps a probe wall-clock (ping -w) so a black-holed
	// target cannot stall the check; unanswered pings count as lost.
	netcheckDeadlineS = "8"
	// netcheckMaxLossPct is the loss percentage at which the sweep aborts.
	// Healthy LAN links lose nothing; a link at 5%+ already means constant
	// TCP retransmission stalls in a latency benchmark.
	netcheckMaxLossPct = 5.0
)

// linkLoss records the measured packet loss of one ring probe.
type linkLoss struct {
	from   string  // probing host alias
	target string  // probed peer address
	pct    float64 // packet loss percentage
}

// pingCommand returns the loss probe run on each host: fixed count and
// interval, a hard deadline, quiet output (only the summary line is parsed).
func pingCommand(target string) string {
	return fmt.Sprintf("ping -c %d -i %s -w %s -q %s",
		netcheckPings, netcheckIntervalS, netcheckDeadlineS, iago.Quote(target))
}

// pingLossRE matches the loss percentage in the ping summary line, e.g.
// "50 packets transmitted, 36 received, 28% packet loss, time 10191ms".
var pingLossRE = regexp.MustCompile(`([0-9.]+)% packet loss`)

// parsePingLoss extracts the packet-loss percentage from ping output.
func parsePingLoss(out string) (float64, error) {
	m := pingLossRE.FindStringSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("no packet-loss summary in ping output: %q", oneLine(out))
	}
	return strconv.ParseFloat(m[1], 64)
}

// ringTargets assigns every host its ring successor's peer address (the same
// address the benchmark itself dials), so each host's link is probed once in
// each direction. A single host has no link to check.
func ringTargets(hosts []hostAssignment) map[string]string {
	if len(hosts) < 2 {
		return nil
	}
	targets := make(map[string]string, len(hosts))
	for i, h := range hosts {
		next := hosts[(i+1)%len(hosts)]
		targets[h.alias] = cmp.Or(next.peerHost, next.alias)
	}
	return targets
}

// netcheckFailure returns an error naming every link at or above the loss
// limit, or nil when all links are below it. Links with minor loss are the
// caller's to log; only limit-or-worse loss aborts the sweep.
func netcheckFailure(losses []linkLoss, limitPct float64) error {
	var bad []string
	for _, l := range losses {
		if l.pct >= limitPct {
			bad = append(bad, fmt.Sprintf("%s -> %s: %.0f%% loss", l.from, l.target, l.pct))
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("network check failed — packet loss at or above %.0f%% (fix the link, exclude the host, or skip with -netcheck=false):\n  %s",
		limitPct, strings.Join(bad, "\n  "))
}

// checkNetworkHealth probes every host's link via ringTargets and returns an
// error when any link loses netcheckMaxLossPct or more. Probe failures (ping
// missing, unparseable output) are logged and skipped rather than fatal: the
// check is a tripwire for lossy links, not a connectivity gate — the SSH
// connections already proved the hosts reachable.
func checkNetworkHealth(g iago.Group, hosts []hostAssignment) error {
	targets := ringTargets(hosts)
	if len(targets) == 0 {
		return nil
	}
	pcts, _ := iago.Collect(withTimeout(g, 60*time.Second), "netcheck", func(ctx context.Context, host iago.Host) (float64, error) {
		target := targets[host.Name()]
		if target == "" {
			return 0, nil
		}
		// ping exits non-zero when packets were lost; the summary line is
		// still printed, so the exit status is ignored and only parse
		// failures are reported.
		out, _ := iago.Output(ctx, host, pingCommand(target))
		pct, err := parsePingLoss(out)
		if err != nil {
			log.Printf("  warning: netcheck %s -> %s: %v", host.Name(), target, err)
			return 0, nil
		}
		if pct > 0 {
			log.Printf("  netcheck %s -> %s: %.0f%% loss", host.Name(), target, pct)
		}
		return pct, nil
	})
	var losses []linkLoss
	for host, pct := range pcts {
		if pct > 0 {
			losses = append(losses, linkLoss{from: host, target: targets[host], pct: pct})
		}
	}
	return netcheckFailure(losses, netcheckMaxLossPct)
}
