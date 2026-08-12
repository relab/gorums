package main

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/relab/iago"
)

// numProbePorts is the number of consecutive ports checked starting at the base
// port, covering up to four benchmark servers per host (N=120 across 30 hosts).
const numProbePorts = 4

// hostDiag holds the diagnostics gathered from a single host.
type hostDiag struct {
	alias     string
	reachable bool
	errMsg    string

	diagFields

	skew time.Duration // remote clock minus local clock (approximate)
	rtt  time.Duration // SSH round-trip for the probe; bounds skew uncertainty

	// Since-boot TCP health counters (see checkTCPAllowlist). retransSegs over
	// outSegs is the retransmit ratio; synRetrans counts loss during connection
	// setup. outSegs == 0 means the host reported no TCP counters.
	retransSegs uint64
	outSegs     uint64
	synRetrans  uint64
}

// checkTCPAllowlist names the /proc/net counters read for the -check table.
// It is separate from the manifest's tcpCounterAllowlist (see tcpstats.go): the
// check reports a since-boot retransmit ratio (RetransSegs/OutSegs) and the
// setup-loss counter (TCPSynRetrans), so it needs OutSegs as the denominator,
// which the per-run manifest deltas do not record.
var checkTCPAllowlist = map[string]bool{
	"Tcp.RetransSegs":      true,
	"Tcp.OutSegs":          true,
	"TcpExt.TCPSynRetrans": true,
}

// checkConcurrency bounds how many hosts are dialed concurrently; the shared
// jump connection means this no longer caps TCP connections to the jump host.
const checkConcurrency = 30

// checkLocalTimeout bounds the driver's local self-probe so a wedged diagnostic
// command cannot hang the whole check.
const checkLocalTimeout = 15 * time.Second

// checkHosts runs connectivity and host diagnostics on each alias using the same
// iago SSH path the sweep uses, so a clean result means the sweep will connect.
// For each host it reports reachability, host info (hostname, kernel, CPUs,
// load), whether any benchmark port in [basePort, basePort+numProbePorts) is in
// use, the count of lingering processes from a prior sweep, and the host's
// clock skew relative to the local machine.
//
// All aliases are dialed through a single shared jump connection (one TCP
// connection to the jump host regardless of the number of targets) using
// iago.DialConcurrency to dial target hosts concurrently. An unreachable or
// slow host is reported as UNREACHABLE rather than aborting the whole check.
//
// selfAlias, when non-empty and present in aliases, is the host this check is
// running on (the driver of a driver-routed check). It is probed locally rather
// than dialed: a host cannot SSH to itself through the generated config — the
// loopback self-connection fails the SSH handshake with EOF — so dialing it
// would always report the driver UNREACHABLE even though it is right here.
// Results are printed as an aligned table sorted by alias.
func checkHosts(aliases []string, sshConfig string, basePort int, prog remoteProgram, selfAlias, remoteRoot string) error {
	command := diagCommand(basePort, prog, remoteRoot)
	results := make(map[string]hostDiag, len(aliases))

	// Probe the driver host directly and drop it from the set dialed over SSH.
	remote := aliases
	if selfAlias != "" && slices.Contains(aliases, selfAlias) {
		results[selfAlias] = diagLocal(selfAlias, command)
		remote = slices.DeleteFunc(slices.Clone(aliases), func(a string) bool { return a == selfAlias })
	}

	if len(remote) > 0 {
		// Dial the remaining aliases concurrently through a single shared jump
		// connection. Dial failures are collected in group.DialErrors rather
		// than aborting.
		group, err := iago.NewSSHGroup(remote, sshConfig, iago.DialConcurrency(checkConcurrency))
		if err != nil {
			for _, alias := range remote {
				results[alias] = hostDiag{alias: alias, errMsg: oneLine(err.Error())}
			}
		} else {
			defer group.Close()
			for alias, dialErr := range group.DialErrors {
				results[alias] = hostDiag{alias: alias, errMsg: oneLine(dialErr.Error())}
			}
			collected, _ := iago.Collect(group, "diag", func(ctx context.Context, host iago.Host) (hostDiag, error) {
				return diagHost(ctx, host, command), nil
			})
			maps.Copy(results, collected)
		}
	}

	if unreachable := printDiagTable(results); unreachable > 0 {
		return fmt.Errorf("%d of %d host(s) unreachable", unreachable, len(results))
	}
	return nil
}

// diagLocal runs the diagnostic command on the local machine (no SSH) and
// returns the parsed result. It is used for the driver host during a
// driver-routed check: the driver cannot SSH to itself through the generated
// config, and it is the very host the check is already running on, so the probe
// is run directly. The command reads the same clock it is measured against, so
// skewRTT naturally recovers a near-zero skew.
func diagLocal(alias, command string) hostDiag {
	d := hostDiag{alias: alias}
	ctx, cancel := context.WithTimeout(context.Background(), checkLocalTimeout)
	defer cancel()

	before := time.Now()
	out, err := exec.CommandContext(ctx, "bash", "-c", command).Output()
	after := time.Now()
	if err != nil {
		d.errMsg = oneLine(err.Error())
		return d
	}

	d.finishDiag(string(out), before, after)
	return d
}

// diagHost runs the diagnostic command on an already-connected host and returns
// the parsed result. Any command failure is captured in the returned hostDiag
// (reachable=false) rather than returned as an error.
func diagHost(ctx context.Context, host iago.Host, command string) hostDiag {
	d := hostDiag{alias: host.Name()}

	before := time.Now()
	out, shErr := iago.Output(ctx, host, command)
	after := time.Now()
	if shErr != nil {
		d.errMsg = shErr.Error()
		return d
	}

	d.finishDiag(out, before, after)
	return d
}

// finishDiag records a successful probe on d: it parses the diagnostic output,
// estimates the clock skew and round-trip time from the probe timestamps, and
// extracts the allowed TCP counters. The raw /proc/net counter dump is appended
// after the KEY=VALUE lines by diagCommand, so both are parsed from the same
// output.
func (d *hostDiag) finishDiag(out string, before, after time.Time) {
	d.reachable = true
	d.diagFields = parseDiag(out)
	d.skew, d.rtt = d.diagFields.skewRTT(before, after)

	tcp := parseAllowedCounters(out, checkTCPAllowlist)
	d.retransSegs = tcp["Tcp.RetransSegs"]
	d.outSegs = tcp["Tcp.OutSegs"]
	d.synRetrans = tcp["TcpExt.TCPSynRetrans"]
}

// skewRTT estimates the remote clock skew (remote minus local) and the network
// round-trip time from the four NTP timestamps of the diag exchange: the local
// times just before (t1) and after (t4) the SSH command, and the remote clock
// sampled at the command's start (t2, EPOCH) and end (t3, EPOCH_END). Following
// NTP, the offset is ((t2-t1)+(t3-t4))/2 and the round-trip delay is
// (t4-t1)-(t3-t2). Sampling the remote clock at both ends lets the delay exclude
// the time the diag script spent running on the host, so neither the skew nor the
// reported uncertainty (rtt/2) is inflated by how long the probe took — which on
// a LAN, where the script's tens of milliseconds dwarf the sub-millisecond
// network path, would otherwise dominate the estimate. Falls back to the
// single-sample midpoint estimate when only EPOCH is present (an older host), and
// to a zero skew with the raw wall-clock span when no epoch was read.
func (f diagFields) skewRTT(before, after time.Time) (skew, rtt time.Duration) {
	if f.epoch <= 0 {
		return 0, after.Sub(before)
	}
	t1, t4 := before.UnixNano(), after.UnixNano()
	t2 := int64(f.epoch * float64(time.Second))
	if f.epochEnd <= 0 {
		// Older host emitted only one epoch: use the local midpoint (biased by
		// the on-host script duration, but better than reporting nothing).
		mid := t1 + (t4-t1)/2
		return time.Duration(t2 - mid), after.Sub(before)
	}
	t3 := int64(f.epochEnd * float64(time.Second))
	offset := ((t2 - t1) + (t3 - t4)) / 2
	// Clock-rate differences or measurement noise can make the remote-measured
	// duration exceed the local span; a negative round-trip is meaningless.
	delay := max((t4-t1)-(t3-t2), 0)
	return time.Duration(offset), time.Duration(delay)
}

// diagCommand builds the best-effort diagnostic shell script run on each host.
// Every datum is emitted as a KEY=VALUE line consumed by parseDiag; failures of
// individual probes are swallowed so the script always exits 0 and one slow or
// missing tool does not fail the whole check.
func diagCommand(basePort int, prog remoteProgram, remoteRoot string) string {
	ports := make([]string, numProbePorts)
	for i := range ports {
		ports[i] = strconv.Itoa(basePort + i)
	}
	portList := strings.Join(ports, " ")

	// EPOCH (first) and EPOCH_END (last) bracket the whole script with the remote
	// clock, so skewRTT can subtract the time the script spent running on the host
	// and avoid biasing the skew by up to half that duration — which dominates on
	// a LAN, where the script's tens of milliseconds swamp the sub-millisecond
	// network path. Note: %% escapes a literal % for Printf; date needs %s.%N.
	return fmt.Sprintf(`echo "EPOCH=$(date +%%s.%%N 2>/dev/null)"
echo "HOST=$(hostname 2>/dev/null)"
echo "KERNEL=$(uname -sr 2>/dev/null)"
echo "CPUS=$(nproc 2>/dev/null)"
echo "LOAD=$(cut -d' ' -f1 /proc/loadavg 2>/dev/null)"
procs=$(pgrep -fc '%[1]s' 2>/dev/null)
echo "PROCS=${procs:-0}"
busy=
for p in %[2]s; do
  if ss -ltnH 2>/dev/null | grep -qE ":$p([[:space:]]|$)"; then busy="$busy $p"; fi
done
echo "PORTSBUSY=$busy"
root=%[3]s
user=${USER:-$(id -un)}
ns="$root/sweep-$user"
storage=missing
[ -d "$root" ] && storage=readonly
[ -d "$root" ] && [ -w "$root" ] && storage=ok
free=$(df -hPk "$root" 2>/dev/null | awk 'NR==2 {print $4}')
stale=0
runs=0
if [ -d "$ns" ]; then
  stale=$(find "$ns" -maxdepth 1 -type f \( -name '*.binpb' -o -name '*.cpuprofile' -o -name '*.memprofile' -o -name 'sweep-*' \) 2>/dev/null | wc -l | tr -d ' ')
  runs=$(find "$ns" -mindepth 1 -maxdepth 1 -type d -name 'sweep-driver-*' 2>/dev/null | wc -l | tr -d ' ')
fi
echo "STORAGE=$storage"
echo "FREE=${free:--}"
echo "STALE=${stale:-0}"
echo "DRIVERRUNS=${runs:-0}"
%[4]s 2>/dev/null
echo "EPOCH_END=$(date +%%s.%%N 2>/dev/null)"`, prog.pgrep(), portList, iago.Quote(remoteRoot), tcpStatsCommand)
}

// failureDiagCommand builds the host-snapshot script run on each host of a
// failed run. Unlike diagCommand (a compact KEY=VALUE probe parsed into a
// table), this emits free-form text appended verbatim to a per-run snapshot
// file for a human to read post-mortem. It captures the host environment (load,
// fd limits), any benchmark processes still running, and the socket state on the
// given ports: listeners, plus all TCP connections including TIME_WAIT entries,
// which survive ~60s after a peer closes and so reveal connection-refused
// failures even though the processes have already exited. Every probe swallows
// its own errors so the script always exits 0 and one missing tool does not
// abort the snapshot.
func failureDiagCommand(ports []string, prog remoteProgram) string {
	portList := strings.Join(ports, " ")
	return fmt.Sprintf(`echo "uptime: $(uptime 2>/dev/null)"
echo "loadavg: $(cat /proc/loadavg 2>/dev/null)"
echo "cpus: $(nproc 2>/dev/null)"
echo "fd_limit_soft: $(ulimit -Sn 2>/dev/null)"
echo "fd_limit_hard: $(ulimit -Hn 2>/dev/null)"
echo "--- benchmark processes ---"
pgrep -fa '%[1]s' 2>/dev/null || echo "(none)"
for p in %[2]s; do
  echo "--- listeners on :$p ---"
  ss -ltnp 2>/dev/null | grep -E ":$p([[:space:]]|$)" || echo "(none)"
  echo "--- connections on :$p (established + time-wait) ---"
  ss -tan 2>/dev/null | grep -E ":$p([[:space:]]|$)" || echo "(none)"
done`, prog.pgrep(), portList)
}

// diagFields holds the parsed KEY=VALUE output of diagCommand.
type diagFields struct {
	hostname   string
	kernel     string
	cpus       string
	load       string
	epoch      float64 // remote clock (seconds) sampled at the script's start
	epochEnd   float64 // remote clock (seconds) sampled at the script's end
	procs      int
	portsBusy  []string
	storage    string
	free       string
	stale      int
	driverRuns int
}

// parseDiag parses the KEY=VALUE lines emitted by diagCommand. Unknown keys are
// ignored and missing keys leave their zero value, so partial output from an
// older or stripped-down host still yields a useful result.
func parseDiag(output string) diagFields {
	var f diagFields
	for line := range strings.Lines(output) {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "HOST":
			f.hostname = value
		case "KERNEL":
			f.kernel = value
		case "CPUS":
			f.cpus = value
		case "LOAD":
			f.load = value
		case "EPOCH":
			f.epoch, _ = strconv.ParseFloat(value, 64)
		case "EPOCH_END":
			f.epochEnd, _ = strconv.ParseFloat(value, 64)
		case "PROCS":
			f.procs, _ = strconv.Atoi(value)
		case "PORTSBUSY":
			f.portsBusy = strings.Fields(value)
		case "STORAGE":
			f.storage = value
		case "FREE":
			f.free = value
		case "STALE":
			f.stale, _ = strconv.Atoi(value)
		case "DRIVERRUNS":
			f.driverRuns, _ = strconv.Atoi(value)
		}
	}
	return f
}

// printDiagTable renders the per-host diagnostics as an aligned table, sorted by
// alias, and prints a trailing summary line flagging any problems.
// printDiagTable prints the per-host diagnostic table and a summary line,
// and returns how many hosts were unreachable, so callers (e.g. checkHosts)
// can fail loudly instead of always reporting success regardless of what the
// table shows.
func printDiagTable(results map[string]hostDiag) (unreachable int) {
	sorted := slices.SortedFunc(maps.Values(results), func(a, b hostDiag) int {
		return compareHost(a.alias, b.alias)
	})

	tw := tabwriter.NewWriter(log.Writer(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "HOST\tSTATUS\tHOSTNAME\tKERNEL\tCPUS\tLOAD\tSKEW\tRETRANS%\tSYN-RETX\tPORTS BUSY\tLINGERING\tSTORAGE\tFREE\tSTALE\tRUNS")

	var withBusyPorts, withLingering, skewed, withStale, badStorage, driverRuns int
	for _, d := range sorted {
		if !d.reachable {
			unreachable++
			fmt.Fprintf(tw, "%s\tUNREACHABLE\t%s\n", d.alias, oneLine(d.errMsg))
			continue
		}

		ports := "-"
		if len(d.portsBusy) > 0 {
			ports = strings.Join(d.portsBusy, ",")
			withBusyPorts++
		}
		lingering := "-"
		if d.procs > 0 {
			lingering = strconv.Itoa(d.procs)
			withLingering++
		}
		if d.skew.Abs() > time.Second {
			skewed++
		}
		if d.stale > 0 {
			withStale++
		}
		if d.storage != "ok" {
			badStorage++
		}
		driverRuns += d.driverRuns

		retransPct, synRetx := formatRetrans(d.retransSegs, d.outSegs, d.synRetrans)
		fmt.Fprintf(tw, "%s\tOK\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			d.alias, d.hostname, d.kernel, d.cpus, d.load, formatSkew(d.skew, d.rtt), retransPct, synRetx, ports, lingering,
			d.storage, d.free, d.stale, d.driverRuns)
	}
	tw.Flush()

	log.Printf("checked %d host(s): %d unreachable, %d with busy ports, %d with lingering processes, %d with clock skew > 1s",
		len(results), unreachable, withBusyPorts, withLingering, skewed)
	log.Printf("remote storage: %d invalid, %d host(s) with stale disposable sweep files; %d driver run directories (use -list for details)",
		badStorage, withStale, driverRuns)
	if withStale > 0 {
		log.Printf("stale files are reported only; -check does not delete them")
	}
	return unreachable
}

// formatRetrans renders the since-boot retransmit ratio (retransSegs/outSegs, as
// a percentage) and the setup-loss counter (synRetrans) for the -check table. It
// returns "-", "-" when outSegs is zero, i.e. the host reported no TCP counters,
// so a missing reading is not mistaken for a healthy 0.00%.
func formatRetrans(retransSegs, outSegs, synRetrans uint64) (retransPct, synRetx string) {
	if outSegs == 0 {
		return "-", "-"
	}
	pct := 100 * float64(retransSegs) / float64(outSegs)
	return fmt.Sprintf("%.2f%%", pct), strconv.FormatUint(synRetrans, 10)
}

// formatSkew renders a clock-skew estimate with its round-trip uncertainty,
// e.g. "+12ms (±3ms)". Skew is shown to the nearest millisecond because the
// estimate cannot be more precise than the SSH round-trip.
func formatSkew(skew, rtt time.Duration) string {
	return fmt.Sprintf("%+dms (±%dms)",
		skew.Round(time.Millisecond).Milliseconds(),
		(rtt / 2).Round(time.Millisecond).Milliseconds())
}

// oneLine collapses s to a single line for tabular output.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
