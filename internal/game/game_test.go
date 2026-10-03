package game

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/deosjr/foc/internal/config"
	dmock "github.com/deosjr/foc/internal/providers/decision/mock"
	lmock "github.com/deosjr/foc/internal/providers/llm/mock"
	"github.com/deosjr/foc/internal/runlog"
	"gopkg.in/yaml.v3"
)

var update = flag.Bool("update", false, "rewrite golden files")

func testConfig() *config.Config {
	c := config.Default()
	c.Map = "../../maps/valley.json"
	c.Ruleset = "../../rulesets/ancient.yaml"
	c.Generals = "../../content/generals.yaml"
	c.Scenario = "../../scenarios/poc.yaml"
	c.Prompts = "../../prompts"
	return c
}

func newTestGame(t *testing.T) (*Game, *runlog.Log) {
	t.Helper()
	log := runlog.New()
	g, err := New(Options{Config: testConfig(), Decision: dmock.New(), LLM: lmock.New(), Log: log, RunDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return g, log
}

func playCampaign(t *testing.T, path string) (*Game, *runlog.Log) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Turns []map[string]string `yaml:"turns"`
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	g, log := newTestGame(t)
	for i := 0; i < 10 && !g.Over(); i++ {
		if i < len(c.Turns) {
			for gid, text := range c.Turns[i] {
				if err := g.SetDraft(gid, text); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := g.EndTurn(context.Background(), nil); err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
	}
	return g, log
}

// stripVolatile removes fields that legitimately differ between runs.
func stripVolatile(t *testing.T, jsonl []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, line := range bytes.Split(bytes.TrimSpace(jsonl), []byte("\n")) {
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		if d, ok := rec["data"].(map[string]any); ok {
			delete(d, "latency_ms")
		}
		b, _ := json.Marshal(rec)
		out.Write(append(b, '\n'))
	}
	return out.Bytes()
}

func TestCampaignGolden(t *testing.T) {
	g, log := playCampaign(t, "../../testdata/campaign1.yaml")
	if !g.Over() {
		t.Fatal("game did not end within 10 turns")
	}
	got := stripVolatile(t, log.Bytes())
	golden := "../../testdata/campaign1.turns.jsonl"
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	if !bytes.Equal(got, want) {
		gl, wl := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
		for i := range gl {
			if i >= len(wl) || gl[i] != wl[i] {
				w := ""
				if i < len(wl) {
					w = wl[i]
				}
				t.Fatalf("turns.jsonl differs from golden at line %d:\ngot:  %.300s\nwant: %.300s", i+1, gl[i], w)
			}
		}
		t.Fatal("turns.jsonl differs from golden (length)")
	}
}

func TestCampaignDeterministic(t *testing.T) {
	_, a := playCampaign(t, "../../testdata/campaign1.yaml")
	_, b := playCampaign(t, "../../testdata/campaign1.yaml")
	if !bytes.Equal(stripVolatile(t, a.Bytes()), stripVolatile(t, b.Bytes())) {
		t.Error("two runs with the same seed and letters produced different logs")
	}
}

func TestReportsReachThePlayer(t *testing.T) {
	g, _ := newTestGame(t)
	g.SetDraft("velk", "March on Hollow Wood.")
	if err := g.EndTurn(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	// Both generals start adjacent to Karsa, so their reports arrive this turn.
	inbox := g.Inbox()
	if len(inbox) != 2 {
		t.Fatalf("inbox has %d letters, want 2", len(inbox))
	}
	for _, l := range inbox {
		if l.WrittenTurn != 1 || l.ArrivedTurn != 1 || l.Body == "" {
			t.Errorf("bad inbox letter %+v", l)
		}
	}
	sent := g.Sent()
	if len(sent) != 1 || sent[0].ReplyID == "" {
		t.Errorf("sent letter should link its reply: %+v", sent)
	}
	if g.Draft("velk") != "" {
		t.Error("drafts should be cleared when the turn ends")
	}
	if g.Turn() != 2 {
		t.Errorf("turn = %d", g.Turn())
	}
}

func TestSave(t *testing.T) {
	g, _ := playCampaign(t, "../../testdata/campaign1.yaml")
	dir, err := g.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"config.yaml", "turns.jsonl", "state.json"} {
		if _, err := os.Stat(dir + "/" + f); err != nil {
			t.Error(err)
		}
	}
}
