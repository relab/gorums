package main

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/relab/iago"
)

// defaultBinaryPath is the local binary built and deployed when -binary is unset.
const defaultBinaryPath = "./cmd/benchmark/benchmark"

// remoteProgram is the program deployed in each host's per-user remote namespace. Every
// remote reference to the binary — its path, the pkill/pgrep pattern, and the
// cleanup command — derives from its basename, so deploying a differently named
// binary requires no other changes.
type remoteProgram struct {
	name string // remote basename, e.g. "sweep-benchmark"
}

// newRemoteProgram derives the remote program from the local binary path,
// prefixing the basename with "sweep-" so the pgrep/pkill patterns match only
// programs this tool deployed. An empty path falls back to the default binary.
func newRemoteProgram(binaryPath string) remoteProgram {
	if binaryPath == "" {
		binaryPath = defaultBinaryPath
	}
	return remoteProgram{name: "sweep-" + filepath.Base(binaryPath)}
}

// path is the absolute path of the program on the remote hosts.
func (p remoteProgram) path(remoteDir string) string { return filepath.Join(remoteDir, p.name) }

// pgrep returns the program name as a pgrep/pkill -f pattern with the first
// character bracketed so the matching command does not match itself.
func (p remoteProgram) pgrep() string { return "[" + p.name[:1] + "]" + p.name[1:] }

// nodeAssignment describes one benchmark node in a distributed run.
type nodeAssignment struct {
	host     string // SSH alias; used for deployment, filenames, and logs
	peerHost string // advertised benchmark address; empty means use host
	port     int
}

// peerAddr returns the host:port address advertised to benchmark peers.
func (n nodeAssignment) peerAddr() string {
	host := cmp.Or(n.peerHost, n.host)
	return net.JoinHostPort(host, strconv.Itoa(n.port))
}

// hostAddr returns the SSH-alias host:port label used in local artifacts.
func (n nodeAssignment) hostAddr() string {
	return net.JoinHostPort(n.host, strconv.Itoa(n.port))
}

// hostAssignment records how one SSH alias should be advertised to peers.
type hostAssignment struct {
	alias    string
	peerHost string
}

// buildNodeAssignments computes host/port assignments for n nodes.
// Nodes are distributed round-robin across the given hosts; nodes beyond
// len(hosts) get successive port offsets on the same hosts.
func buildNodeAssignments(hosts []hostAssignment, n, basePort int) []nodeAssignment {
	numHosts := min(n, len(hosts))
	nodes := make([]nodeAssignment, n)
	for i := range n {
		host := hosts[i%numHosts]
		nodes[i] = nodeAssignment{
			host:     host.alias,
			peerHost: host.peerHost,
			port:     basePort + (i / numHosts),
		}
	}
	return nodes
}

// resultExt is the on-disk extension for result files. Benchmark nodes always
// write the binary proto format (magic header + Report message). sweep also
// converts collected files to human-readable protojson ".json" siblings for
// manual inspection (see convertBinaryResults).
const resultExt = ".binpb"

// resultFilename returns the result file basename for one node's run:
// "<base>_<host>_<port><ext>". The remote path is this name under the host's
// configured per-user namespace.
func resultFilename(base string, node nodeAssignment, ext string) string {
	return fmt.Sprintf("%s_%s_%d%s", base, node.host, node.port, ext)
}

// buildPeerList returns a comma-separated "host:port" string for all nodes.
func buildPeerList(nodes []nodeAssignment) string {
	peers := make([]string, len(nodes))
	for i, n := range nodes {
		peers[i] = n.peerAddr()
	}
	return strings.Join(peers, ",")
}

// resolvePeerHosts resolves one advertised benchmark address per SSH host.
// SSH aliases remain in use for deployment; the returned peerHost values are
// only passed to remote benchmark processes as -self/-remotes addresses.
func resolvePeerHosts(hosts []iago.Host, connectAddr func(string) string) ([]hostAssignment, error) {
	assignments := make([]hostAssignment, len(hosts))
	for i, host := range hosts {
		peerHost, err := resolvePeerHost(host.Name(), host.Address(), connectAddr(host.Name()), net.LookupIP)
		if err != nil {
			return nil, err
		}
		assignments[i] = hostAssignment{alias: host.Name(), peerHost: peerHost}
	}
	return assignments, nil
}

type lookupIPFunc func(string) ([]net.IP, error)

// resolvePeerHost chooses a stable, non-loopback IP address for one benchmark
// host. It prefers the already-connected SSH remote endpoint because SSH config
// HostName entries may be more specific than the alias. If that endpoint is not
// usable, it tries the SSH-config-expanded connect address, then the raw alias.
func resolvePeerHost(alias, sshAddr, sshConfigAddr string, lookup lookupIPFunc) (string, error) {
	candidates := peerHostCandidates(alias, sshAddr, sshConfigAddr)
	var errs []error
	for _, candidate := range candidates {
		if ip := net.ParseIP(candidate); ip != nil {
			if usablePeerIP(ip) {
				return ip.String(), nil
			}
			errs = append(errs, fmt.Errorf("%s: not a usable peer IP", candidate))
			continue
		}
		ips, err := lookup(candidate)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", candidate, err))
			continue
		}
		if ip := choosePeerIP(ips); ip != nil {
			return ip.String(), nil
		}
		errs = append(errs, fmt.Errorf("%s: no usable non-loopback IP in %v", candidate, ips))
	}
	return "", fmt.Errorf("resolve peer address for %s (ssh remote %q): %w", alias, sshAddr, errors.Join(errs...))
}

func peerHostCandidates(alias, sshAddr, sshConfigAddr string) []string {
	var candidates []string
	if sshAddr != "" {
		candidates = append(candidates, hostFromAddr(sshAddr))
	}
	if sshConfigAddr != "" {
		host := hostFromAddr(sshConfigAddr)
		if host != "" && !slices.Contains(candidates, host) {
			candidates = append(candidates, host)
		}
	}
	if alias != "" && !slices.Contains(candidates, alias) {
		candidates = append(candidates, alias)
	}
	return candidates
}

func hostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return strings.Trim(addr, "[]")
	}
	return host
}

func choosePeerIP(ips []net.IP) net.IP {
	for _, ip := range ips {
		if ip4 := ip.To4(); usablePeerIP(ip4) {
			return ip4
		}
	}
	for _, ip := range ips {
		if usablePeerIP(ip) {
			return ip
		}
	}
	return nil
}

func usablePeerIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func peerHostSummary(hosts []hostAssignment) string {
	parts := make([]string, len(hosts))
	for i, h := range hosts {
		parts[i] = h.alias + "=" + h.peerHost
	}
	return strings.Join(parts, ", ")
}

func resolveSSHConfigPath(configFile string) (string, error) {
	if configFile != "" {
		return configFile, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// withTimeout returns a copy of g with the given timeout.
func withTimeout(g iago.Group, d time.Duration) iago.Group {
	g.Timeout = d
	return g
}

// run executes fn on all hosts in g concurrently, collecting all errors.
func run(g iago.Group, name string, fn func(context.Context, iago.Host) error) error {
	var errs iago.Errors
	g.ErrorHandler = errs.Handle
	g.Run(name, fn)
	return errs.Err()
}

// buildBenchmark cross-compiles the benchmark binary for Linux/amd64 to
// outputPath. It must be run from the benchkit module root.
//
// When buildCmd is empty, it runs the built-in default (today's behavior):
//
//	GOOS=linux GOARCH=amd64 go build -o <output> ./cmd/benchmark/
//
// Otherwise buildCmd is a user-supplied build command template (from the -build
// flag or the BENCHKIT_BUILD environment variable), letting a foreign protocol
// supply its own package path or toolchain. The chosen output path is
// substituted for the {{output}} token, or appended as "-o <output>" if the
// token is absent. The command is run via "sh -c" from the benchkit module root with
// GOOS=linux GOARCH=amd64 appended to its environment (a build script may
// override these).
func buildBenchmark(outputPath, buildCmd string) error {
	abs, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	if buildCmd == "" {
		log.Printf("building benchmark binary for linux/amd64 → %s", abs)
		cmd = exec.Command("go", "build", "-o", abs, "./cmd/benchmark/")
	} else {
		script := expandBuildCmd(buildCmd, abs)
		log.Printf("building benchmark binary for linux/amd64 via custom command → %s", abs)
		log.Printf("  $ %s", script)
		cmd = exec.Command("sh", "-c", script)
	}
	return runCrossBuild(cmd)
}

// runCrossBuild sets the linux/amd64 cross-compile environment on cmd, routes
// its output to os.Stderr so a build never contaminates sweep's stdout, and
// runs it. It is the shared tail of the benchmark and sweep builders.
func runCrossBuild(cmd *exec.Cmd) error {
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// expandBuildCmd produces the shell script run for a custom build command. It
// substitutes the shell-quoted absolute output path for every {{output}} token;
// if the template has no such token, it appends "-o <output>" so a bare
// "go build" still writes to the chosen path. The output path is single-quoted
// so paths containing spaces are passed as one argument.
func expandBuildCmd(tmpl, absOutput string) string {
	out := iago.Quote(absOutput)
	if strings.Contains(tmpl, "{{output}}") {
		return strings.ReplaceAll(tmpl, "{{output}}", out)
	}
	return tmpl + " -o " + out
}

// upload deploys the local binary to prog.path() on all hosts in g.
func upload(g iago.Group, localBinary string, cfg *config) error {
	log.Printf("uploading binary to %d host(s)...", len(g.Hosts))
	return run(withTimeout(g, 5*time.Minute), "upload binary", func(ctx context.Context, host iago.Host) error {
		if err := iago.UploadFile(ctx, host, localBinary, cfg.prog.path(cfg.remoteDirs[host.Name()]), iago.NewPerm(0o755)); err != nil {
			return err
		}
		log.Printf("  ok uploaded to %s", host.Name())
		return nil
	})
}

// killLingering terminates any running instances of prog on all hosts.
// It first sends SIGTERM, escalates to SIGKILL if instances survive the
// grace period, and polls to confirm they have exited.
func killLingering(g iago.Group, prog remoteProgram) error {
	script := fmt.Sprintf(`pkill -f '%[1]s' || true
for i in $(seq 1 5); do
  sleep 1
  pgrep -f '%[1]s' > /dev/null || exit 0
done
pkill -KILL -f '%[1]s' || true
for i in $(seq 1 10); do
  sleep 1
  pgrep -f '%[1]s' > /dev/null || exit 0
done
echo "%[2]s still running after SIGKILL" >&2
exit 1`, prog.pgrep(), prog.name)
	return run(withTimeout(g, 30*time.Second), "kill lingering", func(ctx context.Context, host iago.Host) error {
		err := iago.Shell{Command: script}.Apply(ctx, host)
		if isSignalTerm(err) {
			// session.Close() races with an already-exited shell and delivers SIGTERM;
			// treat exit 143 as success since the kill script itself succeeded.
			log.Printf("  ok cleared %s", host.Name())
			return nil
		}
		if err != nil {
			return err
		}
		log.Printf("  ok cleared %s", host.Name())
		return nil
	})
}

// portCheckScript returns a shell script that exits non-zero when any of the
// given ports has a listener, printing each busy port's ss line (including the
// owning process when visible) so the failure names the culprit.
func portCheckScript(ports []string) string {
	return fmt.Sprintf(`busy=
for p in %s; do
  if ss -ltnH 2>/dev/null | grep -qE ":$p([[:space:]]|$)"; then
    busy="$busy $p"
    ss -ltnpH 2>/dev/null | grep -E ":$p([[:space:]]|$)" || true
  fi
done
[ -z "$busy" ]`, strings.Join(ports, " "))
}

// hostPortsByHost groups the nodes' listen ports (as strings) by host alias.
func hostPortsByHost(nodes []nodeAssignment) map[string][]string {
	hostPorts := make(map[string][]string)
	for _, n := range nodes {
		hostPorts[n.host] = append(hostPorts[n.host], strconv.Itoa(n.port))
	}
	return hostPorts
}

// nodesByHost groups the node assignments by host alias.
func nodesByHost(nodes []nodeAssignment) map[string][]nodeAssignment {
	hostNodes := make(map[string][]nodeAssignment)
	for _, n := range nodes {
		hostNodes[n.host] = append(hostNodes[n.host], n)
	}
	return hostNodes
}

// checkPortsFree verifies that every node's listen port is free on its host.
// A node that cannot bind its port makes all peers wait out the full readiness
// deadline (2 minutes per run); this preflight turns that silent stall into an
// immediate error naming the busy port and, when visible, the process holding
// it. Run it after killLingering, which clears this sweep's own processes; a
// remaining listener belongs to someone else (e.g. a concurrent sweep).
func checkPortsFree(g iago.Group, nodes []nodeAssignment) error {
	hostPorts := hostPortsByHost(nodes)
	return run(withTimeout(g, 30*time.Second), "check ports", func(ctx context.Context, host iago.Host) error {
		ports := hostPorts[host.Name()]
		if len(ports) == 0 {
			return nil
		}
		out, err := iago.Output(ctx, host, portCheckScript(ports))
		if err != nil {
			// The script's own [ -z "$busy" ] exits 1 when it found busy
			// ports (an ExitStatus error); anything else — a dial failure, a
			// dropped session — never ran the check at all, so it must not
			// be reported as "port(s) in use".
			if _, ok := errors.AsType[iago.ExitStatus](err); ok {
				return fmt.Errorf("%s: port(s) in use: %s", host.Name(), oneLine(out))
			}
			return fmt.Errorf("%s: port check failed: %w", host.Name(), err)
		}
		return nil
	})
}

// collectFailureDiag gathers a host snapshot from every host of a failed run
// and writes the per-host output to <outdir>/logs/<base>_snapshot.txt. The
// snapshot records load and fd limits (was the host overloaded?), benchmark
// processes still running (slow nodes lingering?), and the socket state on each
// node's port (listeners and TIME_WAIT connections to peers that already
// exited), which together explain post-mortem why a run failed. It is
// best-effort: any error is logged and suppressed so a diagnostic failure never
// masks the original run failure. It runs after the run fails, so most processes
// have exited, but TIME_WAIT sockets persist long enough to remain useful.
func collectFailureDiag(g iago.Group, nodes []nodeAssignment, prog remoteProgram, outdir, base string) {
	hostPorts := hostPortsByHost(nodes)

	dir := filepath.Join(outdir, logSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("  warning: failure snapshot: %v", err)
		return
	}
	f, err := os.Create(snapshotPath(outdir, base))
	if err != nil {
		log.Printf("  warning: failure snapshot: %v", err)
		return
	}
	defer f.Close()

	// Per-host blobs are written under a mutex so concurrent hosts do not
	// interleave their output in the shared snapshot file.
	var mu sync.Mutex
	run(withTimeout(g, 30*time.Second), "failure snapshot", func(ctx context.Context, host iago.Host) error {
		ports := hostPorts[host.Name()]
		if len(ports) == 0 {
			return nil
		}
		out, shErr := iago.Output(ctx, host, failureDiagCommand(ports, prog))
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(f, "===== %s =====\n", host.Name())
		if shErr != nil {
			fmt.Fprintf(f, "(snapshot failed: %v)\n\n", shErr)
			return nil
		}
		io.WriteString(f, out)
		fmt.Fprintln(f)
		return nil
	})
}

// isSignalTerm reports whether err is an SSH exit caused by SIGTERM (status 143).
// This occurs when the SSH session is closed while the remote process has already
// exited, causing a spurious SIGTERM delivery.
func isSignalTerm(err error) bool {
	exitErr, ok := errors.AsType[iago.ExitStatus](err)
	return ok && exitErr.ExitStatus() == 143
}

// launchAndWait starts the benchmark on all assigned nodes and waits for all
// of them to finish. Both stdout and stderr from each remote process are streamed
// to the local log. Leaving either pipe unread exhausts the SSH channel window
// and blocks remote goroutines.
func launchAndWait(g iago.Group, nodes []nodeAssignment, peers string, spec runSpec, base string, cfg *config) error {
	hostNodes := nodesByHost(nodes)
	for _, h := range g.Hosts {
		if _, ok := hostNodes[h.Name()]; !ok {
			hostNodes[h.Name()] = nil
		}
	}

	// Node output streams to a per-run logger (console + <outdir>/logs/<base>.log)
	// instead of sweep.log, so one experiment's output across all nodes can be
	// read in isolation. Fall back to console-only if the log file cannot be
	// created; node output stays visible and the run proceeds.
	runLogger, closeLog, err := newRunLogger(cfg.outDir, base)
	if err != nil {
		log.Printf("  warning: per-run log: %v", err)
		runLogger = log.New(os.Stderr, "", log.LstdFlags)
		closeLog = func() error { return nil }
	}
	defer closeLog()

	launchTimeout := 5*time.Minute + cfg.duration
	return run(withTimeout(g, launchTimeout), "run benchmark", func(ctx context.Context, host iago.Host) error {
		myNodes := hostNodes[host.Name()]
		log.Printf("  -> launching %d node(s) on %s...", len(myNodes), host.Name())

		// Each node runs via RunContext, not Start+Wait, so the launchTimeout
		// deadline is actually enforced: RunContext closes the SSH session when
		// ctx fires, unblocking the wait even if the remote benchmark has hung
		// (a bug in the benchmark, a wedged teardown, a lost peer). Start+Wait
		// ignores ctx entirely — session.Wait() blocks until the remote process
		// exits — so a single hung node would wedge the whole sweep indefinitely
		// (the driver never advances and never writes exit.code). A node left
		// running after the timeout is reaped by the next
		// run's killLingering. Nodes run concurrently so one host's timeout does
		// not serialize behind another's.
		errs := make([]error, len(myNodes))
		var wg sync.WaitGroup
		// Wait for every already-launched node on the way out, including a
		// setup failure partway through the loop below: otherwise an earlier
		// node's goroutines (and its two drain readers) outlive this
		// function, writing to runLogger after closeLog has closed the file.
		defer wg.Wait()
		for i, node := range myNodes {
			cmd, err := host.NewCommand()
			if err != nil {
				return err
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				return err
			}
			stderr, err := cmd.StderrPipe()
			if err != nil {
				return err
			}
			stdoutDone := make(chan struct{})
			stderrDone := make(chan struct{})
			go drain(stdout, node.host, node.port, runLogger, stdoutDone)
			go drain(stderr, node.host, node.port, runLogger, stderrDone)

			wg.Go(func() {
				// io.EOF is the benign session-close signal, not a run failure
				// (see iago.Shell.Apply, which applies the same filter).
				if err := cmd.RunContext(ctx, buildNodeCmd(node, peers, spec, base, cfg)); err != nil && err != io.EOF {
					errs[i] = fmt.Errorf("node %s:%d: %w", node.host, node.port, err)
				}
				<-stdoutDone
				<-stderrDone
			})
		}
		wg.Wait()

		joined := errors.Join(errs...)
		if joined == nil {
			log.Printf("  ok finished %d node(s) on %s", len(myNodes), host.Name())
		}
		return joined
	})
}

// collectResults downloads each node's per-run artifacts (one file per
// extension in exts; the result file, plus profiles when collected) from the
// remote host via SSH (cat over the existing connection) and writes them to
// outdir. Files keep their per-node names so the binary converter and the
// report generator can locate them. A file missing on the remote host is
// logged as a warning
// rather than aborting the collection, so a run with injected faults
// (-fault-kill-after) still yields the surviving nodes' results; the summary
// then reports what was collected.
func collectResults(g iago.Group, base string, nodes []nodeAssignment, cfg *config, exts []string) error {
	hostNodes := nodesByHost(nodes)

	return run(withTimeout(g, 5*time.Minute), "collect results", func(ctx context.Context, host iago.Host) error {
		for _, node := range hostNodes[host.Name()] {
			for _, ext := range exts {
				filename := resultFilename(base, node, ext)
				remotePath := filepath.Join(node.remoteDir(cfg), filename)
				log.Printf("  -> collecting %s from %s...", filename, host.Name())
				// Check first, distinguishing a missing file (salvage the
				// rest of the run) from a transport failure (fail the run):
				// a dropped connection during the cat below would otherwise
				// look identical to a nonexistent file, and the run would be
				// recorded succeeded with silently under-counted results.
				exists, err := iago.FileExists(ctx, host, remotePath)
				if err != nil {
					return fmt.Errorf("checking %s on %s: %w", filename, host.Name(), err)
				}
				if !exists {
					log.Printf("  warning: missing %s on %s", filename, host.Name())
					continue
				}
				localPath := filepath.Join(cfg.outDir, filename)
				f, err := os.Create(localPath)
				if err != nil {
					return err
				}
				err = iago.Shell{
					Command: "cat " + iago.Quote(remotePath),
					Stdout:  f,
				}.Apply(ctx, host)
				err = errors.Join(err, f.Close())
				if err != nil {
					// Drop the empty local file so downstream consumers see a
					// missing node, not a corrupt result.
					_ = os.Remove(localPath)
					if _, ok := errors.AsType[iago.ExitStatus](err); ok {
						// The file passed the existence check above but cat
						// still failed (e.g. removed in between, or
						// unreadable): salvage, matching the missing-file case.
						log.Printf("  warning: %s on %s could not be read: %v", filename, host.Name(), err)
						continue
					}
					return fmt.Errorf("collecting %s from %s: %w", filename, host.Name(), err)
				}
				log.Printf("  ok collected %s", filename)
			}
		}
		return nil
	})
}

// cleanup removes the deployed binary and only the result files this sweep
// created from all hosts. remoteFiles maps each host name to the absolute remote
// paths of the result files produced on that host; an unscoped remote glob is
// avoided so a concurrent sweep's (or another user's) output is never deleted.
// maxCleanupCmdBytes bounds each "rm -f" command's total length, with margin
// under Linux's 128 KiB MAX_ARG_STRLEN limit on a single argument to the
// remote shell (sshd execs "sh -c <command>"). A long sweep with
// -collect-profiles can reach thousands of result paths per host; without
// chunking, one oversized command fails cleanup wholesale, leaving the
// binary and every result file behind.
const maxCleanupCmdBytes = 64 * 1024

func cleanup(g iago.Group, cfg *config, remoteFiles map[string][]string) error {
	return run(withTimeout(g, time.Minute), "cleanup", func(ctx context.Context, host iago.Host) error {
		paths := append([]string{cfg.prog.path(cfg.remoteDirs[host.Name()])}, remoteFiles[host.Name()]...)
		for i := range paths {
			paths[i] = iago.Quote(paths[i])
		}
		for _, chunk := range chunkByLength(paths, maxCleanupCmdBytes) {
			if err := (iago.Shell{Command: "rm -f " + strings.Join(chunk, " ")}).Apply(ctx, host); err != nil {
				return err
			}
		}
		return nil
	})
}

// chunkByLength splits items into consecutive groups whose total joined
// length (one separating space between items) stays within maxBytes. Every
// group holds at least one item, even one that alone exceeds maxBytes, so no
// item is ever dropped.
func chunkByLength(items []string, maxBytes int) [][]string {
	var chunks [][]string
	var current []string
	size := 0
	for _, item := range items {
		if len(current) > 0 && size+1+len(item) > maxBytes {
			chunks = append(chunks, current)
			current = nil
			size = 0
		}
		if len(current) > 0 {
			size++ // separating space
		}
		current = append(current, item)
		size += len(item)
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// buildNodeCmd constructs the remote command string for one benchmark node.
// The binary is expected at cfg.prog.path() on the remote host.
//
// Required flags that the benchmark binary must support:
//
//	-self=host:port       this node's listen address (triggers distributed mode)
//	-remotes=addr,...     comma-separated peer addresses
//	-benchmarks=^name$    exact benchmark name (sweep anchors and QuoteMeta-escapes each entry)
//	-workers=N            concurrent goroutines
//	-payload=N            payload size in bytes
//	-time=duration        measurement duration
//	-output=path          output result file path
//	-rate=N               target sends/sec per node; 0 = unlimited (optional)
//	-send-buffer=N        per-node send queue capacity (optional)
//	-recv-buffer=N        server receive queue capacity (optional)
//	-verbose              verbose logging (optional)
//
// The pass-through flags (-interval, -stats-mode, -rate-step,
// -rate-step-max), -stream-mode (only for dedup), the per-node profile paths
// (-cpuprofile/-memprofile, when -collect-profiles is set), and -extra-args
// are appended only when set, so a binary without them keeps working as long
// as the sweep does not ask for them.
func buildNodeCmd(node nodeAssignment, peers string, spec runSpec, base string, cfg *config) string {
	remoteDir := node.remoteDir(cfg)
	output := filepath.Join(remoteDir, resultFilename(base, node, resultExt))
	parts := []string{
		iago.Quote(cfg.prog.path(remoteDir)),
		fmt.Sprintf("-self=%s", node.peerAddr()),
		fmt.Sprintf("-remotes=%s", peers),
		fmt.Sprintf("-benchmarks=%s", iago.Quote("^"+regexp.QuoteMeta(spec.Benchmark)+"$")),
		fmt.Sprintf("-workers=%d", spec.Workers),
		fmt.Sprintf("-payload=%d", spec.Payload),
		fmt.Sprintf("-time=%s", cfg.duration),
		fmt.Sprintf("-output=%s", iago.Quote(output)),
	}
	if spec.Rate > 0 {
		parts = append(parts, fmt.Sprintf("-rate=%d", spec.Rate))
	}
	if spec.SendBuffer != 0 {
		parts = append(parts, fmt.Sprintf("-send-buffer=%d", spec.SendBuffer))
	}
	if spec.RecvBuffer != 0 {
		parts = append(parts, fmt.Sprintf("-recv-buffer=%d", spec.RecvBuffer))
	}
	if cfg.interval != "" {
		parts = append(parts, fmt.Sprintf("-interval=%s", cfg.interval))
	}
	if cfg.statsMode != "" {
		parts = append(parts, fmt.Sprintf("-stats-mode=%s", cfg.statsMode))
	}
	if cfg.rateStep > 0 {
		parts = append(parts, fmt.Sprintf("-rate-step=%d", cfg.rateStep))
	}
	if cfg.rateStepMax > 0 {
		parts = append(parts, fmt.Sprintf("-rate-step-max=%d", cfg.rateStepMax))
	}
	// Only dedup is passed through: dual is the binary default, and baseline
	// runs a prebuilt binary from before the -stream-mode flag existed.
	if spec.StreamMode == "dedup" {
		parts = append(parts, fmt.Sprintf("-stream-mode=%s", spec.StreamMode))
	}
	if cfg.collectProfiles {
		parts = append(parts,
			fmt.Sprintf("-cpuprofile=%s", iago.Quote(filepath.Join(remoteDir, resultFilename(base, node, cpuProfExt)))),
			fmt.Sprintf("-memprofile=%s", iago.Quote(filepath.Join(remoteDir, resultFilename(base, node, memProfExt)))))
	}
	if cfg.verbose {
		parts = append(parts, "-verbose")
	}
	if cfg.extraArgs != "" {
		parts = append(parts, cfg.extraArgs)
	}
	cmd := strings.Join(parts, " ")
	if stmt := fdLimitStmt(cfg.fdLimit); stmt != "" {
		// The node runs through the remote login shell, so a ulimit builtin ahead
		// of it raises the soft open-file limit for the benchmark process. The
		// exit status of the "stmt; cmd" sequence is the benchmark's, so cmd.Wait
		// still observes the node's real result.
		cmd = stmt + "; " + cmd
	}
	return cmd
}

func (n nodeAssignment) remoteDir(cfg *config) string {
	if dir := cfg.remoteDirs[n.host]; dir != "" {
		return dir
	}
	// Tests and callers constructing configs directly retain the historical
	// location unless they opt into a configured namespace.
	return "/tmp"
}

// fdLimitStmt returns a shell statement that raises the soft open-file limit to
// n descriptors, or "" when n <= 0. The 2>/dev/null suppresses the shell's error
// if n exceeds the hard limit; in that case the limit is left unchanged and the
// command that follows still runs. A large benchmark mesh opens many concurrent
// connections per node, so the common 1024 soft default is easily exhausted.
func fdLimitStmt(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("ulimit -Sn %d 2>/dev/null", n)
}

// drain reads rc line-by-line and logs each line prefixed with host:port using
// the given per-run logger, which writes to the console and the run's log file
// rather than to sweep.log. Closes done when the reader is exhausted.
//
// The scanner's buffer is grown well past bufio.Scanner's 64 KiB default (to
// match offsets.go's own scan of this same log content): a line past the
// default cap stops the scan with bufio.ErrTooLong, and launchAndWait's own
// doc explains why that must not leave rc unread — the SSH channel window
// fills and the remote process blocks on its next write, wedging the run
// until the launch timeout. On any scan error, keep draining rc to
// io.Discard so the channel stays readable even though lines can no longer
// be logged.
func drain(rc io.ReadCloser, host string, port int, logger *log.Logger, done chan struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		logger.Printf("[%s:%d] %s", host, port, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logger.Printf("[%s:%d] pipe read error: %v", host, port, err)
		_, _ = io.Copy(io.Discard, rc)
	}
}

// logSubdir is the subdirectory under the output directory that holds per-run
// node logs, one <base>.log per run. Keeping them out of the top level leaves
// the output directory dominated by result files and manifests.
const logSubdir = "logs"

// runLogPath returns the per-run node log path for base under outdir.
func runLogPath(outdir, base string) string {
	return filepath.Join(outdir, logSubdir, base+".log")
}

// snapshotPath returns the failure-diagnostic snapshot path for base under
// outdir; see collectFailureDiag.
func snapshotPath(outdir, base string) string {
	return filepath.Join(outdir, logSubdir, base+"_snapshot.txt")
}

// newRunLogger creates the per-run node log <outdir>/logs/<base>.log and returns
// a logger writing to both that file and the console, plus a close function.
// Each run's node output (all nodes interleaved chronologically) is isolated in
// its own file so one experiment can be inspected across all nodes without
// wading through every other run's output; sweep.log keeps only orchestration.
func newRunLogger(outdir, base string) (*log.Logger, func() error, error) {
	dir := filepath.Join(outdir, logSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.Create(runLogPath(outdir, base))
	if err != nil {
		return nil, nil, err
	}
	logger := log.New(io.MultiWriter(os.Stderr, f), "", log.LstdFlags)
	return logger, f.Close, nil
}
