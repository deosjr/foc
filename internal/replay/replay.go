// Package replay re-runs a saved game from its run directory: the same
// config and seed, the player's letters read back from turns.jsonl, and every
// model response served from responses.jsonl. It checks that the new
// turns.jsonl matches the original byte for byte, ignoring latency.
package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/deosjr/foc/internal/config"
	"github.com/deosjr/foc/internal/game"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/runlog"
	"github.com/deosjr/foc/internal/wiring"
)

// Result is the outcome of a replay.
type Result struct {
	Turns     int
	Identical bool
	Line      int    // first differing line (1-based), if not identical
	Original  string // that line in the original log
	Replayed  string // that line in the replay
	Output    string // where the replayed log was written
}

// Letters returns the player's dispatches from a turns.jsonl, by the turn
// they were sent, and the last turn the log covers.
func Letters(turns []byte) (map[int][]model.Letter, int, error) {
	byTurn := map[int][]model.Letter{}
	last := 0
	for _, line := range bytes.Split(bytes.TrimSpace(turns), []byte("\n")) {
		var rec struct {
			Turn int             `json:"turn"`
			Kind string          `json:"kind"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, 0, err
		}
		if rec.Turn > last {
			last = rec.Turn
		}
		if rec.Kind != "dispatch" {
			continue
		}
		var l model.Letter
		if err := json.Unmarshal(rec.Data, &l); err != nil {
			return nil, 0, err
		}
		byTurn[l.SentTurn] = append(byTurn[l.SentTurn], l)
	}
	for _, ls := range byTurn {
		sort.Slice(ls, func(i, j int) bool { return ls[i].Seq < ls[j].Seq })
	}
	return byTurn, last, nil
}

// Run replays the game in runDir. Providers must be registered (main
// blank-imports them); in replay mode they are never called, every response
// comes from the recording.
func Run(runDir string) (Result, error) {
	var res Result
	cfg, err := config.Load(filepath.Join(runDir, "config.yaml"))
	if err != nil {
		return res, err
	}
	cfg.Mode, cfg.ReplayFile, cfg.CacheFile = "replay", filepath.Join(runDir, "responses.jsonl"), ""
	original, err := os.ReadFile(filepath.Join(runDir, "turns.jsonl"))
	if err != nil {
		return res, err
	}
	letters, last, err := Letters(original)
	if err != nil {
		return res, fmt.Errorf("reading %s/turns.jsonl: %w", runDir, err)
	}
	log := runlog.New()
	dm, lm, _, err := wiring.Models(cfg, log)
	if err != nil {
		return res, err
	}
	g, err := game.New(game.Options{Config: cfg, Decision: dm, LLM: lm, Log: log})
	if err != nil {
		return res, err
	}
	for t := 1; t <= last && !g.Over(); t++ {
		g.Lock()
		for _, l := range letters[t] {
			if err := g.SetDraft(l.To, l.Body); err != nil {
				g.Unlock()
				return res, fmt.Errorf("turn %d: %w", t, err)
			}
		}
		err := g.EndTurn(context.Background(), nil)
		g.Unlock()
		if err != nil {
			return res, fmt.Errorf("turn %d: %w", t, err)
		}
		res.Turns = t
	}
	replayed := log.Bytes()
	res.Output = filepath.Join(runDir, "replay-turns.jsonl")
	if err := os.WriteFile(res.Output, replayed, 0o644); err != nil {
		return res, err
	}
	a, err := runlog.StripVolatile(original)
	if err != nil {
		return res, err
	}
	b, err := runlog.StripVolatile(replayed)
	if err != nil {
		return res, err
	}
	if bytes.Equal(a, b) {
		res.Identical = true
		return res, nil
	}
	al, bl := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y []byte
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if !bytes.Equal(x, y) {
			res.Line, res.Original, res.Replayed = i+1, string(x), string(y)
			break
		}
	}
	return res, nil
}
