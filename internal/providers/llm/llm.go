// Package llm is the interface to text-generating models. The LLM only
// renders facts the engine has already decided; it never decides outcomes.
package llm

import "context"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	System      string    `json:"system"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	JSONSchema  []byte    `json:"json_schema,omitempty"` // optional; adapters that support structured output use it
}

type Response struct {
	Text      string `json:"text"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	InputTok  int    `json:"input_tokens"`
	OutputTok int    `json:"output_tokens"`
	Raw       []byte `json:"raw,omitempty"`
}

type Model interface {
	Complete(ctx context.Context, req Request) (Response, error)
}
