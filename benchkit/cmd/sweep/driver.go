package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/relab/iago"
)

// The cluster-local driver feature lets the sweep orchestration run on a host
// inside the cluster instead of on a distant laptop. Without it, every per-run
// SSH round-trip and the one-time binary upload to each host cross the WAN from
// the laptop; driven from a cluster-local host, that traffic stays on the LAN
// and the benchmark binary crosses the WAN only once (laptop -> driver).
//
// Every operation that ships a binary or the generated SSH config to the driver
// — a full -driver run, a driver-routed -check, or the explain check — goes
// through the shared cache in driverCacheDir (see cachedUpload), so a laptop
// with nothing changed since the last upload pays no WAN transfer at all, no
// matter which of those entry points it uses.
//
// Two processes cooperate:
//
//   - The launcher (this file, run on the laptop with -driver <host>) cross-
//     builds sweep and the benchmark binary for linux/amd64, uploads whichever
//     of them (plus the generated SSH config) changed since the last upload to
//     the driver's binary cache over a single iago SSH connection, and re-execs
//     sweep on the driver with -driven. It then streams the remote sweep's
//     output and, on clean completion, downloads a compact plot-data export
//     plus any failed-run result files. Raw successful result files stay on the
//     driver until an explicit -collect archives them.
//   - The driven sweep (-driven, run on the driver) is an ordinary sweep with a
//     few laptop-only steps relaxed (no module-root requirement, no build, no
//     replay script, no stale-binary warning); it does all the peer SSH itself.
//
// Authentication to the peers uses the laptop's SSH agent, forwarded to the
// driver when ForwardAgent is set in the SSH config for the driver alias. iago
// authenticates via SSH_AUTH_SOCK, so the forwarded agent is used transparently.
// iago dials the driver once at startup and reuses that connection for all
// upload, exec, and download operations, so the agent is only needed for the
// first few seconds: once the driver has dialed its peers, a laptop disconnect
// no longer affects the run, which is why the remote sweep is detached (setsid)
// and survives the control connection dropping.

// resolveDriver resolves the -driver flag to a concrete host alias and computes
// the benchmark host pool. The driver host is always excluded from the
// benchmark pool when it appears in hosts, so the orchestrator does not perturb
// a co-located replica's measurements. A driver host outside hosts (a dedicated
// head node) leaves the pool unchanged. The sentinel "first" selects hosts[0].
func resolveDriver(driverFlag string, hosts []string) (driver string, benchHosts []string, err error) {
	if driverFlag == "" {
		return "", hosts, nil
	}
	driver, err = resolveDriverHost(driverFlag, hosts)
	if err != nil {
		return "", nil, err
	}
	benchHosts = make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h != driver {
			benchHosts = append(benchHosts, h)
		}
	}
	if len(benchHosts) == 0 {
		return "", nil, fmt.Errorf("no benchmark hosts left after excluding driver %q", driver)
	}
	return driver, benchHosts, nil
}

// resolveDriverHost resolves the -driver flag to a concrete host alias. The
// sentinel "first" selects hosts[0]; any other value is the alias itself. Unlike
// resolveDriver it computes no benchmark pool, so it serves callers (such as the
// driver-routed explain check) that need only the driver host.
func resolveDriverHost(driverFlag string, hosts []string) (string, error) {
	if driverFlag == "first" {
		if len(hosts) == 0 {
			return "", errors.New("-driver first: no hosts to choose from")
		}
		return hosts[0], nil
	}
	return driverFlag, nil
}

// maxNodeCount returns the largest node count across the sweep, used to warn
// when excluding the driver leaves too few hosts for one-node-per-host runs.
func maxNodeCount(sc sweepConfig) int {
	if len(sc.numNodes) == 0 {
		return 0
	}
	return slices.Max(sc.numNodes)
}

// driverCacheDir is the persistent driver-side directory that holds the sweep
// and benchmark binaries and the generated SSH config, shared across every
// -driver entry point (a full run, a driver-routed -check, the explain check).
// Caching them here instead of inside each run's fresh, timestamped work
// directory means a laptop with nothing changed since the last upload pays no
// WAN transfer at all, regardless of which entry point it uses; see
// cachedUpload. It persists across runs until the driver reboots (or /tmp is
// cleared) and is never touched by a run's own cleanup, which only removes its
// own work directory.
const driverCacheName = "cache"

// cachedUpload uploads localPath into the driver's binary cache under name,
// skipping the transfer when the driver already holds a file with the same
// content. Content identity is tracked by a SHA-256 marker file
// (cacheDir/.<name>.sha256) written after every successful upload, rather than
// by mtime or git state: a freshly built binary gets a new mtime on every build
// even when its bytes are unchanged (rsync's default quick check would then
// re-transfer it), and a hash needs no assumption that the working tree matches
// some git commit — so it is correct uniformly for a locally built binary, a
// user-supplied -binary override, and the generated SSH config alike. Returns
// the artifact's absolute path on the driver (cacheDir/name).
func cachedUpload(ctx context.Context, cfg *config, driver string, host iago.Host, cacheDir, localPath, name string, perm iago.Perm) (string, error) {
	hash, err := fileSHA256(localPath)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", name, err)
	}
	remotePath := cacheDir + "/" + name
	markerPath := cacheDir + "/." + name + ".sha256"
	cached, _ := iago.Output(ctx, host, "cat "+iago.Quote(markerPath)+" 2>/dev/null || true")
	if strings.TrimSpace(cached) == hash {
		log.Printf("%s unchanged on %s; skipping upload", name, driver)
		return remotePath, nil
	}
	log.Printf("uploading %s to %s via %s...", name, driver, cfg.transferMode)
	if err := uploadDriverFile(ctx, cfg, driver, host, localPath, remotePath, perm); err != nil {
		return "", fmt.Errorf("upload %s: %w", name, err)
	}
	if err := driverExec(ctx, host, "printf '%s' "+iago.Quote(hash)+" > "+iago.Quote(markerPath)); err != nil {
		log.Printf("warning: record cache marker for %s on %s: %v", name, driver, err)
	}
	return remotePath, nil
}

// fileSHA256 returns the hex-encoded SHA-256 digest of the file at path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// runDriver is the laptop-side launcher: build, ship, re-exec on the driver,
// stream, and collect. It connects only to the driver host (over iago using the
// user's SSH config), never to the peers.
func runDriver(cfg *config, hosts []string) error {
	driver, benchHosts, err := resolveDriver(cfg.driver, hosts)
	if err != nil {
		return err
	}
	if maxN := maxNodeCount(cfg.sweep); len(benchHosts) < maxN {
		log.Printf("warning: %d benchmark host(s) after excluding driver %s, but largest -n is %d; "+
			"nodes will be packed onto fewer hosts", len(benchHosts), driver, maxN)
	}
	if err := requireBenchkitModuleRoot(); err != nil {
		return err
	}
	sha := gitHeadSHA()
	if line := sweepEstimateLine(cfg.sweep, cfg.duration); line != "" {
		log.Print(line)
	}

	// Stage the binaries and SSH config in a local temp dir; they are uploaded
	// to the driver and not needed afterward.
	stage, err := os.MkdirTemp("", "sweep-driver-")
	if err != nil {
		return fmt.Errorf("staging dir: %w", err)
	}
	defer os.RemoveAll(stage)

	benchLocal := cfg.binaryPath
	if benchLocal == "" {
		benchLocal = filepath.Join(stage, "benchmark")
		buildCmd := cfg.buildCmd
		if buildCmd == "" {
			buildCmd = os.Getenv("BENCHKIT_BUILD")
		}
		if err := buildBenchmark(benchLocal, buildCmd); err != nil {
			return fmt.Errorf("build benchmark: %w", err)
		}
	}
	sweepLocal := filepath.Join(stage, "sweep")
	if err := buildSweepBinary(sweepLocal); err != nil {
		return fmt.Errorf("build sweep: %w", err)
	}
	cfgLocal := filepath.Join(stage, "ssh.config")
	if err := os.WriteFile(cfgLocal, []byte(generatedSSHConfig()), 0o644); err != nil {
		return fmt.Errorf("write ssh config: %w", err)
	}

	// Connect to the driver host once and reuse the connection for all operations.
	driverGroup, err := dialDriverGroup(driver, cfg.sshConfig)
	if err != nil {
		return fmt.Errorf("connect to driver: %w", err)
	}
	defer driverGroup.Close()
	host := driverGroup.Hosts[0]
	ctx := context.Background()
	namespace, err := ensureRemoteNamespace(ctx, host, cfg.remoteDir)
	if err != nil {
		return err
	}
	cacheDir := namespace + "/" + driverCacheName
	base := filepath.Base(cfg.outDir) + "-" + time.Now().Format("20060102_150405")
	wd := namespace + "/sweep-driver-" + base
	log.Printf("driver: %s:%s", driver, wd)
	log.Printf("benchmark hosts (%d): %s", len(benchHosts), strings.Join(benchHosts, ","))

	if err := driverExec(ctx, host, "mkdir -p "+iago.Quote(wd)+" "+iago.Quote(cacheDir)); err != nil {
		return fmt.Errorf("create remote work dir: %w", err)
	}
	localRunDir, err := filepath.Abs(cfg.outDir)
	if err != nil {
		return fmt.Errorf("resolve local run dir: %w", err)
	}
	state := lastRunState{
		Driver: driver, RemoteWorkDir: wd, RemoteNamespace: namespace,
		Label: cfg.sweepLabel, LaunchedAt: time.Now(), LocalRunDir: localRunDir,
		SSHConfig: cfg.sshConfig, TransferMode: cfg.transferMode, Collection: "pending",
	}
	if err := writeLastRunState(cfg.rootDir, state); err != nil {
		return fmt.Errorf("record latest driver run: %w", err)
	}
	remoteMeta, err := json.Marshal(struct {
		Label      string    `json:"label"`
		LaunchedAt time.Time `json:"launched_at"`
	}{Label: state.Label, LaunchedAt: state.LaunchedAt})
	if err != nil {
		return fmt.Errorf("encode remote run metadata: %w", err)
	}
	if err := driverExec(ctx, host, "printf '%s\\n' "+iago.Quote(string(remoteMeta))+" > "+iago.Quote(wd+"/run.meta.json")); err != nil {
		return fmt.Errorf("record remote run metadata: %w", err)
	}
	if _, err := writeReplayScript(cfg.outDir, os.Args); err != nil {
		return fmt.Errorf("write replay script: %w", err)
	}
	if _, err := writeCollectScript(cfg.outDir, state); err != nil {
		return fmt.Errorf("write collect script: %w", err)
	}
	for _, u := range []struct {
		local string
		name  string
		perm  iago.Perm
	}{
		{sweepLocal, "sweep", iago.NewPerm(0o755)},
		{benchLocal, "benchmark", iago.NewPerm(0o755)},
		{cfgLocal, "ssh.config", iago.NewPerm(0o644)},
	} {
		if _, err := cachedUpload(ctx, cfg, driver, host, cacheDir, u.local, u.name, u.perm); err != nil {
			return err
		}
	}

	sweepCmd := remoteSweepCommand(cfg, wd, cacheDir, strings.Join(benchHosts, ","), sha)
	llmEnv := driverLLMEnv(cfg)

	if cfg.detach {
		log.Printf("starting remote sweep on %s (detached)", driver)
		runErr := iago.Shell{
			Command: "bash -s",
			Stdin:   strings.NewReader(detachBootstrapScript(wd, sweepCmd, llmEnv, cfg.fdLimit)),
			Stdout:  os.Stderr,
			Stderr:  os.Stderr,
		}.Apply(ctx, host)
		if runErr != nil {
			return fmt.Errorf("start detached run on %s: %w", driver, runErr)
		}
		log.Printf("[driver] waiting for the run to dial its peers (up to %s) before declaring it safe to disconnect...", detachReadyWindow)
		if err := awaitDetachedStartup(ctx, host, driver, wd); err != nil {
			return err
		}
		log.Printf("detached sweep started on %s:%s; laptop is free to disconnect", driver, wd)
		log.Printf("collect results with:")
		log.Printf("  ./cmd/sweep/sweep -collect -outdir %s", cfg.rootDir)
		log.Printf("collect a snapshot before completion with:")
		log.Printf("  ./cmd/sweep/sweep -collect-now -outdir %s", cfg.rootDir)
		return nil
	}

	log.Printf("starting remote sweep on %s (output streamed below)", driver)
	log.Printf("if the connection drops, the run continues; reconnect and collect with:")
	log.Printf("  ./cmd/sweep/sweep -driver %s -collect %s -outdir %s", driver, wd, cfg.rootDir)
	runErr := iago.Shell{
		Command: "bash -s",
		Stdin:   strings.NewReader(bootstrapScript(wd, sweepCmd, llmEnv, cfg.fdLimit)),
		Stdout:  os.Stderr,
		Stderr:  os.Stderr,
	}.Apply(ctx, host)
	if !finishedRemotely(runErr) {
		return fmt.Errorf("connection to driver ended before the sweep finished: %w\n"+
			"the run may still be in progress on %s\n"+
			"reconnect with: ./cmd/sweep/sweep -driver %s -collect %s -outdir %s",
			runErr, driver, driver, wd, cfg.rootDir)
	}
	return collectDriverResults(cfg, host, wd)
}

// runDriverCollect checks a driver run once and downloads it only when finished,
// unless -collect-now requested a best-effort active snapshot. It needs no SSH
// agent because the driver has long since dialed its peers.
func runDriverCollect(cfg *config) error {
	if cfg.driver == "" {
		return errors.New("-collect requires -driver <host>")
	}
	driverGroup, err := dialDriverGroup(cfg.driver, cfg.sshConfig)
	if err != nil {
		return fmt.Errorf("connect to driver: %w", err)
	}
	defer driverGroup.Close()
	host := driverGroup.Hosts[0]

	driver, wd := cfg.driver, cfg.collect
	collectNow := false
	if cfg.collectNow != "" {
		wd = cfg.collectNow
		collectNow = true
	}
	finished, err := iago.FileExists(context.Background(), host, wd+"/exit.code")
	if err != nil {
		return fmt.Errorf("check run status: %w", err)
	}
	if !finished && !collectNow {
		// A missing exit.code means the run is still active only if its
		// directory still exists at all; it also reads this way once the
		// directory has been archived (collectDriverFullResults removes it
		// with rm -rf) or if the path was mistyped. Consult the saved run
		// state, which records exactly this outcome, so the message reflects
		// what actually happened instead of always guessing "still active".
		if state, err := readLastRunState(cfg.rootDir); err == nil && state.RemoteWorkDir == wd && state.Collection != "" && state.Collection != "pending" {
			return fmt.Errorf("run %s:%s was already collected (%s); nothing left to collect", driver, wd, state.Collection)
		}
		if exists, err := iago.DirExists(context.Background(), host, wd); err == nil && !exists {
			return fmt.Errorf("run directory %s:%s not found; it may have already been archived, or the path is incorrect", driver, wd)
		}
		return fmt.Errorf("run %s:%s is still active; retry -collect after it finishes or use -collect-now for a snapshot", driver, wd)
	}
	if !finished {
		log.Printf("collecting an in-progress snapshot from %s:%s; remote data will be retained", driver, wd)
		err := collectDriverSnapshot(cfg, host, wd)
		if err == nil {
			updateLastRunCollection(cfg.rootDir, wd, "snapshot")
		}
		return err
	}
	log.Printf("collecting finished driver run %s:%s", driver, wd)
	return collectDriverResults(cfg, host, wd)
}

// runDriverExplainCheck verifies the triage LLM from the driver, so a laptop
// behind the firewall can confirm it reaches the UiS Ollama server (which only
// the driver can reach). It builds the sweep binary, ships it to a temp dir on
// the driver, runs sweep -explain-check there with the forwarded API key, and
// removes the temp dir. Unlike a full driven sweep it needs no benchmark binary,
// generated SSH config, or peer connections.
func runDriverExplainCheck(cfg *config, hosts []string) error {
	driver, err := resolveDriverHost(cfg.driver, hosts)
	if err != nil {
		return err
	}
	// The key is validated on the laptop in parseFlags, so driverLLMEnv returns a
	// non-empty export here; guard anyway since the check cannot run without it.
	llmEnv := driverLLMEnv(cfg)
	if llmEnv == "" {
		return fmt.Errorf("%s must be set on the laptop to forward to the driver", providerKeyEnv(cfg.explainProvider))
	}

	stage, err := os.MkdirTemp("", "sweep-explain-check-")
	if err != nil {
		return fmt.Errorf("staging dir: %w", err)
	}
	defer os.RemoveAll(stage)
	sweepLocal := filepath.Join(stage, "sweep")
	if err := buildSweepBinary(sweepLocal); err != nil {
		return fmt.Errorf("build sweep: %w", err)
	}

	// No agent forwarding: the check does no peer SSH, so requesting it only
	// yields a noisy "forwarding request denied" when the driver refuses.
	driverGroup, err := iago.NewSSHGroup([]string{driver}, cfg.sshConfig, iago.FailFast(), iago.KeepAlive(driverKeepAlive))
	if err != nil {
		return fmt.Errorf("connect to driver: %w", err)
	}
	defer driverGroup.Close()
	host := driverGroup.Hosts[0]
	ctx := context.Background()
	namespace, err := ensureRemoteNamespace(ctx, host, cfg.remoteDir)
	if err != nil {
		return err
	}
	wd := namespace + "/sweep-explain-check-" + time.Now().Format("20060102_150405")

	if err := driverExec(ctx, host, "mkdir -p "+iago.Quote(wd)); err != nil {
		return fmt.Errorf("create remote work dir: %w", err)
	}
	log.Printf("uploading sweep binary to %s via %s...", driver, cfg.transferMode)
	if err := uploadDriverFile(ctx, cfg, driver, host, sweepLocal, wd+"/sweep", iago.NewPerm(0o755)); err != nil {
		return fmt.Errorf("upload sweep: %w", err)
	}

	log.Printf("running explain check on %s (output streamed below)", driver)
	runErr := iago.Shell{
		Command: "bash -s",
		Stdin:   strings.NewReader(explainCheckScript(wd, llmEnv, cfg.explainProvider, cfg.explainModel)),
		Stdout:  os.Stderr,
		Stderr:  os.Stderr,
	}.Apply(ctx, host)

	// Remove the temp dir regardless of the check result.
	if err := driverExec(ctx, host, "rm -rf "+iago.Quote(wd)); err != nil {
		log.Printf("warning: remote cleanup of %s:%s: %v", driver, wd, err)
	}

	if runErr == nil {
		return nil
	}
	if exitErr, ok := errors.AsType[iago.ExitStatus](runErr); ok {
		return fmt.Errorf("explain check failed on %s (exit status %d)", driver, exitErr.ExitStatus())
	}
	return fmt.Errorf("connection to driver %s ended before the check finished: %w", driver, runErr)
}

// explainCheckScript exports the forwarded API key and runs the uploaded sweep
// binary's -explain-check on the driver. The key is exported (not passed as a
// flag or printed) so it stays out of the argv and the streamed console, the
// same discipline bootstrapScript uses for a full sweep. The trailing "exit"
// is essential: bash -s reads its script from stdin, and iago closes that pipe
// only after the command returns, so without an explicit exit a successful run
// blocks waiting for stdin EOF (set -e already exits on the failure path).
func explainCheckScript(wd, llmEnv, provider, model string) string {
	return fmt.Sprintf(`set -e
export %s
cd %s
./sweep -explain-check -explain-provider %s -explain-model %s
exit 0
`, llmEnv, iago.Quote(wd), iago.Quote(provider), iago.Quote(model))
}

// runDriverCheck runs the -check host diagnostics from the driver rather than
// the laptop, so the reported clock skew is measured against the driver's
// LAN-local clock (the same vantage the benchmark's own ClockSync uses) instead
// of the laptop's, whose WAN round-trip to the cluster otherwise dominates both
// the skew estimate and its uncertainty. It ships the sweep binary plus the
// generated SSH config to the driver's binary cache (skipping either that is
// already up to date, per cachedUpload), runs "sweep -check" there over the
// same hosts (the driver reaches its peers over the LAN using the forwarded
// agent, and probes itself locally — a host cannot SSH to itself — so the
// driver row reads a near-zero skew), and streams the table back. Unlike a full
// driven sweep it uploads no benchmark binary and drives no measurement.
func runDriverCheck(cfg *config, hosts []string) error {
	driver, err := resolveDriverHost(cfg.driver, hosts)
	if err != nil {
		return err
	}

	// Forward the agent: unlike the explain check, the driver-side check SSHes to
	// every peer, authenticating with the laptop's forwarded key.
	driverGroup, err := dialDriverGroup(driver, cfg.sshConfig)
	if err != nil {
		return fmt.Errorf("connect to driver: %w", err)
	}
	defer driverGroup.Close()
	host := driverGroup.Hosts[0]
	ctx := context.Background()
	namespace, err := ensureRemoteNamespace(ctx, host, cfg.remoteDir)
	if err != nil {
		return err
	}
	cacheDir := namespace + "/" + driverCacheName

	if err := driverExec(ctx, host, "mkdir -p "+iago.Quote(cacheDir)); err != nil {
		return fmt.Errorf("create remote cache dir: %w", err)
	}

	stage, err := os.MkdirTemp("", "sweep-driver-check-")
	if err != nil {
		return fmt.Errorf("staging dir: %w", err)
	}
	defer os.RemoveAll(stage)
	sweepLocal := filepath.Join(stage, "sweep")
	if err := buildSweepBinary(sweepLocal); err != nil {
		return fmt.Errorf("build sweep: %w", err)
	}
	cfgLocal := filepath.Join(stage, "ssh.config")
	if err := os.WriteFile(cfgLocal, []byte(generatedSSHConfig()), 0o644); err != nil {
		return fmt.Errorf("write ssh config: %w", err)
	}

	if _, err := cachedUpload(ctx, cfg, driver, host, cacheDir, sweepLocal, "sweep", iago.NewPerm(0o755)); err != nil {
		return err
	}
	if _, err := cachedUpload(ctx, cfg, driver, host, cacheDir, cfgLocal, "ssh.config", iago.NewPerm(0o644)); err != nil {
		return err
	}

	log.Printf("running check on %s over %d host(s) (output streamed below)", driver, len(hosts))
	runErr := iago.Shell{
		Command: "bash -s",
		Stdin:   strings.NewReader(driverCheckScript(cacheDir, strings.Join(hosts, ","), cfg.port, cfg.binaryPath, driver, cfg.remoteDir)),
		Stdout:  os.Stderr,
		Stderr:  os.Stderr,
	}.Apply(ctx, host)

	if runErr == nil {
		return nil
	}
	if exitErr, ok := errors.AsType[iago.ExitStatus](runErr); ok {
		return fmt.Errorf("check failed on %s (exit status %d)", driver, exitErr.ExitStatus())
	}
	return fmt.Errorf("connection to driver %s ended before the check finished: %w", driver, runErr)
}

// driverCheckScript runs the cached sweep binary's -check on the driver over the
// given hosts, using the cached SSH config so the driver reaches its peers the
// same way a driven sweep does. cacheDir is the driver's binary cache
// (driverCacheDir), where cachedUpload placed both files. When binary is set (a
// foreign protocol run), it is forwarded so the lingering-process probe greps
// for the matching program name; otherwise the driver-side default matches the
// benchmark the sweep deploys. self is the driver's own alias, forwarded as
// -self-host so the check probes it locally instead of SSHing to itself (which
// fails on the loopback self-connection). Like explainCheckScript it ends with
// an explicit exit so bash -s does not block waiting for stdin EOF after a
// successful check.
func driverCheckScript(cacheDir, hostsCSV string, port int, binary, self, remoteDir string) string {
	bin := ""
	if binary != "" {
		bin = " -binary " + iago.Quote(binary)
	}
	return fmt.Sprintf(`set -e
cd %s
./sweep -check -hosts %s -config %s/ssh.config -port %d -self-host %s -remote-dir %s%s
exit 0
`, iago.Quote(cacheDir), iago.Quote(hostsCSV), iago.Quote(cacheDir), port, iago.Quote(self), iago.Quote(remoteDir), bin)
}

// finishedRemotely reports whether the remote bootstrap/collect script ran to
// completion. A nil error or an SSH exit error (the remote process exited, even
// non-zero for failed runs) both mean "finished"; any other error type is a
// transport failure and the detached run may still be going.
func finishedRemotely(runErr error) bool {
	if runErr == nil {
		return true
	}
	if exitErr, ok := errors.AsType[iago.ExitStatus](runErr); ok {
		log.Printf("remote sweep finished with non-zero status %d; collecting results", exitErr.ExitStatus())
		return true
	}
	return false
}

// collectMode selects what a -collect downloads from the driver; see
// chooseCollectMode.
type collectMode int

const (
	collectCompact collectMode = iota // first collection: compact plot data only
	collectFull                       // compact already collected: full raw archive + cleanup
	collectSalvage                    // sweep aborted before exporting: partial output as-is
)

// chooseCollectMode picks the collection strategy: a prior compact collection
// (the marker) means this collect archives the full raw results; otherwise
// the compact transfer is downloaded when the driven sweep produced one; a
// missing compact transfer means the sweep died before its export step (e.g.
// a netcheck abort or a mid-sweep crash), so whatever partial output exists
// is salvaged instead of failing on the absent directory.
func chooseCollectMode(compactMarked, compactExists bool) collectMode {
	switch {
	case compactMarked:
		return collectFull
	case compactExists:
		return collectCompact
	default:
		return collectSalvage
	}
}

// collectDriverResults downloads driver results into cfg.outDir. The first
// successful collection downloads only the compact transfer directory produced
// by the driven sweep and leaves the raw driver work dir in place. Once that
// compact collection is marked, a later -collect downloads the full raw archive
// and removes the remote work dir. A run that aborted before exporting any
// compact transfer has its partial output salvaged instead.
func collectDriverResults(cfg *config, host iago.Host, wd string) error {
	// The driven sweep nests its results in a label subdirectory under wd/out
	// (for example wd/out/e1-coord-nscale). Descend into that run directory so
	// its contents land directly in cfg.outDir rather than one level too deep.
	remoteOut := wd + "/out"
	runDir, err := remoteRunDir(context.Background(), host, remoteOut)
	if err != nil {
		return fmt.Errorf("locate remote run dir under %s:%s: %w", host.Name(), remoteOut, err)
	}
	marked, err := iago.FileExists(context.Background(), host, driverCompactMarkerPath(wd))
	if err != nil {
		return fmt.Errorf("check compact collection marker: %w", err)
	}
	compactExists, err := iago.DirExists(context.Background(), host, remoteOut+"/"+runDir+"/"+compactTransferDir)
	if err != nil {
		return fmt.Errorf("check compact transfer dir: %w", err)
	}
	switch chooseCollectMode(marked, compactExists) {
	case collectFull:
		return collectDriverFullResults(cfg, host, wd, remoteOut, runDir)
	case collectCompact:
		return collectDriverCompactResults(cfg, host, wd, remoteOut, runDir)
	default:
		return collectDriverSalvage(cfg, host, wd, remoteOut, runDir)
	}
}

// collectDriverSalvage downloads a driven sweep's partial output when it died
// before exporting a compact transfer (a netcheck abort, a setup failure, or
// a mid-sweep crash): whatever the run directory holds — sweep.log, per-run
// logs, manifests, and any raw result files — is downloaded as-is and binary
// results are converted for inspection. The remote work dir is retained so
// the aborted run can still be examined in place.
func collectDriverSalvage(cfg *config, host iago.Host, wd, remoteOut, runDir string) error {
	remoteDir := remoteOut + "/" + runDir
	log.Printf("no compact transfer on %s — the sweep aborted before exporting results; salvaging partial output", host.Name())
	if err := downloadDriverDir(cfg, host, remoteDir, cfg.outDir); err != nil {
		return fmt.Errorf("download partial output: %w (output remains on %s:%s)", err, host.Name(), wd)
	}
	if n, err := convertDirBinaryResults(cfg.outDir); err != nil {
		log.Printf("warning: convert downloaded results: %v", err)
	} else if n > 0 {
		log.Printf("converted %d binary result file(s) to protojson", n)
	}
	log.Printf("partial driver output collected — see %s for why the sweep aborted", displayPath(filepath.Join(cfg.outDir, "sweep.log")))
	updateLastRunCollection(cfg.rootDir, wd, "recoverable")
	printProblemManifests(cfg.outDir)
	log.Printf("driver work dir retained on %s:%s for inspection; to discard it:", host.Name(), wd)
	log.Printf("  %s", driverCleanupCommand(host.Name(), cfg.sshConfig, wd))
	return nil
}

func collectDriverSnapshot(cfg *config, host iago.Host, wd string) error {
	remoteOut := wd + "/out"
	runDir, err := remoteRunDir(context.Background(), host, remoteOut)
	if err != nil {
		return fmt.Errorf("the active run has not created a collectible output directory yet: %w", err)
	}
	snapshotDir := filepath.Join(cfg.outDir, "snapshot")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return err
	}
	if err := downloadDriverDir(cfg, host, remoteOut+"/"+runDir, snapshotDir); err != nil {
		return fmt.Errorf("download active snapshot: %w", err)
	}
	if n, err := convertDirBinaryResults(snapshotDir); err != nil {
		log.Printf("warning: convert snapshot results: %v", err)
	} else if n > 0 {
		log.Printf("converted %d snapshot result file(s) to protojson", n)
	}
	log.Printf("active snapshot collected in %s; remote run retained", displayPath(snapshotDir))
	return nil
}

func collectDriverCompactResults(cfg *config, host iago.Host, wd, remoteOut, runDir string) error {
	remoteCompact := remoteOut + "/" + runDir + "/" + compactTransferDir
	log.Printf("downloading compact driver results via %s...", cfg.transferMode)
	if err := downloadDriverDir(cfg, host, remoteCompact, cfg.outDir); err != nil {
		return fmt.Errorf("download compact results: %w (raw results remain on %s:%s)", err, host.Name(), wd)
	}
	if err := os.WriteFile(filepath.Join(cfg.outDir, compactMarker), []byte(wd+"\n"), 0o644); err != nil {
		log.Printf("warning: local compact marker: %v", err)
	}
	if err := driverExec(context.Background(), host, "touch "+iago.Quote(driverCompactMarkerPath(wd))); err != nil {
		return fmt.Errorf("mark compact collection: %w (raw results remain on %s:%s)", err, host.Name(), wd)
	}
	log.Printf("driver sweep complete — compact results in %s", displayPath(cfg.outDir))
	log.Printf("sweep log (from driver): %s", displayPath(filepath.Join(cfg.outDir, "sweep.log")))
	printProblemManifests(cfg.outDir)
	log.Printf("raw .binpb results retained on %s:%s", host.Name(), wd)
	updateLastRunCollection(cfg.rootDir, wd, "compact")
	log.Printf("to archive raw results and remove the driver work dir:")
	log.Printf("  ./cmd/sweep/sweep -driver %s -collect %s -outdir %s", host.Name(), wd, cfg.rootDir)
	log.Printf("to discard raw driver results without archiving:")
	log.Printf("  %s", driverCleanupCommand(host.Name(), cfg.sshConfig, wd))
	autoReport(cfg)
	return nil
}

func collectDriverFullResults(cfg *config, host iago.Host, wd, remoteOut, runDir string) error {
	log.Printf("compact results were already collected; downloading full raw archive via %s...", cfg.transferMode)
	remoteDir := remoteOut + "/" + runDir
	if err := downloadDriverDir(cfg, host, remoteDir, cfg.outDir); err != nil {
		return fmt.Errorf("download raw archive: %w (results remain on %s:%s)", err, host.Name(), wd)
	}
	if n, err := convertDirBinaryResults(cfg.outDir); err != nil {
		log.Printf("warning: convert downloaded results: %v", err)
	} else {
		log.Printf("converted %d binary result file(s) to protojson", n)
	}
	if err := driverExec(context.Background(), host, "rm -rf "+iago.Quote(wd)); err != nil {
		log.Printf("warning: remote cleanup of %s:%s: %v", host.Name(), wd, err)
	}
	log.Printf("driver raw archive collected — results in %s", displayPath(cfg.outDir))
	updateLastRunCollection(cfg.rootDir, wd, "archived")
	log.Printf("sweep log (from driver): %s", displayPath(filepath.Join(cfg.outDir, "sweep.log")))
	printProblemManifests(cfg.outDir)
	autoReport(cfg)
	return nil
}

func downloadDriverDir(cfg *config, host iago.Host, remoteDir, localDir string) error {
	var downloadErr error
	if cfg.transferMode == "sftp" {
		downloadErr = driverDownloadDir(context.Background(), host, remoteDir, localDir)
	} else {
		downloadErr = rsyncDownloadDir(host.Name(), cfg.sshConfig, remoteDir, localDir)
	}
	return downloadErr
}

// uploadDriverFile copies localPath to remotePath on the driver, using the
// transfer backend selected by cfg.transferMode. It is the upload counterpart
// of downloadDriverDir.
func uploadDriverFile(ctx context.Context, cfg *config, driver string, host iago.Host, localPath, remotePath string, perm iago.Perm) error {
	if cfg.transferMode == "sftp" {
		return iago.UploadFile(ctx, host, localPath, remotePath, perm)
	}
	return rsyncUploadFile(driver, cfg.sshConfig, localPath, remotePath)
}

func driverCompactMarkerPath(wd string) string {
	return wd + "/" + compactMarker
}

// readyMarkerName is the file the driven sweep touches once it has dialed its
// peers; a -detach launcher watches driverReadyMarkerPath for it to know the
// forwarded agent is no longer needed.
const readyMarkerName = "peers.dialed"

func driverReadyMarkerPath(wd string) string {
	return wd + "/" + readyMarkerName
}

// detachReadyWindow bounds how long -detach waits for a freshly started run
// to finish dialing its peers before it gives up waiting for confirmation.
// The driven sweep needs the laptop's forwarded SSH agent only for that
// one-time dial, then touches the ready marker; until it does, the laptop
// must stay connected or the dial fails with "no valid authentication
// methods". The window must comfortably exceed the driven sweep's setup time
// (an -explain preflight plus dialing every peer), so it is generous: it is a
// backstop, not the common case, since the marker normally appears in
// seconds.
const (
	detachReadyWindow = 3 * time.Minute
	detachReadyPoll   = 2 * time.Second
)

// awaitDetachedStartup waits, right after a detached run starts, until the
// driven sweep either finishes dialing its peers (the ready marker) or dies
// during setup (exit.code), via pollDetachedStartup. A poll error (e.g. a
// transient control-connection hiccup, exactly the kind of session flakiness
// this check exists to route around) must not end the wait early — only a
// definitive answer, or the deadline, may.
func awaitDetachedStartup(ctx context.Context, host iago.Host, driver, wd string) error {
	dialed := func() (bool, error) { return iago.FileExists(ctx, host, driverReadyMarkerPath(wd)) }
	crashed := func() (bool, error) { return iago.FileExists(ctx, host, wd+"/exit.code") }
	tail := func() string {
		out, err := iago.Output(ctx, host, "tail -n 40 "+iago.Quote(wd+"/console.log"))
		if err != nil {
			return ""
		}
		return out
	}
	return pollDetachedStartup(dialed, crashed, tail, driver, wd, detachReadyWindow, detachReadyPoll)
}

// pollDetachedStartup implements the retry and decision logic for
// awaitDetachedStartup against injectable dialed/crashed/tail functions, so
// the timing and error-handling behavior are unit testable without a live SSH
// connection. Each poll checks dialed first: once the ready marker exists the
// run is past its one-time peer dial and safe to leave, even if it has since
// also exited. Only exit.code WITHOUT the marker means the run died before
// dialing — a setup failure — for which it returns a descriptive error with
// the console tail. A poll error on either check only records lastErr and is
// retried; a single hiccup must never make a crashed run look healthy. Once
// the deadline passes with neither answer it logs a warning and returns nil,
// since a stuck poll says nothing about the detached run's own health.
func pollDetachedStartup(dialed, crashed func() (bool, error), tail func() string, driver, wd string, window, interval time.Duration) error {
	deadline := time.Now().Add(window)
	var lastErr error
	for {
		if ok, err := dialed(); err != nil {
			lastErr = err
		} else if ok {
			log.Printf("detached run on %s finished dialing its peers; forwarded agent no longer needed", driver)
			return nil
		} else if ec, err := crashed(); err != nil {
			// Only reachable when the marker is absent: an exit.code here means
			// the run exited before completing its peer dial.
			lastErr = err
		} else if ec {
			return errors.New(earlyExitMessage(driver, wd, tail()))
		}
		if time.Now().After(deadline) {
			log.Printf("warning: could not confirm detached run on %s dialed its peers within %s (last error: %v); proceeding without confirmation — verify with -collect", driver, window, lastErr)
			return nil
		}
		time.Sleep(interval)
	}
}

// earlyExitMessage formats the error returned when a detached run's exit.code
// appears before its peer-dial marker. consoleTail is the tail of the remote
// console.log, or "" if it could not be read.
func earlyExitMessage(driver, wd, consoleTail string) string {
	msg := fmt.Sprintf("detached run on %s exited before dialing its peers; "+
		"this is not a normal completion — check that SSH agent forwarding to %s is working "+
		"and that the driven sweep's flags are valid; raw results remain on %s:%s for inspection",
		driver, driver, driver, wd)
	if consoleTail != "" {
		msg += "\nlast console output:\n" + consoleTail
	}
	return msg
}

func driverCleanupCommand(driver, sshConfig, wd string) string {
	args := []string{"ssh"}
	if sshConfig != "" {
		args = append(args, "-F", sshConfig)
	}
	args = append(args, driver, "rm -rf "+iago.Quote(wd))
	return strings.Join(args, " ")
}

// manifestPathsWithStatus returns the downloaded manifests with the given
// status, so the launcher can surface failed and degraded runs on the laptop
// console (the driver's own listing names the driver-side paths, which do not
// exist locally).
func manifestPathsWithStatus(outDir, status string) []string {
	matches, err := filepath.Glob(filepath.Join(outDir, "*"+manifestSuffix))
	if err != nil {
		return nil
	}
	var paths []string
	for _, p := range matches {
		if manifestStatus(p) == status {
			paths = append(paths, p)
		}
	}
	return paths
}

// printProblemManifests lists the failed and degraded run manifests under
// outDir, each with its diagnostic artifact paths.
func printProblemManifests(outDir string) {
	if failed := manifestPathsWithStatus(outDir, runStatusFailed); len(failed) > 0 {
		log.Printf("failed run manifests (%d):", len(failed))
		for _, p := range failed {
			printFailedRunArtifacts(outDir, p)
		}
	}
	if degraded := manifestPathsWithStatus(outDir, runStatusDegraded); len(degraded) > 0 {
		log.Printf("degraded run manifests (%d):", len(degraded))
		for _, p := range degraded {
			printFailedRunArtifacts(outDir, p)
		}
	}
}

// printFailedRunArtifacts logs a failed run's manifest path plus, when present,
// its node log and failure snapshot, so a reader can jump straight to the
// diagnostic files without hunting for them under logSubdir.
func printFailedRunArtifacts(outDir, manifestFile string) {
	log.Printf("  %s", displayPath(manifestFile))
	base := strings.TrimSuffix(filepath.Base(manifestFile), manifestSuffix)
	if p := runLogPath(outDir, base); fileExists(p) {
		log.Printf("    node log: %s", displayPath(p))
	}
	if p := snapshotPath(outDir, base); fileExists(p) {
		log.Printf("    host snapshot: %s", displayPath(p))
	}
}

// fileExists reports whether path names a regular, readable file.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// remoteSweepCommand builds the shell command that runs the driven sweep on the
// driver. It rebuilds the flags from cfg (not os.Args) so the expanded -test
// values are forwarded and the driver-only flags are rewritten to remote paths.
// binDir is where the sweep and benchmark binaries and the SSH config were
// cached by cachedUpload (see driverCacheDir); it is independent of wd, the
// run's own work directory, so a cached binary is never tied to one run's
// timestamped directory and can be reused by the next.
func remoteSweepCommand(cfg *config, wd, binDir, hostsCSV, sha string) string {
	args := []string{
		"-driven",
		"-git-sha=" + sha,
		"-hosts=" + hostsCSV,
		"-binary=" + binDir + "/benchmark",
		"-config=" + binDir + "/ssh.config",
		"-outdir=" + wd + "/out",
		"-remote-dir=" + cfg.remoteDir,
		"-port=" + strconv.Itoa(cfg.port),
		"-duration=" + cfg.duration.String(),
		"-trim=" + cfg.trim.String(),
		"-sweep=" + cfg.sweepLabel,
		"-n=" + joinInts(cfg.sweep.numNodes),
		"-workers=" + joinInts(cfg.sweep.workers),
		"-payload=" + joinInts(cfg.sweep.payloads),
		"-rate=" + joinInts(cfg.sweep.rates),
		"-benchmarks=" + strings.Join(cfg.sweep.benchmarks, ","),
		"-reps=" + strconv.Itoa(cfg.sweep.reps),
		"-degraded-below=" + strconv.FormatFloat(cfg.degradedBelow, 'g', -1, 64),
		"-degraded-above=" + strconv.FormatFloat(cfg.degradedAbove, 'g', -1, 64),
		"-degraded-latency-below=" + strconv.FormatFloat(cfg.degradedLatencyBelow, 'g', -1, 64),
		"-netcheck=" + strconv.FormatBool(cfg.netcheck),
		"-fd-limit=" + strconv.Itoa(cfg.fdLimit),
	}
	// The buffer axes default to empty, and the list flags reject an empty
	// value, so forward them only when the launcher set them.
	if len(cfg.sweep.sendBuffers) > 0 {
		args = append(args, "-send-buffer="+joinInts(cfg.sweep.sendBuffers))
	}
	if len(cfg.sweep.recvBuffers) > 0 {
		args = append(args, "-recv-buffer="+joinInts(cfg.sweep.recvBuffers))
	}
	if cfg.detach {
		// In -detach the launcher leaves as soon as the driven sweep signals it
		// has dialed its peers, so it must know where to look for that marker.
		args = append(args, "-ready-marker="+driverReadyMarkerPath(wd))
	}
	if cfg.verbose {
		args = append(args, "-verbose")
	}
	if cfg.interval != "" {
		args = append(args, "-interval="+cfg.interval)
	}
	if cfg.statsMode != "" {
		args = append(args, "-stats-mode="+cfg.statsMode)
	}
	if cfg.rateStep > 0 {
		args = append(args, "-rate-step="+strconv.Itoa(cfg.rateStep))
	}
	if cfg.rateStepMax > 0 {
		args = append(args, "-rate-step-max="+strconv.Itoa(cfg.rateStepMax))
	}
	if nonDefaultStreamModes(cfg.sweep.streamModes) {
		args = append(args, "-stream-mode="+strings.Join(cfg.sweep.streamModes, ","))
	}
	if cfg.extraArgs != "" {
		args = append(args, "-extra-args="+cfg.extraArgs)
	}
	switch {
	case cfg.pgo:
		args = append(args, "-pgo")
	case cfg.collectProfiles:
		args = append(args, "-collect-profiles")
	}
	// Triage runs on the driver; the laptop forwards the model choice.
	// The API key is not a flag (see runDriver).
	if cfg.explain {
		args = append(args, "-explain", "-explain-provider="+cfg.explainProvider, "-explain-model="+cfg.explainModel)
		if cfg.explainMaxLog > 0 {
			args = append(args, "-explain-max-log="+strconv.Itoa(cfg.explainMaxLog))
		}
	}
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, iago.Quote(binDir+"/sweep"))
	for _, a := range args {
		parts = append(parts, iago.Quote(a))
	}
	return strings.Join(parts, " ")
}

// driverLLMEnv returns the NAME=value assignment that forwards the LLM API key
// to the driver so it can triage (-explain) or run a connectivity check
// (-explain-check) there, or "" when neither is set or the key is unset on the
// laptop. The value is shell-quoted for safe export; the remote scripts keep it
// out of run.sh, the argv, and console.log. parseFlags already fails fast when
// the key is unset, so in practice this returns a non-empty export whenever
// triage or a check is requested; the empty fallback is defensive.
func driverLLMEnv(cfg *config) string {
	if !cfg.explain && !cfg.explainCheck {
		return ""
	}
	name := providerKeyEnv(cfg.explainProvider)
	val := os.Getenv(name)
	if val == "" {
		log.Printf("warning: -explain set but %s is not set; driver-side triage will be skipped", name)
		return ""
	}
	return name + "=" + iago.Quote(val)
}

// generatedSSHConfig is the minimal SSH config uploaded to the driver. iago
// reads "StrictHostKeyChecking no" as InsecureIgnoreHostKey, so the driver
// needs no seeded known_hosts; authentication is by the forwarded agent, and
// peer aliases resolve through the driver's own resolver (plain "ssh bbN").
func generatedSSHConfig() string {
	return `# Generated by 'sweep -driver': minimal config for driver->peer SSH.
# Auth uses the SSH agent forwarded from the laptop (ForwardAgent in the user's
# SSH config). Host-key checking is disabled because the cluster LAN is trusted
# and the driver has no seeded known_hosts for its peers; peer aliases resolve
# via the driver's own resolver.
Host *
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    LogLevel ERROR
`
}

// bootstrapPreamble creates the remote work dir, writes run.sh, and starts
// the sweep detached (setsid) so it survives a laptop disconnect. The sweep
// command goes through a run.sh written by a quoted heredoc so its embedded
// quotes are not reinterpreted by the bootstrap shell. It is shared by
// bootstrapScript and detachBootstrapScript, which differ only in what they
// do once the detached sweep has started.
//
// envExport, when non-empty, is a NAME=value assignment (e.g. the LLM API key
// for -explain) exported into the bootstrap shell and inherited by the detached
// sweep. It is deliberately kept out of run.sh, the sweep argv, and console.log
// so the secret never lands on disk or in the streamed log.
func bootstrapPreamble(wd, sweepCmd, envExport string, fdLimit int) string {
	export := ""
	if envExport != "" {
		export = "export " + envExport + "\n"
	}
	// Raise the driver-side sweep's own soft open-file limit before exec; the
	// orchestrator holds a connection to every peer, so it hits the same 1024
	// default a node does. The nodes it launches get their own ulimit via
	// buildNodeCmd.
	ulimit := ""
	if stmt := fdLimitStmt(fdLimit); stmt != "" {
		ulimit = stmt + "\n"
	}
	return fmt.Sprintf(`set -e
%sWD=%s
mkdir -p "$WD"
cd "$WD"
rm -f exit.code
# Create the log before tailing so the follower never races the writer; the
# detached sweep appends to the same file.
: > console.log
cat > run.sh <<'SWEEP_EOF'
#!/bin/sh
%sexec %s
SWEEP_EOF
chmod +x run.sh
setsid sh -c './run.sh >> console.log 2>&1; echo $? > exit.code' </dev/null >/dev/null 2>&1 &
echo $! > run.pid
echo "[driver] detached sweep started in $WD"
`, export, iago.Quote(wd), ulimit, sweepCmd)
}

// bootstrapScript starts the detached sweep via bootstrapPreamble, then tails
// its console log until the run records an exit code.
func bootstrapScript(wd, sweepCmd, envExport string, fdLimit int) string {
	return bootstrapPreamble(wd, sweepCmd, envExport, fdLimit) + `tail -n +1 -F console.log &
TAILPID=$!
while [ ! -f exit.code ]; do sleep 1; done
sleep 1
kill "$TAILPID" 2>/dev/null || true
EC=$(cat exit.code)
echo "[driver] sweep exited with status $EC"
exit "$EC"
`
}

// detachBootstrapScript starts the detached sweep via bootstrapPreamble and
// returns immediately, without tailing its console log or waiting for it to
// finish. It is used by -detach so the launcher can confirm the run started
// and exit, leaving collection for a later -collect.
func detachBootstrapScript(wd, sweepCmd, envExport string, fdLimit int) string {
	return bootstrapPreamble(wd, sweepCmd, envExport, fdLimit) + "exit 0\n"
}

// driverKeepAlive is how often the launcher pings the driver control channel.
// The channel can sit quiet for the length of a single benchmark run while the
// driver streams nothing, and crypto/ssh does not honor ServerAliveInterval, so
// without these pings a NAT or firewall idle timeout could silently drop the
// connection mid-sweep.
const driverKeepAlive = 30 * time.Second

// dialDriverGroup connects to the driver host using the user's SSH config.
// Agent forwarding is always requested (equivalent to ssh -A) so that the
// driven sweep on the driver can authenticate to its peers using the
// laptop's keys via the forwarded agent. Keepalives hold the long-lived
// control channel open across quiet stretches of a run.
func dialDriverGroup(driver, sshConfigFile string) (iago.Group, error) {
	return iago.NewSSHGroup([]string{driver}, sshConfigFile,
		iago.FailFast(), iago.ForwardAgent(), iago.KeepAlive(driverKeepAlive))
}

// remoteRunDir returns the name of the run directory the driven sweep created
// under remoteOut (the driver's wd/out). The driven sweep always nests its
// results in a single label subdirectory, whose name the launcher cannot
// reconstruct on reconnect; listing the directory is robust in every case.
func remoteRunDir(ctx context.Context, host iago.Host, remoteOut string) (string, error) {
	out, err := iago.Output(ctx, host, "ls -1 "+iago.Quote(remoteOut))
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("no run directory found")
}

// driverExec runs a shell command on host, streaming output to the console.
func driverExec(ctx context.Context, host iago.Host, command string) error {
	return iago.Shell{
		Command: command,
		Stdout:  os.Stderr,
		Stderr:  os.Stderr,
	}.Apply(ctx, host)
}

// driverDownloadDir downloads the contents of remoteDir on host into localDir
// via SFTP, streaming each file to disk so the result set never has to fit in
// RAM. A pre-scan computes the total byte count so that progress is shown as
// bytes transferred / total (percentage).
func driverDownloadDir(ctx context.Context, host iago.Host, remoteDir, localDir string) error {
	absLocal, err := filepath.Abs(localDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absLocal, 0o755); err != nil {
		return err
	}
	src, err := iago.NewPathFromAbs(remoteDir)
	if err != nil {
		return err
	}
	dest, err := iago.NewPathFromAbs(absLocal)
	if err != nil {
		return err
	}

	dl := iago.DownloadDir{Src: src, Dest: dest}
	total, _ := dl.Size(ctx, host)

	var done int64
	dl.Progress = func(n int64) {
		done += n
		var line string
		if total > 0 {
			line = fmt.Sprintf("downloading: %s / %s (%.0f%%)",
				formatSize(done), formatSize(total), float64(done)/float64(total)*100)
		} else {
			line = fmt.Sprintf("downloading: %s", formatSize(done))
		}
		fmt.Fprintf(os.Stderr, "\r%-72s", line)
	}

	log.Printf("downloading results from %s:%s via SFTP...", host.Name(), remoteDir)
	if err := dl.Apply(ctx, host); err != nil {
		fmt.Fprintln(os.Stderr)
		return err
	}
	fmt.Fprintln(os.Stderr)
	log.Printf("downloaded results from %s:%s → %s", host.Name(), remoteDir, displayPath(localDir))
	return nil
}

// rsyncArgs returns the common rsync arguments for both uploads and downloads:
// -a preserves layout and permissions, -z compresses on the wire, -s sends
// remote paths through the rsync protocol instead of the login shell, --partial
// keeps a partial file so an interrupted transfer resumes on retry, and
// --progress prints per-file progress. A non-empty sshConfig is forwarded to
// ssh via -e so rsync uses the same config the launcher used.
func rsyncArgs(sshConfig string) []string {
	args := []string{"-azs", "--partial", "--progress"}
	if sshConfig != "" {
		args = append(args, "-e", "ssh -F "+iago.Quote(sshConfig))
	}
	return args
}

// rsyncRemoteSpec returns an rsync remote source or destination. The path is
// deliberately not shell-quoted: -s makes rsync send it through the protocol.
func rsyncRemoteSpec(driver, path string) string {
	return driver + ":" + path
}

// requireRsyncSecludedArgs verifies that the local rsync supports -s. The
// system openrsync shipped by macOS identifies as rsync 2.6.9-compatible and
// does not implement this option; using it would otherwise produce a confusing
// transfer failure after the driver connection has already been established.
func requireRsyncSecludedArgs() error {
	cmd := exec.Command("rsync", "-s", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync with -s/--secluded-args is required (install rsync >=3.2.4): %w", err)
	}
	return nil
}

// rsyncUploadFile uploads localPath to driver:remotePath via rsync over SSH.
// Permissions are preserved from the local file (-a flag). On repeated runs
// rsync sends only changed blocks, so a rebuild that touches few bytes is fast.
func rsyncUploadFile(driver, sshConfig, localPath, remotePath string) error {
	if err := requireRsyncSecludedArgs(); err != nil {
		return err
	}
	args := append(rsyncArgs(sshConfig), localPath, rsyncRemoteSpec(driver, remotePath))
	cmd := exec.Command("rsync", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync: %w", err)
	}
	return nil
}

// rsyncDownloadDir downloads the contents of remoteDir on driver into localDir
// via rsync over SSH. Unlike the SFTP-based driverDownloadDir, rsync streams
// to disk, compresses on the wire, prints live progress, and resumes a partial
// transfer on a second attempt — the result set crosses the WAN once, robustly.
func rsyncDownloadDir(driver, sshConfig, remoteDir, localDir string) error {
	if err := requireRsyncSecludedArgs(); err != nil {
		return err
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	// Trailing slashes copy directory contents, not the directory itself.
	args := append(rsyncArgs(sshConfig), rsyncRemoteSpec(driver, remoteDir+"/"), localDir+"/")
	log.Printf("downloading results from %s:%s via rsync (compressed, resumable)...", driver, remoteDir)
	cmd := exec.Command("rsync", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync: %w", err)
	}
	log.Printf("downloaded results from %s:%s → %s", driver, remoteDir, displayPath(localDir))
	return nil
}

// formatSize formats n as a human-readable byte count (e.g. "12.3 MiB").
func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// buildSweepBinary cross-compiles sweep itself for linux/amd64. Run from the
// benchkit module root, which is where its own package path resolves.
func buildSweepBinary(outputPath string) error {
	if err := requireBenchkitModuleRoot(); err != nil {
		return err
	}
	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	log.Printf("building sweep for linux/amd64 → %s", abs)
	return runCrossBuild(exec.Command("go", "build", "-o", abs, "./cmd/sweep"))
}

// joinInts formats an int slice as a comma-separated string for a sweep flag.
func joinInts(xs []int) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = strconv.Itoa(x)
	}
	return strings.Join(s, ",")
}
