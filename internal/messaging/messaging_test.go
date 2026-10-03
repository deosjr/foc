package messaging

import (
	"testing"

	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

func TestDelay(t *testing.T) {
	m, _ := mapdata.LoadMap("../../maps/valley.json")
	r, _ := mapdata.LoadRuleset("../../rulesets/ancient.yaml")
	c := Couriers{Map: m, Rules: r}
	for _, tc := range []struct {
		to   string
		want int
	}{{"karsa", 0}, {"velia", 0}, {"hollow", 1}, {"marren", 1}, {"ilth", 1}, {"kethra", 2}} {
		l := model.Letter{SentTurn: 3}
		c.Send(&l, "karsa", tc.to)
		if l.ArriveTurn != 3+tc.want {
			t.Errorf("karsa -> %s: arrives %d, want %d (route %v)", tc.to, l.ArriveTurn, 3+tc.want, l.Route)
		}
	}
}

func TestNextStanding(t *testing.T) {
	m, _ := mapdata.LoadMap("../../maps/valley.json")
	r, _ := mapdata.LoadRuleset("../../rulesets/ancient.yaml")
	e := &engine.Engine{Map: m, Rules: r}
	s := e.NewState(&mapdata.Scenario{TurnCap: 10})
	s.Armies["a"] = &engine.Army{ID: "a", Side: model.Player, Location: "karsa", Strength: 30}
	march := model.Order{ArmyID: "a", Type: model.MoveToward, Target: "marren"}
	res, _ := e.Resolve(s, []model.Order{march})
	if got := NextStanding(march, res, s); got != march {
		t.Errorf("march should persist en route, got %v", got)
	}
	s.Armies["e"] = &engine.Army{ID: "e", Side: model.Enemy, Location: "hollow", Strength: 40}
	res, _ = e.Resolve(s, []model.Order{march})
	if got := NextStanding(march, res, s); got.Type != model.Hold {
		t.Errorf("march should stop after a lost battle, got %v", got)
	}
	ret := model.Order{ArmyID: "a", Type: model.Retreat, Target: "karsa"}
	if got := NextStanding(ret, res, s); got.Type != model.Hold {
		t.Errorf("retreat should revert to hold, got %v", got)
	}
}

func TestIntercept(t *testing.T) {
	m, _ := mapdata.LoadMap("../../maps/valley.json")
	r, _ := mapdata.LoadRuleset("../../rulesets/ancient.yaml")
	c := Couriers{Map: m, Rules: r}
	route := []string{"karsa", "velia", "hollow", "marren"}
	near := func(p string) bool { return p == "hollow" || p == "marren" }
	draws := 0
	count := func(v float64) func() float64 { return func() float64 { draws++; return v } }
	if at := c.Intercept(route, near, count(0)); at != "hollow" {
		t.Errorf("certain capture: taken at %q, want hollow", at)
	}
	if draws != 2 {
		t.Errorf("rolled %d times, want one per province near the enemy (2)", draws)
	}
	if at := c.Intercept(route, near, count(0.5)); at != "" {
		t.Errorf("a 0.5 draw against a 10%% chance captured the letter at %q", at)
	}
	if at := c.Intercept(route, func(string) bool { return false }, count(0)); at != "" {
		t.Error("a route far from the enemy was intercepted")
	}
}
