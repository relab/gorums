package main

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// verifyFixture describes a collected run directory for the verify tests. The
// zero mutations describe a healthy smoke run: every expected run succeeded on
// all hosts, agg.csv has throughput and latency for every row, and the clock
// drift is tiny.
type verifyFixture struct {
	label     string
	specs     []runSpec
	hosts     []string                // host:port per node, shared by all runs
	manifests map[string]*runManifest // by run base; nil entry = no manifest
	snapshot  map[string]*runManifest // by run base, under snapshot/ (a -collect-now copy)
	raw       map[string]string       // file name → contents, at the top level
	agg       []aggRunRecord
	noAgg     bool
	logs      map[string]string // file name → contents, under logs/
}

// smokeSpecs returns the runs that -test 3 -benchmarks <benchmark>
// -stream-mode dual,dedup produces.
func smokeSpecs(benchmark string) []runSpec {
	sc := sweepConfig{
		numNodes: []int{3}, workers: []int{1}, payloads: []int{0}, rates: []int{0},
		benchmarks: []string{benchmark}, streamModes: []string{"dual", "dedup"}, reps: 1,
	}
	return slices.Collect(sc.runSpecs())
}

func newVerifyFixture(benchmark string) *verifyFixture {
	return newLabeledVerifyFixture("smoke", benchmark)
}

func newLabeledVerifyFixture(label, benchmark string) *verifyFixture {
	f := &verifyFixture{
		label:     label,
		specs:     smokeSpecs(benchmark),
		hosts:     []string{"h1:9000", "h2:9000", "h3:9000"},
		manifests: map[string]*runManifest{},
		snapshot:  map[string]*runManifest{},
		logs: map[string]string{
			label + "_" + benchmark + "_N3_W1_P0_Sdual_r1.log": "[offsets node 1 (10.0.0.1:9000)] peer 2: before=-297µs after=-301µs drift=-3µs\n",
		},
	}
	for _, spec := range f.specs {
		m := &runManifest{runSpec: spec, Label: f.label, Status: runStatusSucceeded, Hosts: f.hosts}
		for i, h := range f.hosts {
			m.NodeMap = append(m.NodeMap, nodeMapEntry{ID: uint32(i + 1), Host: h})
		}
		f.manifests[runBase(f.label, spec)] = m
		f.agg = append(f.agg, aggRunRecord{
			Dimensions: spec.Dimensions,
			reps:       1,
			throughput: aggStat{mean: 1000, n: 1},
			p50US:      aggStat{mean: 200, n: 1},
		})
	}
	return f
}

// first returns the manifest of the first expected run.
func (f *verifyFixture) first() *runManifest {
	return f.manifests[runBase(f.label, f.specs[0])]
}

func (f *verifyFixture) write(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeManifests := func(dir string, manifests map[string]*runManifest) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for base, m := range manifests {
			if m == nil {
				continue
			}
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath(dir, base), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeManifests(dir, f.manifests)
	writeManifests(filepath.Join(dir, "snapshot"), f.snapshot)
	for name, contents := range f.raw {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !f.noAgg {
		if err := os.MkdirAll(filepath.Join(dir, reportSubdir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := writeAggRunsCSV(filepath.Join(dir, reportSubdir, "agg.csv"), f.agg); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, logSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range f.logs {
		if err := os.WriteFile(filepath.Join(dir, logSubdir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func (f *verifyFixture) options() verifyOptions {
	return verifyOptions{label: f.label, specs: f.specs, hosts: len(f.hosts), maxDrift: time.Millisecond}
}

// hasFinding reports whether findings has one for criterion whose detail
// contains detail.
func hasFinding(findings []verifyFinding, criterion, detail string) bool {
	return slices.ContainsFunc(findings, func(f verifyFinding) bool {
		return f.criterion == criterion && strings.Contains(f.detail, detail)
	})
}

func TestVerifyRun(t *testing.T) {
	tests := []struct {
		name        string
		label       string // default "smoke"
		benchmark   string
		mutate      func(f *verifyFixture)
		opts        func(o *verifyOptions)
		criterion   string // "" means no findings
		detail      string
		wantSkipped []string
	}{
		{name: "Healthy"},
		{
			name: "NoManifests",
			mutate: func(f *verifyFixture) {
				for base := range f.manifests {
					f.manifests[base] = nil
				}
			},
			criterion: "runs", detail: "0 of 2 runs have a manifest",
		},
		{
			name:      "MissingRun",
			mutate:    func(f *verifyFixture) { f.manifests[runBase(f.label, f.specs[1])] = nil },
			criterion: "runs", detail: "1 of 2 runs have a manifest",
		},
		{
			name:      "StillStarted",
			mutate:    func(f *verifyFixture) { f.first().Status = "started" },
			criterion: "status", detail: "status started",
		},
		{
			name: "Degraded",
			mutate: func(f *verifyFixture) {
				m := f.first()
				m.Status = runStatusDegraded
				m.DegradedNodes = []degradedNode{{Host: "h2:9000", Reason: "throughput below median", Relative: 0.1}}
			},
			criterion: "status", detail: "status degraded",
		},
		{
			name: "Failed",
			mutate: func(f *verifyFixture) {
				m := f.first()
				m.Status = "failed"
				m.Error = "dial h2:9000: connection refused"
			},
			criterion: "status", detail: "status failed",
		},
		{
			name: "DroppedPeer",
			mutate: func(f *verifyFixture) {
				m := f.first()
				m.NodeMap[2].Host = "h1:9001" // node 3 moved onto h1
			},
			criterion: "hosts", detail: "2 of 3 hosts",
		},
		{
			name:      "ZeroThroughput",
			mutate:    func(f *verifyFixture) { f.agg[0].throughput = aggStat{mean: 0, n: 1} },
			criterion: "throughput", detail: "throughput 0",
		},
		{
			name:      "NoLatency",
			benchmark: "SymmetricMulticast",
			mutate:    func(f *verifyFixture) { f.agg[0].p50US = aggStat{} },
			criterion: "latency", detail: "no p50 latency",
		},
		{
			name:      "NoAggCSV",
			mutate:    func(f *verifyFixture) { f.noAgg = true },
			criterion: "runs", detail: "report/agg.csv missing",
		},
		{
			name: "Drift",
			mutate: func(f *verifyFixture) {
				f.logs["smoke_x_N3_W1_P0_Sdedup_r1.log"] = "[offsets node 2 (10.0.0.2:9000)] peer 3: before=10ms after=11ms drift=-1500µs\n"
			},
			criterion: "drift", detail: "max drift 1.5ms > 1ms",
		},
		{
			name:        "NoOffsets",
			mutate:      func(f *verifyFixture) { f.logs = nil },
			wantSkipped: []string{"drift"},
		},
		{
			name:        "HostsCheckOff",
			mutate:      func(f *verifyFixture) { f.first().NodeMap[2].Host = "h1:9001" },
			opts:        func(o *verifyOptions) { o.hosts = 0 },
			wantSkipped: []string{"hosts"},
		},
		{
			// A run the verify flags do not expect, e.g. because the launch and
			// verify flags drifted apart, must not hide a failure.
			name: "UnexpectedRun",
			mutate: func(f *verifyFixture) {
				spec := smokeSpecs("SymmetricMulticast")[0]
				f.manifests[runBase(f.label, spec)] = &runManifest{runSpec: spec, Label: f.label, Status: "failed"}
			},
			criterion: "runs", detail: "unexpected run",
		},
		{
			// A manifest that cannot be parsed cannot be compared with the
			// expected runs, so it must fail the check, not vanish.
			name:      "MalformedExtraManifest",
			mutate:    func(f *verifyFixture) { f.raw = map[string]string{"smoke_bogus" + manifestSuffix: "{"} },
			criterion: "runs", detail: "unreadable manifest",
		},
		{
			name: "MalformedExpectedManifest",
			mutate: func(f *verifyFixture) {
				base := runBase(f.label, f.specs[0])
				f.manifests[base] = nil
				f.raw = map[string]string{base + manifestSuffix: "not json"}
			},
			criterion: "runs", detail: "unreadable manifest",
		},
		{
			// A spec buffer size of 0 selects the binary's default, which the
			// agg row records as the size the binary used (seen in the Docker
			// smoke run: send_buffer 4096).
			name: "AggRecordsBinaryBufferDefaults",
			mutate: func(f *verifyFixture) {
				for i := range f.agg {
					f.agg[i].SendBuffer = 4096
					f.agg[i].RecvBuffer = 1024
				}
			},
		},
		{
			// A run that succeeded but produced no measurements has no agg row.
			name:      "MissingAggRow",
			mutate:    func(f *verifyFixture) { f.agg = f.agg[:1] },
			criterion: "runs", detail: "no agg.csv row",
		},
		{
			// A -collect-now snapshot left under snapshot/ is stale; the
			// top-level manifest is the final one. "test" sorts after "snapshot".
			name:  "StaleSnapshotCopy",
			label: "test",
			mutate: func(f *verifyFixture) {
				stale := *f.first()
				stale.Status = "started"
				f.snapshot[runBase(f.label, f.specs[0])] = &stale
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLabeledVerifyFixture(cmp.Or(tt.label, "smoke"), cmp.Or(tt.benchmark, "SymmetricQuorumCall"))
			if tt.mutate != nil {
				tt.mutate(f)
			}
			opts := f.options()
			if tt.opts != nil {
				tt.opts(&opts)
			}
			findings, skipped, err := verifyRun(f.write(t), opts)
			if err != nil {
				t.Fatalf("verifyRun: %v", err)
			}
			if tt.criterion == "" && len(findings) > 0 {
				t.Errorf("findings = %+v, want none", findings)
			}
			if tt.criterion != "" && !hasFinding(findings, tt.criterion, tt.detail) {
				t.Errorf("findings = %+v, want %s finding containing %q", findings, tt.criterion, tt.detail)
			}
			if !slices.Equal(skipped, tt.wantSkipped) {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
		})
	}
}

// TestVerifySummaryNamesFailures checks that a failing summary names the
// degraded nodes by host:port and gives the first line of a failed run's
// error, with | escaped so the Markdown table keeps its columns.
func TestVerifySummaryNamesFailures(t *testing.T) {
	f := newVerifyFixture("SymmetricQuorumCall")
	failed := f.first()
	failed.Status = "failed"
	failed.Error = "dial 10.0.0.2:9000: connection refused | retry\nsecond line"
	degraded := f.manifests[runBase(f.label, f.specs[1])]
	degraded.Status = runStatusDegraded
	degraded.DegradedNodes = []degradedNode{
		{Host: "192.0.2.19:9000", Reason: "throughput below median", Relative: 0.1},
		{Host: "h3:9000", Reason: "throughput below median", Relative: 0.1},
	}
	dir := f.write(t)

	findings, skipped, err := verifyRun(dir, f.options())
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := writeVerifySummary(&out, findings, skipped); err != nil {
		t.Fatal(err)
	}
	summary := out.String()
	if !strings.HasPrefix(summary, "# Smoke verification: FAIL") {
		t.Errorf("summary does not start with the FAIL heading:\n%s", summary)
	}
	for _, want := range []string{
		`status failed: dial 10.0.0.2:9000: connection refused \| retry |`,
		"status degraded (192.0.2.19:9000, h3:9000)",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "second line") {
		t.Errorf("summary holds more than the first error line:\n%s", summary)
	}
}

func TestVerifySummaryPass(t *testing.T) {
	var out strings.Builder
	if err := writeVerifySummary(&out, nil, []string{"drift"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "# Smoke verification: PASS") {
		t.Errorf("summary does not start with the PASS heading:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "drift") {
		t.Errorf("summary does not list the skipped criterion:\n%s", out.String())
	}
}

// TestVerifyMode runs the -verify mode as the CLI does: the expected runs come
// from the same sweep flags (-test 3, -benchmarks, -stream-mode, -sweep).
func TestVerifyMode(t *testing.T) {
	newCfg := func(dir, summary string) *config {
		cfg := &config{
			sweepLabel: "smoke",
			sweep: sweepConfig{
				rates: []int{0}, reps: 1,
				benchmarks:  []string{"SymmetricQuorumCall"},
				streamModes: []string{"dual", "dedup"},
			},
			verifyDir: dir, summaryPath: summary, maxDrift: time.Millisecond,
		}
		applyTestMode(cfg, 3, true, true)
		cfg.verifyHosts = 3
		return cfg
	}
	t.Run("Healthy", func(t *testing.T) {
		summary := filepath.Join(t.TempDir(), "verdict.md")
		ok, err := runVerify(newCfg(newVerifyFixture("SymmetricQuorumCall").write(t), summary))
		if err != nil || !ok {
			t.Fatalf("runVerify = %v, %v; want true, nil", ok, err)
		}
		data, err := os.ReadFile(summary)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(data), "# Smoke verification: PASS") {
			t.Errorf("summary file:\n%s", data)
		}
	})
	t.Run("Failed", func(t *testing.T) {
		f := newVerifyFixture("SymmetricQuorumCall")
		f.first().Status = "failed"
		ok, err := runVerify(newCfg(f.write(t), ""))
		if err != nil || ok {
			t.Fatalf("runVerify = %v, %v; want false, nil", ok, err)
		}
	})
}
