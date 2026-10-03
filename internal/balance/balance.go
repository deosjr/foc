// Package balance plays the scenario directly against the engine with
// simple player bots that give orders with perfect knowledge (no letters,
// no fog), to measure how quickly and how a game ends. It is a tuning tool:
// the bots are an upper bound on a player's grip, not a player model.
package balance

import (
	"fmt"
	"io"
	"sort"

	"github.com/deosjr/foc/internal/enemy"
	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// Bot gives the player's orders for a turn.
type Bot struct {
	Name   string
	Orders func(e *engine.Engine, s *engine.GameState) []model.Order
}

// Result is how one simulated game ended.
type Result struct {
	Bot     string
	Turns   int
	Winner  model.Side
	Outcome string
	Player  int // player strength at the end
	Enemy   int
}

// Play runs one game to its end.
func Play(e *engine.Engine, scn *mapdata.Scenario, ai enemy.AI, bot Bot) (Result, error) {
	s := e.NewState(scn)
	for !s.Over {
		orders := append(bot.Orders(e, s), ai.Orders(s)...)
		if _, err := e.Resolve(s, orders); err != nil {
			return Result{}, fmt.Errorf("%s, turn %d: %w", bot.Name, s.Turn, err)
		}
		e.Evaluate(s, scn.TurnCap)
		if !s.Over {
			s.Turn++
		}
	}
	r := Result{Bot: bot.Name, Turns: s.Turn, Winner: s.Winner, Outcome: s.Outcome}
	for _, a := range s.Armies {
		if a.Side == model.Player {
			r.Player += a.Strength
		} else {
			r.Enemy += a.Strength
		}
	}
	return r, nil
}

func playerArmies(s *engine.GameState) []*engine.Army {
	var out []*engine.Army
	for _, id := range s.ArmyIDs() {
		if a := s.Armies[id]; a.Side == model.Player {
			out = append(out, a)
		}
	}
	return out
}

// nearestTarget is the nearest supply centre the player does not own.
func nearestTarget(e *engine.Engine, s *engine.GameState, from string) string {
	best, bestD := "", -1
	for _, p := range e.Map.IDs() {
		if prov := s.Provinces[p]; prov.Supply && prov.Owner != model.Player {
			if d := e.Map.Distance(from, p); bestD < 0 || d < bestD {
				best, bestD = p, d
			}
		}
	}
	return best
}

// Bots are the player strategies the simulator tries.
var Bots = []Bot{
	{"rush", func(e *engine.Engine, s *engine.GameState) []model.Order {
		var out []model.Order
		for _, a := range playerArmies(s) {
			if t := nearestTarget(e, s, a.Location); t != "" {
				out = append(out, model.Order{ArmyID: a.ID, Type: model.MoveToward, Target: t})
			}
		}
		return out
	}},
	{"steady", steady},
	{"turtle", func(e *engine.Engine, s *engine.GameState) []model.Order {
		var out []model.Order
		for _, a := range playerArmies(s) {
			out = append(out, model.Order{ArmyID: a.ID, Type: model.Entrench})
		}
		return out
	}},
}

// Report plays every bot and writes a table.
func Report(w io.Writer, e *engine.Engine, scn *mapdata.Scenario, newAI func() enemy.AI) error {
	fmt.Fprintf(w, "%-7s %5s  %-7s %6s %6s  %s\n", "bot", "turns", "winner", "player", "enemy", "outcome")
	for _, b := range Bots {
		r, err := Play(e, scn, newAI(), b)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-7s %5d  %-7s %6d %6d  %s\n", r.Bot, r.Turns, r.Winner, r.Player*e.Rules.MenPerStrength, r.Enemy*e.Rules.MenPerStrength, r.Outcome)
	}
	return nil
}

// steady is a stand-in for a sensible player: one army garrisons Karsa; the
// others move together toward the nearest supply centre we lack, attack
// only when the attack (with supports from those next to the fight) beats
// the defence, and otherwise dig in where they stand and wait for levies.
func steady(e *engine.Engine, s *engine.GameState) []model.Order {
	armies := playerArmies(s)
	if len(armies) == 0 {
		return nil
	}
	capital := e.Map.Capital(model.Player)
	sort.Slice(armies, func(i, j int) bool {
		di, dj := e.Map.Distance(armies[i].Location, capital), e.Map.Distance(armies[j].Location, capital)
		if di != dj {
			return di < dj
		}
		return armies[i].ID < armies[j].ID
	})
	var out []model.Order
	g := armies[0]
	if g.Location == capital {
		out = append(out, model.Order{ArmyID: g.ID, Type: model.Entrench})
	} else {
		out = append(out, model.Order{ArmyID: g.ID, Type: model.MoveToward, Target: capital})
	}
	field := armies[1:]
	if len(field) == 0 {
		return out
	}
	sort.Slice(field, func(i, j int) bool { return field[i].Strength > field[j].Strength })
	lead := field[0]
	target := nearestTarget(e, s, lead.Location)
	if target == "" {
		target = lead.Location
	}
	into := e.Map.NextStep(lead.Location, target)
	defence := 0.0
	for _, a := range s.ArmiesIn(into) {
		if a.Side == model.Enemy {
			mult := e.Map.Defence(into)
			if a.Entrenched >= 1 {
				mult += e.Rules.EntrenchBonus
			}
			defence += float64(a.Strength) * mult
		}
	}
	attack := float64(lead.Strength)
	var helpers []*engine.Army
	for _, a := range field[1:] {
		if e.Map.Adjacent(a.Location, into) {
			attack += e.Rules.SupportFraction * float64(a.Strength)
			helpers = append(helpers, a)
		}
	}
	if defence == 0 || attack > defence*1.1 {
		out = append(out, model.Order{ArmyID: lead.ID, Type: model.MoveToward, Target: target})
		for _, a := range helpers {
			out = append(out, model.Order{ArmyID: a.ID, Type: model.Support, SupportArmyID: lead.ID})
		}
		for _, a := range field[1:] {
			if !e.Map.Adjacent(a.Location, into) {
				out = append(out, model.Order{ArmyID: a.ID, Type: model.MoveToward, Target: lead.Location})
			}
		}
		return out
	}
	// Not strong enough: gather next to the lead and dig in.
	out = append(out, model.Order{ArmyID: lead.ID, Type: model.Entrench})
	for _, a := range field[1:] {
		if a.Location == lead.Location || e.Map.Adjacent(a.Location, into) {
			out = append(out, model.Order{ArmyID: a.ID, Type: model.Entrench})
		} else {
			out = append(out, model.Order{ArmyID: a.ID, Type: model.MoveToward, Target: lead.Location})
		}
	}
	return out
}
