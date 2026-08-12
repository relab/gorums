package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
)

// Provider identifiers accepted by -explain-provider. Each maps to a distinct
// client because the endpoints differ in both path and wire format: local uses
// Ollama's native /api/chat API (the UiS server is proxied so that only the
// /api/ routes are reachable; the OpenAI-compatible /v1 path returns an empty
// 200 there), openai uses the OpenAI /v1/chat/completions API, and claude uses
// the Anthropic Messages API.
const (
	providerLocal  = "local"
	providerOpenAI = "openai"
	providerClaude = "claude"
)

// Default API endpoints per provider; each provider's Diagnose appends the path
// its API expects.
const (
	defaultLocalEndpoint  = "https://ollama.ux.uis.no"
	defaultOpenAIEndpoint = "https://api.openai.com"
	defaultClaudeEndpoint = "https://api.anthropic.com"
)

// Environment variables holding each provider's API key. Keys are read from the
// environment only, never from flags or disk, so they are not committed.
const (
	envLocalKey  = "OLLAMA_API_KEY"
	envOpenAIKey = "OPENAI_API_KEY"
	envClaudeKey = "ANTHROPIC_API_KEY"
)

// anthropicVersion is the required Anthropic API version header value.
const anthropicVersion = "2023-06-01"

// maxResponseTokens bounds the diagnosis length; a short plain-text verdict
// fits comfortably.
const maxResponseTokens = 1024

// llmProvider sends a system+user prompt to a configured model and returns the
// model's plain-text reply. The context bounds the request.
type llmProvider interface {
	Diagnose(ctx context.Context, system, user string) (string, error)
}

// newProvider builds the provider selected by cfg.explainProvider, reading the
// API key from the provider's environment variable. It fails if the provider is
// unknown, the model is empty, or the key variable is unset.
func newProvider(cfg *config) (llmProvider, error) {
	if cfg.explainModel == "" {
		return nil, fmt.Errorf("-explain-model is required (e.g. -explain-model llama3.3)")
	}
	key, err := requireKey(providerKeyEnv(cfg.explainProvider))
	if err != nil {
		return nil, err
	}
	switch cfg.explainProvider {
	case providerLocal:
		return &ollamaProvider{chatClient{baseURL: defaultLocalEndpoint, apiKey: key, model: cfg.explainModel, client: http.DefaultClient}}, nil
	case providerOpenAI:
		return &openAIProvider{chatClient{baseURL: defaultOpenAIEndpoint, apiKey: key, model: cfg.explainModel, client: http.DefaultClient}}, nil
	case providerClaude:
		return &anthropicProvider{chatClient{baseURL: defaultClaudeEndpoint, apiKey: key, model: cfg.explainModel, client: http.DefaultClient}}, nil
	default:
		return nil, fmt.Errorf("unknown -explain-provider %q (use local, openai, or claude)", cfg.explainProvider)
	}
}

// providerKeyEnv returns the environment variable holding the given provider's
// API key. An unknown provider maps to the local key; newProvider rejects the
// provider itself, so the caller still gets a clear error.
func providerKeyEnv(provider string) string {
	switch provider {
	case providerOpenAI:
		return envOpenAIKey
	case providerClaude:
		return envClaudeKey
	default:
		return envLocalKey
	}
}

// requireKey reads an API key from the named environment variable, failing with
// a message that names the variable when it is unset or empty.
func requireKey(name string) (string, error) {
	key := os.Getenv(name)
	if key == "" {
		return "", fmt.Errorf("%s is not set; export your API key in that environment variable", name)
	}
	return key, nil
}

// chatClient holds the endpoint, credentials, model, and HTTP client shared by
// every provider. Each provider embeds it and differs only in the request path,
// wire format, and response shape.
type chatClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// ollamaProvider talks to Ollama's native /api/chat endpoint. It takes the same
// role/content messages as the OpenAI API but replies with a single message
// object rather than a choices array, and lives under /api/ so it works through
// the UiS server's proxy, which does not forward the OpenAI-compatible /v1 path.
type ollamaProvider struct{ chatClient }

func (p *ollamaProvider) Diagnose(ctx context.Context, system, user string) (string, error) {
	reqBody := chatReqBody(p.model, system, user)
	var respBody struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	url := strings.TrimRight(p.baseURL, "/") + "/api/chat"
	header := http.Header{"Authorization": {"Bearer " + p.apiKey}}
	if err := postJSON(ctx, p.client, url, header, reqBody, &respBody); err != nil {
		return "", err
	}
	content := strings.TrimSpace(respBody.Message.Content)
	if content == "" {
		return "", fmt.Errorf("model returned an empty message")
	}
	return content, nil
}

// openAIProvider talks to an OpenAI-compatible /v1/chat/completions endpoint,
// used by the openai provider (and any other host that speaks that wire format).
type openAIProvider struct{ chatClient }

func (p *openAIProvider) Diagnose(ctx context.Context, system, user string) (string, error) {
	reqBody := chatReqBody(p.model, system, user)
	var respBody struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	url := strings.TrimRight(p.baseURL, "/") + "/v1/chat/completions"
	header := http.Header{"Authorization": {"Bearer " + p.apiKey}}
	if err := postJSON(ctx, p.client, url, header, reqBody, &respBody); err != nil {
		return "", err
	}
	if len(respBody.Choices) == 0 {
		return "", fmt.Errorf("model returned no choices")
	}
	return strings.TrimSpace(respBody.Choices[0].Message.Content), nil
}

// anthropicProvider talks to the Anthropic Messages API (/v1/messages). The
// system prompt is a top-level field; the user prompt is the sole message.
type anthropicProvider struct{ chatClient }

func (p *anthropicProvider) Diagnose(ctx context.Context, system, user string) (string, error) {
	reqBody := map[string]any{
		"model":      p.model,
		"max_tokens": maxResponseTokens,
		"system":     system,
		"messages": []map[string]string{
			{"role": "user", "content": user},
		},
	}
	var respBody struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	url := strings.TrimRight(p.baseURL, "/") + "/v1/messages"
	header := http.Header{
		"X-Api-Key":         {p.apiKey},
		"Anthropic-Version": {anthropicVersion},
	}
	if err := postJSON(ctx, p.client, url, header, reqBody, &respBody); err != nil {
		return "", err
	}
	if len(respBody.Content) == 0 {
		return "", fmt.Errorf("model returned no content")
	}
	return strings.TrimSpace(respBody.Content[0].Text), nil
}

// chatReqBody builds the request body shared by the Ollama and OpenAI chat
// APIs: the model plus a system and user message, with streaming disabled.
func chatReqBody(model, system, user string) map[string]any {
	return map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"stream": false,
	}
}

// postJSON marshals body to JSON, POSTs it to url with Content-Type and the
// extra headers, and decodes a successful JSON reply into out. A non-2xx
// response is returned as an error including the response body for diagnosis.
func postJSON(ctx context.Context, client *http.Client, url string, header http.Header, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	maps.Copy(req.Header, header)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response (%s): %w", resp.Status, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, bodySnippet(data))
	}
	// A 2xx with an empty or non-JSON body is the failure that "unexpected end of
	// JSON input" used to hide: name the status, the body length, and a snippet so
	// the cause (empty reply, gateway error page, truncated stream) is visible in
	// the log without re-running the failing scenario.
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding response (%s, %d-byte body): %w; body: %q", resp.Status, len(data), err, bodySnippet(data))
	}
	return nil
}

// maxBodySnippet bounds the response-body excerpt included in error messages so
// a multi-kilobyte error page does not flood the log.
const maxBodySnippet = 512

// bodySnippet returns a trimmed, length-bounded rendering of an HTTP response
// body for error messages. An empty body renders as the empty string, so a
// 0-byte reply is unambiguous in the message.
func bodySnippet(data []byte) string {
	s := strings.TrimSpace(string(data))
	if len(s) > maxBodySnippet {
		return fmt.Sprintf("%s... [+%d bytes]", s[:maxBodySnippet], len(s)-maxBodySnippet)
	}
	return s
}
