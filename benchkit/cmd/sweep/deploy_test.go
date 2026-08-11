package main

import (
	"bytes"
	"io"
	"log"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums/benchkit"
)

// TestBuildNodeCmd verifies the remote command line for one node: the required
// contract flags are always present, while rate, the pass-through flags, and
// -extra-args are appended only when set.
func TestBuildNodeCmd(t *testing.T) {
	node := nodeAssignment{host: "bb1", port: 9000}
	const peers = "bb1:9000,bb2:9000"
	const base = "run_Symmetric_N2_W1_P0"
	params := runSpec{Dimensions: benchkit.Dimensions{
		Nodes: 2, Workers: 1, Payload: 0, Benchmark: "Symmetric",
	}}
	const required = "'/tmp/sweep-benchmark' -self=bb1:9000 -remotes=bb1:9000,bb2:9000" +
		" -benchmarks='^Symmetric$' -workers=1 -payload=0 -time=10s" +
		" -output='/tmp/run_Symmetric_N2_W1_P0_bb1_9000.binpb'"

	tests := []struct {
		name       string
		cfg        config
		rate       int
		streamMode string
		want       string
	}{
		{
			name: "DefaultsOmitOptionalFlags",
			want: required,
		},
		{
			name:       "ExplicitDualOmitted",
			streamMode: "dual",
			want:       required,
		},
		{
			name: "RateAppendedWhenSet",
			rate: 5000,
			want: required + " -rate=5000",
		},
		{
			name: "PassThroughFlagsAppendedWhenSet",
			cfg: config{
				interval:    "250ms",
				statsMode:   "hdr",
				rateStep:    1000,
				rateStepMax: 8000,
			},
			rate: 1000,
			want: required + " -rate=1000 -interval=250ms -stats-mode=hdr" +
				" -rate-step=1000 -rate-step-max=8000",
		},
		{
			name:       "StreamDedupAppendedWhenSet",
			streamMode: "dedup",
			want:       required + " -stream-mode=dedup",
		},
		{
			name:       "BaselineOmitted",
			streamMode: "baseline",
			want:       required,
		},
		{
			name: "ExtraArgsAppendedVerbatim",
			cfg:  config{extraArgs: "-quorum-size=3 -send-buffer=64"},
			want: required + " -quorum-size=3 -send-buffer=64",
		},
		{
			name: "VerboseBeforeExtraArgs",
			cfg:  config{verbose: true, extraArgs: "-quorum-size=3"},
			want: required + " -verbose -quorum-size=3",
		},
		{
			name: "CollectProfilesAppendsProfileFlags",
			cfg:  config{collectProfiles: true},
			want: required + " -cpuprofile='/tmp/run_Symmetric_N2_W1_P0_bb1_9000.cpu.prof'" +
				" -memprofile='/tmp/run_Symmetric_N2_W1_P0_bb1_9000.mem.prof'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.prog = newRemoteProgram("")
			cfg.duration = 10 * time.Second
			p := params
			p.Rate = tt.rate
			p.StreamMode = tt.streamMode
			if got := buildNodeCmd(node, peers, p, base, &cfg); got != tt.want {
				t.Errorf("buildNodeCmd =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestBuildNodeCmdUsesConfiguredRemoteNamespace(t *testing.T) {
	node := nodeAssignment{host: "bb1", port: 9000}
	cfg := &config{
		duration:   time.Second,
		prog:       newRemoteProgram("benchmark"),
		remoteDirs: map[string]string{"bb1": "/local/sweep meling"},
	}
	got := buildNodeCmd(node, "bb1:9000", runSpec{
		Dimensions: benchkit.Dimensions{Benchmark: "Symmetric"},
	}, "run", cfg)
	for _, want := range []string{
		"'/local/sweep meling/sweep-benchmark'",
		"-output='/local/sweep meling/run_bb1_9000.binpb'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("buildNodeCmd() missing %q:\n%s", want, got)
		}
	}
}

// TestBuildNodeCmdQuotesResultPaths verifies that -output, -cpuprofile, and
// -memprofile are each shell-quoted as a single argument. base embeds the
// user-chosen -sweep label verbatim (see runBase), so a label containing a
// space or shell metacharacter previously broke every remote node launch,
// since -benchmarks was quoted but these path flags were not.
func TestBuildNodeCmdQuotesResultPaths(t *testing.T) {
	node := nodeAssignment{host: "bb1", port: 9000}
	cfg := &config{duration: time.Second, prog: newRemoteProgram(""), collectProfiles: true}
	const base = "exp 1" // a label with a space, as runBase would produce

	got := buildNodeCmd(node, "bb1:9000", runSpec{Dimensions: benchkit.Dimensions{Benchmark: "Q"}}, base, cfg)
	for _, want := range []string{
		"-output='/tmp/exp 1_bb1_9000.binpb'",
		"-cpuprofile='/tmp/exp 1_bb1_9000.cpu.prof'",
		"-memprofile='/tmp/exp 1_bb1_9000.mem.prof'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("buildNodeCmd missing %q:\n%s", want, got)
		}
	}
}

func TestBuildNodeCmdBufferFlags(t *testing.T) {
	node := nodeAssignment{host: "bb1", port: 9000}
	cfg := &config{duration: time.Second, prog: newRemoteProgram("")}
	base := runSpec{Dimensions: benchkit.Dimensions{
		Benchmark: "Q", Nodes: 1, Workers: 1,
	}}

	if got := buildNodeCmd(node, "bb1:9000", base, "run", cfg); strings.Contains(got, "-send-buffer") || strings.Contains(got, "-recv-buffer") {
		t.Fatalf("zero buffers emitted flags: %s", got)
	}
	base.SendBuffer, base.RecvBuffer = 256, 16
	got := buildNodeCmd(node, "bb1:9000", base, "run", cfg)
	for _, want := range []string{"-send-buffer=256", "-recv-buffer=16"} {
		if !strings.Contains(got, want) {
			t.Errorf("buildNodeCmd missing %q: %s", want, got)
		}
	}
}

func TestFdLimitStmt(t *testing.T) {
	if got := fdLimitStmt(0); got != "" {
		t.Errorf("fdLimitStmt(0) = %q, want empty", got)
	}
	if got := fdLimitStmt(-1); got != "" {
		t.Errorf("fdLimitStmt(-1) = %q, want empty", got)
	}
	if got, want := fdLimitStmt(65536), "ulimit -Sn 65536 2>/dev/null"; got != want {
		t.Errorf("fdLimitStmt(65536) = %q, want %q", got, want)
	}
}

func TestBuildNodeCmdFdLimit(t *testing.T) {
	node := nodeAssignment{host: "bb1", port: 9000}
	const peers = "bb1:9000,bb2:9000"
	const base = "run_Symmetric_N2_W1_P0"
	p := runSpec{Dimensions: benchkit.Dimensions{
		Nodes: 2, Workers: 1, Payload: 0, Benchmark: "Symmetric",
	}}

	// With a limit set, the ulimit statement prefixes the node command so it runs
	// under the raised soft limit; the benchmark's exit status still propagates.
	cfg := config{prog: newRemoteProgram(""), duration: 10 * time.Second, fdLimit: 65536}
	got := buildNodeCmd(node, peers, p, base, &cfg)
	if !strings.HasPrefix(got, "ulimit -Sn 65536 2>/dev/null; '/tmp/sweep-benchmark' ") {
		t.Errorf("buildNodeCmd missing fd-limit prefix\ngot: %s", got)
	}

	// With the limit disabled, the command is unchanged (no shell prefix).
	cfg.fdLimit = 0
	if got := buildNodeCmd(node, peers, p, base, &cfg); strings.Contains(got, "ulimit") {
		t.Errorf("buildNodeCmd added ulimit with fdLimit=0\ngot: %s", got)
	}
}

func TestBuildNodeCmdUsesPeerAddressForGorumsAndAliasForArtifacts(t *testing.T) {
	node := nodeAssignment{host: "bb25", peerHost: "152.94.162.19", port: 9000}
	peers := buildPeerList([]nodeAssignment{
		{host: "bb16", peerHost: "152.94.162.26", port: 9000},
		node,
	})
	const base = "run_Symmetric_N2_W1_P0"
	params := runSpec{Dimensions: benchkit.Dimensions{
		Nodes: 2, Workers: 1, Payload: 0, Benchmark: "Symmetric",
	}}
	cfg := &config{
		prog:     newRemoteProgram(""),
		duration: 10 * time.Second,
	}
	want := "'/tmp/sweep-benchmark' -self=152.94.162.19:9000" +
		" -remotes=152.94.162.26:9000,152.94.162.19:9000" +
		" -benchmarks='^Symmetric$' -workers=1 -payload=0 -time=10s" +
		" -output='/tmp/run_Symmetric_N2_W1_P0_bb25_9000.binpb'"
	if got := buildNodeCmd(node, peers, params, base, cfg); got != want {
		t.Errorf("buildNodeCmd =\n  %q\nwant\n  %q", got, want)
	}
}

func TestResolvePeerHost(t *testing.T) {
	dnsErr := func(name string) error {
		return &net.DNSError{Err: "no such host", Name: name}
	}
	lookupFrom := func(records map[string][]net.IP) lookupIPFunc {
		return func(name string) ([]net.IP, error) {
			ips, ok := records[name]
			if !ok {
				return nil, dnsErr(name)
			}
			return ips, nil
		}
	}

	tests := []struct {
		name    string
		alias   string
		sshAddr string
		cfgAddr string
		records map[string][]net.IP
		want    string
		wantErr string
	}{
		{
			name:    "NumericSSHAddress",
			alias:   "bb1",
			sshAddr: "152.94.162.11:22",
			want:    "152.94.162.11",
		},
		{
			name:    "ResolveSSHHostPreferIPv4",
			alias:   "bb1",
			sshAddr: "bb1.example.test:22",
			records: map[string][]net.IP{
				"bb1.example.test": {
					net.ParseIP("2001:db8::1"),
					net.ParseIP("152.94.162.11"),
				},
			},
			want: "152.94.162.11",
		},
		{
			name:    "FallbackToAlias",
			alias:   "bb1",
			sshAddr: "proxy-name:22",
			records: map[string][]net.IP{
				"bb1": {net.ParseIP("152.94.162.11")},
			},
			want: "152.94.162.11",
		},
		{
			name:    "UseSSHConfigHostnameAfterProxyJumpRemoteAddr",
			alias:   "bb1",
			sshAddr: "0.0.0.0:0",
			cfgAddr: "bb1.ux.uis.no:22",
			records: map[string][]net.IP{
				"bb1.ux.uis.no": {net.ParseIP("152.94.162.11")},
			},
			want: "152.94.162.11",
		},
		{
			name:    "RejectLoopback",
			alias:   "bb1",
			sshAddr: "127.0.0.1:22",
			records: map[string][]net.IP{
				"bb1": {net.ParseIP("152.94.162.11")},
			},
			want: "152.94.162.11",
		},
		{
			name:    "NoUsableAddress",
			alias:   "bb1",
			sshAddr: "localhost:22",
			records: map[string][]net.IP{
				"localhost": {net.ParseIP("127.0.0.1")},
				"bb1":       {net.ParseIP("127.0.1.1")},
			},
			wantErr: "no usable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePeerHost(tt.alias, tt.sshAddr, tt.cfgAddr, lookupFrom(tt.records))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolvePeerHost error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePeerHost: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolvePeerHost = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPeerHostSummary(t *testing.T) {
	hosts := []hostAssignment{
		{alias: "bb1", peerHost: "152.94.162.11"},
		{alias: "bb2", peerHost: "152.94.162.12"},
	}
	if got, want := peerHostSummary(hosts), "bb1=152.94.162.11, bb2=152.94.162.12"; got != want {
		t.Errorf("peerHostSummary = %q, want %q", got, want)
	}
}

// TestPortCheckScript verifies the preflight script probes exactly the given
// ports and fails (non-empty $busy) when any of them has a listener.
func TestPortCheckScript(t *testing.T) {
	const want = `busy=
for p in 9000 9001; do
  if ss -ltnH 2>/dev/null | grep -qE ":$p([[:space:]]|$)"; then
    busy="$busy $p"
    ss -ltnpH 2>/dev/null | grep -E ":$p([[:space:]]|$)" || true
  fi
done
[ -z "$busy" ]`
	if got := portCheckScript([]string{"9000", "9001"}); got != want {
		t.Errorf("portCheckScript =\n%s\nwant\n%s", got, want)
	}
}

// TestBuildNodeAssignments verifies the round-robin host placement and the
// basePort + i/numHosts port-offset arithmetic that every result filename,
// port check, and manifest entry is built from.
func TestBuildNodeAssignments(t *testing.T) {
	h := func(alias string) hostAssignment { return hostAssignment{alias: alias} }

	tests := []struct {
		name     string
		hosts    []hostAssignment
		n        int
		basePort int
		want     []nodeAssignment
	}{
		{
			name:     "OneNodePerHost",
			hosts:    []hostAssignment{h("bb1"), h("bb2")},
			n:        2,
			basePort: 9000,
			want: []nodeAssignment{
				{host: "bb1", port: 9000},
				{host: "bb2", port: 9000},
			},
		},
		{
			name:     "UnevenRatioAcrossTwoHosts",
			hosts:    []hostAssignment{h("bb1"), h("bb2")},
			n:        5,
			basePort: 9000,
			want: []nodeAssignment{
				{host: "bb1", port: 9000},
				{host: "bb2", port: 9000},
				{host: "bb1", port: 9001},
				{host: "bb2", port: 9001},
				{host: "bb1", port: 9002},
			},
		},
		{
			name:     "FewerNodesThanHostsUsesOnlyFirstN",
			hosts:    []hostAssignment{h("bb1"), h("bb2"), h("bb3")},
			n:        1,
			basePort: 9000,
			want: []nodeAssignment{
				{host: "bb1", port: 9000},
			},
		},
		{
			name:     "SingleHostStacksPorts",
			hosts:    []hostAssignment{h("bb1")},
			n:        3,
			basePort: 9000,
			want: []nodeAssignment{
				{host: "bb1", port: 9000},
				{host: "bb1", port: 9001},
				{host: "bb1", port: 9002},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildNodeAssignments(tt.hosts, tt.n, tt.basePort)
			if !slices.Equal(got, tt.want) {
				t.Errorf("buildNodeAssignments(%v, %d, %d) =\n  %v\nwant\n  %v",
					tt.hosts, tt.n, tt.basePort, got, tt.want)
			}
		})
	}
}

func TestResultFilename(t *testing.T) {
	tests := []struct {
		name string
		base string
		node nodeAssignment
		ext  string
		want string
	}{
		{
			name: "json extension",
			base: "nscale_SymmetricQuorumCall_N9_C1_P0",
			node: nodeAssignment{host: "bb1", port: 9000},
			ext:  ".json",
			want: "nscale_SymmetricQuorumCall_N9_C1_P0_bb1_9000.json",
		},
		{
			name: "binary extension with port offset on shared host",
			base: "test_Symmetric_N60_C1_P0",
			node: nodeAssignment{host: "bb30", port: 9001},
			ext:  ".binpb",
			want: "test_Symmetric_N60_C1_P0_bb30_9001.binpb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resultFilename(tt.base, tt.node, tt.ext); got != tt.want {
				t.Errorf("resultFilename(%q, %+v, %q) = %q, want %q", tt.base, tt.node, tt.ext, got, tt.want)
			}
		})
	}
}

func TestExpandBuildCmd(t *testing.T) {
	const out = "/work/gorums/cmd/benchmark/benchmark"
	const quoted = "'/work/gorums/cmd/benchmark/benchmark'"
	tests := []struct {
		name string
		tmpl string
		abs  string
		want string
	}{
		{
			name: "token substituted",
			tmpl: "go build -o {{output}} ./cmd/pbft-bench",
			abs:  out,
			want: "go build -o " + quoted + " ./cmd/pbft-bench",
		},
		{
			name: "token in make variable",
			tmpl: "make pbft-bench OUT={{output}}",
			abs:  out,
			want: "make pbft-bench OUT=" + quoted,
		},
		{
			name: "no token appends -o",
			tmpl: "go build ./cmd/pbft-bench",
			abs:  out,
			want: "go build ./cmd/pbft-bench -o " + quoted,
		},
		{
			name: "path with spaces stays one argument",
			tmpl: "go build ./cmd/benchmark",
			abs:  "/home/Team UIS/gorums/bench",
			want: "go build ./cmd/benchmark -o '/home/Team UIS/gorums/bench'",
		},
		{
			name: "single quote in path is escaped",
			tmpl: "go build {{output}}",
			abs:  "/tmp/a'b/bench",
			want: `go build '/tmp/a'\''b/bench'`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandBuildCmd(tt.tmpl, tt.abs); got != tt.want {
				t.Errorf("expandBuildCmd(%q, %q) = %q, want %q", tt.tmpl, tt.abs, got, tt.want)
			}
		})
	}
}

func TestNewRemoteProgram(t *testing.T) {
	tests := []struct {
		name       string
		binaryPath string
		wantName   string
		wantPath   string
		wantPgrep  string
	}{
		{
			name:       "empty path uses default binary",
			binaryPath: "",
			wantName:   "sweep-benchmark",
			wantPath:   "/tmp/sweep-benchmark",
			wantPgrep:  "[s]weep-benchmark",
		},
		{
			name:       "default binary path",
			binaryPath: defaultBinaryPath,
			wantName:   "sweep-benchmark",
			wantPath:   "/tmp/sweep-benchmark",
			wantPgrep:  "[s]weep-benchmark",
		},
		{
			name:       "custom binary keeps its basename",
			binaryPath: "/some/dir/myprog",
			wantName:   "sweep-myprog",
			wantPath:   "/tmp/sweep-myprog",
			wantPgrep:  "[s]weep-myprog",
		},
		{
			name:       "bare basename",
			binaryPath: "raft-bench",
			wantName:   "sweep-raft-bench",
			wantPath:   "/tmp/sweep-raft-bench",
			wantPgrep:  "[s]weep-raft-bench",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := newRemoteProgram(tt.binaryPath)
			if prog.name != tt.wantName {
				t.Errorf("name = %q, want %q", prog.name, tt.wantName)
			}
			if got := prog.path("/tmp"); got != tt.wantPath {
				t.Errorf("path() = %q, want %q", got, tt.wantPath)
			}
			if got := prog.pgrep(); got != tt.wantPgrep {
				t.Errorf("pgrep() = %q, want %q", got, tt.wantPgrep)
			}
		})
	}
}

// TestDrainHandlesLineOverDefaultScannerLimit verifies that drain logs a line
// well past bufio.Scanner's 64 KiB default token size (a large diagnostic
// dump or panic trace is a realistic case), instead of stopping the scan with
// bufio.ErrTooLong the way the un-grown default buffer would.
func TestDrainHandlesLineOverDefaultScannerLimit(t *testing.T) {
	longLine := strings.Repeat("x", 100*1024) // over the 64 KiB default, under the 1 MiB cap
	rc := io.NopCloser(strings.NewReader(longLine + "\nshort\n"))
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	done := make(chan struct{})

	drain(rc, "host1", 9000, logger, done)
	<-done

	got := buf.String()
	if !strings.Contains(got, longLine) {
		t.Error("drain did not log the long line; the scanner buffer was not grown")
	}
	if !strings.Contains(got, "short") {
		t.Error("drain did not log the line following the long one")
	}
}

// drainCountingReader wraps a Reader and counts the bytes actually read from
// it, so a test can confirm a reader was fully drained rather than abandoned
// partway through.
type drainCountingReader struct {
	r     io.Reader
	total int
}

func (d *drainCountingReader) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	d.total += n
	return n, err
}

// TestDrainKeepsReadingAfterScanTooLong verifies that drain does not abandon
// rc after a scan error (a line exceeding even the grown 1 MiB buffer): it
// must keep consuming rc to EOF so the underlying pipe (an SSH channel, in
// production) does not fill and wedge the remote process's next write, an
// invariant launchAndWait's own doc warns about.
func TestDrainKeepsReadingAfterScanTooLong(t *testing.T) {
	tooLong := strings.Repeat("y", 2*1024*1024) // over the 1 MiB cap
	trailing := "\nmore data after the oversized line\n"
	src := &drainCountingReader{r: strings.NewReader(tooLong + trailing)}
	rc := io.NopCloser(src)
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	done := make(chan struct{})

	drain(rc, "host1", 9000, logger, done)
	<-done

	if !strings.Contains(buf.String(), "pipe read error") {
		t.Error("drain did not log the scan error")
	}
	wantTotal := len(tooLong) + len(trailing)
	if src.total < wantTotal {
		t.Errorf("bytes read from rc = %d, want at least %d (drain must keep draining after a scan error)", src.total, wantTotal)
	}
}

// TestChunkByLength verifies the grouping cleanup relies on to keep each "rm
// -f" command under Linux's MAX_ARG_STRLEN: consecutive items are packed into
// a group up to maxBytes, a new group starts before exceeding it, and no
// item is ever dropped, even a single oversized one.
func TestChunkByLength(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		maxBytes int
		want     [][]string
	}{
		{"Empty", nil, 10, nil},
		{"AllFitInOneChunk", []string{"a", "b", "c"}, 10, [][]string{{"a", "b", "c"}}},
		{
			name:     "SplitsWhenExceedingLimit",
			items:    []string{"aaa", "bbb", "ccc", "ddd"},
			maxBytes: 7, // "aaa bbb" = 7 fits; adding " ccc" would exceed it
			want:     [][]string{{"aaa", "bbb"}, {"ccc", "ddd"}},
		},
		{
			name:     "SingleItemExceedingLimitKeptAlone",
			items:    []string{"short", "way-too-long-item", "short2"},
			maxBytes: 5,
			want:     [][]string{{"short"}, {"way-too-long-item"}, {"short2"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkByLength(tt.items, tt.maxBytes)
			if !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("chunkByLength(%v, %d) = %v, want %v", tt.items, tt.maxBytes, got, tt.want)
			}
			var gotItems []string
			for _, chunk := range got {
				gotItems = append(gotItems, chunk...)
			}
			if !slices.Equal(gotItems, tt.items) {
				t.Errorf("chunkByLength dropped or reordered items: got %v, want %v", gotItems, tt.items)
			}
		})
	}
}
