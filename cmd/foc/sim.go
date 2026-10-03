package main

import (
	"flag"
	"os"

	"github.com/deosjr/foc/internal/balance"
	"github.com/deosjr/foc/internal/enemy"
	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
)

// runSim implements `foc sim`: play the scenario with simple player bots
// against the enemy AI, straight through the engine, to check balance.
func runSim(args []string) error {
	fs := flag.NewFlagSet("sim", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	scenario := fs.String("scenario", "", "scenario file (overrides config)")
	fs.Parse(args)
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if *scenario != "" {
		cfg.Scenario = *scenario
	}
	m, err := mapdata.LoadMap(cfg.Map)
	if err != nil {
		return err
	}
	rules, err := mapdata.LoadRuleset(cfg.Ruleset)
	if err != nil {
		return err
	}
	scn, err := mapdata.LoadScenario(cfg.Scenario, m)
	if err != nil {
		return err
	}
	e := &engine.Engine{Map: m, Rules: rules}
	return balance.Report(os.Stdout, e, scn, func() enemy.AI {
		if scn.EnemyAI == "heuristic" {
			return &enemy.Heuristic{Map: m}
		}
		return enemy.NewScripted(scn.EnemyRoutes)
	})
}
