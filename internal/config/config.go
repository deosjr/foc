// Package config loads config.yaml.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/deosjr/foc/internal/generals"
	"github.com/deosjr/foc/internal/providers"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Seed       uint64 `yaml:"seed"`
	Mode       string `yaml:"mode"` // live | record | replay
	ReplayFile string `yaml:"replay_file"`
	// CacheFile, if set, caches model responses across runs during
	// development (keyed by provider, model and request).
	CacheFile string `yaml:"cache_file"`

	Decision       providers.ProviderConfig `yaml:"decision"`
	LLM            providers.ProviderConfig `yaml:"llm"`
	Interpretation struct {
		generals.Thresholds `yaml:",inline"`
		Rationale           bool `yaml:"rationale"`
	} `yaml:"interpretation"`
	Report struct {
		MinWords int `yaml:"min_words"`
		MaxWords int `yaml:"max_words"`
	} `yaml:"report"`

	Ruleset  string `yaml:"ruleset"`
	Map      string `yaml:"map"`
	Generals string `yaml:"generals"`
	Scenario string `yaml:"scenario"`
	Prompts  string `yaml:"prompts"` // directory with questions.yaml and report.tmpl
	RunsDir  string `yaml:"runs_dir"`
}

// Default is the offline configuration: mock providers, no keys needed.
func Default() *Config {
	c := &Config{
		Seed: 42, Mode: "live",
		Decision: providers.ProviderConfig{Provider: "mock", Timeout: 10 * time.Second},
		LLM:      providers.ProviderConfig{Provider: "mock", Temperature: 0.8, MaxTokens: 400, Timeout: 30 * time.Second},
		Ruleset:  "rulesets/ancient.yaml", Map: "maps/valley.json", Generals: "content/generals.yaml",
		Scenario: "scenarios/poc.yaml", Prompts: "prompts", RunsDir: "runs",
	}
	c.Interpretation.Thresholds = generals.Thresholds{Clear: 0.8, Unclear: 0.4, Plausibility: 0.35}
	c.Interpretation.Rationale = true
	c.Report.MinWords, c.Report.MaxWords = 30, 260
	return c
}

// Load reads a config file over the defaults. An empty path gives defaults.
func Load(path string) (*Config, error) {
	c := Default()
	if path == "" {
		return c, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	switch c.Mode {
	case "live", "record", "replay":
	default:
		return nil, fmt.Errorf("%s: mode must be live, record or replay, not %q", path, c.Mode)
	}
	if c.Mode == "replay" && c.ReplayFile == "" {
		return nil, fmt.Errorf("%s: replay mode needs replay_file", path)
	}
	return c, nil
}
