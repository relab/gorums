package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// verifyOptions states what a healthy smoke run looks like for [verifyRun].
type verifyOptions struct {
	label    string        // sweep label; manifests are <label>_….manifest.json
	specs    []runSpec     // expected runs, from sweepConfig.runSpecs
	hosts    int           // expected distinct hosts per run (the -test N value); 0 skips the check
	maxDrift time.Duration // largest allowed absolute clock drift
}

// verifyFinding is one failed smoke criterion.
type verifyFinding struct {
	criterion string // "runs", "status", "hosts", "throughput", "latency", or "drift"
	run       string // run base name or configuration, or "" for sweep-wide findings
	detail    string // counts, statuses, errors, values, and hosts
}

// verifyRun checks the collected run directory dir against opts:
//
//   - the run manifests in dir are exactly the expected runs, and
//     report/agg.csv exists with a row for every expected configuration;
//   - every manifest has status succeeded;
//   - every run used opts.hosts distinct hosts;
//   - every agg.csv row has throughput and a p50 latency above zero;
//   - the largest absolute clock drift in the node logs is at most opts.maxDrift.
//
// It reads the manifests at the top level of dir, where every -collect mode
// puts them; copies in subdirectories, such as a -collect-now snapshot, are
// ignored. A manifest it cannot read or parse is a finding. It returns the
// failed criteria as findings, and the names of the criteria it could not check
// (hosts when opts.hosts is 0, drift when the logs have no offset lines). The
// error reports only I/O and parse failures.
func verifyRun(dir string, opts verifyOptions) (findings []verifyFinding, skipped []string, err error) {
	if len(opts.specs) == 0 {
		return nil, nil, errors.New("verify: no expected runs")
	}
	manifests, unreadable, err := readRunManifests(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, base := range unreadable {
		findings = append(findings, verifyFinding{"runs", base, "unreadable manifest"})
	}

	present := 0
	expected := make(map[string]bool, len(opts.specs))
	for _, spec := range opts.specs {
		base := runBase(opts.label, spec)
		expected[base] = true
		m, ok := manifests[base]
		if !ok {
			findings = append(findings, verifyFinding{"runs", base, "no manifest"})
			continue
		}
		present++
		if m.Status != runStatusSucceeded {
			findings = append(findings, verifyFinding{"status", base, statusDetail(m)})
		}
		if opts.hosts > 0 {
			if got := distinctHosts(m); got != opts.hosts {
				findings = append(findings, verifyFinding{"hosts", base, fmt.Sprintf("%d of %d hosts", got, opts.hosts)})
			}
		}
	}
	if present < len(opts.specs) {
		findings = append(findings, verifyFinding{"runs", "", fmt.Sprintf("%d of %d runs have a manifest", present, len(opts.specs))})
	}
	for _, base := range slices.Sorted(maps.Keys(manifests)) {
		if !expected[base] {
			findings = append(findings, verifyFinding{"runs", base, "unexpected run (status " + manifests[base].Status + ")"})
		}
	}
	if opts.hosts <= 0 {
		skipped = append(skipped, "hosts")
	}

	aggFindings, err := verifyAggCSV(filepath.Join(dir, reportSubdir, "agg.csv"), opts.specs)
	if err != nil {
		return nil, nil, err
	}
	findings = append(findings, aggFindings...)

	samples, err := collectOffsets(filepath.Join(dir, logSubdir))
	if err != nil {
		return nil, nil, err
	}
	if len(samples) == 0 {
		skipped = append(skipped, "drift")
	} else {
		maxUS := 0.0
		for _, s := range samples {
			maxUS = max(maxUS, s.driftUS)
		}
		if drift := time.Duration(maxUS * float64(time.Microsecond)); drift > opts.maxDrift {
			findings = append(findings, verifyFinding{"drift", "", fmt.Sprintf("max drift %v > %v", drift, opts.maxDrift)})
		}
	}
	return findings, skipped, nil
}

// readRunManifests reads every run manifest at the top level of dir, keyed by
// run base name. Unlike the report's reader, it does not skip a manifest it
// cannot read or parse: it returns that run's base name in unreadable, so the
// smoke check fails instead of losing the run.
func readRunManifests(dir string) (manifests map[string]runManifest, unreadable []string, err error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*"+manifestSuffix))
	if err != nil {
		return nil, nil, err
	}
	manifests = make(map[string]runManifest, len(paths))
	for _, path := range paths {
		base := strings.TrimSuffix(filepath.Base(path), manifestSuffix)
		data, err := os.ReadFile(path)
		var m runManifest
		if err == nil {
			err = json.Unmarshal(data, &m)
		}
		if err != nil {
			unreadable = append(unreadable, base)
			continue
		}
		manifests[base] = m
	}
	return manifests, unreadable, nil
}

// distinctHosts counts the distinct hosts that ran the nodes of m.
func distinctHosts(m runManifest) int {
	addrs := m.Hosts
	if len(m.NodeMap) > 0 {
		addrs = nil
		for _, e := range m.NodeMap {
			addrs = append(addrs, e.Host)
		}
	}
	hosts := map[string]bool{}
	for _, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		hosts[host] = true
	}
	return len(hosts)
}

// statusDetail describes the status of m for a finding: the status, the first
// line of the run's error, and the degraded nodes by host:port, e.g.
// "status degraded (bb2:9000, bb5:9000)".
func statusDetail(m runManifest) string {
	detail := "status " + m.Status
	if line, _, _ := strings.Cut(strings.TrimSpace(m.Error), "\n"); line != "" {
		detail += ": " + line
	}
	var hosts []string
	for _, d := range m.DegradedNodes {
		hosts = append(hosts, d.Host)
	}
	if len(hosts) > 0 {
		detail += " (" + strings.Join(hosts, ", ") + ")"
	}
	return detail
}

// verifyAggCSV checks the agg.csv at path: it must have a row for the
// configuration of every expected run, and every row must have throughput and
// a p50 latency above zero. A run that succeeded without measurements has no
// row.
func verifyAggCSV(path string, specs []runSpec) ([]verifyFinding, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return []verifyFinding{{"runs", "", "report/agg.csv missing"}}, nil
	}
	var findings []verifyFinding
	var rows []map[string]string // dimension column → value, one per agg row
	err := forEachCSVRow(path, false, func(record []string, cols map[string]int) error {
		field := func(name string) string {
			if i, ok := cols[name]; ok && i < len(record) {
				return record[i]
			}
			return ""
		}
		dims := map[string]string{}
		for _, name := range dimensionColumns() {
			dims[name] = field(name)
		}
		rows = append(rows, dims)
		config := fmt.Sprintf("%s N%s S%s", field("benchmark"), field("nodes"), field("stream_mode"))
		if v := field("throughput"); !positive(v) {
			findings = append(findings, verifyFinding{"throughput", config, "throughput " + cmp.Or(v, "-")})
		}
		if !positive(field("p50_us")) {
			findings = append(findings, verifyFinding{"latency", config, "no p50 latency"})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	reported := map[string]bool{}
	for _, spec := range specs {
		key := strings.Join(dimensionValues(spec.Dimensions), ",")
		if reported[key] || slices.ContainsFunc(rows, func(row map[string]string) bool { return aggRowMatches(row, spec) }) {
			continue
		}
		reported[key] = true
		config := fmt.Sprintf("%s N%d S%s", spec.Benchmark, spec.Nodes, spec.StreamMode)
		findings = append(findings, verifyFinding{"runs", config, "no agg.csv row"})
	}
	return findings, nil
}

// aggRowMatches reports whether an agg.csv row, given as dimension column →
// value, holds the configuration of spec. A spec buffer size of 0 selects the
// binary's default, and the row records the size the binary used, so 0 matches
// any send_buffer or recv_buffer value.
func aggRowMatches(row map[string]string, spec runSpec) bool {
	want := dimensionValues(spec.Dimensions)
	for i, name := range dimensionColumns() {
		if (name == "send_buffer" || name == "recv_buffer") && want[i] == "0" {
			continue
		}
		if row[name] != want[i] {
			return false
		}
	}
	return true
}

// runVerify runs the -verify mode: it checks cfg.verifyDir against the runs the
// sweep flags in cfg describe, logs the verdict, and writes it to
// cfg.summaryPath when set. It returns whether every criterion passed.
func runVerify(cfg *config) (bool, error) {
	opts := verifyOptions{
		label:    cfg.sweepLabel,
		specs:    slices.Collect(cfg.sweep.runSpecs()),
		hosts:    cfg.verifyHosts,
		maxDrift: cfg.maxDrift,
	}
	findings, skipped, err := verifyRun(cfg.verifyDir, opts)
	if err != nil {
		return false, err
	}
	var summary strings.Builder
	if err := writeVerifySummary(&summary, findings, skipped); err != nil {
		return false, err
	}
	log.Printf("verify %s:\n%s", displayPath(cfg.verifyDir), summary.String())
	if cfg.summaryPath != "" {
		if err := os.WriteFile(cfg.summaryPath, []byte(summary.String()), 0o644); err != nil {
			return false, err
		}
	}
	return len(findings) == 0, nil
}

// writeVerifySummary writes the verdict as Markdown: a PASS or FAIL heading, a
// table of the findings, and the criteria that could not be checked.
func writeVerifySummary(w io.Writer, findings []verifyFinding, skipped []string) error {
	verdict := "PASS"
	if len(findings) > 0 {
		verdict = "FAIL"
	}
	cell := strings.NewReplacer("|", `\|`, "\n", " ").Replace
	var b strings.Builder
	fmt.Fprintf(&b, "# Smoke verification: %s\n", verdict)
	if len(findings) > 0 {
		b.WriteString("\n| Criterion | Run | Detail |\n| --- | --- | --- |\n")
		for _, f := range findings {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", cell(f.criterion), cell(cmp.Or(f.run, "all")), cell(f.detail))
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(&b, "\nNot checked (no data or disabled): %s.\n", strings.Join(skipped, ", "))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// positive reports whether s parses as a number above zero.
func positive(s string) bool {
	v, err := strconv.ParseFloat(s, 64)
	return err == nil && v > 0
}
