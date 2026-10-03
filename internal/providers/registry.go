// Package providers builds decision and LLM models by name. Each adapter
// package registers a factory in init(); cmd/foc blank-imports them all.
package providers

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/deosjr/foc/internal/providers/decision"
	"github.com/deosjr/foc/internal/providers/llm"
)

// ProviderConfig is one provider's section of config.yaml.
type ProviderConfig struct {
	Provider    string        `yaml:"provider"`
	Model       string        `yaml:"model"`
	Endpoint    string        `yaml:"endpoint"`
	APIKeyEnv   string        `yaml:"api_key_env"`
	Temperature float64       `yaml:"temperature"`
	MaxTokens   int           `yaml:"max_tokens"`
	Timeout     time.Duration `yaml:"timeout"`
}

// APIKey reads the key from the configured environment variable. Keys are
// never read from config files.
func (c ProviderConfig) APIKey() string {
	if c.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(c.APIKeyEnv)
}

// RequireAPIKey is APIKey, failing if the variable is unset.
func (c ProviderConfig) RequireAPIKey() (string, error) {
	if c.APIKeyEnv == "" {
		return "", fmt.Errorf("provider %s: api_key_env is not set in config", c.Provider)
	}
	k := os.Getenv(c.APIKeyEnv)
	if k == "" {
		return "", fmt.Errorf("provider %s: environment variable %s is empty", c.Provider, c.APIKeyEnv)
	}
	return k, nil
}

// Deps carries what adapters may need.
type Deps struct {
	HTTP *http.Client
	LLM  llm.Model // the built LLM, for the "llm" decision provider
}

type DecisionFactory func(cfg ProviderConfig, deps Deps) (decision.Model, error)
type LLMFactory func(cfg ProviderConfig, deps Deps) (llm.Model, error)

var (
	mu        sync.Mutex
	decisions = map[string]DecisionFactory{}
	llms      = map[string]LLMFactory{}
)

func RegisterDecision(name string, f DecisionFactory) {
	mu.Lock()
	defer mu.Unlock()
	decisions[name] = f
}

func RegisterLLM(name string, f LLMFactory) {
	mu.Lock()
	defer mu.Unlock()
	llms[name] = f
}

func BuildDecision(cfg ProviderConfig, deps Deps) (decision.Model, error) {
	mu.Lock()
	f, ok := decisions[cfg.Provider]
	mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown decision provider %q (known: %v)", cfg.Provider, names(decisions))
	}
	return f(cfg, deps)
}

func BuildLLM(cfg ProviderConfig, deps Deps) (llm.Model, error) {
	mu.Lock()
	f, ok := llms[cfg.Provider]
	mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown llm provider %q (known: %v)", cfg.Provider, names(llms))
	}
	return f(cfg, deps)
}

func names[T any](m map[string]T) []string {
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// HTTPError is a non-2xx response from a provider. The retry middleware
// retries it when Retryable reports true.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("http %d: %s", e.Status, e.Body) }

func (e *HTTPError) Retryable() bool { return e.Status == 429 || e.Status >= 500 }
