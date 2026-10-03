// Package enemy issues structured orders for enemy armies directly; the
// enemy never goes through the letter pipeline.
package enemy

import (
	"sort"

	"github.com/deosjr/foc/internal/engine"
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
