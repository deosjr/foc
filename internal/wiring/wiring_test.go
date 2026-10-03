package wiring

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/deosjr/foc/internal/config"
	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/decision"
	"github.com/deosjr/foc/internal/providers/llm"
	"github.com/deosjr/foc/internal/runlog"
)

// countingLLM answers decision prompts with fixed JSON and counts calls.
type countingLLM struct{ calls *int }

func (c countingLLM) Complete(context.Context, llm.Request) (llm.Response, error) {
	*c.calls++
	return llm.Response{Text: `{"q": 0.5}`}, nil
}

// Two different LLMs behind the "llm" decision provider must not share
// cached decisions.
func TestDecisionCacheKeyIncludesTheLLM(t *testing.T) {
	calls := 0
	for _, name := range []string{"test-llm-a", "test-llm-b"} {
		providers.RegisterLLM(name, func(providers.ProviderConfig, providers.Deps) (llm.Model, error) {
			return countingLLM{&calls}, nil
		})
	}
	providers.RegisterDecision("llm", func(cfg providers.ProviderConfig, deps providers.Deps) (decision.Model, error) {
		return passthrough{deps.LLM}, nil
	})
	cache := filepath.Join(t.TempDir(), "cache.jsonl")
	req := decision.Request{State: "s", Questions: []decision.Question{{ID: "q", Kind: decision.KindScore}}}
	for _, name := range []string{"test-llm-a", "test-llm-b", "test-llm-a"} {
		cfg := config.Default()
		cfg.CacheFile = cache
		cfg.Decision.Provider = "llm"
		cfg.LLM.Provider = name
		d, _, err := Models(cfg, runlog.New(), "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Decide(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Errorf("LLM called %d times, want 2 (once per distinct LLM; the third run is cached)", calls)
	}
}

type passthrough struct{ l llm.Model }

func (p passthrough) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	if _, err := p.l.Complete(ctx, llm.Request{}); err != nil {
		return decision.Response{}, err
	}
	return decision.Response{Answers: []decision.Answer{{QuestionID: "q", Score: 0.5}}}, nil
}
