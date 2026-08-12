// Command sweep runs a distributed parameter sweep of the gorums benchmark
// across a cluster of machines reachable via SSH.
//
// Usage:
//
//	sweep [flags]
//
// Host selection:
//
// The -hosts flag is required. It accepts a comma-separated list of SSH host
// aliases where each token may be:
//
//   - A PREFIX[lo-hi]SUFFIX numeric range expanded to one alias per integer,
//     e.g. "bb[1-30]" → bb1, bb2, …, bb30.
//   - A glob pattern (*, ?) matched against non-wildcard Host entries in the
//     SSH config, e.g. "bb*" enumerates explicit bb1, bb2, … entries.
//   - A literal alias returned verbatim.
//
// For example:
//
//	-hosts 'bb[1-30]'         bb1, bb2, ..., bb30
//	-hosts 'bb[1-7],nebula'   bb1..bb7 plus nebula
//	-hosts 'rack[1-2]-node'   rack1-node, rack2-node
//	-hosts 'bb*'              all explicit bb* entries in SSH config
//
// Quote the value so the shell does not interpret the brackets.
//
// The -check diagnostic mode operates on the selected hosts and then exits:
//
//	-check      report reachability, host info, busy benchmark ports, lingering
//	            benchmark processes, and clock skew, using the sweep's SSH path
//
// Any binary deployed via -binary must support these flags:
//
//	-self=host:port           this node's listen address (distributed mode)
//	-remotes=addr,...         comma-separated peer addresses
//	-benchmarks=name,...      comma-separated exact benchmark names to run
//	-workers=N                concurrent goroutines
//	-payload=N                payload size in bytes
//	-time=duration            measurement duration
//	-output=path              write result file to this path
//	-rate=N                   target sends/sec per node; 0 = unlimited (optional)
//	-stream-mode=dual|dedup   symmetric stream topology (optional; default dual)
//	-verbose                  verbose logging (optional)
//
// For a foreign protocol, pass a prebuilt linux/amd64 binary via -binary; sweep
// uploads and runs it unchanged. When -binary is omitted, sweep builds the
// binary itself: it uses the -build command (or the BENCHKIT_BUILD environment
// variable) if set, substituting the chosen output path for the {{output}}
// token, and otherwise falls back to the built-in default
// "go build -o <output> ./cmd/benchmark". The custom command is run via "sh -c"
// from the benchkit module root with GOOS=linux GOARCH=amd64 in its environment.
//
// Cluster-local driver:
//
// When the controlling laptop is far from the cluster, every per-run SSH
// round-trip and the one-time binary upload to each host cross the WAN. The
// -driver flag moves the orchestration onto a cluster-local host so that
// traffic stays on the LAN and the benchmark binary crosses the WAN only once:
//
//	-driver <host>   run the orchestration on this SSH host (LAN-local to peers)
//	-driver first    use the first -hosts entry as the driver
//
// The driver host is excluded from the benchmark pool so the orchestrator does
// not perturb a co-located replica; a -driver host outside -hosts (a dedicated
// head node) leaves the pool unchanged. The laptop cross-builds sweep and the
// benchmark binary, ships them plus a generated SSH config to the driver over
// the user's own ssh/scp, and re-execs sweep there with -driven. The SSH agent
// is forwarded (ssh -A) so the driver authenticates to peers with the laptop's
// keys. The remote sweep is detached, so it survives a laptop disconnect; by
// default the laptop streams its output live and, on completion, downloads
// compact plot data plus any failed-run result files. Raw successful .binpb
// files stay in the driver work dir for optional archival. Reconnect to a
// detached run, or archive raw results after compact collection, with:
//
//	-collect [remote-work-dir]
//
// -detach skips the streaming and waiting: the run starts, the launcher
// records the run in <outdir>/.sweep-last.json and exits, so closing the
// laptop lid right away is an intentional clean exit rather than a dropped
// connection:
//
//	-driver <host> -detach
//
// A bare -collect uses the saved driver and path for the latest launch.
// -collect-now [path] takes a best-effort snapshot even while a run is active,
// and -list shows active, completed, raw-pending, and recoverable driver runs.
// Remote files live below <remote-dir>/sweep-$USER (default root: /tmp).
//
// A run whose compact transfer was built before a change to what gets exported
// can be rebuilt where its raw files still are, then downloaded again:
//
//	-export-compact <work-dir>   rebuild plotdata/ and compact-transfer/, then exit
//
// The -driven and -git-sha flags are internal: the launcher sets them on the
// driver and they are not meant to be passed by hand.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/relab/iago"
)

// config holds all runtime configuration. It is internal to the sweep tool.
type config struct {
	binaryPath string
	buildCmd   string
	sshConfig  string
	rootDir    string // -outdir: root directory that holds per-run subdirectories
	outDir     string // actual run directory under rootDir
	port       int
	duration   time.Duration
	trim       time.Duration
	sweepLabel string
	verbose    bool
	check      bool
	sweep      sweepConfig
	prog       remoteProgram

	// Pass-through flags forwarded verbatim to every benchmark node.
	interval    string // -interval; empty = binary default
	statsMode   string // -stats-mode; empty = binary default
	rateStep    int    // -rate-step; 0 = no ramp
	rateStepMax int    // -rate-step-max; 0 = no ramp
	extraArgs   string // -extra-args; appended verbatim

	collectProfiles bool // -collect-profiles: gather per-node CPU/heap profiles
	pgo             bool // -pgo: merge collected CPU profiles into default.pgo

	// -plot: (re)generate the Typst report from a collected output directory,
	// without running a sweep. The remaining fields filter that report.
	plotDir         string   // -plot: output directory to render; empty = run a sweep
	includeDegraded bool     // -include-degraded: keep degraded reps in the aggregates
	excludeRuns     []string // -exclude-run: run base names to drop
	excludeDims     []string // -exclude: DIM=VALUE tokens to drop (e.g. nodes=58)

	// exportCSVDir: regenerate the human- and grep-friendly runs.csv/nodes.csv
	// pair from a collected directory's plotdata.binpb, without running a
	// sweep or generating a report (see exportPlotCSV).
	exportCSVDir string // -export-csv: output directory whose plotdata.binpb to export; empty = do not export

	// exportCompactDir: rebuild the reduced plot data, the event streams, and
	// the compact-transfer directory from a sweep work directory's raw result
	// files (see prepareCompactTransfer). This is the driver-side half of a
	// download: it runs where the raw files are, so a run whose compact transfer
	// predates a change to what gets exported can be rebuilt and re-downloaded
	// without shipping the raw archive across the WAN.
	exportCompactDir string // -export-compact: sweep work directory to rebuild; empty = do not rebuild

	// The degradation bounds each node's measurement must respect, relative to
	// the run median (see degraded.go); a non-positive value disables that
	// check.
	degradedBelow        float64 // -degraded-below: minimum throughput
	degradedAbove        float64 // -degraded-above: maximum throughput
	degradedLatencyBelow float64 // -degraded-latency-below: minimum median latency

	// netcheck probes every host link with a ping ring before the sweep and
	// aborts on heavy packet loss (see netcheck.go).
	netcheck bool // -netcheck

	// fdLimit raises the soft open-file limit (ulimit -Sn) for each launched
	// benchmark node and, in driven mode, the driver-side sweep process. The
	// default lifts the common 1024 soft limit that a large mesh exhausts; 0
	// disables the change and uses the host default (see fdLimitStmt).
	fdLimit int // -fd-limit

	// Cluster-local driver: run the orchestration on a host inside the cluster
	// so per-run SSH and the binary upload stay on the LAN (see driver.go).
	driver       string // -driver: driver host alias, or "first" for hosts[0]; "" = run locally
	collect      string // -collect [path]: collect a finished driver run; no path selects the latest
	collectNow   string // -collect-now [path]: snapshot a run even when it is still active
	list         bool   // -list: list driver runs and their collection status
	remoteDir    string // -remote-dir: remote storage root; each user gets sweep-$USER below it
	remoteDirs   map[string]string
	detach       bool   // -detach: with -driver, start the run and exit immediately
	driven       bool   // -driven (internal): this process is the orchestrator on the driver
	gitSHA       string // -git-sha (internal): HEAD forwarded by the launcher for manifests
	readyMarker  string // -ready-marker (internal): path to touch once peers are dialed
	selfHost     string // -self-host (internal): alias of the host running a -check, probed locally instead of via SSH
	transferMode string // -transfer: file transfer backend for driver uploads/downloads (rsync or sftp)

	// LLM failure triage: when -explain is set, the failed runs are diagnosed by
	// a model after the sweep completes. With -driver the driven sweep does this
	// on the driver before exporting, so the diagnoses travel back in the manifests
	// (see explain.go and llm.go).
	explain         bool   // -explain: triage failed runs after the sweep
	explainCheck    bool   // -explain-check: verify the triage LLM responds, then exit
	explainProvider string // -explain-provider: local, openai, or claude
	explainModel    string // -explain-model: model name passed to the provider
	explainMaxLog   int    // -explain-max-log: head+tail byte cap on the node log
}

func main() {
	cfg, hosts := parseFlags()
	cfg.prog = newRemoteProgram(cfg.binaryPath)

	// -export-csv regenerates the human- and grep-friendly runs.csv/nodes.csv
	// pair from a collected directory's plotdata.binpb and exits. It runs no
	// sweep and needs no hosts, so it is handled before any of the
	// connection-oriented modes.
	if cfg.exportCSVDir != "" {
		if err := exportPlotCSV(cfg.exportCSVDir); err != nil {
			log.Fatalf("export-csv: %v", err)
		}
		log.Printf("exported %s", filepath.Join(cfg.exportCSVDir, plotDataDir))
		return
	}

	// -export-compact rebuilds a finished run's compact transfer from the raw
	// result files still in its work directory and exits. Like -export-csv it
	// runs no sweep and needs no hosts, but it runs where the raw files are —
	// the driver — so the laptop can then download the rebuilt directory.
	if cfg.exportCompactDir != "" {
		summary, err := prepareCompactTransfer(cfg.exportCompactDir, cfg.collectProfiles)
		if err != nil {
			log.Fatalf("export-compact: %v", err)
		}
		logCompactTransfer(cfg.exportCompactDir, summary)
		return
	}

	// -plot regenerates the report from a collected output directory and exits.
	// It runs no sweep and needs no hosts, so it is handled before any of the
	// connection-oriented modes.
	if cfg.plotDir != "" {
		if err := generateReport(cfg.plotDir, reportOptionsFromConfig(cfg)); err != nil {
			log.Fatalf("plot: %v", err)
		}
		return
	}

	// -check exercises the same iago SSH path the sweep uses, so a clean result
	// means the sweep will connect. It connects to each host independently, so
	// unreachable hosts are reported rather than aborting the whole check. With
	// -driver it ships the check to the driver so clock skew is measured against
	// the driver's LAN-local clock (the vantage the benchmark's own ClockSync
	// uses) instead of the laptop's, whose WAN round-trip otherwise dominates the
	// skew estimate and its uncertainty.
	if cfg.check {
		check := func(*config) error {
			return checkHosts(hosts, cfg.sshConfig, cfg.port, cfg.prog, cfg.selfHost, cfg.remoteDir)
		}
		if cfg.driver != "" && !cfg.driven {
			check = func(cfg *config) error { return runDriverCheck(cfg, hosts) }
		}
		if err := check(cfg); err != nil {
			log.Fatalf("check: %v", err)
		}
		return
	}

	// -explain-check verifies the triage LLM answers a trivial prompt and exits,
	// so a misconfigured provider/model/key/endpoint is caught on demand instead
	// of only when a failed run needs triage. With -driver it ships the check to
	// the driver, the only side of the firewall that can reach the UiS Ollama
	// server; otherwise it runs locally against a provider the laptop can reach.
	if cfg.explainCheck {
		check := runExplainCheck
		if cfg.driver != "" && !cfg.driven {
			check = func(cfg *config) error { return runDriverExplainCheck(cfg, hosts) }
		}
		if err := check(cfg); err != nil {
			log.Fatalf("explain-check: %v", err)
		}
		return
	}

	// Reconnect to a detached driver run: download its results and exit.
	if cfg.collect != "" || cfg.collectNow != "" {
		if err := runDriverCollect(cfg); err != nil {
			log.Fatalf("driver collect: %v", err)
		}
		return
	}
	if cfg.list {
		if err := runDriverList(cfg); err != nil {
			log.Fatalf("driver list: %v", err)
		}
		return
	}
	// Launch the orchestration on a cluster-local driver host: build, ship,
	// re-exec sweep there with -driven, stream, and collect. The launcher talks
	// only to the driver; the driven sweep does all the peer SSH itself.
	if cfg.driver != "" && !cfg.driven {
		if err := runDriver(cfg, hosts); err != nil {
			log.Fatalf("driver: %v", err)
		}
		return
	}

	// The module-root requirement, build, replay script, and stale-binary warning
	// are laptop-only steps; the driven sweep on the driver skips them (it runs
	// from a temp work dir with a pre-built binary and a forwarded git SHA).
	if !cfg.driven {
		if err := requireBenchkitModuleRoot(); err != nil {
			log.Fatal(err)
		}
	}

	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		log.Fatalf("creating output directory: %v", err)
	}

	// Mirror orchestration log output to <outdir>/sweep.log so a failed sweep
	// can be diagnosed after the fact; the console alone is gone once the
	// terminal scrolls or closes. Per-node benchmark output does not go here:
	// each run streams it to <outdir>/logs/<base>.log instead (see newRunLogger),
	// keeping sweep.log a readable high-level narrative.
	sweepLogPath := filepath.Join(cfg.outDir, "sweep.log")
	logFile, err := os.Create(sweepLogPath)
	if err != nil {
		log.Fatalf("creating sweep log: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stderr, logFile))

	// The replay script and stale-binary check are laptop-only: on the driver
	// os.Args is the driven command (not the user's), there is no repo HEAD, and
	// the binary was freshly built by the launcher. The launcher writes the
	// replay script locally and forwards the laptop's HEAD via -git-sha.
	gitSHA := cfg.gitSHA
	if !cfg.driven {
		replayScriptPath, err := writeReplayScript(cfg.outDir, os.Args)
		if err != nil {
			log.Fatalf("creating replay script: %v", err)
		}
		log.Printf("replay script: %s", replayScriptPath)

		// Catch a sweep binary built before the latest commits: it silently runs
		// outdated code, so warn before doing any work.
		gitSHA = gitHeadSHA()
		warnIfStaleBinary(gitSHA)
	}

	// Verify the triage LLM before spending the sweep: a broken -explain is far
	// cheaper to fix now than after an hour-long run whose failed runs can no
	// longer be easily re-triaged. This runs where triage will run (the driver
	// for a driven sweep), so it exercises that host's reachability and key.
	if cfg.explain {
		if err := runExplainCheck(cfg); err != nil {
			log.Fatalf("explain preflight failed: %v\nfix the model or endpoint, or rerun without -explain to skip triage", err)
		}
	}

	if line := sweepEstimateLine(cfg.sweep, cfg.duration); line != "" {
		log.Print(line)
	}

	// Build the benchmark binary if a pre-built path was not provided.
	// Precedence: -binary (prebuilt) > -build flag > $BENCHKIT_BUILD > the
	// built-in gorums default.
	if cfg.binaryPath == "" {
		cfg.binaryPath = defaultBinaryPath
		buildCmd := cfg.buildCmd
		if buildCmd == "" {
			buildCmd = os.Getenv("BENCHKIT_BUILD")
		}
		if err := buildBenchmark(cfg.binaryPath, buildCmd); err != nil {
			log.Fatalf("build: %v", err)
		}
	}
	binAbs, err := filepath.Abs(cfg.binaryPath)
	if err != nil {
		log.Fatalf("binary path: %v", err)
	}

	// Limit connections to the physical hosts actually needed for this sweep.
	maxPhysical := 0
	for params := range cfg.sweep.params() {
		maxPhysical = max(maxPhysical, min(params.Nodes, len(hosts)))
	}
	if maxPhysical < len(hosts) {
		hosts = hosts[:maxPhysical]
	}

	log.Printf("connecting to %d host(s)...", len(hosts))
	sshConfigPath, err := resolveSSHConfigPath(cfg.sshConfig)
	if err != nil {
		log.Fatalf("SSH config path: %v", err)
	}
	sshConfig, err := iago.ParseSSHConfig(sshConfigPath)
	if err != nil {
		log.Fatalf("SSH config: %v", err)
	}
	group, err := iago.NewSSHGroup(hosts, cfg.sshConfig)
	if err != nil {
		log.Fatalf("SSH group: %v", err)
	}
	defer group.Close()

	// The peer dial above is the only time the driven sweep needs the laptop's
	// forwarded SSH agent (all later per-run work reuses these connections). A
	// -detach launcher watches for this marker to learn the dial succeeded, so
	// it can keep the laptop connected exactly until the forwarded agent is no
	// longer needed and then report that it is safe to disconnect.
	if cfg.readyMarker != "" {
		if err := os.WriteFile(cfg.readyMarker, []byte("dialed\n"), 0o644); err != nil {
			log.Printf("warning: could not write ready marker %s: %v", cfg.readyMarker, err)
		}
	}

	allHosts, err := resolvePeerHosts(group.Hosts, sshConfig.ConnectAddr)
	if err != nil {
		log.Fatalf("resolve peer hosts: %v", err)
	}
	log.Printf("resolved peer addresses: %s", peerHostSummary(allHosts))
	cfg.remoteDirs = make(map[string]string, len(group.Hosts))
	for _, host := range group.Hosts {
		namespace, err := ensureRemoteNamespace(context.Background(), host, cfg.remoteDir)
		if err != nil {
			log.Fatalf("remote storage: %v", err)
		}
		cfg.remoteDirs[host.Name()] = namespace
	}

	// A lossy link would not fail any run — TCP retransmits through it — but
	// it silently destroys the measurements, so probe every link before
	// spending minutes on the sweep.
	if cfg.netcheck {
		log.Println("checking link health (ping ring)...")
		if err := checkNetworkHealth(group, allHosts); err != nil {
			log.Fatalf("%v", err)
		}
	}

	// Kill any lingering processes and deploy a fresh binary.
	log.Println("killing lingering processes...")
	if err := killLingering(group, cfg.prog); err != nil {
		log.Printf("warning: kill: %v", err)
	}
	if err := upload(group, binAbs, cfg); err != nil {
		log.Fatalf("upload: %v", err)
	}
	log.Println("deployment complete")

	// Execute the sweep.
	total := countRuns(cfg.sweep)
	log.Printf("starting sweep: %d run(s), output → %s", total, displayPath(cfg.outDir))
	// The static estimate was printed before build and SSH setup. Progress below
	// recomputes the same lower/upper bounds from the remaining run count only.
	runNum := 0
	// remoteFiles tracks the result files this sweep creates per host so cleanup
	// removes only what it produced, not every result file on the cluster.
	remoteFiles := make(map[string][]string)
	// collectExts are the per-node artifacts downloaded after each run; profile
	// artifacts join the result file when -collect-profiles is set.
	collectExts := []string{resultExt}
	if cfg.collectProfiles {
		collectExts = append(collectExts, cpuProfExt, memProfExt)
	}
	var failedManifests, degradedManifests []string
	finalizeRun := func(base string, nodes []nodeAssignment, o runOutcome) {
		if err := updateManifestOutcome(cfg.outDir, base, o); err != nil {
			log.Printf("  warning: manifest outcome: %v", err)
		}
		switch o.status {
		case runStatusFailed:
			failedManifests = append(failedManifests, manifestPath(cfg.outDir, base))
		case runStatusDegraded:
			degradedManifests = append(degradedManifests, manifestPath(cfg.outDir, base))
		}
		// Post-incident health probe: the implicated host(s) are re-probed
		// immediately, while the evidence (load, retransmit counters) is
		// still fresh — see health.go.
		if hosts := healthProbeHosts(o, base, nodes); len(hosts) > 0 {
			if path := runHealthProbe(group, allHosts, cfg.prog, cfg.port, hosts, cfg.outDir, base, cfg.remoteDir); path != "" {
				log.Printf("  health probe (%s): %s", strings.Join(hosts, ","), path)
			}
		}
	}
	for params := range cfg.sweep.params() {
		runNum++
		base := runBase(cfg.sweepLabel, params)
		nodes := buildNodeAssignments(allHosts, params.Nodes, cfg.port)
		peers := buildPeerList(nodes)
		writeManifest(cfg.outDir, base, params, nodes, cfg, gitSHA, binAbs)
		for _, n := range nodes {
			for _, ext := range collectExts {
				remoteFiles[n.host] = append(remoteFiles[n.host], filepath.Join(cfg.remoteDirs[n.host], resultFilename(base, n, ext)))
			}
		}

		numHosts := min(params.Nodes, len(allHosts))
		sub := group
		sub.Hosts = group.Hosts[:numHosts]

		// Progress goes through log so each run is timestamped (stalls are then
		// visible in the log) and recorded in sweep.log.
		log.Printf("[%d/%d] %-8s  N=%-4d  workers=%-4d  payload=%-6d  rate=%-8d  stream=%-5s  bench=%s",
			runNum, total, cfg.sweepLabel,
			params.Nodes, params.Workers, params.Payload, params.Rate, params.StreamMode, params.Benchmark)
		// Refresh the completion range using only the static per-run bounds.
		if runNum > 1 {
			log.Print(sweepProgressLine(time.Now(), cfg.duration, runNum-1, total))
		}

		if err := killLingering(sub, cfg.prog); err != nil {
			log.Printf("  warning: kill: %v", err)
		}
		// A node that cannot bind its port stalls the whole run for the full
		// readiness deadline; skip the run immediately instead.
		if err := checkPortsFree(sub, nodes); err != nil {
			log.Printf("  error: %v — skipping run", err)
			collectFailureDiag(sub, nodes, cfg.prog, cfg.outDir, base)
			collected, missing := countResultFiles(cfg.outDir, base, nodes)
			finalizeRun(base, nodes, runOutcome{
				status: runStatusFailed, err: err, failurePhase: failurePhaseSetup,
				collectedFiles: collected, missingFiles: missing,
			})
			continue
		}
		// TCP counters are snapshotted around the run so the manifest records
		// each host's retransmission/timeout deltas — the evidence that points
		// at a lossy link when a run comes out degraded (see tcpstats.go).
		tcpBefore := captureTCPStats(sub)
		if err := launchAndWait(sub, nodes, peers, params, base, cfg); err != nil {
			log.Printf("  error: %v", err)
			// Snapshot host and socket state before collecting results: the
			// SSH round-trips for collection take seconds, during which
			// TIME_WAIT sockets and lingering processes decay.
			collectFailureDiag(sub, nodes, cfg.prog, cfg.outDir, base)
			tcpStats := diffTCPStats(tcpBefore, captureTCPStats(sub))
			if collectErr := collectRunArtifacts(sub, base, nodes, cfg, collectExts); collectErr != nil {
				log.Printf("  warning: partial result collection: %v", collectErr)
			}
			// Zero result files means the run never reached measurement (a
			// launch or AwaitReady failure); a partial set means nodes ran but
			// some failed mid-benchmark.
			collected, missing := countResultFiles(cfg.outDir, base, nodes)
			phase := failurePhaseSetup
			if collected > 0 {
				phase = failurePhaseMeasurement
			}
			finalizeRun(base, nodes, runOutcome{
				status: runStatusFailed, err: err, failurePhase: phase,
				collectedFiles: collected, missingFiles: missing, tcpStats: tcpStats,
			})
			continue
		}
		tcpStats := diffTCPStats(tcpBefore, captureTCPStats(sub))
		if err := collectRunArtifacts(sub, base, nodes, cfg, collectExts); err != nil {
			log.Printf("  error collecting results: %v", err)
			collected, missing := countResultFiles(cfg.outDir, base, nodes)
			finalizeRun(base, nodes, runOutcome{
				status: runStatusFailed, err: err, failurePhase: failurePhaseCollection,
				collectedFiles: collected, missingFiles: missing, tcpStats: tcpStats,
			})
			continue
		}
		collected, missing := countResultFiles(cfg.outDir, base, nodes)
		outcome := runOutcome{
			status: runStatusSucceeded, collectedFiles: collected, missingFiles: missing,
			tcpStats: tcpStats,
		}
		// A run whose nodes all completed can still be worthless when one node's
		// measurement does not belong with its peers' — it ran far slower (a
		// lossy network link), reported far more work than a symmetric benchmark
		// allows, or "completed" calls faster than the network permits. Flag it
		// so the contaminated aggregate is not silently mixed into the results.
		measurements := collectNodeMeasurements(cfg.outDir, base, nodes, cfg.trim)
		if deg := findDegradedNodes(measurements, cfg.degradedBounds()); len(deg) > 0 {
			outcome.status = runStatusDegraded
			outcome.degraded = deg
			for _, d := range deg {
				log.Printf("  warning: degraded node %v%s", d, degradedTCPSummary(d.Host, tcpStats))
			}
		}
		finalizeRun(base, nodes, outcome)
	}

	// Remove the binary and result files from remote hosts.
	log.Println("cleaning up remote hosts...")
	if err := cleanup(group, cfg, remoteFiles); err != nil {
		log.Printf("warning: cleanup: %v", err)
	}

	if cfg.pgo {
		if err := mergeCPUProfiles(cfg.outDir); err != nil {
			log.Printf("warning: pgo merge: %v", err)
		} else {
			log.Printf("PGO profile written to %s", displayPath(filepath.Join(cfg.outDir, "default.pgo")))
		}
	}

	// Triage failed runs with the LLM before exporting, so the diagnoses land in
	// the manifests that the driven sweep packs into compact-transfer (and that a
	// local sweep leaves in place). Best-effort: never fail the sweep over a triage error.
	if cfg.explain && len(failedManifests) > 0 {
		triageFailedRuns(cfg)
	}

	if cfg.driven {
		summary, err := prepareCompactTransfer(cfg.outDir, cfg.collectProfiles)
		if err != nil {
			log.Fatalf("compact plot data: %v", err)
		}
		logCompactTransfer(cfg.outDir, summary)
	}

	log.Printf("sweep complete — results in %s", displayPath(cfg.outDir))
	log.Printf("sweep log: %s", displayPath(sweepLogPath))
	log.Printf("per-run node logs: %s", displayPath(filepath.Join(cfg.outDir, logSubdir)))
	log.Printf("run manifests: %s", displayPath(filepath.Join(cfg.outDir, "*"+manifestSuffix)))
	if len(failedManifests) > 0 {
		log.Printf("failed run manifests:")
		for _, path := range failedManifests {
			printFailedRunArtifacts(cfg.outDir, path)
		}
	}
	if len(degradedManifests) > 0 {
		log.Printf("degraded run manifests (a node's measurement did not belong with its peers'; see each manifest's degraded_nodes reason):")
		for _, path := range degradedManifests {
			printFailedRunArtifacts(cfg.outDir, path)
		}
	}
	// A driven sweep leaves report generation to the laptop that collects it;
	// a local sweep has its results in place, so build the report now.
	if !cfg.driven {
		autoReport(cfg)
	}
	if cfg.driven {
		if err := copyIfExists(sweepLogPath, filepath.Join(cfg.outDir, compactTransferDir, "sweep.log")); err != nil {
			log.Printf("warning: refresh compact sweep log: %v", err)
		}
	}
}

func collectRunArtifacts(g iago.Group, base string, nodes []nodeAssignment, cfg *config, collectExts []string) error {
	err := collectResults(g, base, nodes, cfg, collectExts)
	// Bridge collected binary files to protojson for local sweeps. Driven sweeps
	// keep the remote output compact by reducing successful runs to plot CSVs;
	// if the raw archive is collected later, driver.go regenerates protojson on
	// the laptop from the downloaded binpb files.
	if !cfg.driven {
		convertBinaryResults(cfg.outDir, base, nodes)
	}
	printRunSummary(cfg.outDir, base, nodes, cfg.trim)
	return err
}

func parseFlags() (*config, []string) {
	// Seed the sweep ranges with their defaults; the comma-separated list flags
	// below replace a range wholesale when the corresponding flag is given.
	cfg := &config{
		sweep: sweepConfig{
			numNodes:    []int{9},
			workers:     []int{1},
			payloads:    []int{0},
			rates:       []int{0},
			benchmarks:  []string{"SymmetricQuorumCall"},
			streamModes: []string{"dual"},
			reps:        1,
		},
	}
	flag.StringVar(&cfg.binaryPath, "binary", "", "pre-built linux/amd64 binary path (required for a foreign protocol; default: auto-build)")
	flag.StringVar(&cfg.buildCmd, "build", "", "build command for the auto-build path; {{output}} is replaced with the output path (default: go build ./cmd/benchmark; overrides $BENCHKIT_BUILD)")
	flag.StringVar(&cfg.sshConfig, "config", "", "SSH config file (default: ~/.ssh/config)")
	flag.StringVar(&cfg.rootDir, "outdir", defaultOutRoot, "root output directory for sweep runs (default: out)")
	flag.IntVar(&cfg.port, "port", 9000, "base port for benchmark nodes")
	flag.DurationVar(&cfg.duration, "duration", 10*time.Second, "measurement duration per run")
	flag.DurationVar(&cfg.trim, "trim", 0, "drop interval samples before this offset when summarizing (0 = no trim)")
	flag.Float64Var(&cfg.degradedBelow, "degraded-below", 0.5, "flag a run as degraded when a node's throughput falls below this fraction of the run median (0 disables)")
	flag.Float64Var(&cfg.degradedAbove, "degraded-above", 2, "flag a run as degraded when a node's throughput exceeds this multiple of the run median, which a symmetric benchmark cannot produce (0 disables)")
	flag.Float64Var(&cfg.degradedLatencyBelow, "degraded-latency-below", 0.2, "flag a run as degraded when a node's median latency falls below this fraction of the run median, too fast for the network round trip the benchmark measures (0 disables)")
	flag.BoolVar(&cfg.netcheck, "netcheck", true, "probe every host link with a ping ring before the sweep and abort on heavy packet loss")
	flag.IntVar(&cfg.fdLimit, "fd-limit", 65536, "raise the soft open-file limit (ulimit -Sn) for launched benchmark nodes and, in driven mode, the driver-side sweep; 0 uses the host default")
	flag.StringVar(&cfg.sweepLabel, "sweep", "run", "label prefix for output filenames and, when set explicitly, the run directory name")
	flag.BoolVar(&cfg.verbose, "verbose", false, "pass -verbose to benchmark nodes")
	flag.BoolVar(&cfg.check, "check", false, "run connectivity and host diagnostics on matched hosts, then exit")

	// Pass-through flags forwarded to every benchmark node (not swept).
	flag.StringVar(&cfg.interval, "interval", "", "pass -interval to benchmark nodes (e.g. 250ms, 0 disables events; empty = binary default)")
	flag.StringVar(&cfg.statsMode, "stats-mode", "", "pass -stats-mode to benchmark nodes (exact or hdr; empty = binary default)")
	flag.IntVar(&cfg.rateStep, "rate-step", 0, "pass -rate-step to benchmark nodes (ops/s per ramp step; 0 = no ramp)")
	flag.IntVar(&cfg.rateStepMax, "rate-step-max", 0, "pass -rate-step-max to benchmark nodes (ceiling ops/s; 0 = no ramp)")
	flag.StringVar(&cfg.extraArgs, "extra-args", "", "extra flags appended verbatim to every benchmark node command")
	flag.BoolVar(&cfg.collectProfiles, "collect-profiles", false, "pass -cpuprofile/-memprofile to every node and download the profiles alongside the result files")
	flag.BoolVar(&cfg.pgo, "pgo", false, "merge the collected CPU profiles into <outdir>/default.pgo for profile-guided optimization (implies -collect-profiles)")

	// Cluster-local driver flags.
	flag.StringVar(&cfg.driver, "driver", "", "run the sweep orchestration on this cluster-local SSH host ('first' = first -hosts entry); the binary crosses the WAN once and all per-run SSH stays on the cluster LAN")
	flag.Var(optionalPathFlag{&cfg.collect}, "collect", "collect a finished driver run; optional path selects a run, otherwise use the latest run from <outdir>/.sweep-last.json")
	flag.Var(optionalPathFlag{&cfg.collectNow}, "collect-now", "collect a best-effort snapshot now; optional path selects a run, otherwise use the latest run")
	flag.BoolVar(&cfg.list, "list", false, "list active, completed, raw-pending, and recoverable driver runs")
	flag.StringVar(&cfg.remoteDir, "remote-dir", "/tmp", "remote storage root; sweep creates and uses <root>/sweep-$USER")
	flag.BoolVar(&cfg.detach, "detach", false, "with -driver, start the run and exit immediately without streaming or waiting; reconnect later with -driver <host> -collect <remote-work-dir>")
	flag.StringVar(&cfg.transferMode, "transfer", "rsync", "file transfer backend for -driver uploads and downloads: rsync >=3.2.4 (default) or sftp")
	flag.BoolVar(&cfg.driven, "driven", false, "internal: set automatically on the cluster-local driver; relaxes laptop-only steps (repo root, build, replay script)")
	flag.StringVar(&cfg.gitSHA, "git-sha", "", "internal: repository HEAD forwarded by -driver so the driven sweep records it in manifests")
	flag.StringVar(&cfg.readyMarker, "ready-marker", "", "internal: path the driven sweep touches once its peers are dialed, so a -detach launcher knows the forwarded agent is no longer needed")
	flag.StringVar(&cfg.selfHost, "self-host", "", "internal: set by a driver-routed -check to the driver's own alias, which is probed locally instead of over SSH (a host cannot SSH to itself)")

	// LLM failure-triage flags. -explain triages the sweep's failed runs after it
	// completes; with -driver the driven sweep does it on the driver. The rest
	// configure the model backend. API keys come from the environment, never flags.
	flag.BoolVar(&cfg.explain, "explain", false, "after the sweep, triage failed runs with an LLM (runs on the driver when -driver is set)")
	flag.BoolVar(&cfg.explainCheck, "explain-check", false, "verify the -explain LLM answers a trivial prompt, then exit")
	flag.StringVar(&cfg.explainProvider, "explain-provider", providerLocal, "LLM backend for -explain: local (default), openai, or claude")
	flag.StringVar(&cfg.explainModel, "explain-model", "", "model name for -explain (e.g. llama3.3, gpt-4o, claude-opus-4-8); required with -explain")
	flag.IntVar(&cfg.explainMaxLog, "explain-max-log", defaultExplainMaxLog, "head+tail byte cap on the node log included in the -explain prompt")

	flag.StringVar(&cfg.plotDir, "plot", "", "regenerate the Typst report from this collected output directory, then exit (no sweep)")
	flag.BoolVar(&cfg.includeDegraded, "include-degraded", false, "with -plot, keep degraded repetitions in the aggregate figures")
	flag.Var(stringListFlag(&cfg.excludeRuns), "exclude-run", "with -plot, comma-separated run base names to drop from the report")
	flag.Var(stringListFlag(&cfg.excludeDims), "exclude", "with -plot, comma-separated DIM=VALUE tokens to drop (DIM: benchmark, nodes, workers, payload, rate, send_buffer, recv_buffer, stream_mode)")
	flag.StringVar(&cfg.exportCSVDir, "export-csv", "", "regenerate plotdata/runs.csv and plotdata/nodes.csv from this collected directory's plotdata.binpb, then exit (no sweep, no report)")
	flag.StringVar(&cfg.exportCompactDir, "export-compact", "", "rebuild plotdata/ and compact-transfer/ from this sweep work directory's raw result files, then exit (run on the driver; -collect-profiles includes the profiles)")
	flag.Var(intListFlag(&cfg.sweep.numNodes), "n", "comma-separated node counts to sweep")
	flag.Var(intListFlag(&cfg.sweep.workers), "workers", "comma-separated worker counts to sweep")
	flag.Var(intListFlag(&cfg.sweep.payloads), "payload", "comma-separated payload sizes in bytes to sweep")
	flag.Var(intListFlag(&cfg.sweep.rates), "rate", "comma-separated target sends/sec per node to sweep; 0 = unlimited (saturating)")
	flag.Var(intListFlag(&cfg.sweep.sendBuffers), "send-buffer", "comma-separated per-node send queue capacities to sweep (default: the binary's own)")
	flag.Var(intListFlag(&cfg.sweep.recvBuffers), "recv-buffer", "comma-separated server receive queue capacities to sweep (default: the binary's own)")
	flag.Var(stringListFlag(&cfg.sweep.benchmarks), "benchmarks", "comma-separated benchmark names to run")
	flag.Var(stringListFlag(&cfg.sweep.streamModes), "stream-mode", "comma-separated stream modes to sweep: dual,dedup; or baseline alone (requires -binary)")
	flag.IntVar(&cfg.sweep.reps, "reps", 1, "repetitions per parameter combination")

	var (
		rawHosts string
		testN    int
	)
	flag.StringVar(&rawHosts, "hosts", "", "SSH host aliases: PREFIX[lo-hi]SUFFIX ranges (e.g. 'bb[1-30]'), glob patterns (e.g. 'bb*'), or comma-separated literals")
	flag.IntVar(&testN, "test", 0, "quick smoke test with N nodes (0 = full sweep)")
	os.Args = normalizeOptionalPathArgs(os.Args)
	flag.Parse()
	if flag.NArg() > 0 {
		log.Fatalf("unexpected positional argument(s): %v", flag.Args())
	}

	var sweepExplicit, benchmarksExplicit, transferExplicit bool
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "sweep":
			sweepExplicit = true
		case "benchmarks":
			benchmarksExplicit = true
		case "transfer":
			transferExplicit = true
		}
	})

	// -collect only reconnects to a detached run on the driver, -plot and
	// -export-csv only read an existing output directory, and a local
	// -explain-check talks only to the LLM, so none needs host selection. A
	// driver-routed -explain-check still needs a host (to resolve the driver),
	// which resolveDriverHost reports if it is missing.
	collecting := cfg.collect != "" || cfg.collectNow != ""
	if cfg.collect != "" && cfg.collectNow != "" {
		log.Fatal("only one of -collect and -collect-now may be passed")
	}
	if rawHosts == "" && !collecting && !cfg.list && cfg.plotDir == "" && cfg.exportCSVDir == "" && !cfg.explainCheck {
		log.Fatal("-hosts is required; use 'bb[1-30]' for range expansion or 'bb*' to match SSH config entries")
	}
	if cfg.remoteDir == "" || !filepath.IsAbs(cfg.remoteDir) || filepath.Clean(cfg.remoteDir) == "/" {
		log.Fatal("-remote-dir must be an absolute path other than /")
	}
	if cfg.detach && cfg.driver == "" {
		log.Fatal("-detach requires -driver <host>")
	}
	if cfg.detach && (collecting || cfg.list) {
		log.Fatal("-detach cannot be combined with collection or listing")
	}
	// Fail on the laptop before running an entire sweep if -explain is
	// misconfigured. The key check runs here, not just at triage time, so a
	// driven sweep aborts before shipping anything when the laptop lacks the key
	// it would forward to the driver.
	if cfg.explain || cfg.explainCheck {
		if cfg.explainModel == "" {
			log.Fatal("-explain/-explain-check requires -explain-model (e.g. -explain-model llama3.3)")
		}
		if key := providerKeyEnv(cfg.explainProvider); os.Getenv(key) == "" {
			log.Fatalf("-explain/-explain-check requires the %s environment variable to be set", key)
		}
	}
	if cfg.pgo {
		cfg.collectProfiles = true
	}
	if err := validateStreamModes(cfg.sweep.streamModes, cfg.binaryPath); err != nil {
		log.Fatalf("-stream-mode: %v", err)
	}

	if testN > 0 {
		cfg.sweep.numNodes = []int{testN}
		cfg.sweep.workers = []int{1}
		cfg.sweep.payloads = []int{0}
		if !benchmarksExplicit {
			cfg.sweep.benchmarks = []string{"SymmetricQuorumCall"}
		}
		cfg.duration = 5 * time.Second
		cfg.sweepLabel = "test"
		sweepExplicit = true
	}

	if cfg.collect == latestRunSentinel || cfg.collectNow == latestRunSentinel || (cfg.list && cfg.driver == "") {
		state, err := readLastRunState(cfg.rootDir)
		if err != nil {
			log.Fatal(err)
		}
		if cfg.driver == "" {
			cfg.driver = state.Driver
		}
		if cfg.collect == latestRunSentinel {
			cfg.collect = state.RemoteWorkDir
		}
		if cfg.collectNow == latestRunSentinel {
			cfg.collectNow = state.RemoteWorkDir
		}
		cfg.outDir = state.LocalRunDir
		if cfg.sshConfig == "" {
			cfg.sshConfig = state.SSHConfig
		}
		if !transferExplicit && state.TransferMode != "" {
			cfg.transferMode = state.TransferMode
		}
	}
	if collecting && cfg.driver == "" {
		log.Fatal("an explicit collection path requires -driver <host>; omit the path to use the latest run")
	}

	now := time.Now()
	if cfg.outDir == "" {
		collectPath := cfg.collect
		if collectPath == "" {
			collectPath = cfg.collectNow
		}
		cfg.outDir = resolveOutputDir(cfg.rootDir, now, cfg.sweepLabel, sweepExplicit, collectPath)
	}
	readOnlyMode := cfg.check || cfg.list || cfg.explainCheck || cfg.plotDir != "" || cfg.exportCSVDir != ""
	if collecting {
		if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
			log.Fatalf("output directory: %v", err)
		}
	} else if !readOnlyMode {
		if err := prepareOutputDir(cfg.outDir); err != nil {
			log.Fatalf("output directory: %v", err)
		}
	} else if err := os.MkdirAll(cfg.rootDir, 0o755); err != nil && cfg.list {
		log.Fatalf("output directory: %v", err)
	}

	if rawHosts == "" {
		// Collection and listing need no benchmark hosts.
		return cfg, nil
	}
	hosts, err := iago.ParseHosts(rawHosts, cfg.sshConfig)
	if err != nil {
		log.Fatalf("invalid -hosts: %v", err)
	}
	if len(hosts) == 0 {
		log.Fatalf("-hosts %q matched no hosts", rawHosts)
	}
	return cfg, hosts
}

func countRuns(sc sweepConfig) int {
	streamModes := sc.streamModes
	if len(streamModes) == 0 {
		streamModes = []string{"dual"}
	}
	return max(sc.reps, 1) * len(sc.numNodes) * len(sc.workers) * len(sc.payloads) *
		len(sc.rates) * len(bufferValues(sc.sendBuffers)) * len(bufferValues(sc.recvBuffers)) *
		len(sc.benchmarks) * len(streamModes)
}

// validateStreamModes checks the swept stream modes. The baseline mode runs a
// prebuilt binary from before stream modes existed, so it requires -binary and
// cannot be mixed with dual or dedup in the same invocation, since one sweep
// deploys exactly one benchmark binary.
func validateStreamModes(modes []string, binaryPath string) error {
	if len(modes) == 0 {
		return nil
	}
	for _, mode := range modes {
		switch mode {
		case "dual", "dedup", "baseline":
		default:
			return fmt.Errorf("invalid %q (want: dual, dedup, or baseline)", mode)
		}
	}
	if slices.Contains(modes, "baseline") {
		if len(modes) != 1 {
			return fmt.Errorf("baseline cannot be mixed with other modes; run it as a separate sweep")
		}
		if binaryPath == "" {
			return fmt.Errorf("baseline requires -binary with a prebuilt benchmark binary")
		}
	}
	return nil
}
