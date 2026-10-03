// Package engine owns the ground truth: game state, deterministic
// resolution, victory and what each general perceives.
package engine

import (
	"fmt"
	"sort"

	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// Province is the mutable state of one province.
type Province struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Terrain  string     `json:"terrain"`
	Supply   bool       `json:"supply"`
	Capital  bool       `json:"capital"`
	Owner    model.Side `json:"owner"`
	Adjacent []string   `json:"adjacent"`
	Pos      [2]float64 `json:"pos"`
}

// Army is one army on the map.
type Army struct {
	ID         string     `json:"id"`
	Side       model.Side `json:"side"`
	Location   string     `json:"location"`
	Strength   int        `json:"strength"` // hundreds of men
	GeneralID  string     `json:"general,omitempty"`
	Commander  string     `json:"commander,omitempty"`
	Entrenched int        `json:"entrenched"`
}

// GameState is the truth. It is never shown to the player outside debug and review.
type GameState struct {
	Turn      int                    `json:"turn"`
	Provinces map[string]*Province   `json:"provinces"`
	Armies    map[string]*Army       `json:"armies"`
	Standing  map[string]model.Order `json:"standing"` // by general id
	Over      bool                   `json:"over"`
	Winner    model.Side             `json:"winner"`
	Outcome   string                 `json:"outcome"`
}

// Engine bundles the static map and ruleset that resolution needs.
type Engine struct {
	Map   *mapdata.Map
	Rules *mapdata.Ruleset
}

// NewState builds the starting state for a scenario. Turn starts at 1.
func (e *Engine) NewState(scn *mapdata.Scenario) *GameState {
	s := &GameState{
		Turn:      1,
		Provinces: map[string]*Province{},
		Armies:    map[string]*Army{},
		Standing:  map[string]model.Order{},
	}
	for _, p := range e.Map.Provinces {
		s.Provinces[p.ID] = &Province{
			ID: p.ID, Name: p.Name, Terrain: p.Terrain, Supply: p.Supply,
			Capital: p.Capital, Owner: p.Owner, Pos: p.Pos,
			Adjacent: append([]string(nil), e.Map.Neighbours(p.ID)...),
		}
	}
	for _, a := range scn.Armies {
		s.Armies[a.ID] = &Army{
			ID: a.ID, Side: a.Side, Location: a.Location, Strength: a.Strength,
			GeneralID: a.General, Commander: a.Commander,
		}
		if a.General != "" {
			s.Standing[a.General] = model.Order{ArmyID: a.ID, Type: model.Hold}
		}
	}
	return s
}

// Clone deep-copies the state.
func (s *GameState) Clone() *GameState {
	c := *s
	c.Provinces = make(map[string]*Province, len(s.Provinces))
	for k, v := range s.Provinces {
		p := *v
		p.Adjacent = append([]string(nil), v.Adjacent...)
		c.Provinces[k] = &p
	}
	c.Armies = make(map[string]*Army, len(s.Armies))
	for k, v := range s.Armies {
		a := *v
		c.Armies[k] = &a
	}
	c.Standing = make(map[string]model.Order, len(s.Standing))
	for k, v := range s.Standing {
		c.Standing[k] = v
	}
	return &c
}

// ArmyIDs returns all army ids, sorted.
func (s *GameState) ArmyIDs() []string {
	ids := make([]string, 0, len(s.Armies))
	for id := range s.Armies {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ArmyOf returns the army led by a general, or nil if it has disbanded.
func (s *GameState) ArmyOf(generalID string) *Army {
	for _, id := range s.ArmyIDs() {
		if a := s.Armies[id]; a.GeneralID == generalID {
			return a
		}
	}
	return nil
}

// ArmiesIn returns the armies in a province, sorted by id.
func (s *GameState) ArmiesIn(province string) []*Army {
	var out []*Army
	for _, id := range s.ArmyIDs() {
		if a := s.Armies[id]; a.Location == province {
			out = append(out, a)
		}
	}
	return out
}

// SupplyCount returns how many supply centres a side owns.
func (s *GameState) SupplyCount(side model.Side) (owned, total int) {
	for _, p := range s.Provinces {
		if p.Supply {
			total++
			if p.Owner == side {
				owned++
			}
		}
	}
	return owned, total
}

// Evaluate checks for victory or defeat after a turn has resolved, and for
// the turn cap. It sets Over, Winner and Outcome.
func (e *Engine) Evaluate(s *GameState, turnCap int) {
	playerCap := e.Map.Capital(model.Player)
	enemyCap := e.Map.Capital(model.Enemy)
	playerArmies := 0
	for _, a := range s.Armies {
		if a.Side == model.Player {
			playerArmies++
		}
	}
	owned, total := s.SupplyCount(model.Player)
	switch {
	case s.Provinces[playerCap].Owner != model.Player:
		s.Over, s.Winner = true, model.Enemy
		s.Outcome = fmt.Sprintf("%s has fallen to the enemy.", s.Provinces[playerCap].Name)
	case playerArmies == 0:
		s.Over, s.Winner = true, model.Enemy
		s.Outcome = "All your armies have been destroyed."
	case s.Provinces[enemyCap].Owner == model.Player:
		s.Over, s.Winner = true, model.Player
		s.Outcome = fmt.Sprintf("Your army has taken %s.", s.Provinces[enemyCap].Name)
	case owned >= e.Rules.Victory.SupplyCentresToWin:
		s.Over, s.Winner = true, model.Player
		s.Outcome = fmt.Sprintf("You hold %d of %d supply centres.", owned, total)
	case s.Turn >= turnCap:
		enemyOwned, _ := s.SupplyCount(model.Enemy)
		s.Over = true
		switch {
		case owned > enemyOwned:
			s.Winner = model.Player
		case enemyOwned > owned:
			s.Winner = model.Enemy
		default:
			s.Winner = model.Neutral
		}
		s.Outcome = fmt.Sprintf("The campaign ends after turn %d. You hold %d of %d supply centres; the enemy holds %d.",
			s.Turn, owned, total, enemyOwned)
	}
}
