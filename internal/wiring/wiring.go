// Package wiring builds the configured providers and wraps them in the
// shared middleware. Provider packages must be blank-imported by main.
package wiring

import (
	"net/http"
	"path/filepath"

	"github.com/deosjr/foc/internal/config"
	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/decision"
	"github.com/deosjr/foc/internal/providers/llm"
	"github.com/deosjr/foc/internal/providers/middleware"
	"github.com/deosjr/foc/internal/runlog"
)

// Models builds the LLM first (the "llm" decision provider needs it), then
// the decision model. runDir is where record mode writes responses.jsonl.
func Models(cfg *config.Config, log *runlog.Log, runDir string) (decision.Model, llm.Model, error) {
	var cache, replay, record *middleware.Store
	var err error
	if cfg.CacheFile != "" && cfg.Mode != "replay" {
		if cache, err = middleware.OpenStore(cfg.CacheFile, false); err != nil {
			return nil, nil, err
		}
	}
	switch cfg.Mode {
	case "replay":
		if replay, err = middleware.OpenStore(cfg.ReplayFile, true); err != nil {
			return nil, nil, err
		}
	case "record":
		if record, err = middleware.OpenStore(filepath.Join(runDir, "responses.jsonl"), false); err != nil {
			return nil, nil, err
		}
	}
	opts := func(p providers.ProviderConfig) middleware.Options {
		return middleware.Options{Name: p.Provider, Model: p.Model, Timeout: p.Timeout,
			Log: log, Cache: cache, Replay: replay, Record: record}
	}
	deps := providers.Deps{HTTP: &http.Client{}}
	rawLLM, err := providers.BuildLLM(cfg.LLM, deps)
	if err != nil {
		return nil, nil, err
	}
	l := &middleware.LLM{Inner: rawLLM, Opts: opts(cfg.LLM)}
	deps.LLM = l
	rawDecision, err := providers.BuildDecision(cfg.Decision, deps)
	if err != nil {
		return nil, nil, err
	}
	d := &middleware.Decision{Inner: rawDecision, Opts: opts(cfg.Decision)}
	return d, l, nil
}
