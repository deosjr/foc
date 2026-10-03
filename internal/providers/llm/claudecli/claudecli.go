// Package claudecli is an LLM adapter that runs `claude -p` (Claude Code in
// print mode). It authenticates with the user's Claude Code login instead of
// an API key. Each call is a separate Claude Code process, started in an
// empty scratch directory with tools, MCP servers, user settings and session
// persistence switched off where the installed version supports it, so the
// user's plugins and hooks do not run on every call.
package claudecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/llm"
)

func init() {
	providers.RegisterLLM("claudecli", New)
}

type Model struct {
	cfg     providers.ProviderConfig
	command string
	dir     string   // empty scratch directory every call runs in
	flags   []string // isolation flags the installed CLI supports
	sem     chan struct{}
}

// isolation lists the flags we would like, each with the help-text token
// that shows the installed version supports it. Unsupported ones are skipped.
var isolation = []struct {
	token string
	args  []string
}{
	{"--safe-mode", []string{"--safe-mode"}},                 // no plugins, hooks, MCP, skills or CLAUDE.md; auth still works
	{"--tools", []string{"--tools", ""}},                     // no built-in tools
	{"--strict-mcp-config", []string{"--strict-mcp-config"}}, // no MCP servers
	{"--setting-sources", []string{"--setting-sources", "project"}},
	{"--no-session-persistence", []string{"--no-session-persistence"}},
	{"--max-turns", []string{"--max-turns", "1"}},
}

// New finds the CLI, reads its help to see which isolation flags it has,
// and makes the scratch directory.
func New(cfg providers.ProviderConfig, _ providers.Deps) (llm.Model, error) {
	command := cfg.Command
	if command == "" {
		command = "claude"
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("claudecli: %q not found; install Claude Code and sign in first", command)
	}
	help, err := exec.Command(path, "--help").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("claudecli: %s --help: %v", path, err)
	}
	if !bytes.Contains(help, []byte("--print")) || !bytes.Contains(help, []byte("--output-format")) {
		return nil, errors.New("claudecli: this claude CLI has no --print / --output-format; update Claude Code")
	}
	if !bytes.Contains(help, []byte("--system-prompt")) {
		return nil, errors.New("claudecli: this claude CLI has no --system-prompt; update Claude Code")
	}
	m := &Model{cfg: cfg, command: path}
	for _, f := range isolation {
		if bytes.Contains(help, []byte(f.token)) {
			m.flags = append(m.flags, f.args...)
		}
	}
	if m.dir, err = os.MkdirTemp("", "foc-claudecli-"); err != nil {
		return nil, err
	}
	n := cfg.Concurrency
	if n <= 0 {
		n = 4
	}
	m.sem = make(chan struct{}, n)
	return m, nil
}

// Flags returns the isolation flags in use, for logging and tests.
func (m *Model) Flags() []string { return m.flags }

// prompt flattens the conversation into one prompt: print mode takes a
// single user turn, so earlier turns (a rejected draft and the complaint
// about it) are quoted as context.
func prompt(msgs []llm.Message) string {
	if len(msgs) == 1 {
		return msgs[0].Content
	}
	var b strings.Builder
	for i, msg := range msgs[:len(msgs)-1] {
		who := "MY REQUEST"
		if msg.Role == llm.RoleAssistant {
			who = "YOUR EARLIER REPLY"
		}
		if i > 0 && msg.Role == llm.RoleUser {
			who = "MY FOLLOW-UP"
		}
		fmt.Fprintf(&b, "%s:\n<<<\n%s\n>>>\n\n", who, msg.Content)
	}
	fmt.Fprintf(&b, "NOW:\n%s", msgs[len(msgs)-1].Content)
	return b.String()
}

type result struct {
	Type      string                     `json:"type"`
	Subtype   string                     `json:"subtype"`
	IsError   bool                       `json:"is_error"`
	Result    string                     `json:"result"`
	CostUSD   float64                    `json:"total_cost_usd"`
	ModelUsed map[string]json.RawMessage `json:"modelUsage"`
	Usage     struct {
		InputTokens   int `json:"input_tokens"`
		CacheCreation int `json:"cache_creation_input_tokens"`
		CacheRead     int `json:"cache_read_input_tokens"`
		OutputTokens  int `json:"output_tokens"`
	} `json:"usage"`
}

func (m *Model) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	select {
	case m.sem <- struct{}{}:
		defer func() { <-m.sem }()
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	}
	args := []string{"-p", "--output-format", "json", "--system-prompt", req.System}
	if m.cfg.Model != "" {
		args = append(args, "--model", m.cfg.Model)
	}
	args = append(args, m.flags...)
	cmd := exec.CommandContext(ctx, m.command, args...)
	cmd.Dir = m.dir
	cmd.Stdin = strings.NewReader(prompt(req.Messages))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return llm.Response{}, ctx.Err()
		}
		return llm.Response{}, fmt.Errorf("claudecli: %v: %s", err, tail(stderr.String()+stdout.String()))
	}
	var r result
	if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
		return llm.Response{}, fmt.Errorf("claudecli: unexpected output: %s", tail(stdout.String()))
	}
	if r.IsError {
		return llm.Response{}, fmt.Errorf("claudecli: %s: %s", r.Subtype, tail(r.Result))
	}
	if strings.TrimSpace(r.Result) == "" {
		return llm.Response{}, errors.New("claudecli: empty result")
	}
	model := mainModel(r.ModelUsed, m.cfg.Model)
	return llm.Response{
		Text: r.Result, Provider: "claudecli", Model: model,
		InputTok:  r.Usage.InputTokens + r.Usage.CacheCreation + r.Usage.CacheRead,
		OutputTok: r.Usage.OutputTokens, Raw: stdout.Bytes(),
	}, nil
}

// mainModel picks the model that wrote the most output; Claude Code may
// also report small helper calls on another model. Ties go to the lowest id.
func mainModel(usage map[string]json.RawMessage, fallback string) string {
	best, bestOut := fallback, -1
	for id, raw := range usage {
		var u struct {
			OutputTokens int `json:"outputTokens"`
		}
		json.Unmarshal(raw, &u)
		if u.OutputTokens > bestOut || (u.OutputTokens == bestOut && id < best) {
			best, bestOut = id, u.OutputTokens
		}
	}
	return best
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = "…" + s[len(s)-400:]
	}
	return s
}
