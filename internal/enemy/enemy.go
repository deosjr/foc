// Package enemy issues structured orders for enemy armies directly; the
// enemy never goes through the letter pipeline.
package enemy

import (
	"sort"

	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// Scripted marches each enemy army along a fixed list of waypoints. A
// waypoint counts as reached once the army stands in it; until then the army
// keeps marching toward it, retrying after any bounce. After the last
// waypoint it holds.
type Scripted struct {
	Routes map[string][]string `json:"routes"`
	Next   map[string]int      `json:"next"` // army id -> index of the current waypoint
}

func NewScripted(routes map[string][]string) *Scripted {
	return &Scripted{Routes: routes, Next: map[string]int{}}
}

// Orders returns one order per living enemy army, sorted by army id.
func (e *Scripted) Orders(s *engine.GameState) []model.Order {
	var ids []string
	for id, a := range s.Armies {
		if a.Side == model.Enemy {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var out []model.Order
	for _, id := range ids {
		a := s.Armies[id]
		route := e.Routes[id]
		for e.Next[id] < len(route) && a.Location == route[e.Next[id]] {
			e.Next[id]++
		}
		if e.Next[id] >= len(route) {
			out = append(out, model.Order{ArmyID: id, Type: model.Hold})
			continue
		}
		out = append(out, model.Order{ArmyID: id, Type: model.MoveToward, Target: route[e.Next[id]]})
	}
	return out
}

// AI issues orders for every enemy army.
type AI interface {
	Orders(s *engine.GameState) []model.Order
}

// Heuristic is the deliberately dumb enemy from the spec:
//   - If no army holds the capital, the lowest-id army next to it moves in;
//     an army in the capital stays there, entrenched, as its garrison. (Read
//     literally, "move back when it is empty" makes two armies take turns
//     leaving and re-entering it.)
//   - Every other army marches on the nearest supply centre it does not
//     own, ties broken by province id.
//   - An army supports another enemy army attacking a province next to it,
//     where a player army stands, instead of marching itself.
type Heuristic struct {
	Map *mapdata.Map
}

func (h *Heuristic) Orders(s *engine.GameState) []model.Order {
	var ids []string
	for id, a := range s.Armies {
		if a.Side == model.Enemy {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	capital := h.Map.Capital(model.Enemy)
	orders := map[string]model.Order{}

	garrisoned := false
	for _, id := range ids {
		if s.Armies[id].Location == capital && !garrisoned {
			orders[id] = model.Order{ArmyID: id, Type: model.Entrench}
			garrisoned = true
		}
	}
	if !garrisoned {
		for _, id := range ids {
			if h.Map.Adjacent(s.Armies[id].Location, capital) {
				orders[id] = model.Order{ArmyID: id, Type: model.MoveToward, Target: capital}
				break
			}
		}
	}
	for _, id := range ids {
		if _, done := orders[id]; done {
			continue
		}
		a := s.Armies[id]
		best, bestD := "", -1
		for _, p := range h.Map.IDs() {
			prov := s.Provinces[p]
			if !prov.Supply || prov.Owner == model.Enemy {
				continue
			}
			if d := h.Map.Distance(a.Location, p); bestD < 0 || d < bestD {
				best, bestD = p, d
			}
		}
		if best == "" {
			orders[id] = model.Order{ArmyID: id, Type: model.Hold}
			continue
		}
		orders[id] = model.Order{ArmyID: id, Type: model.MoveToward, Target: best}
	}
	// Supports: an army helps another's attack on a player army next door.
	for _, id := range ids {
		o := orders[id]
		if o.Type != model.MoveToward || o.Target == capital {
			continue
		}
		a := s.Armies[id]
		for _, other := range ids {
			oo := orders[other]
			if other == id || oo.Type != model.MoveToward {
				continue
			}
			into := h.Map.NextStep(s.Armies[other].Location, oo.Target)
			playerThere := false
			for _, x := range s.ArmiesIn(into) {
				playerThere = playerThere || x.Side == model.Player
			}
			if playerThere && h.Map.Adjacent(a.Location, into) {
				orders[id] = model.Order{ArmyID: id, Type: model.Support, SupportArmyID: other}
				break
			}
		}
	}
	out := make([]model.Order, 0, len(ids))
	for _, id := range ids {
		out = append(out, orders[id])
	}
	return out
}
