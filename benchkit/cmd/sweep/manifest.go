package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

// manifestSuffix is the filename suffix of per-run manifests, appended to the
// run base. The report generator discovers runs by this suffix.
const manifestSuffix = ".manifest.json"

const (
	runStatusStarted   = "started"
	runStatusSucceeded = "succeeded"
	runStatusFailed    = "failed"

	// runStatusDegraded marks a run that completed with all result files
	// collected but where at least one node's throughput fell below the
	// -degraded-below fraction of the run median (see degraded.go). Its data
	// is intact and flows into the per-node plot data for diagnosis, but the
	// aggregate is contaminated by the slow node, so headline plots treat it
	// like a failure.
	runStatusDegraded = "degraded"
)

// Failure phases recorded in a failed run's manifest. They let a reader tell,
// without scanning sweep.log, whether a failed run produced any usable results:
//
//   - setup: the run failed before any node wrote a result file (a port
//     conflict, or a launch/AwaitReady failure that left zero result files).
//   - measurement: nodes ran but some failed mid-benchmark, so only a subset of
//     result files were collected.
//   - collection: all nodes finished but the result files could not be
//     downloaded.
const (
	failurePhaseSetup       = "setup"
	failurePhaseMeasurement = "measurement"
	failurePhaseCollection  = "collection"
)

// runManifest describes one sweep run: how it was configured and which
// per-node result files it is expected to produce. sweep writes it as
// <base>.manifest.json into the output directory before launching the nodes,
// so a results directory is self-describing even when a run fails, and
// the report generator can group per-node files without parsing
// filenames.
type runManifest struct {
	runSpec
	Label     string `json:"label"`               // sweep label prefix
	Duration  string `json:"duration"`            // measurement duration
	Trim      string `json:"trim,omitempty"`      // summary trim offset; consumers apply the same trim
	Timestamp string `json:"timestamp"`           // RFC 3339 launch time
	Completed string `json:"completed,omitempty"` // RFC 3339 completion time
	Status    string `json:"status"`              // started, succeeded, or failed
	Error     string `json:"error,omitempty"`     // failure summary when status is failed

	FailurePhase   string   `json:"failure_phase,omitempty"`   // setup, measurement, or collection
	CollectedFiles int      `json:"collected_files,omitempty"` // result files present at completion
	MissingFiles   []string `json:"missing_files,omitempty"`   // expected result files that are absent

	DegradedNodes []degradedNode `json:"degraded_nodes,omitempty"` // nodes below -degraded-below of the run median

	// TCPStats holds per-host TCP counter deltas over the run (host alias →
	// counter → increase), recorded for every run as loss forensics; see
	// tcpstats.go. A host with no advanced counters is omitted.
	TCPStats map[string]map[string]uint64 `json:"tcp_stats,omitempty"`

	Diagnosis string `json:"diagnosis,omitempty"` // LLM triage verdict from sweep -explain

	GitSHA  string         `json:"git_sha,omitempty"`  // repository HEAD, best effort
	Binary  string         `json:"binary,omitempty"`   // deployed binary path
	Hosts   []string       `json:"hosts"`              // host:port per node
	Files   []string       `json:"files"`              // per-node result file basenames
	NodeMap []nodeMapEntry `json:"node_map,omitempty"` // per-node alias, peer address, and Gorums ID
}

type nodeMapEntry struct {
	ID          uint32 `json:"id"`           // Gorums node ID after peer-address sorting
	Host        string `json:"host"`         // SSH-alias host:port used for artifacts
	PeerAddress string `json:"peer_address"` // advertised benchmark peer address
	File        string `json:"file"`         // expected result file basename
}

// trimString renders the sweep's -trim for the manifest: the duration string
// when set, empty (omitted from the JSON) when zero.
func trimString(trim time.Duration) string {
	if trim <= 0 {
		return ""
	}
	return trim.String()
}

// writeManifest writes the manifest for one run to <outdir>/<base>.manifest.json.
// Failures are logged, not fatal: the manifest is a convenience for consumers,
// and the run itself proceeds without it.
func writeManifest(outdir, base string, spec runSpec, nodes []nodeAssignment, cfg *config, gitSHA, binary string) {
	if spec.StreamMode == "" {
		spec.StreamMode = "dual"
	}
	m := runManifest{
		runSpec:   spec,
		Label:     cfg.sweepLabel,
		Duration:  cfg.duration.String(),
		Trim:      trimString(cfg.trim),
		Timestamp: time.Now().Format(time.RFC3339),
		Status:    runStatusStarted,
		GitSHA:    gitSHA,
		Binary:    binary,
	}
	ids := gorumsNodeIDs(nodes)
	for _, n := range nodes {
		hostAddr := n.hostAddr()
		peerAddr := n.peerAddr()
		file := resultFilename(base, n, resultExt)
		m.Hosts = append(m.Hosts, hostAddr)
		m.Files = append(m.Files, file)
		m.NodeMap = append(m.NodeMap, nodeMapEntry{
			ID:          ids[peerAddr],
			Host:        hostAddr,
			PeerAddress: peerAddr,
			File:        file,
		})
	}
	data, err := json.MarshalIndent(&m, "", "  ")
	if err == nil {
		err = os.WriteFile(manifestPath(outdir, base), append(data, '\n'), 0o644)
	}
	if err != nil {
		log.Printf("  warning: manifest: %v", err)
	}
}

func manifestPath(outdir, base string) string {
	return filepath.Join(outdir, base+manifestSuffix)
}

// runOutcome bundles the post-run fields recorded in a manifest by
// updateManifestOutcome. status is required; the remaining fields describe a
// failure (failurePhase, err) and how many of the expected result files were
// collected (collectedFiles, missingFiles), recorded on both success and
// failure so a manifest reader can confirm coverage without scanning the
// output directory.
type runOutcome struct {
	status         string                       // runStatusSucceeded, runStatusDegraded, or runStatusFailed
	err            error                        // failure cause; nil on success
	failurePhase   string                       // setup, measurement, or collection; empty on success
	collectedFiles int                          // result files present in the output directory
	missingFiles   []string                     // expected result file basenames that are absent
	degraded       []degradedNode               // nodes below the degraded threshold; set iff status is degraded
	tcpStats       map[string]map[string]uint64 // per-host TCP counter deltas over the run
}

// updateManifestOutcome records a run's final outcome in its manifest. It reads
// the started manifest, sets the completion time, status, error, failure phase,
// and result-file counts, and rewrites the file.
func updateManifestOutcome(outdir, base string, o runOutcome) error {
	return updateManifest(outdir, base, func(m *runManifest) {
		m.Status = o.status
		m.Completed = time.Now().Format(time.RFC3339)
		if o.err != nil {
			m.Error = oneLine(o.err.Error())
		} else {
			m.Error = ""
		}
		m.FailurePhase = o.failurePhase
		m.CollectedFiles = o.collectedFiles
		m.MissingFiles = o.missingFiles
		m.DegradedNodes = o.degraded
		m.TCPStats = o.tcpStats
	})
}

// updateManifestDiagnosis records an LLM triage verdict in a run's manifest. It
// reads the manifest, sets the diagnosis field, and rewrites the file, leaving
// every other field intact. Re-running sweep -explain overwrites the previous
// verdict rather than appending, so the manifest holds only the latest one.
func updateManifestDiagnosis(outdir, base, diagnosis string) error {
	return updateManifest(outdir, base, func(m *runManifest) {
		m.Diagnosis = diagnosis
	})
}

func updateManifest(outdir, base string, update func(*runManifest)) error {
	path := manifestPath(outdir, base)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m runManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	update(&m)
	data, err = json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// manifestStatus reads a manifest file and returns its status field, or "" if
// the file cannot be read or parsed. Used by the driver launcher to find failed
// runs among the downloaded manifests.
func manifestStatus(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var m runManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	return m.Status
}

// countResultFiles reports how many of a run's expected per-node result files
// are present in outdir, and the basenames of those that are absent. It drives
// failure-phase classification (zero present after a launch failure means the
// run never reached measurement) and records collection coverage on success.
func countResultFiles(outdir, base string, nodes []nodeAssignment) (collected int, missing []string) {
	for _, n := range nodes {
		name := resultFilename(base, n, resultExt)
		if _, err := os.Stat(filepath.Join(outdir, name)); err == nil {
			collected++
		} else {
			missing = append(missing, name)
		}
	}
	return collected, missing
}

func gorumsNodeIDs(nodes []nodeAssignment) map[string]uint32 {
	peers := make([]string, len(nodes))
	for i, n := range nodes {
		peers[i] = n.peerAddr()
	}
	slices.Sort(peers)
	ids := make(map[string]uint32, len(peers))
	for i, peer := range peers {
		ids[peer] = uint32(i + 1)
	}
	return ids
}

// gitHeadSHA returns the repository HEAD commit hash, or "" when unavailable
// (e.g. a sweep run outside a git checkout).
func gitHeadSHA() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// warnIfStaleBinary warns when the running sweep binary was built from a
// different commit than the repository HEAD. Such a binary silently runs
// outdated code (e.g. a fix committed after the binary was last built), which
// is easy to miss because the sweep otherwise proceeds normally.
func warnIfStaleBinary(headSHA string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	var rev string
	if i := slices.IndexFunc(info.Settings, func(s debug.BuildSetting) bool {
		return s.Key == "vcs.revision"
	}); i >= 0 {
		rev = info.Settings[i].Value
	}
	if msg := staleBinaryWarning(rev, headSHA); msg != "" {
		log.Printf("warning: %s", msg)
	}
}

// staleBinaryWarning returns a warning message when binaryRev (the commit the
// binary was built from) differs from headSHA (the repository HEAD), and ""
// when they match or either is unknown.
func staleBinaryWarning(binaryRev, headSHA string) string {
	if binaryRev == "" || headSHA == "" || binaryRev == headSHA {
		return ""
	}
	return fmt.Sprintf("sweep binary built from commit %.12s but repository is at %.12s — rebuild with %q",
		binaryRev, headSHA, rebuildSweepCommand)
}
