// Package mock is an offline LLM: it fills the template letter from the facts
// in a report request. It never touches the network.
package mock

import (
	"context"
	"strings"

	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/llm"
	"github.com/deosjr/foc/internal/report"
)

func init() {
	providers.RegisterLLM("mock", func(cfg providers.ProviderConfig, _ providers.Deps) (llm.Model, error) {
		return New(), nil
	})
}

type Model struct{}

func New() *Model { return &Model{} }

func (m *Model) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	text := "To my sovereign,\n\nI have your letter and will do as you command.\n\nYour servant"
	for _, msg := range req.Messages {
		if msg.Role != llm.RoleUser {
			continue
		}
		if f, ok := report.FactsFromPrompt(msg.Content); ok {
			text = report.Fallback(f)
			break
		}
		if f, ok := report.RationaleFromPrompt(msg.Content); ok {
			text = report.FallbackRationale(f)
			break
		}
	}
	return llm.Response{
		Text: text, Provider: "mock", Model: "template",
		InputTok: len(strings.Fields(req.System)), OutputTok: len(strings.Fields(text)),
	}, nil
}
