// Package anthropic is a thin LLM adapter for the Anthropic Messages API
// over plain net/http.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/llm"
)

const (
	defaultEndpoint = "https://api.anthropic.com"
	apiVersion      = "2023-06-01"
)

func init() {
	providers.RegisterLLM("anthropic", New)
}

type Model struct {
	cfg      providers.ProviderConfig
	key      string
	endpoint string
	http     *http.Client
}

// New builds the adapter. The API key comes from the environment variable
// named in config (default ANTHROPIC_API_KEY).
func New(cfg providers.ProviderConfig, deps providers.Deps) (llm.Model, error) {
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "ANTHROPIC_API_KEY"
	}
	if cfg.Model == "" {
		return nil, errors.New("anthropic: model must be set in config")
	}
	key, err := cfg.RequireAPIKey()
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	client := deps.HTTP
	if client == nil {
		client = &http.Client{}
	}
	return &Model{cfg: cfg, key: key, endpoint: endpoint, http: client}, nil
}

// AcceptsSampling reports whether a model accepts temperature. Current
// models (Claude Sonnet 5, Claude Opus 4.7 and later, Fable) reject sampling
// parameters with a 400, so only older families get it.
func AcceptsSampling(model string) bool {
	for _, prefix := range []string{
		"claude-3", "claude-haiku-4", "claude-sonnet-4",
		"claude-opus-4-0", "claude-opus-4-1", "claude-opus-4-5", "claude-opus-4-6",
	} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type outputConfig struct {
	Effort string          `json:"effort,omitempty"`
	Format json.RawMessage `json:"format,omitempty"`
}

type request struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	System       string        `json:"system,omitempty"`
	Messages     []message     `json:"messages"`
	Temperature  *float64      `json:"temperature,omitempty"`
	OutputConfig *outputConfig `json:"output_config,omitempty"`
}

type response struct {
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason  string `json:"stop_reason"`
	StopDetails *struct {
		Category    string `json:"category"`
		Explanation string `json:"explanation"`
	} `json:"stop_details"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (m *Model) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	body := request{Model: m.cfg.Model, MaxTokens: req.MaxTokens, System: req.System}
	if body.MaxTokens <= 0 {
		body.MaxTokens = 1024
	}
	for _, msg := range req.Messages {
		body.Messages = append(body.Messages, message{Role: string(msg.Role), Content: msg.Content})
	}
	if AcceptsSampling(m.cfg.Model) {
		t := req.Temperature
		body.Temperature = &t
	}
	if m.cfg.Effort != "" || len(req.JSONSchema) > 0 {
		body.OutputConfig = &outputConfig{Effort: m.cfg.Effort}
		if len(req.JSONSchema) > 0 {
			body.OutputConfig.Format, _ = json.Marshal(map[string]any{
				"type": "json_schema", "schema": json.RawMessage(req.JSONSchema),
			})
		}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return llm.Response{}, err
	}
	hreq.Header.Set("content-type", "application/json")
	hreq.Header.Set("x-api-key", m.key)
	hreq.Header.Set("anthropic-version", apiVersion)
	hresp, err := m.http.Do(hreq)
	if err != nil {
		return llm.Response{}, err
	}
	defer hresp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(hresp.Body, 4<<20))
	if err != nil {
		return llm.Response{}, err
	}
	if hresp.StatusCode/100 != 2 {
		var ae apiError
		msg := string(raw)
		if json.Unmarshal(raw, &ae) == nil && ae.Error.Message != "" {
			msg = ae.Error.Type + ": " + ae.Error.Message
		}
		return llm.Response{}, &providers.HTTPError{Status: hresp.StatusCode, Body: msg}
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return llm.Response{}, fmt.Errorf("anthropic: bad response: %w", err)
	}
	if r.StopReason == "refusal" {
		cat := ""
		if r.StopDetails != nil {
			cat = r.StopDetails.Category
		}
		return llm.Response{}, fmt.Errorf("anthropic: request refused (category %q)", cat)
	}
	var text strings.Builder
	for _, block := range r.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		return llm.Response{}, fmt.Errorf("anthropic: no text in response (stop_reason %s)", r.StopReason)
	}
	return llm.Response{
		Text: text.String(), Provider: "anthropic", Model: r.Model,
		InputTok: r.Usage.InputTokens, OutputTok: r.Usage.OutputTokens, Raw: raw,
	}, nil
}
