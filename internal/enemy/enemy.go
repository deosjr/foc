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

// Heuristic is a simple enemy that sees the true state. Each turn:
//   - Garrison: an army in the capital stays there, dug in, while any player
//     army is within two provinces of it; if the capital is empty and
//     threatened, the nearest army goes back. (Winter musters raise a new
//     army in an empty capital.)
//   - Every other army, in id order:
//     1. attacks a neighbouring player army it can beat, counting half the
//     strength of free fellow armies that are next to the fight (they
//     then support it);
//     2. if the player armies next to it could beat it where it stands,
//     digs in on its own supply centre and refuses battle, or falls back to
//     the nearest supply centre of ours;
//     3. takes an empty supply centre next to it, or guards one of its own
//     that a player army is next to;
//     4. otherwise marches on the nearest supply centre it does not hold,
//     unless the next step holds a player army it cannot beat, in which
//     case it digs in.
type Heuristic struct {
	Map *mapdata.Map
	// EntrenchBonus is the ruleset's bonus for a dug-in defender, used when
	// judging fights. Zero means 0.25.
	EntrenchBonus float64
}

func (h *Heuristic) bonus() float64 {
	if h.EntrenchBonus == 0 {
		return 0.25
	}
	return h.EntrenchBonus
}

// defence is what a side's armies in p would put up against an attack.
func (h *Heuristic) defence(s *engine.GameState, p string, side model.Side) float64 {
	total := 0.0
	for _, a := range s.ArmiesIn(p) {
		if a.Side != side {
			continue
		}
		mult := h.Map.Defence(p)
		if a.Entrenched >= 1 { // will be dug in by the time we arrive
			mult += h.bonus()
		}
		total += float64(a.Strength) * mult
	}
	return total
}

func (h *Heuristic) Orders(s *engine.GameState) []model.Order {
	var ids []string
	for id, a := range s.Armies {
		if a.Side == model.Enemy {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	m := h.Map
	capital := m.Capital(model.Enemy)
	orders := map[string]model.Order{}
	nearCapital := false
	for _, a := range s.Armies {
		if a.Side == model.Player && m.Distance(a.Location, capital) <= 2 {
			nearCapital = true
		}
	}

	// Garrison.
	garrisoned := false
	for _, id := range ids {
		if s.Armies[id].Location == capital && nearCapital && !garrisoned {
			orders[id] = model.Order{ArmyID: id, Type: model.Entrench}
			garrisoned = true
		}
	}
	if nearCapital && !garrisoned && len(s.ArmiesIn(capital)) == 0 {
		best, bestD := "", -1
		for _, id := range ids {
			if d := m.Distance(s.Armies[id].Location, capital); bestD < 0 || d < bestD {
				best, bestD = id, d
			}
		}
		if best != "" {
			orders[best] = model.Order{ArmyID: best, Type: model.MoveToward, Target: capital}
		}
	}

	free := func(id string) bool { _, done := orders[id]; return !done }
	playerIn := func(p string) bool {
		for _, a := range s.ArmiesIn(p) {
			if a.Side == model.Player {
				return true
			}
		}
		return false
	}
	for _, id := range ids {
		if !free(id) {
			continue
		}
		a := s.Armies[id]
		here := a.Location

		// 1. Opportunity: a neighbouring player army we can beat.
		bestP, bestMargin := "", 0.0
		var bestHelpers []string
		for _, n := range m.Neighbours(here) {
			if !playerIn(n) {
				continue
			}
			power := float64(a.Strength)
			var helpers []string
			for _, other := range ids {
				o := s.Armies[other]
				if other != id && free(other) && m.Adjacent(o.Location, n) {
					power += 0.5 * float64(o.Strength)
					helpers = append(helpers, other)
				}
			}
			if margin := power - h.defence(s, n, model.Player); margin > bestMargin {
				bestP, bestMargin, bestHelpers = n, margin, helpers
			}
		}
		if bestP != "" {
			orders[id] = model.Order{ArmyID: id, Type: model.MoveToward, Target: bestP}
			for _, hlp := range bestHelpers {
				orders[hlp] = model.Order{ArmyID: hlp, Type: model.Support, SupportArmyID: id}
			}
			continue
		}

		// 2. Danger: could the player armies next to us beat us here?
		threat := 0.0
		for _, n := range m.Neighbours(here) {
			for _, x := range s.ArmiesIn(n) {
				if x.Side == model.Player {
					threat += float64(x.Strength)
				}
			}
		}
		mult := m.Defence(here)
		if a.Entrenched >= 1 {
			mult += h.bonus()
		}
		if threat > float64(a.Strength)*mult && here != capital {
			// On one of our supply centres: dig in and hold it, unless the
			// odds are hopeless even dug in.
			if s.Provinces[here].Supply && s.Provinces[here].Owner == model.Enemy &&
				threat <= float64(a.Strength)*(m.Defence(here)+h.bonus())*1.5 {
				orders[id] = model.Order{ArmyID: id, Type: model.Entrench, Stance: model.StanceRefuse}
				continue
			}
			// Otherwise fall back to the nearest supply centre of ours that
			// no player army stands in.
			refuge, refugeD := "", -1
			for _, p := range m.IDs() {
				prov := s.Provinces[p]
				if !prov.Supply || prov.Owner != model.Enemy || p == here || playerIn(p) {
					continue
				}
				if d := m.Distance(here, p); refugeD < 0 || d < refugeD {
					refuge, refugeD = p, d
				}
			}
			if refuge != "" && !playerIn(m.NextStep(here, refuge)) {
				orders[id] = model.Order{ArmyID: id, Type: model.MoveToward, Target: refuge}
				continue
			}
		}

		// 3. Take an empty supply centre next door, or guard a threatened one.
		took := false
		for _, n := range m.Neighbours(here) {
			prov := s.Provinces[n]
			if !prov.Supply || len(s.ArmiesIn(n)) > 0 {
				continue
			}
			threatened := false
			for _, nn := range m.Neighbours(n) {
				threatened = threatened || playerIn(nn)
			}
			if prov.Owner != model.Enemy || threatened {
				orders[id] = model.Order{ArmyID: id, Type: model.MoveToward, Target: n}
				took = true
				break
			}
		}
		if took {
			continue
		}

		// 4. March on the nearest supply centre we lack, unless blocked.
		best, bestD := "", -1
		for _, p := range m.IDs() {
			prov := s.Provinces[p]
			if !prov.Supply || prov.Owner == model.Enemy {
				continue
			}
			if d := m.Distance(here, p); bestD < 0 || d < bestD {
				best, bestD = p, d
			}
		}
		if best == "" {
			orders[id] = model.Order{ArmyID: id, Type: model.Entrench}
			continue
		}
		next := m.NextStep(here, best)
		if playerIn(next) && h.defence(s, next, model.Player) >= float64(a.Strength) {
			orders[id] = model.Order{ArmyID: id, Type: model.Entrench}
			continue
		}
		orders[id] = model.Order{ArmyID: id, Type: model.MoveToward, Target: best}
	}
	out := make([]model.Order, 0, len(ids))
	for _, id := range ids {
		out = append(out, orders[id])
	}
	return out
}
