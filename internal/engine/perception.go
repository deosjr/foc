package engine

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/deosjr/foc/internal/model"
)

// ObservedBattle is a battle as one general saw it.
type ObservedBattle struct {
	Place           string `json:"place"`
	Result          string `json:"result"` // "won" | "lost" | "stand-off"
	EnemyLosses     int    `json:"enemy_losses"`
	TrueEnemyLosses int    `json:"true_enemy_losses"`
}

// Sighting is enemy strength seen in a province.
type Sighting struct {
	Province     string `json:"province"`
	Strength     int    `json:"strength"`
	TrueStrength int    `json:"true_strength"`
	Exact        bool   `json:"exact"`
}

// Contact is a friendly army seen nearby.
type Contact struct {
	GeneralID string `json:"general"`
	Province  string `json:"province"`
}

// Observation is what one general actually saw this turn, before his
// personality distorts it. True values ride along for the review.
type Observation struct {
	GeneralID   string           `json:"general"`
	ArmyID      string           `json:"army"`
	Turn        int              `json:"turn"`
	Order       model.Order      `json:"order"`
	Start       string           `json:"start"`
	Location    string           `json:"location"`
	Strength    int              `json:"strength"`
	Losses      int              `json:"losses"`
	Disbanded   bool             `json:"disbanded"`
	Blocked     *Bounce          `json:"blocked,omitempty"`
	RetreatedTo string           `json:"retreated_to,omitempty"`
	Battles     []ObservedBattle `json:"battles"`
	Sightings   []Sighting       `json:"sightings"`
	Quiet       []string         `json:"quiet"` // adjacent provinces seen free of the enemy
	Friendly    []Contact        `json:"friendly"`
}

func roundTo(x float64, step int) int {
	if step <= 1 {
		return roundHalfUp(x)
	}
	return roundHalfUp(x/float64(step)) * step
}

// Perceive computes a general's observation after resolution. Noise draws
// come from rng in a fixed order: battles in result order, then provinces in
// id order.
func (e *Engine) Perceive(s *GameState, res *Result, generalID, armyID string, rng *rand.Rand) Observation {
	start := res.Start[armyID]
	obs := Observation{GeneralID: generalID, ArmyID: armyID, Turn: res.Turn, Start: start.Location}
	for _, o := range res.Orders {
		if o.ArmyID == armyID {
			obs.Order = o
		}
	}
	if b, ok := res.BounceOf(armyID); ok {
		obs.Blocked = &b
	}
	if d, ok := res.DislodgeOf(armyID); ok {
		obs.RetreatedTo = d.To
	}
	a := s.Armies[armyID]
	if a == nil {
		obs.Disbanded = true
		obs.Location = start.Location
		obs.Losses = start.Strength
	} else {
		obs.Location = a.Location
		obs.Strength = a.Strength
		obs.Losses = start.Strength - a.Strength
	}

	noise := func(x float64, n float64) float64 { return x * (1 - n + 2*n*rng.Float64()) }
	for _, b := range res.Battles {
		side, ok := b.Participated(armyID)
		if !ok {
			continue
		}
		ob := ObservedBattle{Place: b.Province}
		switch b.Winner {
		case "":
			ob.Result = "stand-off"
		case side:
			ob.Result = "won"
		default:
			ob.Result = "lost"
		}
		for _, id := range b.Armies[side.Opponent()] {
			ob.TrueEnemyLosses += b.Losses[id]
		}
		ob.EnemyLosses = roundHalfUp(noise(float64(ob.TrueEnemyLosses), e.Rules.Perception.EnemyLossesNoise))
		obs.Battles = append(obs.Battles, ob)
	}
	if obs.Disbanded {
		return obs
	}

	// Own province and its neighbours, in id order.
	seen := append([]string{obs.Location}, e.Map.Neighbours(obs.Location)...)
	sort.Strings(seen)
	step := e.Rules.Perception.StrengthRounding
	for _, p := range seen {
		enemy := 0
		for _, other := range s.ArmiesIn(p) {
			switch {
			case other.ID == armyID:
			case other.Side == start.Side.Opponent():
				enemy += other.Strength
			case other.Side == start.Side && other.GeneralID != "":
				obs.Friendly = append(obs.Friendly, Contact{GeneralID: other.GeneralID, Province: p})
			}
		}
		if enemy == 0 {
			if p != obs.Location {
				obs.Quiet = append(obs.Quiet, p)
			}
			continue
		}
		seenStr := int(math.Max(float64(step), float64(roundTo(noise(float64(enemy), e.Rules.Perception.AdjacentStrengthNoise), step))))
		obs.Sightings = append(obs.Sightings, Sighting{Province: p, Strength: seenStr, TrueStrength: enemy})
	}
	return obs
}
