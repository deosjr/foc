package balance

import (
	"testing"

	"github.com/deosjr/foc/internal/enemy"
	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// The balance targets for the full scenario: no bot ends the game early by
// winning, recklessness is punished and pure defence loses.
func TestFullScenarioBalance(t *testing.T) {
	m, _ := mapdata.LoadMap("../../maps/valley.json")
	r, _ := mapdata.LoadRuleset("../../rulesets/ancient.yaml")
	scn, err := mapdata.LoadScenario("../../scenarios/full.yaml", m)
	if err != nil {
		t.Fatal(err)
	}
	e := &engine.Engine{Map: m, Rules: r}
	results := map[string]Result{}
	for _, b := range Bots {
		res, err := Play(e, scn, &enemy.Heuristic{Map: m}, b)
		if err != nil {
			t.Fatal(err)
		}
		results[b.Name] = res
		if res.Winner == model.Player && res.Turns < 12 {
			t.Errorf("%s won on turn %d; games should not end that early", b.Name, res.Turns)
		}
	}
	if results["rush"].Winner == model.Player {
		t.Errorf("reckless rushing should not win: %+v", results["rush"])
	}
	if results["turtle"].Winner == model.Player {
		t.Errorf("pure defence should not win: %+v", results["turtle"])
	}
	if results["steady"].Turns < 12 {
		t.Errorf("a sensible game should run long: %+v", results["steady"])
	}
}
