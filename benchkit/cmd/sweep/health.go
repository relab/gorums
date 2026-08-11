package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/relab/iago"
)

// Post-incident health probes capture transient host evidence immediately after
// failed or degraded runs and store it beside the run artifacts.

const (
	// healthProbeTimeout bounds the whole probe (diag command plus the
	// ping-ring bonus) so a stuck or unreachable host cannot delay the sweep
	// beyond a few seconds; the probe runs synchronously between one run
	// finishing and the next starting.
	healthProbeTimeout = 8 * time.Second
	// healthProbePings and healthProbePingDeadlineS size the ping-ring bonus
	// check. They are tighter than netcheck's preflight probe (netcheck.go),
	// which affords a full 8s deadline before the sweep even starts; this
	// probe runs mid-sweep and must stay cheap.
	healthProbePings         = 5
	healthProbePingDeadlineS = "2"
	// healthProbePeers caps how many other hosts are ping-checked from each
	// implicated host.
	healthProbePeers = 2
)

// healthProbeHosts returns the SSH-alias hosts implicated by a run's outcome:
// the hosts of nodes whose result file is missing for a failed run, or the
// flagged hosts for a degraded run. Returns nil for a successful run or when
// no host is implicated (e.g. a failure recorded before any node was
// assigned a host).
func healthProbeHosts(o runOutcome, base string, nodes []nodeAssignment) []string {
	switch o.status {
	case runStatusFailed:
		return missingFileHosts(o.missingFiles, base, nodes)
	case runStatusDegraded:
		return degradedNodeHosts(o.degraded)
	default:
		return nil
	}
}

// missingFileHosts returns the deduplicated host aliases of nodes whose
// expected result file appears in missing, in node order.
func missingFileHosts(missing []string, base string, nodes []nodeAssignment) []string {
	if len(missing) == 0 {
		return nil
	}
	missingSet := make(map[string]bool, len(missing))
	for _, m := range missing {
		missingSet[m] = true
	}
	var hosts []string
	seen := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if !missingSet[resultFilename(base, n, resultExt)] || seen[n.host] {
			continue
		}
		seen[n.host] = true
		hosts = append(hosts, n.host)
	}
	return hosts
}

// degradedNodeHosts returns the deduplicated host aliases of the flagged
// nodes, in the given (worst-first) order. degradedNode.Host is a
// "host:port" label; only the bare alias is dialable via SSH.
func degradedNodeHosts(degraded []degradedNode) []string {
	if len(degraded) == 0 {
		return nil
	}
	var hosts []string
	seen := make(map[string]bool, len(degraded))
	for _, d := range degraded {
		host := hostFromAddr(d.Host)
		if seen[host] {
			continue
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	return hosts
}

// healthyPingTargets returns up to n peer addresses of hosts not in
// implicated, for the health probe's ping-ring bonus check. Order follows
// allHosts, so the choice is deterministic.
func healthyPingTargets(allHosts []hostAssignment, implicated map[string]bool, n int) []string {
	var targets []string
	for _, h := range allHosts {
		if implicated[h.alias] {
			continue
		}
		targets = append(targets, cmp.Or(h.peerHost, h.alias))
		if len(targets) == n {
			break
		}
	}
	return targets
}

// healthProbeCommand returns the ping command for the health probe's
// ping-ring bonus check: fewer pings and a shorter deadline than netcheck's
// preflight probe, since this runs mid-sweep and must stay cheap.
func healthProbeCommand(target string) string {
	return fmt.Sprintf("ping -c %d -i 0.2 -w %s -q %s",
		healthProbePings, healthProbePingDeadlineS, iago.Quote(target))
}

// healthProbePath returns the post-incident health-probe path for base under
// outdir.
func healthProbePath(outdir, base string) string {
	return filepath.Join(outdir, logSubdir, base+"_health.txt")
}

// runHealthProbe gathers a lightweight diagnostic snapshot (load, busy
// ports, and TCP counters via diagCommand, plus a ping-ring check against a
// couple of healthy peers) from each implicated host and writes it to
// <outdir>/logs/<base>_health.txt. remoteRoot is the storage namespace
// diagCommand inspects for free space and staleness (cfg.remoteDir, not a
// hardcoded path), so the probe reports on the directory the sweep actually
// uses. It reuses the diag.go probe machinery over g's already-connected SSH
// sessions rather than opening new connections. It returns the written path,
// or "" when there was nothing to probe or the probe could not even be
// started; either way it is best-effort — a probe failure is logged and
// swallowed, never propagated, so it cannot fail or delay the sweep beyond
// healthProbeTimeout.
func runHealthProbe(g iago.Group, allHosts []hostAssignment, prog remoteProgram, basePort int, hosts []string, outdir, base, remoteRoot string) string {
	if len(hosts) == 0 {
		return ""
	}
	implicated := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		implicated[h] = true
	}
	sub := g
	sub.Hosts = nil
	for _, h := range g.Hosts {
		if implicated[h.Name()] {
			sub.Hosts = append(sub.Hosts, h)
		}
	}
	if len(sub.Hosts) == 0 {
		return ""
	}

	dir := filepath.Join(outdir, logSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("  warning: health probe: %v", err)
		return ""
	}
	path := healthProbePath(outdir, base)
	f, err := os.Create(path)
	if err != nil {
		log.Printf("  warning: health probe: %v", err)
		return ""
	}
	defer f.Close()

	command := diagCommand(basePort, prog, remoteRoot)
	targets := healthyPingTargets(allHosts, implicated, healthProbePeers)

	var mu sync.Mutex
	run(withTimeout(sub, healthProbeTimeout), "health probe", func(ctx context.Context, host iago.Host) error {
		out, shErr := iago.Output(ctx, host, command)
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(f, "===== %s =====\n", host.Name())
		if shErr != nil {
			fmt.Fprintf(f, "(probe failed: %v)\n\n", shErr)
			return nil
		}
		io.WriteString(f, out)
		fmt.Fprintln(f)
		for _, target := range targets {
			pingOut, pingErr := iago.Output(ctx, host, healthProbeCommand(target))
			if pingErr != nil {
				continue
			}
			fmt.Fprintf(f, "--- ping %s ---\n%s\n", target, strings.TrimRight(pingOut, "\n"))
		}
		fmt.Fprintln(f)
		return nil
	})
	return path
}
