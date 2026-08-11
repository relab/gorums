package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// defaultExplainMaxLog caps the node log included in the triage prompt. The log
// is sent as a head+tail window so the startup phase and the final errors both
// survive; the default keeps a typical prompt within a local model's context.
const defaultExplainMaxLog = 64 << 10 // 64 KiB

// explainTimeout bounds a single model request.
const explainTimeout = 2 * time.Minute

// explainCheckTimeout bounds the connectivity check, which sends a trivial
// prompt and so should answer far faster than a real diagnosis.
const explainCheckTimeout = 30 * time.Second

// explainCheckSystem and explainCheckUser form a minimal prompt that any working
// model answers in a few tokens. The check cares only that a non-empty reply
// comes back, not what it says, so the prompt is kept tiny to stay fast.
const (
	explainCheckSystem = "You are a connectivity check."
	explainCheckUser   = "Reply with the single word OK."
)

// pingProvider sends the minimal check prompt and returns the trimmed reply,
// treating a reachable model that returns an empty reply as a failure. It backs
// both -explain-check and the pre-sweep preflight.
func pingProvider(ctx context.Context, p llmProvider) (string, error) {
	reply, err := p.Diagnose(ctx, explainCheckSystem, explainCheckUser)
	if err != nil {
		return "", err
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", fmt.Errorf("model returned an empty reply")
	}
	return reply, nil
}

// runExplainCheck builds the configured provider and verifies it answers a
// trivial prompt, printing the model, round-trip latency, and reply. It backs
// the -explain-check flag, so any misconfiguration (unknown provider, missing
// key, unreachable endpoint, empty reply) surfaces on demand without waiting for
// a failed run to triage.
func runExplainCheck(cfg *config) error {
	provider, err := newProvider(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), explainCheckTimeout)
	defer cancel()
	start := time.Now()
	reply, err := pingProvider(ctx, provider)
	if err != nil {
		return fmt.Errorf("%s/%s: %w", cfg.explainProvider, cfg.explainModel, err)
	}
	log.Printf("explain check OK: %s/%s replied in %s: %q",
		cfg.explainProvider, cfg.explainModel, time.Since(start).Round(time.Millisecond), reply)
	return nil
}

// salientKeywords flag log lines worth surfacing to the model even when they
// fall in the elided middle of a trimmed log. The match is case-insensitive
// substring; over-inclusion (an occasional benign line) is preferable to losing
// the one line that explains the failure. "not ready", "stall", and "127.0.1.1"
// are the fingerprints of the Type A AwaitReady failures; "refused", "timeout",
// and "deadline" mark the Type B connection errors.
var salientKeywords = []string{
	"error", "warn", "fail", "panic", "fatal",
	"refused", "timeout", "timed out", "deadline", "cancel", "incomplete", "unavailable",
	"not ready", "stall", "unreachable", "127.0.1.1",
}

// explainSystemPrompt primes the model with the benchkit failure taxonomy and
// one worked example, then states the required output. It mirrors the reasoning
// in doc/benchkit-troubleshooting.html, distilled to plain text because the doc
// is outside the sweep module and not shipped with the binary; keep the two in
// sync when the taxonomy changes.
const explainSystemPrompt = `You are a distributed-systems benchmark engineer triaging a failed run of the
gorums "benchkit" toolkit. A sweep launches N peer nodes over SSH that form a
full gRPC mesh, run a timed quorum-call benchmark, write per-node result files,
then exit. You are given the run's artifacts and must diagnose the failure.

Failure taxonomy:

  Type A - setup failure (AwaitReady / inbound unreachability). A node cannot
  receive inbound peer connections, so it stalls in AwaitReady waiting for the
  full mesh. After a 20s stall timeout every node cancels and reports "remote
  peers not ready". No result files are produced; the process exits with status
  1. failure_phase is usually "setup". A common root cause: a host binds its
  listener to a loopback address (127.0.1.1 via /etc/hosts) instead of its real
  interface, so peers cannot reach it; the effect is episodic and host-specific.

  Type B - measurement failure (linger too short for completion skew). All
  nodes pass AwaitReady, but during measurement some nodes run slower than
  others. Fast nodes finish, write results, linger briefly, then close their
  listeners and exit. A slow node issuing a later quorum call hits "connection
  refused" or an incomplete-call error. Partial result files are produced (the
  fast nodes succeeded); failure_phase is usually "measurement".

Worked example (Type A): At N=25, all nodes logged "remote peers not ready:
... inbound peers not ready (connected 24/25, missing node 7 ...)". One host
(bb16, node 7) reported connected 2/25 - nearly isolated. Because AwaitReady
needs every node to see all peers, that one host stalled the whole cluster
until the 20s timer fired. Root cause: that host bound its listener to
127.0.1.1:9000 while every other host bound its real address. Fix: bind the
wildcard address. The suspect host is the one with the lowest connected count
and/or the one named as "missing" by its peers.

Diagnose this run in at most ~8 lines of plain text, structured as:
  Failure phase: <setup|measurement|collection, and why>
  Suspect host:  <host:port, or "none/cluster-wide" if no single host stands out>
  Probable cause: <one or two sentences>
  Next step:     <one concrete action to confirm or fix>
Ground every claim in the artifacts. If the evidence is insufficient, say so.`

// failedRun identifies a failed run by its base name, from which the manifest
// and per-run artifact paths are derived.
type failedRun struct {
	base string
}

// triageFailedRuns diagnoses the failed runs in cfg.outDir with the configured
// LLM, printing each verdict and recording it in the run's manifest. It runs on
// whichever host executed the sweep: on the driver for a driven sweep, or on
// the laptop for a local one. It is best-effort — any error is logged and never
// aborts the sweep or the export — because a missing API key or an unreachable
// model must not lose results.
func triageFailedRuns(cfg *config) {
	provider, err := newProvider(cfg)
	if err != nil {
		log.Printf("warning: explain: %v", err)
		return
	}
	runs, err := discoverFailedRuns(cfg.outDir)
	if err != nil {
		log.Printf("warning: explain: %v", err)
		return
	}
	if len(runs) == 0 {
		return
	}
	log.Printf("triaging %d failed run(s) with %s/%s...", len(runs), cfg.explainProvider, cfg.explainModel)
	for _, r := range runs {
		if err := explainRun(cfg, provider, cfg.outDir, r); err != nil {
			log.Printf("  warning: explain %s: %v", r.base, err)
		}
	}
}

// explainRun triages a single failed run: gather artifacts, query the model, and
// print and persist the verdict.
func explainRun(cfg *config, provider llmProvider, dir string, r failedRun) error {
	prompt, err := gatherArtifacts(dir, r.base, cfg.explainMaxLog)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), explainTimeout)
	defer cancel()
	verdict, err := provider.Diagnose(ctx, explainSystemPrompt, prompt)
	if err != nil {
		return err
	}
	fmt.Printf("\n===== diagnosis: %s =====\n%s\n", r.base, verdict)
	if err := updateManifestDiagnosis(dir, r.base, verdict); err != nil {
		return fmt.Errorf("recording diagnosis: %w", err)
	}
	return nil
}

// discoverFailedRuns returns the failed runs in dir, identified by their
// manifests (files ending in manifestSuffix with status "failed"). The base name
// is the manifest filename with the suffix stripped.
func discoverFailedRuns(dir string) ([]failedRun, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*"+manifestSuffix))
	if err != nil {
		return nil, err
	}
	var runs []failedRun
	for _, path := range paths {
		if manifestStatus(path) != runStatusFailed {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(path), manifestSuffix)
		runs = append(runs, failedRun{base: base})
	}
	return runs, nil
}

// gatherArtifacts assembles the labeled artifact bundle for one failed run: the
// full manifest, the failure snapshot if present, the trimmed node log, and any
// matching summary rows. Sections that are absent are skipped silently.
func gatherArtifacts(dir, base string, maxLog int) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Run base: %s\n", base)

	// The manifest is mandatory: it carries the failure phase, collected/missing
	// file counts, and the node map.
	manifest, err := os.ReadFile(manifestPath(dir, base))
	if err != nil {
		return "", fmt.Errorf("reading manifest: %w", err)
	}
	appendSection(&b, "manifest.json", string(manifest))

	if snap, err := os.ReadFile(snapshotPath(dir, base)); err == nil {
		appendSection(&b, "host snapshot (logs/"+base+"_snapshot.txt)", string(snap))
	}
	if logData, err := os.ReadFile(runLogPath(dir, base)); err == nil {
		// Grep the whole log first so error/warning lines survive even when they
		// fall in the elided middle of the head+tail window below.
		if notable := salientLog(logData, maxLog); notable != "" {
			appendSection(&b, "notable log lines (errors/warnings across the full log)", notable)
		}
		appendSection(&b, "node log (logs/"+base+".log, head+tail)", trimLog(logData, maxLog))
	}
	if rows := summaryRows(dir, base); rows != "" {
		appendSection(&b, "summary (plotdata.binpb)", rows)
	}
	return b.String(), nil
}

// appendSection writes a labeled, fenced artifact section to b.
func appendSection(b *strings.Builder, title, body string) {
	fmt.Fprintf(b, "\n--- %s ---\n%s\n", title, strings.TrimRight(body, "\n"))
}

// trimLog returns log data unchanged when it fits within maxLog bytes; otherwise
// it keeps the first and last halves of the budget with an elision marker
// between them, so both the startup phase and the final errors survive.
func trimLog(data []byte, maxLog int) string {
	if maxLog <= 0 || len(data) <= maxLog {
		return string(data)
	}
	half := maxLog / 2
	head := data[:half]
	tail := data[len(data)-half:]
	elided := len(data) - 2*half
	return fmt.Sprintf("%s\n... [%d bytes elided] ...\n%s", head, elided, tail)
}

// salientLog returns the log lines matching salientKeywords, in order, capped at
// maxBytes (with a marker noting how many were omitted). It scans the full log,
// so a critical line in the trimmed-away middle still reaches the model. It
// returns "" when nothing matches.
func salientLog(data []byte, maxBytes int) string {
	var b strings.Builder
	var matched, kept int
	for line := range strings.SplitSeq(string(data), "\n") {
		if !isSalient(line) {
			continue
		}
		matched++
		if maxBytes > 0 && b.Len()+len(line)+1 > maxBytes {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		kept++
	}
	if matched == 0 {
		return ""
	}
	if kept < matched {
		fmt.Fprintf(&b, "... [%d more matching line(s) omitted] ...", matched-kept)
	}
	return strings.TrimRight(b.String(), "\n")
}

// isSalient reports whether a log line contains any salientKeyword.
func isSalient(line string) bool {
	lower := strings.ToLower(line)
	return slices.ContainsFunc(salientKeywords, func(kw string) bool {
		return strings.Contains(lower, kw)
	})
}

// summaryRows returns the plotdata.binpb rows for one run's base as CSV text
// (header plus matching rows), for the LLM triage prompt. It reads the
// compact plotdata.binpb directly rather than plotdata/runs.csv: the sweep
// pipeline writes only the binpb by default, so reading the CSV found this
// section empty on every fresh output directory (runs.csv exists only after
// a manual -export-csv).
func summaryRows(dir, base string) string {
	pd, err := readPlotData(dir)
	if err != nil {
		return ""
	}
	runs, _ := plotRecordsFromMessage(pd)
	matched := slices.DeleteFunc(runs, func(r plotRunRecord) bool { return r.base != base })
	if len(matched) == 0 {
		return ""
	}
	var b strings.Builder
	if err := writeCSVTo(&b, plotRunsCSVHeader(), matched, plotRunCSVFields); err != nil {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}
