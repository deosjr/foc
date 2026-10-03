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
	Reason string `json:"reason"` // "lost" | "standoff" | "field" (won a field battle) | "retreat-blocked"
}

// SupportResult is what became of one Support order.
type SupportResult struct {
	Supporter string `json:"supporter"`
	Supported string `json:"supported"`
	Into      string `json:"into"`   // where the supported army meant to fight or hold
	Status    string `json:"status"` // "given" | "cut" | "too-far" | "same-province"
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
	Supports  []SupportResult     `json:"supports"`
}

// SupportOf returns what became of an army's Support order, if it gave one.
func (r *Result) SupportOf(armyID string) (SupportResult, bool) {
	for _, s := range r.Supports {
		if s.Supporter == armyID {
			return s, true
		}
	}
	return SupportResult{}, false
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

// resolver holds the working state of one resolution.
type resolver struct {
	e        *Engine
	s        *GameState
	ids      []string
	loc      map[string]string
	dest     map[string]string
	intended map[string]string    // destination before any bounce
	dugIn    map[string]bool      // entrenched for a second turn or more
	support  map[string][]support // supported army -> supports given to it
}

type support struct {
	from string // supporting army
	into string // province the support counts in
}

// Resolve applies all orders simultaneously and deterministically, mutating
// the state. Armies without an order hold.
//
//  1. Each army's intended destination; non-moving orders stay.
//  2. Entrenchment counts up; it gives its bonus from the second turn.
//  3. Supports are checked: the supporter must be next to where the
//     supported army fights, and is cut if an enemy moves on it (except from
//     the province the support is aimed into).
//  4. Field battles: opposing armies moving into each other's provinces meet
//     on the road between. That is their only battle this turn: the loser
//     falls back, the winner holds the road and does not advance.
//  5. Province contests, iterated to a fixed point: a defender has strength
//     × (terrain + entrenchment) + supports, an attacker strength + supports.
//     Highest power takes the province; a tie is a stand-off.
//  6. Dislodged defenders retreat toward their capital, never into a
//     province an attacker came from or one held by the enemy; else disband.
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
		if err := e.validate(s, a, o); err != nil {
			return nil, err
		}
		byArmy[o.ArmyID] = o
	}

	r := &resolver{e: e, s: s, ids: s.ArmyIDs(), loc: map[string]string{}, dest: map[string]string{},
		intended: map[string]string{}, dugIn: map[string]bool{}, support: map[string][]support{}}
	ids, loc, dest := r.ids, r.loc, r.dest

	// 1–2. Intentions and entrenchment.
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
		if o.Type == model.Entrench {
			a.Entrenched++
		} else {
			a.Entrenched = 0
		}
		r.dugIn[id] = a.Entrenched >= 2
		r.intended[id] = dest[id]
	}

	// 3. Supports.
	for _, id := range ids {
		o := byArmy[id]
		if o.Type != model.Support {
			continue
		}
		sr := SupportResult{Supporter: id, Supported: o.SupportArmyID, Into: r.intended[o.SupportArmyID], Status: "given"}
		switch {
		case loc[id] == sr.Into:
			sr.Status = "same-province" // armies in one province already fight together
		case !e.Map.Adjacent(loc[id], sr.Into):
			sr.Status = "too-far"
		default:
			for _, x := range ids {
				if s.Armies[x].Side == s.Armies[id].Side.Opponent() && r.intended[x] == loc[id] && loc[x] != sr.Into {
					sr.Status = "cut"
					break
				}
			}
		}
		if sr.Status == "given" {
			r.support[sr.Supported] = append(r.support[sr.Supported], support{from: id, into: sr.Into})
		}
		res.Supports = append(res.Supports, sr)
	}

	// 4. Field battles.
	type pair struct{ a, b string }
	roads := map[pair]bool{}
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
				roads[p] = true
			}
		}
	}
	var roadKeys []pair
	for p := range roads {
		roadKeys = append(roadKeys, p)
	}
	sort.Slice(roadKeys, func(i, j int) bool {
		if roadKeys[i].a != roadKeys[j].a {
			return roadKeys[i].a < roadKeys[j].a
		}
		return roadKeys[i].b < roadKeys[j].b
	})
	for _, p := range roadKeys {
		b := Battle{Province: p.a, Border: p.b, Armies: map[model.Side][]string{}, Power: map[model.Side]float64{}}
		var fighters []string
		for _, id := range ids {
			if (loc[id] == p.a && dest[id] == p.b) || (loc[id] == p.b && dest[id] == p.a) {
				side := s.Armies[id].Side
				b.Armies[side] = append(b.Armies[side], id)
				b.Power[side] += float64(s.Armies[id].Strength) + r.supportFor(id, dest[id])
				fighters = append(fighters, id)
			}
		}
		b.Winner = winner(b.Power)
		for _, id := range fighters {
			reason := "lost"
			switch b.Winner {
			case "":
				reason = "standoff"
			case s.Armies[id].Side:
				reason = "field"
			}
			res.Bounces = append(res.Bounces, Bounce{Army: id, Target: dest[id], Reason: reason})
			dest[id] = loc[id]
		}
		e.applyCasualties(s, &b)
		res.Battles = append(res.Battles, b)
	}

	// 5. Province contests, iterated to a fixed point: a bounced attacker
	// becomes a defender of its home province, which can change that contest.
	bounced := map[string][]string{} // province -> attackers that bounced from it
	provinces := e.Map.IDs()
	for changed := true; changed; {
		changed = false
		for _, p := range provinces {
			power, members := r.contest(p, nil)
			if len(members) < 2 {
				continue
			}
			w := winner(power)
			for _, side := range sides {
				if side == w {
					continue
				}
				for _, id := range members[side] {
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
	attackersFrom := map[string][]string{} // dislodged army -> provinces its attackers came from
	var final []Battle
	for _, p := range provinces {
		power, members := r.contest(p, bounced[p])
		if len(members) < 2 {
			continue
		}
		b := Battle{Province: p, Armies: members, Power: power, Winner: winner(power)}
		if b.Winner != "" {
			for _, id := range members[b.Winner.Opponent()] {
				if loc[id] == p && dest[id] == p {
					dislodged = append(dislodged, id)
					for _, w := range members[b.Winner] {
						if loc[w] != p {
							attackersFrom[id] = append(attackersFrom[id], loc[w])
						}
					}
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
	// 6. Dislodged armies retreat toward their own capital, or disband.
	sort.Strings(dislodged)
	gone := map[string]bool{}
	for _, id := range dislodged {
		a := s.Armies[id]
		to := ""
		if a.Strength >= e.Rules.DisbandBelow {
			to = e.retreatFor(s, a, loc[id], dest, attackersFrom[id])
		}
		res.Dislodged = append(res.Dislodged, Dislodge{Army: id, From: loc[id], To: to})
		a.Entrenched = 0
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

func (e *Engine) validate(s *GameState, a *Army, o model.Order) error {
	switch o.Type {
	case model.Hold, model.Entrench:
		return nil
	case model.MoveToward:
		if e.Map.Province(o.Target) == nil {
			return fmt.Errorf("army %s: MoveToward unknown province %q", a.ID, o.Target)
		}
		return nil
	case model.Retreat, model.Scout:
		if !e.Map.Adjacent(a.Location, o.Target) {
			return fmt.Errorf("army %s: %s target %q is not adjacent to %s", a.ID, o.Type, o.Target, a.Location)
		}
		return nil
	case model.Support:
		other := s.Armies[o.SupportArmyID]
		if other == nil || other.Side != a.Side || other.ID == a.ID {
			return fmt.Errorf("army %s: cannot support %q", a.ID, o.SupportArmyID)
		}
		return nil
	}
	return fmt.Errorf("army %s: unknown order type %q", a.ID, o.Type)
}

func (e *Engine) occupiedBy(s *GameState, province string, side model.Side) bool {
	for _, a := range s.ArmiesIn(province) {
		if a.Side == side {
			return true
		}
	}
	return false
}

// supportFor sums the support given to an army fighting in a province.
func (r *resolver) supportFor(id, province string) float64 {
	total := 0.0
	for _, sp := range r.support[id] {
		if sp.into == province {
			if a := r.s.Armies[sp.from]; a != nil {
				total += r.e.Rules.SupportFraction * float64(a.Strength)
			}
		}
	}
	return total
}

// contest gathers every army whose destination is p (plus extra bounced
// attackers) and computes each side's power: defenders get the terrain
// multiplier and any entrenchment bonus, attackers fight at face value;
// both add their supports.
func (r *resolver) contest(p string, extra []string) (map[model.Side]float64, map[model.Side][]string) {
	power := map[model.Side]float64{}
	members := map[model.Side][]string{}
	add := func(id string) {
		a := r.s.Armies[id]
		str := float64(a.Strength)
		if r.loc[id] == p && r.dest[id] == p {
			mult := r.e.Map.Defence(p)
			if r.dugIn[id] {
				mult += r.e.Rules.EntrenchBonus
			}
			str *= mult
		}
		// Supports count where the fight is, also for a bounced attacker
		// re-counted in the battle it lost.
		power[a.Side] += str + r.supportFor(id, p)
		members[a.Side] = append(members[a.Side], id)
	}
	for _, id := range r.ids {
		if r.dest[id] == p {
			add(id)
		}
	}
	for _, id := range extra {
		if r.dest[id] != p {
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
// closest to the army's own capital, ties broken by province id. It never
// retreats into a province its attackers came from.
func (e *Engine) retreatFor(s *GameState, a *Army, from string, dest map[string]string, attackersFrom []string) string {
	capital := e.Map.Capital(a.Side)
	best, bestD := "", math.MaxInt
	for _, n := range e.Map.Neighbours(from) { // sorted by id
		hostile := false
		for _, af := range attackersFrom {
			if af == n {
				hostile = true
			}
		}
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
