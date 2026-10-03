// Package openaicompat is an LLM adapter for any OpenAI-style chat
// completions endpoint: OpenAI, OpenRouter, Ollama, llama.cpp, vLLM.
package openaicompat

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

func init() {
	providers.RegisterLLM("openaicompat", New)
}

type Model struct {
	cfg      providers.ProviderConfig
	key      string
	endpoint string
	http     *http.Client
}

// New builds the adapter. endpoint is required (e.g.
// http://localhost:11434/v1); the API key is optional, for local servers.
func New(cfg providers.ProviderConfig, deps providers.Deps) (llm.Model, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("openaicompat: endpoint must be set in config, e.g. http://localhost:11434/v1")
	}
	if cfg.Model == "" {
		return nil, errors.New("openaicompat: model must be set in config")
	}
	key := ""
	if cfg.APIKeyEnv != "" {
		var err error
		if key, err = cfg.RequireAPIKey(); err != nil {
			return nil, err
		}
	}
	client := deps.HTTP
	if client == nil {
		client = &http.Client{}
	}
	return &Model{cfg: cfg, key: key, endpoint: strings.TrimRight(cfg.Endpoint, "/"), http: client}, nil
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Model          string    `json:"model"`
	Messages       []message `json:"messages"`
	MaxTokens      int       `json:"max_tokens,omitempty"`
	Temperature    float64   `json:"temperature"`
	ResponseFormat any       `json:"response_format,omitempty"`
}

type response struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (m *Model) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	body := request{Model: m.cfg.Model, MaxTokens: req.MaxTokens, Temperature: req.Temperature}
	if req.System != "" {
		body.Messages = append(body.Messages, message{Role: "system", Content: req.System})
	}
	for _, msg := range req.Messages {
		body.Messages = append(body.Messages, message{Role: string(msg.Role), Content: msg.Content})
	}
	if m.cfg.StructuredOutput && len(req.JSONSchema) > 0 {
		body.ResponseFormat = map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "answers", "strict": true, "schema": json.RawMessage(req.JSONSchema)},
		}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return llm.Response{}, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return llm.Response{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	if m.key != "" {
		hreq.Header.Set("Authorization", "Bearer "+m.key)
	}
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
		return llm.Response{}, &providers.HTTPError{Status: hresp.StatusCode, Body: string(raw)}
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return llm.Response{}, fmt.Errorf("openaicompat: bad response: %w", err)
	}
	if len(r.Choices) == 0 || r.Choices[0].Message.Content == "" {
		return llm.Response{}, errors.New("openaicompat: no text in response")
	}
	return llm.Response{
		Text: r.Choices[0].Message.Content, Provider: "openaicompat", Model: r.Model,
		InputTok: r.Usage.PromptTokens, OutputTok: r.Usage.CompletionTokens, Raw: raw,
	}, nil
}
