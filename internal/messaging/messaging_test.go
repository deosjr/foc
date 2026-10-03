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
