package replay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deosjr/foc/internal/config"
	"github.com/deosjr/foc/internal/game"
	_ "github.com/deosjr/foc/internal/providers/decision/mock"
	_ "github.com/deosjr/foc/internal/providers/llm/mock"
	"github.com/deosjr/foc/internal/runlog"
	"github.com/deosjr/foc/internal/wiring"
)

// play records a game the way main does: real wiring, recorder, autosave.
func play(t *testing.T, scenario string, letters []map[string]string, turns int) string {
	t.Helper()
	cfg := config.Default()
	cfg.Map, cfg.Ruleset, cfg.Generals = "../../maps/valley.json", "../../rulesets/ancient.yaml", "../../content/generals.yaml"
	cfg.Scenario, cfg.Prompts = "../../scenarios/"+scenario, "../../prompts"
	dir := t.TempDir()
	log := runlog.New()
	dm, lm, rec, err := wiring.Models(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	g, err := game.New(game.Options{Config: cfg, Decision: dm, LLM: lm, Log: log, RunDir: dir, Recorder: rec})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < turns && !g.Over(); i++ {
		if i < len(letters) {
			for id, text := range letters[i] {
				g.SetDraft(id, text)
			}
		}
		if err := g.EndTurn(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var letters = []map[string]string{
	{"velk": "March on Marren and take it.", "saris": "Dig in on the Duna Hills.", "hesk": "Support Ione Saris."},
	{},
	{"hesk": "Send riders into Duna Hills.", "saris": "March!"},
	{"velk": "Press on toward Sarnos."},
}

func TestReplayIsExact(t *testing.T) {
	dir := play(t, "full.yaml", letters, 8)
	for _, f := range []string{"config.yaml", "turns.jsonl", "responses.jsonl", "state.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("autosave did not write %s: %v", f, err)
		}
	}
	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Identical {
		t.Fatalf("replay differs at line %d:\noriginal: %.400s\nreplayed: %.400s", res.Line, res.Original, res.Replayed)
	}
	if res.Turns < 4 {
		t.Errorf("replayed only %d turns", res.Turns)
	}
}

func TestReplayNoticesADifferentRun(t *testing.T) {
	dir := play(t, "poc.yaml", letters, 6)
	path := filepath.Join(dir, "config.yaml")
	cfg, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(cfg), "seed: 42", "seed: 7", 1)), 0o644)
	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Identical || res.Line == 0 {
		t.Error("a replay with another seed should not match the original")
	}
}
