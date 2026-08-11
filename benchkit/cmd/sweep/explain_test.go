package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relab/gorums/benchkit"
)

// writeFailedManifest writes a started manifest for base and marks it failed in
// the given phase, returning the node assignments used.
func writeFailedManifest(t *testing.T, dir, base, phase string) []nodeAssignment {
	t.Helper()
	cfg := &config{sweepLabel: "nscale", duration: 10 * time.Second}
	p := runSpec{
		Dimensions: benchkit.Dimensions{Nodes: 2, Workers: 1, Benchmark: "Symmetric"},
		Rep:        1,
	}
	nodes := []nodeAssignment{
		{host: "bb1", peerHost: "152.94.162.21", port: 9000},
		{host: "bb2", peerHost: "152.94.162.11", port: 9000},
	}
	writeManifest(dir, base, p, nodes, cfg, "abc123", "/tmp/bench")
	if err := updateManifestOutcome(dir, base, runOutcome{
		status:       runStatusFailed,
		failurePhase: phase,
	}); err != nil {
		t.Fatalf("update outcome: %v", err)
	}
	return nodes
}

// TestUpdateManifestDiagnosis verifies that the diagnosis is recorded without
// disturbing any other manifest field, and that a second call overwrites it.
func TestUpdateManifestDiagnosis(t *testing.T) {
	dir := t.TempDir()
	const base = "nscale_Symmetric_N2_W1_r1"
	writeFailedManifest(t, dir, base, failurePhaseSetup)

	if err := updateManifestDiagnosis(dir, base, "first verdict"); err != nil {
		t.Fatalf("updateManifestDiagnosis: %v", err)
	}
	m := readManifest(t, dir, base)
	if m.Diagnosis != "first verdict" {
		t.Errorf("diagnosis = %q, want %q", m.Diagnosis, "first verdict")
	}
	// Other fields must survive the read-modify-write.
	if m.Status != runStatusFailed || m.FailurePhase != failurePhaseSetup ||
		m.Benchmark != "Symmetric" || m.Nodes != 2 {
		t.Errorf("write-back disturbed other fields: %+v", m)
	}

	if err := updateManifestDiagnosis(dir, base, "second verdict"); err != nil {
		t.Fatalf("updateManifestDiagnosis (overwrite): %v", err)
	}
	if m := readManifest(t, dir, base); m.Diagnosis != "second verdict" {
		t.Errorf("diagnosis after overwrite = %q, want %q", m.Diagnosis, "second verdict")
	}
}

// TestTrimLog checks that a log within the cap is returned verbatim and an
// over-cap log is reduced to a head+tail window with an elision marker.
func TestTrimLog(t *testing.T) {
	small := []byte("line1\nline2\n")
	if got := trimLog(small, 1024); got != string(small) {
		t.Errorf("under-cap log altered: %q", got)
	}

	big := []byte(strings.Repeat("A", 400) + strings.Repeat("B", 400))
	const cap = 200
	got := trimLog(big, cap)
	if !strings.HasPrefix(got, strings.Repeat("A", 100)) {
		t.Errorf("trimmed log missing head: %q", got[:min(40, len(got))])
	}
	if !strings.HasSuffix(got, strings.Repeat("B", 100)) {
		t.Errorf("trimmed log missing tail")
	}
	if !strings.Contains(got, "bytes elided") {
		t.Errorf("trimmed log missing elision marker: %q", got)
	}
	// Head + tail keep cap bytes; the marker adds a bounded, small overhead.
	if len(got) > cap+64 {
		t.Errorf("trimmed log %d bytes exceeds cap %d plus marker", len(got), cap)
	}
}

// TestDiscoverFailedRuns verifies that only failed runs are selected and that
// the base name is recovered from the manifest filename.
func TestDiscoverFailedRuns(t *testing.T) {
	dir := t.TempDir()
	writeFailedManifest(t, dir, "run_Symmetric_N2_W1_r1", failurePhaseSetup)
	writeFailedManifest(t, dir, "run_Symmetric_N2_W1_r2", failurePhaseMeasurement)
	// A succeeded run must be ignored.
	cfg := &config{sweepLabel: "run", duration: time.Second}
	okNodes := []nodeAssignment{{host: "bb1", peerHost: "10.0.0.1", port: 9000}}
	writeManifest(dir, "run_Symmetric_N1_W1_r1", runSpec{
		Dimensions: benchkit.Dimensions{Nodes: 1, Workers: 1, Benchmark: "Symmetric"},
		Rep:        1,
	}, okNodes, cfg, "", "")
	if err := updateManifestOutcome(dir, "run_Symmetric_N1_W1_r1", runOutcome{status: runStatusSucceeded}); err != nil {
		t.Fatalf("update outcome: %v", err)
	}

	runs, err := discoverFailedRuns(dir)
	if err != nil {
		t.Fatalf("discoverFailedRuns: %v", err)
	}
	got := make([]string, len(runs))
	for i, r := range runs {
		got[i] = r.base
	}
	slices.Sort(got)
	want := []string{"run_Symmetric_N2_W1_r1", "run_Symmetric_N2_W1_r2"}
	if !slices.Equal(got, want) {
		t.Errorf("failed runs = %v, want %v", got, want)
	}
}

// TestGatherArtifacts checks that the bundle includes the manifest, the host
// snapshot, and a trimmed node log, each under a labeled section.
func TestGatherArtifacts(t *testing.T) {
	dir := t.TempDir()
	const base = "run_Symmetric_N2_W1_r1"
	writeFailedManifest(t, dir, base, failurePhaseSetup)

	logsDir := filepath.Join(dir, logSubdir)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, base+"_snapshot.txt"), []byte("===== bb1 =====\nload 0.5\n"), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	// A benign head and tail with a single critical line buried in the middle,
	// large enough that the head+tail window elides the middle. The grep must
	// still surface the buried line.
	head := strings.Repeat("[bb1:9000] tick\n", 200)
	buried := "[bb16:9000] remote peers not ready: connection refused\n"
	tail := strings.Repeat("[bb2:9000] tick\n", 200)
	if err := os.WriteFile(filepath.Join(logsDir, base+".log"), []byte(head+buried+tail), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	bundle, err := gatherArtifacts(dir, base, 1000)
	if err != nil {
		t.Fatalf("gatherArtifacts: %v", err)
	}
	for _, want := range []string{"manifest.json", "host snapshot", "node log", "===== bb1 =====", "bytes elided", "notable log lines", "connection refused"} {
		if !strings.Contains(bundle, want) {
			t.Errorf("bundle missing %q", want)
		}
	}
	if strings.Contains(bundle, "plotdata/runs.csv") {
		t.Errorf("bundle should omit absent summary section")
	}
}

// TestSummaryRowsReadsCompactPlotData verifies that summaryRows reads the
// compact plotdata.binpb the sweep pipeline actually writes, not
// plotdata/runs.csv (which exists only after a manual -export-csv and was
// previously always empty on a fresh output directory), and filters to the
// requested run's base.
func TestSummaryRowsReadsCompactPlotData(t *testing.T) {
	dir := t.TempDir()
	const base1, base2 = "e1_Q_N1_W1_P0", "e1_Q_N2_W1_P0"
	n := nodeAssignment{host: "bb1", port: 9000}
	for _, base := range []string{base1, base2} {
		writePlotManifest(t, dir, base, runStatusSucceeded, 1, "", []string{resultFilename(base, n, resultExt)})
		writePlotReport(t, dir, base, n, "bb1:9000", benchkit.Result_builder{
			Config:     plotRunConfig("Q", 1, 1, 0, 0),
			Throughput: 10,
			Latencies:  []int64{1000, 2000},
		}.Build())
	}
	if err := writeCompactPlotData(dir); err != nil {
		t.Fatalf("writeCompactPlotData: %v", err)
	}

	rows := summaryRows(dir, base1)
	if rows == "" {
		t.Fatal("summaryRows returned empty for a run present in plotdata.binpb")
	}
	lines := strings.Split(rows, "\n")
	if len(lines) != 2 {
		t.Fatalf("rows = %d lines, want 2 (header + one matching run)", len(lines))
	}
	if !strings.HasPrefix(lines[0], "base,label,status,rep,") {
		t.Errorf("header = %q, want it to start with the CSV column names", lines[0])
	}
	if !strings.HasPrefix(lines[1], base1+",") {
		t.Errorf("data row = %q, want it to start with %q", lines[1], base1+",")
	}
	if strings.Contains(rows, base2) {
		t.Errorf("rows include the other run's base %q, want only %q", base2, base1)
	}

	if got := summaryRows(dir, "no-such-run"); got != "" {
		t.Errorf("summaryRows(unmatched base) = %q, want empty", got)
	}
}

// TestSalientLog verifies that error/warning lines are extracted from anywhere
// in the log, that a no-match log yields "", and that the byte cap is honored
// with an omission marker.
func TestSalientLog(t *testing.T) {
	log := []byte("starting up\n" +
		"[bb1:9000] all good\n" +
		"[bb16:9000] inbound peers not ready: no new peer for 20s\n" +
		"[bb2:9000] WARNING: offered rate not sustained\n" +
		"shutting down\n")
	got := salientLog(log, 0)
	if !strings.Contains(got, "not ready") || !strings.Contains(got, "WARNING") {
		t.Errorf("salient lines missing expected matches: %q", got)
	}
	if strings.Contains(got, "all good") || strings.Contains(got, "starting up") {
		t.Errorf("salient lines include benign lines: %q", got)
	}

	if got := salientLog([]byte("line a\nline b\n"), 0); got != "" {
		t.Errorf("no-match log = %q, want empty", got)
	}

	many := []byte(strings.Repeat("[bb1:9000] connection refused\n", 100))
	capped := salientLog(many, 200)
	if len(capped) > 200+64 {
		t.Errorf("capped salient log = %d bytes, want <= cap plus marker", len(capped))
	}
	if !strings.Contains(capped, "more matching line(s) omitted") {
		t.Errorf("capped salient log missing omission marker: %q", capped)
	}
}

// TestOpenAIProviderDiagnose verifies the request shape and reply parsing for
// the OpenAI-compatible client used by the local and openai providers.
func TestOpenAIProviderDiagnose(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &gotBody); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"  verdict text  "}}]}`)
	}))
	defer srv.Close()

	p := &openAIProvider{chatClient{baseURL: srv.URL, apiKey: "k-123", model: "llama3.3", client: srv.Client()}}
	got, err := p.Diagnose(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if got != "verdict text" {
		t.Errorf("verdict = %q, want trimmed %q", got, "verdict text")
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer k-123" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotBody.Model != "llama3.3" || len(gotBody.Messages) != 2 ||
		gotBody.Messages[0].Role != "system" || gotBody.Messages[0].Content != "sys" ||
		gotBody.Messages[1].Role != "user" || gotBody.Messages[1].Content != "usr" {
		t.Errorf("request body = %+v", gotBody)
	}
}

// TestOllamaProviderDiagnose verifies the request shape and reply parsing for
// the native Ollama /api/chat client used by the local provider, including the
// guard that turns an empty message into an error.
func TestOllamaProviderDiagnose(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		var gotAuth, gotPath string
		var gotBody struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotPath = r.URL.Path
			data, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(data, &gotBody); err != nil {
				t.Errorf("unmarshal request: %v", err)
			}
			// Native replies carry a single message object, not a choices array,
			// and may include a thinking field the client must ignore.
			io.WriteString(w, `{"message":{"role":"assistant","content":"  verdict text  ","thinking":"reasoning"},"done":true}`)
		}))
		defer srv.Close()

		p := &ollamaProvider{chatClient{baseURL: srv.URL, apiKey: "k-123", model: "gemma4:31b", client: srv.Client()}}
		got, err := p.Diagnose(context.Background(), "sys", "usr")
		if err != nil {
			t.Fatalf("Diagnose: %v", err)
		}
		if got != "verdict text" {
			t.Errorf("verdict = %q, want trimmed %q", got, "verdict text")
		}
		if gotPath != "/api/chat" {
			t.Errorf("path = %q, want /api/chat", gotPath)
		}
		if gotAuth != "Bearer k-123" {
			t.Errorf("auth = %q", gotAuth)
		}
		if gotBody.Model != "gemma4:31b" || gotBody.Stream != false || len(gotBody.Messages) != 2 ||
			gotBody.Messages[0].Role != "system" || gotBody.Messages[0].Content != "sys" ||
			gotBody.Messages[1].Role != "user" || gotBody.Messages[1].Content != "usr" {
			t.Errorf("request body = %+v", gotBody)
		}
	})

	t.Run("empty message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"message":{"role":"assistant","content":""},"done":true}`)
		}))
		defer srv.Close()
		p := &ollamaProvider{chatClient{baseURL: srv.URL, apiKey: "k", model: "m", client: srv.Client()}}
		if _, err := p.Diagnose(context.Background(), "s", "u"); err == nil {
			t.Error("want error for empty message")
		}
	})
}

// TestAnthropicProviderDiagnose verifies the request shape and reply parsing for
// the Anthropic Messages API client used by the claude provider.
func TestAnthropicProviderDiagnose(t *testing.T) {
	var gotKey, gotVersion, gotPath, gotSystem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		gotVersion = r.Header.Get("Anthropic-Version")
		gotPath = r.URL.Path
		var body struct {
			System string `json:"system"`
		}
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
		gotSystem = body.System
		io.WriteString(w, `{"content":[{"text":"claude verdict"}]}`)
	}))
	defer srv.Close()

	p := &anthropicProvider{chatClient{baseURL: srv.URL, apiKey: "sk-ant", model: "claude-opus-4-8", client: srv.Client()}}
	got, err := p.Diagnose(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if got != "claude verdict" {
		t.Errorf("verdict = %q", got)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != "sk-ant" || gotVersion != anthropicVersion {
		t.Errorf("headers: key=%q version=%q", gotKey, gotVersion)
	}
	if gotSystem != "sys" {
		t.Errorf("system = %q, want %q", gotSystem, "sys")
	}
}

// TestProviderErrorStatus verifies that a non-2xx reply surfaces as an error
// carrying the response body.
func TestProviderErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()

	p := &openAIProvider{chatClient{baseURL: srv.URL, apiKey: "k", model: "m", client: srv.Client()}}
	_, err := p.Diagnose(context.Background(), "s", "u")
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("error = %v, want one mentioning the response body", err)
	}
}

// TestProviderEmptyBody reproduces the failure that surfaced only as "unexpected
// end of JSON input": a 2xx reply with an empty body. The error must now name the
// status and the zero-length body so the cause is visible without re-running.
func TestProviderEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 200 OK with no body written.
	}))
	defer srv.Close()

	p := &openAIProvider{chatClient{baseURL: srv.URL, apiKey: "k", model: "m", client: srv.Client()}}
	_, err := p.Diagnose(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("want error for empty 2xx body")
	}
	for _, want := range []string{"decoding response", "200 OK", "0-byte"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestProviderMalformedBody verifies that a 2xx reply with non-JSON content
// surfaces as a decode error that includes a snippet of the offending body.
func TestProviderMalformedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<html>gateway timeout</html>")
	}))
	defer srv.Close()

	p := &openAIProvider{chatClient{baseURL: srv.URL, apiKey: "k", model: "m", client: srv.Client()}}
	_, err := p.Diagnose(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("want error for malformed 2xx body")
	}
	if !strings.Contains(err.Error(), "decoding response") || !strings.Contains(err.Error(), "gateway timeout") {
		t.Errorf("error %q missing decode context or body snippet", err.Error())
	}
}

// TestPingProvider verifies the connectivity check: a non-empty reply passes and
// is returned trimmed, while a reachable model that returns an empty reply fails.
func TestPingProvider(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"choices":[{"message":{"content":"  OK  "}}]}`)
		}))
		defer srv.Close()
		p := &openAIProvider{chatClient{baseURL: srv.URL, apiKey: "k", model: "m", client: srv.Client()}}
		got, err := pingProvider(context.Background(), p)
		if err != nil {
			t.Fatalf("pingProvider: %v", err)
		}
		if got != "OK" {
			t.Errorf("reply = %q, want trimmed %q", got, "OK")
		}
	})

	t.Run("empty reply", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"choices":[{"message":{"content":"   "}}]}`)
		}))
		defer srv.Close()
		p := &openAIProvider{chatClient{baseURL: srv.URL, apiKey: "k", model: "m", client: srv.Client()}}
		if _, err := pingProvider(context.Background(), p); err == nil {
			t.Error("want error for empty reply")
		}
	})
}

// TestNewProvider checks provider selection, endpoint defaults, and the
// required-model and required-key guards.
func TestNewProvider(t *testing.T) {
	t.Run("missing model", func(t *testing.T) {
		if _, err := newProvider(&config{explainProvider: providerLocal}); err == nil {
			t.Error("want error when -explain-model is empty")
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Setenv(envLocalKey, "")
		if _, err := newProvider(&config{explainProvider: providerLocal, explainModel: "llama3.3"}); err == nil {
			t.Error("want error when key env var is unset")
		}
	})

	t.Run("unknown provider", func(t *testing.T) {
		t.Setenv(envLocalKey, "k")
		if _, err := newProvider(&config{explainProvider: "bogus", explainModel: "m"}); err == nil {
			t.Error("want error for unknown provider")
		}
	})

	t.Run("local selects ollama", func(t *testing.T) {
		t.Setenv(envLocalKey, "k")
		p, err := newProvider(&config{explainProvider: providerLocal, explainModel: "llama3.3"})
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		op, ok := p.(*ollamaProvider)
		if !ok {
			t.Fatalf("provider type = %T, want *ollamaProvider", p)
		}
		if op.baseURL != defaultLocalEndpoint {
			t.Errorf("baseURL = %q, want %q", op.baseURL, defaultLocalEndpoint)
		}
	})

	t.Run("claude selects anthropic", func(t *testing.T) {
		t.Setenv(envClaudeKey, "sk")
		p, err := newProvider(&config{explainProvider: providerClaude, explainModel: "claude-opus-4-8"})
		if err != nil {
			t.Fatalf("newProvider: %v", err)
		}
		ap, ok := p.(*anthropicProvider)
		if !ok {
			t.Fatalf("provider type = %T, want *anthropicProvider", p)
		}
		if ap.baseURL != defaultClaudeEndpoint {
			t.Errorf("baseURL = %q, want %q", ap.baseURL, defaultClaudeEndpoint)
		}
	})
}
