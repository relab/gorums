package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// mockExitStatus simulates an SSH exit error for TestFinishedRemotely.
type mockExitStatus struct{ code int }

func (e *mockExitStatus) Error() string   { return fmt.Sprintf("exit status %d", e.code) }
func (e *mockExitStatus) ExitStatus() int { return e.code }

func TestResolveDriver(t *testing.T) {
	tests := []struct {
		name       string
		driver     string
		hosts      []string
		wantDriver string
		wantBench  []string
		wantErr    bool
	}{
		{name: "off", driver: "", hosts: []string{"bb1", "bb2"}, wantDriver: "", wantBench: []string{"bb1", "bb2"}},
		{name: "first", driver: "first", hosts: []string{"bb1", "bb2", "bb3"}, wantDriver: "bb1", wantBench: []string{"bb2", "bb3"}},
		{name: "explicit in hosts", driver: "bb2", hosts: []string{"bb1", "bb2", "bb3"}, wantDriver: "bb2", wantBench: []string{"bb1", "bb3"}},
		{name: "explicit outside hosts", driver: "driver", hosts: []string{"bb1", "bb2"}, wantDriver: "driver", wantBench: []string{"bb1", "bb2"}},
		{name: "first with no hosts", driver: "first", hosts: nil, wantErr: true},
		{name: "drains pool", driver: "bb1", hosts: []string{"bb1"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver, bench, err := resolveDriver(tt.driver, tt.hosts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if driver != tt.wantDriver {
				t.Errorf("driver = %q, want %q", driver, tt.wantDriver)
			}
			if !slices.Equal(bench, tt.wantBench) {
				t.Errorf("bench = %v, want %v", bench, tt.wantBench)
			}
		})
	}
}

// TestResolveDriverHost verifies that the host-only resolver selects hosts[0]
// for "first", returns an explicit alias unchanged (even outside hosts), and
// rejects "first" with no hosts. Unlike resolveDriver it never drains a pool,
// since the explain check needs only the driver host.
func TestResolveDriverHost(t *testing.T) {
	tests := []struct {
		name    string
		driver  string
		hosts   []string
		want    string
		wantErr bool
	}{
		{name: "first", driver: "first", hosts: []string{"bb1", "bb2"}, want: "bb1"},
		{name: "explicit", driver: "bb2", hosts: []string{"bb1", "bb2"}, want: "bb2"},
		{name: "explicit single host", driver: "bb1", hosts: []string{"bb1"}, want: "bb1"},
		{name: "outside hosts", driver: "driver", hosts: []string{"bb1"}, want: "driver"},
		{name: "first with no hosts", driver: "first", hosts: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDriverHost(tt.driver, tt.hosts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("driver = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExplainCheckScript verifies the remote check script exports the forwarded
// key and invokes -explain-check with the provider and model, so the key reaches
// the binary without appearing in its argv.
func TestExplainCheckScript(t *testing.T) {
	script := explainCheckScript("/tmp/sweep-explain-check-x", "OLLAMA_API_KEY='secret'", "local", "llama3.3")
	for _, want := range []string{
		"export OLLAMA_API_KEY='secret'",
		"cd '/tmp/sweep-explain-check-x'",
		"-explain-check",
		"-explain-provider 'local'",
		"-explain-model 'llama3.3'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "secret -explain-check") {
		t.Error("key must not be passed on the command line")
	}
	// bash -s reads its script from stdin, which iago closes only after the
	// command returns; without an explicit exit, a successful check blocks
	// waiting for stdin EOF. The trailing exit must be present.
	if !strings.Contains(script, "\nexit 0") {
		t.Errorf("script must end with an explicit exit to avoid a stdin-EOF hang:\n%s", script)
	}
}

func TestDriverCheckScript(t *testing.T) {
	script := driverCheckScript("/tmp/sweep-driver-check-x", "bb1,bb2,bb3", 9000, "", "bb1", "/local")
	for _, want := range []string{
		"cd '/tmp/sweep-driver-check-x'",
		"-check",
		"-hosts 'bb1,bb2,bb3'",
		"-config '/tmp/sweep-driver-check-x'/ssh.config",
		"-port 9000",
		// The driver forwards its own alias so the check probes it locally
		// instead of SSHing to itself (which fails the loopback handshake).
		"-self-host 'bb1'",
		"-remote-dir '/local'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q:\n%s", want, script)
		}
	}
	// With no -binary, the driver-side default matches the deployed benchmark;
	// the flag must be omitted rather than passed empty.
	if strings.Contains(script, "-binary") {
		t.Errorf("script must omit -binary when none is set:\n%s", script)
	}
	// bash -s reads its script from stdin, which iago closes only after the
	// command returns; without an explicit exit, a successful check blocks
	// waiting for stdin EOF. The trailing exit must be present.
	if !strings.Contains(script, "\nexit 0") {
		t.Errorf("script must end with an explicit exit to avoid a stdin-EOF hang:\n%s", script)
	}
}

func TestDriverCheckScriptBinary(t *testing.T) {
	script := driverCheckScript("/tmp/wd", "bb1", 9000, "./cmd/otherproto/bench", "bb1", "/tmp")
	if !strings.Contains(script, "-binary './cmd/otherproto/bench'") {
		t.Errorf("script must forward -binary when set:\n%s", script)
	}
}

func TestRemoteSweepCommand(t *testing.T) {
	cfg := &config{
		port:          9000,
		duration:      10 * time.Second,
		trim:          time.Second,
		sweepLabel:    "e1",
		verbose:       true,
		interval:      "250ms",
		statsMode:     "hdr",
		rateStep:      1000,
		rateStepMax:   8000,
		extraArgs:     "-fault-kill-after=5s",
		pgo:           true,
		degradedBelow: 0.5,
		netcheck:      true,
		sweep: sweepConfig{
			numNodes:    []int{3, 5, 9},
			workers:     []int{1, 4},
			payloads:    []int{0},
			rates:       []int{0},
			benchmarks:  []string{"SymmetricQuorumCall", "QuorumCall"},
			streamModes: []string{"dual", "dedup"},
			reps:        3,
		},
	}
	cmd := remoteSweepCommand(cfg, "/tmp/sweep-driver-e1-x", "/tmp/sweep-driver-cache", "bb2,bb3", "deadbeef")

	mustContain := []string{
		// The sweep executable, -binary, and -config come from binDir (the
		// driver's persistent binary cache), decoupled from wd (the run's own,
		// timestamped work directory used only for -outdir and the ready marker).
		"/tmp/sweep-driver-cache/sweep'",
		"'-driven'",
		"'-git-sha=deadbeef'",
		"'-hosts=bb2,bb3'",
		"'-binary=/tmp/sweep-driver-cache/benchmark'",
		"'-config=/tmp/sweep-driver-cache/ssh.config'",
		"'-outdir=/tmp/sweep-driver-e1-x/out'",
		"'-port=9000'",
		"'-duration=10s'",
		"'-trim=1s'",
		"'-sweep=e1'",
		"'-n=3,5,9'",
		"'-workers=1,4'",
		"'-payload=0'",
		"'-rate=0'",
		"'-benchmarks=SymmetricQuorumCall,QuorumCall'",
		"'-stream-mode=dual,dedup'",
		"'-reps=3'",
		"'-verbose'",
		"'-interval=250ms'",
		"'-stats-mode=hdr'",
		"'-rate-step=1000'",
		"'-rate-step-max=8000'",
		"'-extra-args=-fault-kill-after=5s'",
		"'-pgo'",
		"'-degraded-below=0.5'",
		"'-netcheck=true'",
	}
	for _, want := range mustContain {
		if !strings.Contains(cmd, want) {
			t.Errorf("command missing %q\ngot: %s", want, cmd)
		}
	}
	// -driver/-collect/-test/-build must never reach the driven sweep.
	for _, bad := range []string{"-driver=", "-collect", "-test", "-build", "-collect-profiles"} {
		if strings.Contains(cmd, bad) {
			t.Errorf("command should not contain %q\ngot: %s", bad, cmd)
		}
	}
}

func TestRemoteSweepCommandProfilesWithoutPGO(t *testing.T) {
	cfg := &config{
		port: 9000, duration: time.Second, sweepLabel: "p", collectProfiles: true,
		sweep: sweepConfig{numNodes: []int{3}, workers: []int{1}, payloads: []int{0}, rates: []int{0}, benchmarks: []string{"QuorumCall"}, reps: 1},
	}
	cmd := remoteSweepCommand(cfg, "/tmp/wd", "/tmp/wd", "bb1", "")
	if !strings.Contains(cmd, "'-collect-profiles'") {
		t.Errorf("expected -collect-profiles\ngot: %s", cmd)
	}
	if strings.Contains(cmd, "'-pgo'") {
		t.Errorf("did not expect -pgo\ngot: %s", cmd)
	}
}

func TestRemoteSweepCommandOmitsDefaultStreamMode(t *testing.T) {
	cfg := &config{
		port: 9000, duration: time.Second, sweepLabel: "p",
		sweep: sweepConfig{
			numNodes: []int{3}, workers: []int{1}, payloads: []int{0},
			rates: []int{0}, benchmarks: []string{"QuorumCall"}, streamModes: []string{"dual"}, reps: 1,
		},
	}
	cmd := remoteSweepCommand(cfg, "/tmp/wd", "/tmp/wd", "bb1", "")
	if strings.Contains(cmd, "-stream-mode") {
		t.Errorf("default-only stream mode should be omitted\ngot: %s", cmd)
	}
}

func TestRemoteSweepCommandExplain(t *testing.T) {
	cfg := &config{
		port: 9000, duration: time.Second, sweepLabel: "e1",
		explain: true, explainProvider: "local", explainModel: "llama3.3", explainMaxLog: 4096,
		sweep: sweepConfig{numNodes: []int{3}, workers: []int{1}, payloads: []int{0}, rates: []int{0}, benchmarks: []string{"QuorumCall"}, reps: 1},
	}
	cmd := remoteSweepCommand(cfg, "/tmp/wd", "/tmp/wd", "bb1", "")
	for _, want := range []string{"'-explain'", "'-explain-provider=local'", "'-explain-model=llama3.3'", "'-explain-max-log=4096'"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command missing %q\ngot: %s", want, cmd)
		}
	}
	// The API key is never forwarded as a flag.
	if strings.Contains(cmd, "API_KEY") || strings.Contains(cmd, "-explain-key") {
		t.Errorf("command leaked an API key\ngot: %s", cmd)
	}

	// Without -explain, none of the flags appear.
	cfg.explain = false
	if got := remoteSweepCommand(cfg, "/tmp/wd", "/tmp/wd", "bb1", ""); strings.Contains(got, "-explain") {
		t.Errorf("command should omit -explain when disabled\ngot: %s", got)
	}
}

func TestRemoteSweepCommandDetach(t *testing.T) {
	cfg := &config{
		port: 9000, duration: time.Second, sweepLabel: "e1", detach: true,
		sweep: sweepConfig{numNodes: []int{3}, workers: []int{1}, payloads: []int{0}, rates: []int{0}, benchmarks: []string{"QuorumCall"}, reps: 1},
	}
	// The ready marker lives under wd (the run's own work directory), not binDir
	// (the driver's persistent binary cache), since it is per-run state.
	if got := remoteSweepCommand(cfg, "/tmp/sweep-driver-e1-x", "/tmp/sweep-driver-cache", "bb1", ""); !strings.Contains(got, "'-ready-marker=/tmp/sweep-driver-e1-x/"+readyMarkerName+"'") {
		t.Errorf("detached command missing -ready-marker\ngot: %s", got)
	}
	// Without -detach the launcher streams and collects itself, so the driven
	// sweep needs no ready marker.
	cfg.detach = false
	if got := remoteSweepCommand(cfg, "/tmp/wd", "/tmp/wd", "bb1", ""); strings.Contains(got, "-ready-marker") {
		t.Errorf("non-detached command should omit -ready-marker\ngot: %s", got)
	}
}

func TestDriverLLMEnv(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		if got := driverLLMEnv(&config{explain: false}); got != "" {
			t.Errorf("driverLLMEnv = %q, want empty when -explain off", got)
		}
	})
	t.Run("key present", func(t *testing.T) {
		t.Setenv(envLocalKey, "3secret3")
		got := driverLLMEnv(&config{explain: true, explainProvider: providerLocal})
		if got != envLocalKey+"='3secret3'" {
			t.Errorf("driverLLMEnv = %q, want %s='3secret3'", got, envLocalKey)
		}
	})
	t.Run("key missing", func(t *testing.T) {
		t.Setenv(envLocalKey, "")
		if got := driverLLMEnv(&config{explain: true, explainProvider: providerLocal}); got != "" {
			t.Errorf("driverLLMEnv = %q, want empty when key unset", got)
		}
	})
}

func TestRsyncArgs(t *testing.T) {
	tests := []struct {
		name      string
		sshConfig string
		want      []string
	}{
		{
			name:      "no ssh config",
			sshConfig: "",
			want:      []string{"-azs", "--partial", "--progress"},
		},
		{
			name:      "custom ssh config",
			sshConfig: "/home/me/.ssh/cluster",
			want:      []string{"-azs", "--partial", "--progress", "-e", "ssh -F '/home/me/.ssh/cluster'"},
		},
		{
			name:      "ssh config with spaces",
			sshConfig: "/home/me/SSH configs/cluster",
			want:      []string{"-azs", "--partial", "--progress", "-e", "ssh -F '/home/me/SSH configs/cluster'"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rsyncArgs(tt.sshConfig)
			if !slices.Equal(got, tt.want) {
				t.Errorf("rsyncArgs(%q) = %v, want %v", tt.sshConfig, got, tt.want)
			}
		})
	}
}

func TestRsyncRemoteSpecPreservesPath(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		path   string
		want   string
	}{
		{"Simple", "driver", "/tmp/results", "driver:/tmp/results"},
		{"Spaces", "driver", "/scratch/team data/results", "driver:/scratch/team data/results"},
		{"SingleQuote", "driver", "/scratch/team's/results", "driver:/scratch/team's/results"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rsyncRemoteSpec(tt.driver, tt.path); got != tt.want {
				t.Errorf("rsyncRemoteSpec(%q, %q) = %q, want %q", tt.driver, tt.path, got, tt.want)
			}
		})
	}
}

func TestFinishedRemotely(t *testing.T) {
	if !finishedRemotely(nil) {
		t.Error("nil error should mean finished")
	}
	if !finishedRemotely(&mockExitStatus{code: 3}) {
		t.Error("a non-zero remote exit should mean finished (with failed runs)")
	}
	if finishedRemotely(errors.New("connection reset")) {
		t.Error("a transport error should mean not finished")
	}
}

func TestMaxNodeCount(t *testing.T) {
	if got := maxNodeCount(sweepConfig{numNodes: []int{3, 17, 9}}); got != 17 {
		t.Errorf("maxNodeCount = %d, want 17", got)
	}
}

func TestJoinInts(t *testing.T) {
	if got := joinInts([]int{1, 4, 16}); got != "1,4,16" {
		t.Errorf("joinInts = %q, want %q", got, "1,4,16")
	}
	if got := joinInts([]int{7}); got != "7" {
		t.Errorf("joinInts = %q, want %q", got, "7")
	}
}

func TestGeneratedSSHConfig(t *testing.T) {
	cfg := generatedSSHConfig()
	for _, want := range []string{"Host *", "StrictHostKeyChecking no"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("generated ssh config missing %q", want)
		}
	}
}

// TestChooseCollectMode verifies the -collect decision: a prior compact
// collection (the marker) means the full raw archive is wanted; otherwise a
// present compact transfer is downloaded; and a missing compact transfer (the
// driven sweep aborted before exporting, e.g. a netcheck failure) falls back
// to salvaging partial output instead of failing with a raw rsync error.
func TestChooseCollectMode(t *testing.T) {
	tests := []struct {
		name           string
		marked, exists bool
		want           collectMode
	}{
		{name: "first collect", marked: false, exists: true, want: collectCompact},
		{name: "second collect archives raw", marked: true, exists: true, want: collectFull},
		{name: "aborted before export", marked: false, exists: false, want: collectSalvage},
		{name: "marked but transfer gone", marked: true, exists: false, want: collectFull},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseCollectMode(tt.marked, tt.exists); got != tt.want {
				t.Errorf("chooseCollectMode(%v, %v) = %v, want %v", tt.marked, tt.exists, got, tt.want)
			}
		})
	}
}

func TestDriverCompactMarkerPath(t *testing.T) {
	if got := driverCompactMarkerPath("/tmp/sweep-driver-e1"); got != "/tmp/sweep-driver-e1/compact.collected" {
		t.Fatalf("driverCompactMarkerPath = %q", got)
	}
}

func TestDriverCleanupCommand(t *testing.T) {
	cmd := driverCleanupCommand("bb1", "/home/me/ssh.config", "/tmp/sweep-driver-e1")
	for _, want := range []string{"ssh", "-F", "/home/me/ssh.config", "bb1", "rm -rf"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("cleanup command missing %q\ngot: %s", want, cmd)
		}
	}
	if want := "'/tmp/sweep-driver-e1'"; !strings.Contains(cmd, want) {
		t.Errorf("cleanup command missing quoted work dir %q\ngot: %s", want, cmd)
	}
	if strings.Contains(cmd, "'ssh'") || strings.Contains(cmd, "'bb1'") {
		t.Errorf("cleanup command should not quote plain tokens\ngot: %s", cmd)
	}
}

func TestBootstrapScript(t *testing.T) {
	s := bootstrapScript("/tmp/wd", "'/tmp/wd/sweep' '-driven'", "", 65536)
	for _, want := range []string{"setsid", "exit.code", "tail -n +1 -F console.log", "WD='/tmp/wd'", "ulimit -Sn 65536 2>/dev/null", "exec '/tmp/wd/sweep' '-driven'"} {
		if !strings.Contains(s, want) {
			t.Errorf("bootstrap script missing %q\ngot:\n%s", want, s)
		}
	}
	if strings.Contains(s, "export ") {
		t.Errorf("bootstrap script exports env with no envExport given:\n%s", s)
	}
}

func TestBootstrapScriptNoFdLimit(t *testing.T) {
	s := bootstrapScript("/tmp/wd", "'/tmp/wd/sweep' '-driven'", "", 0)
	if strings.Contains(s, "ulimit") {
		t.Errorf("bootstrap script raised fd limit with fdLimit=0\ngot:\n%s", s)
	}
}

func TestBootstrapScriptEnvExport(t *testing.T) {
	s := bootstrapScript("/tmp/wd", "'/tmp/wd/sweep' '-driven'", "OLLAMA_API_KEY='secret'", 65536)
	if !strings.Contains(s, "export OLLAMA_API_KEY='secret'") {
		t.Errorf("bootstrap script missing env export\ngot:\n%s", s)
	}
	// The key must reach the run via the inherited environment only — never the
	// run.sh body (which is written to disk) nor the sweep argv.
	if strings.Contains(s, "exec OLLAMA_API_KEY=") || strings.Contains(s, "-explain-key") {
		t.Errorf("key leaked into run.sh body or argv\ngot:\n%s", s)
	}
}

// pollTestWindow and pollTestInterval keep the poll-logic tests fast and
// deterministic (real time.Sleep, but on the order of milliseconds).
const (
	pollTestWindow   = 60 * time.Millisecond
	pollTestInterval = 10 * time.Millisecond
)

// pollCheck helpers build the dialed/crashed closures the tests inject.
func constCheck(v bool) func() (bool, error) { return func() (bool, error) { return v, nil } }
func errCheck() func() (bool, error) {
	return func() (bool, error) { return false, errors.New("transient session error") }
}

// TestPollDetachedStartupDialed: once the peer-dial marker appears, the wait
// ends cleanly (safe to disconnect).
func TestPollDetachedStartupDialed(t *testing.T) {
	err := pollDetachedStartup(constCheck(true), constCheck(false), func() string { return "" }, "bb1", "/tmp/wd", pollTestWindow, pollTestInterval)
	if err != nil {
		t.Fatalf("expected a safe verdict once dialed, got error: %v", err)
	}
}

// TestPollDetachedStartupCrash: exit.code without the dial marker is a setup
// failure and must be reported with the console tail.
func TestPollDetachedStartupCrash(t *testing.T) {
	err := pollDetachedStartup(constCheck(false), constCheck(true), func() string { return "boom" }, "bb1", "/tmp/wd", pollTestWindow, pollTestInterval)
	if err == nil {
		t.Fatal("expected an error for a crash before dialing, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected error to include the console tail\ngot: %v", err)
	}
}

// TestPollDetachedStartupDialedBeatsCrash: if both the marker and exit.code
// exist (a run that dialed and then finished), the dial marker wins — it is a
// success, not a setup crash. Guards against a false crash report on a run
// that completed within the window.
func TestPollDetachedStartupDialedBeatsCrash(t *testing.T) {
	err := pollDetachedStartup(constCheck(true), constCheck(true), func() string { return "done" }, "bb1", "/tmp/wd", pollTestWindow, pollTestInterval)
	if err != nil {
		t.Fatalf("a run that dialed then exited is not a setup crash: %v", err)
	}
}

// TestPollDetachedStartupDialsAfterWaiting: the marker appearing on a later
// poll (slow setup) still resolves to a safe verdict.
func TestPollDetachedStartupDialsAfterWaiting(t *testing.T) {
	calls := 0
	dialed := func() (bool, error) { calls++; return calls >= 2, nil }
	err := pollDetachedStartup(dialed, constCheck(false), func() string { return "" }, "bb1", "/tmp/wd", pollTestWindow, pollTestInterval)
	if err != nil {
		t.Fatalf("expected a safe verdict once the marker appears: %v", err)
	}
	if calls < 2 {
		t.Errorf("expected the wait to poll until the marker appeared, got %d call(s)", calls)
	}
}

// TestPollDetachedStartupDetectsCrashAfterTransientError guards against the
// bug where a single poll error (a flaky SSH session, exactly what this wait
// exists to route around) made the launcher stop early: a crash discovered on
// a later poll must still be reported.
func TestPollDetachedStartupDetectsCrashAfterTransientError(t *testing.T) {
	calls := 0
	crashed := func() (bool, error) {
		calls++
		if calls == 1 {
			return false, errors.New("transient session error")
		}
		return true, nil
	}
	err := pollDetachedStartup(constCheck(false), crashed, func() string { return "crashed" }, "bb1", "/tmp/wd", pollTestWindow, pollTestInterval)
	if err == nil {
		t.Fatal("expected the crash discovered after the transient error to be reported")
	}
}

// TestPollDetachedStartupAllPollsFail confirms that persistent poll failures
// across the whole window are treated as inconclusive (not a crash), since a
// broken wait says nothing about whether the detached run is fine.
func TestPollDetachedStartupAllPollsFail(t *testing.T) {
	err := pollDetachedStartup(errCheck(), errCheck(), func() string { return "" }, "bb1", "/tmp/wd", pollTestWindow, pollTestInterval)
	if err != nil {
		t.Fatalf("persistent poll failures should be inconclusive, not a reported crash: %v", err)
	}
}

func TestEarlyExitMessage(t *testing.T) {
	msg := earlyExitMessage("bb1", "/tmp/wd", "")
	for _, want := range []string{"bb1", "/tmp/wd", "not a normal completion"} {
		if !strings.Contains(msg, want) {
			t.Errorf("early exit message missing %q\ngot: %s", want, msg)
		}
	}
	if strings.Contains(msg, "last console output") {
		t.Errorf("early exit message should not mention console output with an empty tail\ngot: %s", msg)
	}

	withTail := earlyExitMessage("bb1", "/tmp/wd", "connecting to 25 host(s)...\nno valid authentication methods found for bb2")
	if !strings.Contains(withTail, "no valid authentication methods found for bb2") {
		t.Errorf("early exit message missing console tail\ngot: %s", withTail)
	}
}

func TestDetachBootstrapScript(t *testing.T) {
	s := detachBootstrapScript("/tmp/wd", "'/tmp/wd/sweep' '-driven'", "", 65536)
	for _, want := range []string{"setsid", "run.sh", "[driver] detached sweep started in $WD", "WD='/tmp/wd'", "exec '/tmp/wd/sweep' '-driven'"} {
		if !strings.Contains(s, want) {
			t.Errorf("detach bootstrap script missing %q\ngot:\n%s", want, s)
		}
	}
	for _, notWant := range []string{"tail -n +1 -F console.log", "while [ ! -f exit.code ]"} {
		if strings.Contains(s, notWant) {
			t.Errorf("detach bootstrap script should not wait on the run, found %q\ngot:\n%s", notWant, s)
		}
	}
	if strings.Contains(s, "export ") {
		t.Errorf("detach bootstrap script exports env with no envExport given:\n%s", s)
	}
}

func TestDetachBootstrapScriptEnvExport(t *testing.T) {
	s := detachBootstrapScript("/tmp/wd", "'/tmp/wd/sweep' '-driven'", "OLLAMA_API_KEY='secret'", 65536)
	if !strings.Contains(s, "export OLLAMA_API_KEY='secret'") {
		t.Errorf("detach bootstrap script missing env export\ngot:\n%s", s)
	}
	if strings.Contains(s, "exec OLLAMA_API_KEY=") || strings.Contains(s, "-explain-key") {
		t.Errorf("key leaked into run.sh body or argv\ngot:\n%s", s)
	}
}

// TestFileSHA256 guards the content hash cachedUpload relies on to decide
// whether a binary needs re-uploading: identical bytes must hash identically
// regardless of file name or path, and different bytes must hash differently,
// or a stale binary could be mistaken for a current one (or vice versa).
func TestFileSHA256(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a")
	pathB := filepath.Join(dir, "b")
	pathC := filepath.Join(dir, "c")
	if err := os.WriteFile(pathA, []byte("same content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte("same content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathC, []byte("different content"), 0o644); err != nil {
		t.Fatal(err)
	}

	hashA, err := fileSHA256(pathA)
	if err != nil {
		t.Fatalf("fileSHA256(a): %v", err)
	}
	hashB, err := fileSHA256(pathB)
	if err != nil {
		t.Fatalf("fileSHA256(b): %v", err)
	}
	hashC, err := fileSHA256(pathC)
	if err != nil {
		t.Fatalf("fileSHA256(c): %v", err)
	}

	if hashA != hashB {
		t.Errorf("identical content hashed differently: %q vs %q", hashA, hashB)
	}
	if hashA == hashC {
		t.Errorf("different content hashed identically: %q", hashA)
	}
	if _, err := fileSHA256(filepath.Join(dir, "missing")); err == nil {
		t.Error("expected an error hashing a missing file")
	}
}
