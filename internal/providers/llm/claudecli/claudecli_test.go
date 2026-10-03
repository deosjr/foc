package claudecli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/llm"
)

func fake(t *testing.T) *Model {
	t.Helper()
	path, _ := filepath.Abs("testdata/fake-claude")
	m, err := New(providers.ProviderConfig{Provider: "claudecli", Command: path, Model: "sonnet"}, providers.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	return m.(*Model)
}

func TestComplete(t *testing.T) {
	m := fake(t)
	resp, err := m.Complete(context.Background(), llm.Request{
		System:   "You are Damar Velk.",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "FACTS: {}"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-p --output-format json --system-prompt You are Damar Velk.", "--model sonnet", "--tools", "--strict-mcp-config", "PROMPT[FACTS: {}]"} {
		if !strings.Contains(resp.Text, want) {
			t.Errorf("missing %q in %s", want, resp.Text)
		}
	}
	// Flags the installed CLI does not list are left out.
	if strings.Contains(resp.Text, "--setting-sources") || strings.Contains(resp.Text, "--max-turns") {
		t.Errorf("used an unsupported flag: %s", resp.Text)
	}
	// Every call runs in the empty scratch directory, not the project.
	if wd, _ := os.Getwd(); strings.Contains(resp.Text, "CWD["+wd+"]") || !strings.Contains(resp.Text, "foc-claudecli-") {
		t.Errorf("not run in the scratch directory: %s", resp.Text)
	}
	if resp.InputTok != 11 || resp.OutputTok != 7 || resp.Model != "claude-sonnet-5" {
		t.Errorf("usage/model = %d %d %q", resp.InputTok, resp.OutputTok, resp.Model)
	}
}

func TestRetryConversationIsFlattened(t *testing.T) {
	m := fake(t)
	resp, err := m.Complete(context.Background(), llm.Request{System: "s", Messages: []llm.Message{
		{Role: llm.RoleUser, Content: "write a letter"},
		{Role: llm.RoleAssistant, Content: "a bad letter"},
		{Role: llm.RoleUser, Content: "fix the number"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"MY REQUEST:", "YOUR EARLIER REPLY:", "a bad letter", "NOW:\nfix the number"} {
		if !strings.Contains(resp.Text, want) {
			t.Errorf("missing %q in %s", want, resp.Text)
		}
	}
}

func TestErrors(t *testing.T) {
	m := fake(t)
	for _, text := range []string{"FAIL-EXIT", "FAIL-RESULT"} {
		_, err := m.Complete(context.Background(), llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: text}}})
		if err == nil {
			t.Errorf("%s: no error", text)
		}
	}
	if _, err := New(providers.ProviderConfig{Command: "no-such-claude-binary"}, providers.Deps{}); err == nil {
		t.Error("a missing CLI should fail at start-up")
	}
}
