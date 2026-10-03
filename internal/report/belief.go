package report

import (
	"sort"

	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// Belief is the player's map: built ONLY from delivered letters (and the
// scenario's opening intelligence).
type Belief struct {
	Provinces map[string]*BeliefEntry   `json:"provinces"`
	Generals  map[string]*GeneralBelief `json:"generals"`
}

// BeliefEntry is what the player believes about one province.
type BeliefEntry struct {
	LastKnownOwner   model.Side `json:"owner"`
	EnemyStrength    *int       `json:"enemy_strength,omitempty"`
	EnemyEstimate    bool       `json:"enemy_estimate"`
	FriendlyGenerals []string   `json:"friendly_generals"` // general ids
	AsOfTurn         int        `json:"as_of"`
	Source           string     `json:"source"` // general id, or "intel"
}

// GeneralBelief is where the player last heard a general was.
type GeneralBelief struct {
	Province  string `json:"province"`
	AsOfTurn  int    `json:"as_of"`
	Source    string `json:"source"`
	Destroyed bool   `json:"destroyed"`
}

// NewBelief is the opening picture: the map's owners, the generals' starting
// positions and the scenario's intelligence, all as of turn 0.
func NewBelief(m *mapdata.Map, scn *mapdata.Scenario) *Belief {
	b := &Belief{Provinces: map[string]*BeliefEntry{}, Generals: map[string]*GeneralBelief{}}
	for _, p := range m.Provinces {
		b.Provinces[p.ID] = &BeliefEntry{LastKnownOwner: p.Owner, Source: "intel"}
	}
	for _, a := range scn.Armies {
		if a.General != "" {
			b.Generals[a.General] = &GeneralBelief{Province: a.Location, Source: "intel"}
		}
	}
	for _, in := range scn.Intel {
		s := in.EnemyStrength
		e := b.Provinces[in.Province]
		e.EnemyStrength, e.EnemyEstimate = &s, true
	}
	b.refresh()
	return b
}

// Apply updates the belief from one delivered report's facts. Newer news
// about a place replaces older news; stale news never overwrites fresh.
func (b *Belief) Apply(f ReportFacts, generalID string, m *mapdata.Map, generalIDs map[string]string) {
	w := f.WrittenTurn
	id := func(name string) string {
		for _, p := range m.Provinces {
			if p.Name == name {
				return p.ID
			}
		}
		return ""
	}
	entry := func(name string) *BeliefEntry {
		e := b.Provinces[id(name)]
		if e == nil || e.AsOfTurn > w {
			return nil
		}
		e.AsOfTurn, e.Source = w, generalID
		return e
	}
	setGeneral := func(gid, province string, destroyed bool) {
		g := b.Generals[gid]
		if g == nil {
			g = &GeneralBelief{}
			b.Generals[gid] = g
		}
		if g.AsOfTurn > w {
			return
		}
		g.Province, g.AsOfTurn, g.Source, g.Destroyed = province, w, generalID, destroyed
	}

	setGeneral(generalID, id(f.Location), f.ArmyDestroyed)
	if !f.ArmyDestroyed {
		if e := entry(f.Location); e != nil {
			e.LastKnownOwner, e.EnemyStrength, e.EnemyEstimate = model.Player, nil, false
		}
	}
	for _, bt := range f.Battles {
		if bt.Result == "lost" && bt.Place != f.Location {
			// The enemy held or took the place.
			if e := entry(bt.Place); e != nil {
				e.LastKnownOwner = model.Enemy
			}
		}
	}
	for _, s := range f.Sightings {
		if e := entry(s.Province); e != nil {
			n := s.EnemyStrength
			e.LastKnownOwner, e.EnemyStrength, e.EnemyEstimate = model.Enemy, &n, s.Certainty != "certain"
		}
	}
	for _, q := range f.NoEnemySeenIn {
		if e := entry(q); e != nil {
			e.EnemyStrength, e.EnemyEstimate = nil, false
		}
	}
	for _, c := range f.FriendlyContacts {
		if gid := generalIDs[c.General]; gid != "" {
			setGeneral(gid, id(c.Province), false)
		}
	}
	b.refresh()
}

// refresh recomputes which generals are believed to be in each province.
func (b *Belief) refresh() {
	for _, e := range b.Provinces {
		e.FriendlyGenerals = nil
	}
	ids := make([]string, 0, len(b.Generals))
	for id := range b.Generals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		g := b.Generals[id]
		if e := b.Provinces[g.Province]; e != nil && !g.Destroyed {
			e.FriendlyGenerals = append(e.FriendlyGenerals, id)
		}
	}
}
