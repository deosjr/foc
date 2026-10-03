package enemy

import (
	"testing"

	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

func setup(t *testing.T) (*engine.Engine, *mapdata.Map) {
	t.Helper()
	m, err := mapdata.LoadMap("../../maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := mapdata.LoadRuleset("../../rulesets/ancient.yaml")
	return &engine.Engine{Map: m, Rules: r}, m
}

func orderOf(orders []model.Order, id string) model.Order {
	for _, o := range orders {
		if o.ArmyID == id {
			return o
		}
	}
	return model.Order{}
}

func TestHeuristic(t *testing.T) {
	e, m := setup(t)
	s := e.NewState(&mapdata.Scenario{TurnCap: 20})
	s.Armies["e-1"] = &engine.Army{ID: "e-1", Side: model.Enemy, Location: "sarnos", Strength: 30}
	s.Armies["e-2"] = &engine.Army{ID: "e-2", Side: model.Enemy, Location: "ilth", Strength: 25}
	h := &Heuristic{Map: m}

	o := h.Orders(s)
	if got := orderOf(o, "e-1"); got.Type != model.MoveToward || got.Target != "kethra" {
		t.Errorf("empty capital: e-1 should move in, got %v", got)
	}
	if got := orderOf(o, "e-2"); got.Type != model.MoveToward || got.Target != "marren" {
		t.Errorf("e-2 should march on the nearest supply centre it lacks, got %v", got)
	}
	// Once inside, the garrison stays, entrenched.
	s.Armies["e-1"].Location = "kethra"
	if got := orderOf(h.Orders(s), "e-1"); got.Type != model.Entrench {
		t.Errorf("garrison should entrench, got %v", got)
	}
	// Support: of two armies next to Marren, where a player army stands,
	// one attacks and the other supports it.
	s.Armies["p"] = &engine.Army{ID: "p", Side: model.Player, Location: "marren", Strength: 30}
	s.Armies["e-3"] = &engine.Army{ID: "e-3", Side: model.Enemy, Location: "sarnos", Strength: 20}
	o2, o3 := orderOf(h.Orders(s), "e-2"), orderOf(h.Orders(s), "e-3")
	if !(o2.Type == model.Support && o2.SupportArmyID == "e-3" && o3.Type == model.MoveToward && o3.Target == "marren") {
		t.Errorf("want one attack on Marren and one support: e-2 %v, e-3 %v", o2, o3)
	}
	// The orders must be valid for the engine.
	if _, err := e.Resolve(s, h.Orders(s)); err != nil {
		t.Fatal(err)
	}
}
