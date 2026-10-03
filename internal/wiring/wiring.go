// Package wiring builds the configured providers and wraps them in the
// shared middleware. Provider packages must be blank-imported by main.
package wiring

import (
	"net/http"

	"github.com/deosjr/foc/internal/config"
	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/decision"
	"github.com/deosjr/foc/internal/providers/llm"
	"github.com/deosjr/foc/internal/providers/middleware"
	"github.com/deosjr/foc/internal/runlog"
)

// Models builds the LLM first (the "llm" decision provider needs it), then
// the decision model. Outside replay mode every response (and failure) is
// recorded in the returned in-memory store, which the game writes to its run
// directory as responses.jsonl so the run can be replayed exactly.
func Models(cfg *config.Config, log *runlog.Log) (decision.Model, llm.Model, *middleware.Store, error) {
	var cache, replay, record *middleware.Store
	var err error
	if cfg.CacheFile != "" && cfg.Mode != "replay" {
		if cache, err = middleware.OpenStore(cfg.CacheFile, false); err != nil {
			return nil, nil, nil, err
		}
	}
	if cfg.Mode == "replay" {
		if replay, err = middleware.OpenStore(cfg.ReplayFile, true); err != nil {
			return nil, nil, nil, err
		}
	} else {
		record = middleware.NewMemoryStore()
	}
	opts := func(p providers.ProviderConfig) middleware.Options {
		return middleware.Options{Name: p.Provider, Model: p.Model, Timeout: p.Timeout,
			Log: log, Cache: cache, Replay: replay, Record: record}
	}
	deps := providers.Deps{HTTP: &http.Client{}}
	rawLLM, err := providers.BuildLLM(cfg.LLM, deps)
	if err != nil {
		return nil, nil, nil, err
	}
	l := &middleware.LLM{Inner: rawLLM, Opts: opts(cfg.LLM)}
	deps.LLM = l
	rawDecision, err := providers.BuildDecision(cfg.Decision, deps)
	if err != nil {
		return nil, nil, nil, err
	}
	dopts := opts(cfg.Decision)
	if cfg.Decision.Provider == "llm" {
		// The "llm" decision provider is only as good as the LLM under it,
		// so that LLM is part of the cache key.
		dopts.Model = cfg.LLM.Provider + "/" + cfg.LLM.Model
	}
	d := &middleware.Decision{Inner: rawDecision, Opts: dopts}
	return d, l, record, nil
}
