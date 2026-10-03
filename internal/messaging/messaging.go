// Package messaging handles couriers (routes and delays) and how standing
// orders carry over from turn to turn.
package messaging

import (
	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// Couriers computes routes and delays.
type Couriers struct {
	Map   *mapdata.Map
	Rules *mapdata.Ruleset
}

// Route is the shortest road from one province to another.
func (c Couriers) Route(from, to string) []string { return c.Map.Path(from, to) }

// Delay is the number of turns a courier needs for a route:
// floor(hops / provinces_per_turn).
func (c Couriers) Delay(route []string) int {
	hops := len(route) - 1
	if hops < 0 {
		hops = 0
	}
	return hops / c.Rules.Courier.ProvincesPerTurn
}

// Send stamps a letter with its route and arrival turn.
func (c Couriers) Send(l *model.Letter, from, to string) {
	l.Route = c.Route(from, to)
	l.ArriveTurn = l.SentTurn + c.Delay(l.Route)
}

// Intercept rolls for a courier on a route: for each province on it that
// holds or neighbours an enemy army (enemyNear), a chance of capture. Every
// qualifying province is rolled, in route order, so the stream stays aligned.
// It returns where the letter was taken, or "".
func (c Couriers) Intercept(route []string, enemyNear func(province string) bool, draw func() float64) string {
	taken := ""
	for _, p := range route {
		if enemyNear(p) && draw() < c.Rules.Courier.Interception && taken == "" {
			taken = p
		}
	}
	return taken
}

// NextStanding returns the standing order a general carries into the next
// turn. MoveToward persists until the target is reached or the army loses a
// battle on the way; Entrench persists; every other order reverts to Hold.
func NextStanding(o model.Order, res *engine.Result, s *engine.GameState) model.Order {
	hold := model.Order{ArmyID: o.ArmyID, Type: model.Hold}
	a := s.Armies[o.ArmyID]
	if a == nil {
		return hold
	}
	if o.Type == model.Entrench {
		// Digging in persists until other orders arrive, unless driven out.
		if _, ok := res.DislodgeOf(o.ArmyID); ok {
			return hold
		}
		return o
	}
	if o.Type != model.MoveToward {
		return hold
	}
	if a.Location == o.Target {
		return hold
	}
	if b, ok := res.BounceOf(o.ArmyID); ok && b.Reason == "lost" {
		return hold
	}
	if _, ok := res.DislodgeOf(o.ArmyID); ok {
		return hold
	}
	return o
}
