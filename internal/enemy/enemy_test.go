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
	newState := func() *engine.GameState { return e.NewState(&mapdata.Scenario{TurnCap: 20}) }
	h := &Heuristic{Map: m}

	t.Run("marches on the nearest supply centre it lacks", func(t *testing.T) {
		s := newState()
		s.Armies["e-1"] = &engine.Army{ID: "e-1", Side: model.Enemy, Location: "ilth", Strength: 25}
		if got := orderOf(h.Orders(s), "e-1"); got.Type != model.MoveToward || got.Target != "marren" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("garrisons the capital when a player army is near", func(t *testing.T) {
		s := newState()
		s.Armies["e-1"] = &engine.Army{ID: "e-1", Side: model.Enemy, Location: "kethra", Strength: 25}
		s.Armies["p"] = &engine.Army{ID: "p", Side: model.Player, Location: "marren", Strength: 30}
		if got := orderOf(h.Orders(s), "e-1"); got.Type != model.Entrench {
			t.Errorf("got %v", got)
		}
	})
	t.Run("attacks a beatable army with help", func(t *testing.T) {
		s := newState()
		s.Armies["e-0"] = &engine.Army{ID: "e-0", Side: model.Enemy, Location: "kethra", Strength: 10} // garrison
		s.Armies["e-1"] = &engine.Army{ID: "e-1", Side: model.Enemy, Location: "sarnos", Strength: 25}
		s.Armies["e-2"] = &engine.Army{ID: "e-2", Side: model.Enemy, Location: "ilth", Strength: 20}
		s.Armies["p"] = &engine.Army{ID: "p", Side: model.Player, Location: "marren", Strength: 30}
		o := h.Orders(s)
		// 25 + 10 = 35 > 30 on open ground.
		if a, b := orderOf(o, "e-1"), orderOf(o, "e-2"); a.Type != model.MoveToward || a.Target != "marren" || b.Type != model.Support || b.SupportArmyID != "e-1" {
			t.Errorf("want e-1 attacking Marren with e-2 in support: %v / %v", a, b)
		}
	})
	t.Run("does not throw itself at a stronger army", func(t *testing.T) {
		s := newState()
		s.Armies["e-1"] = &engine.Army{ID: "e-1", Side: model.Enemy, Location: "ilth", Strength: 20}
		s.Armies["p"] = &engine.Army{ID: "p", Side: model.Player, Location: "marren", Strength: 30}
		if got := orderOf(h.Orders(s), "e-1"); got.Type == model.MoveToward && got.Target == "marren" {
			t.Errorf("attacked a stronger army: %v", got)
		}
	})
	t.Run("digs in on its own supply centre when outnumbered", func(t *testing.T) {
		s := newState()
		s.Armies["e-0"] = &engine.Army{ID: "e-0", Side: model.Enemy, Location: "kethra", Strength: 10} // garrison
		s.Armies["e-1"] = &engine.Army{ID: "e-1", Side: model.Enemy, Location: "sarnos", Strength: 25}
		s.Armies["p"] = &engine.Army{ID: "p", Side: model.Player, Location: "marren", Strength: 30}
		if got := orderOf(h.Orders(s), "e-1"); got.Type != model.Entrench {
			t.Errorf("got %v", got)
		}
	})
	t.Run("orders are valid for the engine", func(t *testing.T) {
		s := newState()
		s.Armies["e-1"] = &engine.Army{ID: "e-1", Side: model.Enemy, Location: "sarnos", Strength: 30}
		s.Armies["e-2"] = &engine.Army{ID: "e-2", Side: model.Enemy, Location: "kethra", Strength: 25}
		s.Armies["p"] = &engine.Army{ID: "p", Side: model.Player, Location: "hollow", Strength: 30}
		if _, err := e.Resolve(s, h.Orders(s)); err != nil {
			t.Fatal(err)
		}
	})
}
