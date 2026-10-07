package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
