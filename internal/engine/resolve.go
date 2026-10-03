package engine

import (
	"fmt"
	"math"
	"sort"

	"github.com/deosjr/foc/internal/model"
)

// Battle records one fight. A border battle (two armies moving into each
// other's provinces) has Border set to the second province.
type Battle struct {
	Province string                  `json:"province"`
	Border   string                  `json:"border,omitempty"`
	Armies   map[model.Side][]string `json:"armies"`
	Power    map[model.Side]float64  `json:"power"`
	Winner   model.Side              `json:"winner"` // "" for a stand-off
	Losses   map[string]int          `json:"losses"` // by army id
}

// Participated reports whether an army fought in this battle.
func (b Battle) Participated(armyID string) (model.Side, bool) {
	for side, ids := range b.Armies {
		for _, id := range ids {
			if id == armyID {
				return side, true
			}
		}
	}
	return "", false
}

// Move is a successful change of province.
type Move struct {
	Army string `json:"army"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Bounce is a move that failed: the army stayed home.
type Bounce struct {
	Army   string `json:"army"`
	Target string `json:"target"`
	Reason string `json:"reason"` // "lost" | "standoff" | "retreat-blocked"
}

// Dislodge is a defender driven out of its province.
type Dislodge struct {
	Army string `json:"army"`
	From string `json:"from"`
	To   string `json:"to"` // "" if it had nowhere to go and disbanded
}

// Capture is a change of province owner.
type Capture struct {
	Province string     `json:"province"`
	From     model.Side `json:"from"`
	To       model.Side `json:"to"`
}

// ArmySnap is an army's position and strength at the start of resolution.
type ArmySnap struct {
	Location string     `json:"location"`
	Strength int        `json:"strength"`
	Side     model.Side `json:"side"`
}

// Result is everything that happened during one resolution.
type Result struct {
	Turn      int                 `json:"turn"`
	Orders    []model.Order       `json:"orders"`
	Start     map[string]ArmySnap `json:"start"`
	Moves     []Move              `json:"moves"`
	Bounces   []Bounce            `json:"bounces"`
	Battles   []Battle            `json:"battles"`
	Dislodged []Dislodge          `json:"dislodged"`
	Disbanded []string            `json:"disbanded"`
	Captures  []Capture           `json:"captures"`
}

// BounceOf returns the bounce for an army, if its move failed.
func (r *Result) BounceOf(armyID string) (Bounce, bool) {
	for _, b := range r.Bounces {
		if b.Army == armyID {
			return b, true
		}
	}
	return Bounce{}, false
}

// DislodgeOf returns the dislodgement of an army, if any.
func (r *Result) DislodgeOf(armyID string) (Dislodge, bool) {
	for _, d := range r.Dislodged {
		if d.Army == armyID {
			return d, true
		}
	}
	return Dislodge{}, false
}

// WasDisbanded reports whether an army disbanded this turn.
func (r *Result) WasDisbanded(armyID string) bool {
	for _, id := range r.Disbanded {
		if id == armyID {
			return true
		}
	}
	return false
}

// sides is the fixed order in which per-side maps are walked, so that
// records come out the same on every run.
var sides = []model.Side{model.Enemy, model.Player}

func roundHalfUp(x float64) int { return int(math.Floor(x + 0.5)) }

// Resolve applies all orders simultaneously and deterministically, mutating
// the state. Armies without an order hold.
func (e *Engine) Resolve(s *GameState, orders []model.Order) (*Result, error) {
	res := &Result{Turn: s.Turn, Start: map[string]ArmySnap{}}
	byArmy := map[string]model.Order{}
	for _, o := range orders {
		a := s.Armies[o.ArmyID]
		if a == nil {
			return nil, fmt.Errorf("order for unknown army %q", o.ArmyID)
		}
		if _, dup := byArmy[o.ArmyID]; dup {
			return nil, fmt.Errorf("two orders for army %q", o.ArmyID)
		}
		if err := e.validate(a, o); err != nil {
			return nil, err
		}
		byArmy[o.ArmyID] = o
	}

	ids := s.ArmyIDs()
	loc := map[string]string{}
	dest := map[string]string{}
	for _, id := range ids {
		a := s.Armies[id]
		res.Start[id] = ArmySnap{Location: a.Location, Strength: a.Strength, Side: a.Side}
		o, ok := byArmy[id]
		if !ok {
			o = model.Order{ArmyID: id, Type: model.Hold}
		}
		res.Orders = append(res.Orders, o)
		loc[id] = a.Location
		dest[id] = a.Location
		switch o.Type {
		case model.MoveToward:
			dest[id] = e.Map.NextStep(a.Location, o.Target)
		case model.Retreat:
			// A retreat cannot attack: it fails if an enemy holds the target.
			if e.occupiedBy(s, o.Target, a.Side.Opponent()) {
				res.Bounces = append(res.Bounces, Bounce{Army: id, Target: o.Target, Reason: "retreat-blocked"})
			} else {
				dest[id] = o.Target
			}
		}
	}
	// Border battles: opposing armies moving into each other's provinces.
	type pair struct{ a, b string }
	borders := map[pair]bool{}
	for _, x := range ids {
		for _, y := range ids {
			ax, ay := s.Armies[x], s.Armies[y]
			if ax.Side != ay.Side.Opponent() || loc[x] == loc[y] {
				continue
			}
			if dest[x] == loc[y] && dest[y] == loc[x] {
				p := pair{loc[x], loc[y]}
				if p.b < p.a {
					p = pair{p.b, p.a}
				}
				borders[p] = true
			}
		}
	}
	var borderKeys []pair
	for p := range borders {
		borderKeys = append(borderKeys, p)
	}
	sort.Slice(borderKeys, func(i, j int) bool {
		if borderKeys[i].a != borderKeys[j].a {
			return borderKeys[i].a < borderKeys[j].a
		}
		return borderKeys[i].b < borderKeys[j].b
	})
	for _, p := range borderKeys {
		b := Battle{Province: p.a, Border: p.b, Armies: map[model.Side][]string{}, Power: map[model.Side]float64{}}
		for _, id := range ids {
			if (loc[id] == p.a && dest[id] == p.b) || (loc[id] == p.b && dest[id] == p.a) {
				side := s.Armies[id].Side
				b.Armies[side] = append(b.Armies[side], id)
				b.Power[side] += float64(s.Armies[id].Strength)
			}
		}
		b.Winner = winner(b.Power)
		for _, side := range sides {
			members := b.Armies[side]
			if side != b.Winner {
				for _, id := range members {
					// The loser stays home; on a stand-off both do.
					reason := "lost"
					if b.Winner == "" {
						reason = "standoff"
					}
					res.Bounces = append(res.Bounces, Bounce{Army: id, Target: dest[id], Reason: reason})
					dest[id] = loc[id]
				}
			}
		}
		e.applyCasualties(s, &b)
		res.Battles = append(res.Battles, b)
	}

	// Province contests, iterated to a fixed point: a bounced attacker becomes
	// a defender of its home province, which can change that contest.
	bounced := map[string][]string{} // province -> attackers that bounced from it
	provinces := e.Map.IDs()
	for changed := true; changed; {
		changed = false
		for _, p := range provinces {
			power, members := e.contest(s, p, ids, loc, dest, nil)
			if len(members) < 2 {
				continue
			}
			w := winner(power)
			for _, side := range sides {
				list := members[side]
				if side == w {
					continue
				}
				for _, id := range list {
					if loc[id] != p { // an attacker that lost or stood off
						reason := "lost"
						if w == "" {
							reason = "standoff"
						}
						res.Bounces = append(res.Bounces, Bounce{Army: id, Target: p, Reason: reason})
						bounced[p] = append(bounced[p], id)
						dest[id] = loc[id]
						changed = true
					}
				}
			}
		}
	}

	// Final battles in the settled configuration, including bounced attackers.
	// All winners are decided before any casualties are taken, so the order
	// in which battles are recorded cannot change their outcomes.
	var dislodged []string
	var final []Battle
	for _, p := range provinces {
		power, members := e.contest(s, p, ids, loc, dest, bounced[p])
		if len(members) < 2 {
			continue
		}
		b := Battle{Province: p, Armies: members, Power: power, Winner: winner(power)}
		if b.Winner != "" {
			for _, id := range members[b.Winner.Opponent()] {
				if loc[id] == p && dest[id] == p {
					dislodged = append(dislodged, id)
				}
			}
		}
		final = append(final, b)
	}
	for i := range final {
		e.applyCasualties(s, &final[i])
		res.Battles = append(res.Battles, final[i])
	}

	for _, id := range ids {
		if dest[id] != loc[id] {
			res.Moves = append(res.Moves, Move{Army: id, From: loc[id], To: dest[id]})
		}
	}
	// Dislodged armies retreat toward their own capital, or disband.
	sort.Strings(dislodged)
	gone := map[string]bool{}
	for _, id := range dislodged {
		a := s.Armies[id]
		to := ""
		if a.Strength >= e.Rules.DisbandBelow {
			to = e.retreatFor(s, a, loc[id], dest)
		}
		res.Dislodged = append(res.Dislodged, Dislodge{Army: id, From: loc[id], To: to})
		if to == "" {
			gone[id] = true
		} else {
			dest[id] = to
		}
	}
	for _, id := range ids {
		if s.Armies[id].Strength < e.Rules.DisbandBelow {
			gone[id] = true
		}
	}
	for _, id := range ids {
		if gone[id] {
			res.Disbanded = append(res.Disbanded, id)
			delete(s.Armies, id)
			continue
		}
		s.Armies[id].Location = dest[id]
	}

	// A province changes hands when one side alone ends the turn in it.
	for _, p := range provinces {
		sides := map[model.Side]bool{}
		for _, a := range s.ArmiesIn(p) {
			sides[a.Side] = true
		}
		if len(sides) != 1 {
			continue
		}
		for side := range sides {
			if prov := s.Provinces[p]; prov.Owner != side {
				res.Captures = append(res.Captures, Capture{Province: p, From: prov.Owner, To: side})
				prov.Owner = side
			}
		}
	}
	return res, nil
}

func (e *Engine) validate(a *Army, o model.Order) error {
	switch o.Type {
	case model.Hold:
		return nil
	case model.MoveToward:
		if e.Map.Province(o.Target) == nil {
			return fmt.Errorf("army %s: MoveToward unknown province %q", a.ID, o.Target)
		}
		return nil
	case model.Retreat:
		if !e.Map.Adjacent(a.Location, o.Target) {
			return fmt.Errorf("army %s: Retreat target %q is not adjacent to %s", a.ID, o.Target, a.Location)
		}
		return nil
	}
	return fmt.Errorf("army %s: order type %q is not supported yet", a.ID, o.Type)
}

func (e *Engine) occupiedBy(s *GameState, province string, side model.Side) bool {
	for _, a := range s.ArmiesIn(province) {
		if a.Side == side {
			return true
		}
	}
	return false
}

// contest gathers every army whose destination is p (plus extra bounced
// attackers) and computes each side's power: defenders get the terrain
// multiplier, attackers fight at face value.
func (e *Engine) contest(s *GameState, p string, ids []string, loc, dest map[string]string, extra []string) (map[model.Side]float64, map[model.Side][]string) {
	power := map[model.Side]float64{}
	members := map[model.Side][]string{}
	add := func(id string) {
		a := s.Armies[id]
		str := float64(a.Strength)
		if loc[id] == p && dest[id] == p {
			str *= e.Map.Defence(p)
		}
		power[a.Side] += str
		members[a.Side] = append(members[a.Side], id)
	}
	for _, id := range ids {
		if dest[id] == p {
			add(id)
		}
	}
	for _, id := range extra {
		if dest[id] != p {
			add(id)
		}
	}
	for side := range members {
		sort.Strings(members[side])
	}
	return power, members
}

// winner returns the side with strictly the highest power, or "" on a tie.
func winner(power map[model.Side]float64) model.Side {
	var best model.Side
	bestP, tie := -1.0, false
	for _, side := range []model.Side{model.Enemy, model.Neutral, model.Player} {
		p, ok := power[side]
		if !ok {
			continue
		}
		switch {
		case p > bestP+1e-9:
			best, bestP, tie = side, p, false
		case math.Abs(p-bestP) <= 1e-9:
			tie = true
		}
	}
	if tie {
		return ""
	}
	return best
}

func (e *Engine) applyCasualties(s *GameState, b *Battle) {
	b.Losses = map[string]int{}
	for _, side := range sides {
		list := b.Armies[side]
		rate := e.Rules.Casualties.Loser
		switch b.Winner {
		case "":
			rate = e.Rules.Casualties.Standoff
		case side:
			rate = e.Rules.Casualties.Winner
		}
		for _, id := range list {
			a := s.Armies[id]
			loss := roundHalfUp(float64(a.Strength) * rate)
			a.Strength -= loss
			b.Losses[id] = loss
		}
	}
}

// retreatFor picks the adjacent province with no opposing army that is
// closest to the army's own capital, ties broken by province id.
func (e *Engine) retreatFor(s *GameState, a *Army, from string, dest map[string]string) string {
	capital := e.Map.Capital(a.Side)
	best, bestD := "", math.MaxInt
	for _, n := range e.Map.Neighbours(from) { // sorted by id
		hostile := false
		for id, d := range dest {
			if d == n && s.Armies[id] != nil && s.Armies[id].Side == a.Side.Opponent() {
				hostile = true
				break
			}
		}
		if hostile {
			continue
		}
		if d := e.Map.Distance(n, capital); d < bestD {
			best, bestD = n, d
		}
	}
	return best
}
