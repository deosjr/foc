package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deosjr/foc/internal/eval"
	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/runlog"
	"github.com/deosjr/foc/internal/wiring"
)

// runEval implements `foc eval`: score a decision provider on labelled letters.
func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	configPath := fs.String("config", "", "config file (default: config.yaml if present, else mock defaults)")
	letters := fs.String("letters", "testdata/eval/letters.yaml", "labelled letters")
	provider := fs.String("decision", "", "decision provider to evaluate (overrides config)")
	verbose := fs.Bool("v", false, "show each letter and its full action distribution")
	concurrency := fs.Int("j", 4, "letters evaluated in parallel")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if *provider != "" {
		cfg.Decision.Provider = *provider
	}
	m, err := mapdata.LoadMap(cfg.Map)
	if err != nil {
		return err
	}
	gens, err := mapdata.LoadGenerals(cfg.Generals)
	if err != nil {
		return err
	}
	qs, err := interpret.LoadQuestions(filepath.Join(cfg.Prompts, "questions.yaml"))
	if err != nil {
		return err
	}
	ls, err := eval.Load(*letters, m, gens)
	if err != nil {
		return err
	}
	dm, _, _, err := wiring.Models(cfg, runlog.New())
	if err != nil {
		return err
	}
	results, sum := eval.Run(context.Background(), dm, ls, eval.Setup{
		Map: m, Generals: gens, Questions: qs, Thresholds: cfg.Interpretation.Thresholds, Concurrency: *concurrency,
	})
	name := cfg.Decision.Provider
	if name == "llm" {
		name = fmt.Sprintf("llm via %s %s", cfg.LLM.Provider, cfg.LLM.Model)
	}
	eval.Report(os.Stdout, name, results, sum, *verbose)
	return nil
}
