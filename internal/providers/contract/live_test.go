//go:build live

package contract

import (
	"context"
	"os"
	"testing"

	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/llm/anthropic"
)

// Run with: go test -tags live ./internal/providers/contract/ -run Live
// Model defaults to claude-sonnet-5; override with FOC_LIVE_MODEL.
func TestLiveAnthropic(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	model := os.Getenv("FOC_LIVE_MODEL")
	if model == "" {
		model = "claude-sonnet-5"
	}
	m, err := anthropic.New(providers.ProviderConfig{Model: model, Effort: "low"}, providers.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := m.Complete(context.Background(), sample)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s (%d in, %d out): %s", resp.Model, resp.InputTok, resp.OutputTok, resp.Text)
}
