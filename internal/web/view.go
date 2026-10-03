package web

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/deosjr/foc/internal/game"
	"github.com/deosjr/foc/internal/generals"
	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/report"
)

// PlayerView is the only game data the browser ever receives outside debug
// mode. It is built from the belief, the player's own letters and delivered
// letters, never from the true state.
type PlayerView struct {
	Turn     int
	Season   string
	Belief   report.Belief
	Generals []GeneralView
	Drafts   map[string]string // general id -> draft text
	Sent     []game.SentLetter
	Inbox    []game.InboxLetter
	Over     bool
	Outcome  string
	Map      MapView
	Debug    map[string]*DebugView // letter id -> truth behind it; nil unless --debug
}

// GeneralView is one composer's header.
type GeneralView struct {
	game.GeneralInfo
	ProvinceName string
	Draft        string
	Found        []string
	Unknown      []string
	Sent         []game.SentLetter
	Closed       bool // game over, or the general is known to be lost
	Debug        bool
}

// MapView is the believed map, laid out for the SVG template. All the logic
// lives here; the template only draws.
type MapView struct {
	Provinces []ProvinceMark
	Roads     []Road
	Tokens    []Token
	Highlight string // letter id whose mentions are highlighted
}

type ProvinceMark struct {
	ID, Name  string
	X, Y      float64
	Owner     string // player | enemy | neutral
	Badge     string // "T5": the turn the knowledge dates from
	Supply    bool
	Capital   bool
	Enemy     string // reported enemy strength, "~" prefixed when an estimate
	Title     string
	Highlight bool
	Stale     bool
}

type Road struct{ X1, Y1, X2, Y2 float64 }

type Token struct {
	X, Y     float64
	Initials string
	Opacity  float64
	Title    string
}

func initials(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		if w == "the" {
			continue
		}
		b.WriteString(w[:1])
		if b.Len() == 2 {
			break
		}
	}
	return b.String()
}

// BuildView assembles the PlayerView. Call with the game lock held.
func BuildView(g *game.Game, debug bool, highlight string) PlayerView {
	v := PlayerView{
		Turn: g.Turn(), Season: g.Season(), Belief: *g.Belief(),
		Drafts: map[string]string{}, Sent: g.Sent(), Inbox: g.Inbox(),
		Over: g.Over(), Outcome: g.Outcome(),
	}
	for _, info := range g.Generals() {
		gv := GeneralView{GeneralInfo: info, ProvinceName: g.Map.NameOf(info.Province), Draft: g.Draft(info.ID),
			Closed: g.Over() || info.Destroyed, Debug: debug}
		gv.Found, gv.Unknown = g.Recognised(gv.Draft)
		for _, s := range v.Sent {
			if s.To == info.ID {
				gv.Sent = append(gv.Sent, s)
			}
		}
		// Newest first.
		sort.SliceStable(gv.Sent, func(i, j int) bool { return gv.Sent[i].SentTurn > gv.Sent[j].SentTurn })
		if gv.Draft != "" {
			v.Drafts[info.ID] = gv.Draft
		}
		v.Generals = append(v.Generals, gv)
	}
	v.Map = buildMap(g.Map, v, highlight, g)
	if debug {
		v.Debug = map[string]*DebugView{}
		for _, l := range v.Inbox {
			if d := g.DebugFor(l.ID); d != nil {
				v.Debug[l.ID] = buildDebug(g, d)
			}
		}
	}
	return v
}

func buildMap(m *mapdata.Map, v PlayerView, highlight string, g *game.Game) MapView {
	mv := MapView{Highlight: highlight}
	marked := map[string]bool{}
	if highlight != "" {
		if l, ok := g.InboxLetter(highlight); ok {
			for _, id := range l.Mentions {
				marked[id] = true
			}
		}
	}
	for _, e := range m.Edges {
		a, b := m.Province(e[0]), m.Province(e[1])
		mv.Roads = append(mv.Roads, Road{a.Pos[0], a.Pos[1], b.Pos[0], b.Pos[1]})
	}
	for _, p := range m.Provinces {
		e := v.Belief.Provinces[p.ID]
		pm := ProvinceMark{
			ID: p.ID, Name: p.Name, X: p.Pos[0], Y: p.Pos[1],
			Owner: string(e.LastKnownOwner), Supply: p.Supply, Capital: p.Capital,
			Badge: fmt.Sprintf("T%d", e.AsOfTurn), Highlight: marked[p.ID],
			Stale: v.Turn-e.AsOfTurn > 3,
		}
		title := []string{fmt.Sprintf("%s (%s) — last known %s, as of turn %d", p.Name, p.Terrain, e.LastKnownOwner, e.AsOfTurn)}
		if e.EnemyStrength != nil {
			pm.Enemy = fmt.Sprint(*e.EnemyStrength * report.MenPerStrength)
			if e.EnemyEstimate {
				pm.Enemy = "~" + pm.Enemy
			}
			title = append(title, "enemy reported: "+pm.Enemy+" men")
		}
		pm.Title = strings.Join(title, "\n")
		mv.Provinces = append(mv.Provinces, pm)
	}
	// General tokens at their last reported location, fading with age.
	byProvince := map[string][]GeneralView{}
	for _, gv := range v.Generals {
		if gv.Destroyed || gv.Province == "" {
			continue
		}
		byProvince[gv.Province] = append(byProvince[gv.Province], gv)
	}
	for pid, gens := range byProvince {
		p := m.Province(pid)
		for i, gv := range gens {
			age := v.Turn - gv.AsOfTurn
			mv.Tokens = append(mv.Tokens, Token{
				X:        p.Pos[0] - 13*float64(len(gens)-1) + 26*float64(i),
				Y:        p.Pos[1],
				Initials: initials(gv.Name),
				Opacity:  math.Max(0.35, 1-0.15*float64(age-1)),
				Title:    fmt.Sprintf("%s, reported here as of turn %d", gv.Name, gv.AsOfTurn),
			})
		}
	}
	sort.Slice(mv.Tokens, func(i, j int) bool { return mv.Tokens[i].Title < mv.Tokens[j].Title })
	return mv
}

// DebugView is the truth behind one report, for the debug drawer.
type DebugView struct {
	Interpretations []InterpView
	Order           string
	OrderSource     string
	Rows            []DistortRow // true vs observed vs reported
	Attempts        int
	Violations      [][]string
	FellBack        bool
	Error           string
	FactsJSON       string
}

type InterpView struct {
	LetterID   string
	Letter     string
	SentTurn   int
	Action     []Prob
	Targets    []Prob
	Engagement float64
	Addressed  float64
	Plausible  float64
	Reweighted []Prob
	Draw       float64
	Step       string
	Outcome    string
	Order      string
	Error      string
}

type Prob struct {
	Key    string
	P      float64
	Weight float64
	Chosen bool
}

type DistortRow struct {
	What                     string
	True, Observed, Reported string
}

func probs(p map[string]float64, weights map[string]float64, name func(string) string, chosen string, limit int) []Prob {
	var out []Prob
	for k, v := range p {
		out = append(out, Prob{Key: name(k), P: v, Weight: weights[k], Chosen: k == chosen})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].P != out[j].P {
			return out[i].P > out[j].P
		}
		return out[i].Key < out[j].Key
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func describe(o *model.Order, m *mapdata.Map) string {
	if o == nil {
		return "(keeps standing order)"
	}
	return interpret.DescribeOrder(*o, m)
}

func buildDebug(g *game.Game, d *game.TurnDebug) *DebugView {
	m := g.Map
	name := func(k string) string {
		if p := m.Province(k); p != nil {
			return p.Name
		}
		return k
	}
	dv := &DebugView{
		Order: interpret.DescribeOrder(d.Order, m), OrderSource: d.OrderSource,
		Attempts: d.Written.Attempts, Violations: d.Written.Violations,
		FellBack: d.Written.FellBack, Error: d.Written.Error,
	}
	for _, in := range d.Interpretations {
		iv := InterpView{
			LetterID: in.LetterID, Engagement: in.Parsed.Engagement, Addressed: in.Parsed.Addressed,
			Plausible: in.Parsed.Plausible, Draw: in.RNGDraw, Step: in.Step, Outcome: in.Outcome,
			Order: describe(in.Order, m), Error: in.Error,
		}
		if l := g.DebugLetter(in.LetterID); l != nil {
			iv.Letter, iv.SentTurn = l.Body, l.SentTurn
		}
		chosen := ""
		if in.Outcome == generals.OutcomeOrder && in.Order != nil {
			switch in.Order.Type {
			case model.Hold:
				chosen = "hold"
			case model.MoveToward:
				chosen = "move"
			case model.Retreat:
				chosen = "retreat"
			}
		}
		iv.Action = probs(in.Parsed.Action, nil, name, chosen, 0)
		iv.Targets = probs(in.Parsed.Target, nil, name, "", 4)
		if in.Reweighted != nil {
			iv.Reweighted = probs(in.Reweighted, in.Weights, name, chosen, 0)
		}
		dv.Interpretations = append(dv.Interpretations, iv)
	}
	o, f := d.Observation, d.Facts
	hundreds := func(n int) string { return fmt.Sprint(n * report.MenPerStrength) }
	dv.Rows = append(dv.Rows, DistortRow{"own losses", hundreds(o.Losses), hundreds(o.Losses), hundreds(f.OwnLosses)})
	for i, b := range o.Battles {
		rep := ""
		if i < len(f.Battles) {
			rep = hundreds(f.Battles[i].EnemyLosses)
		}
		dv.Rows = append(dv.Rows, DistortRow{"enemy losses at " + m.NameOf(b.Place),
			hundreds(b.TrueEnemyLosses), hundreds(b.EnemyLosses), rep})
	}
	for i, s := range o.Sightings {
		rep := ""
		if i < len(f.Sightings) {
			rep = hundreds(f.Sightings[i].EnemyStrength)
		}
		dv.Rows = append(dv.Rows, DistortRow{"enemy in " + m.NameOf(s.Province),
			hundreds(s.TrueStrength), hundreds(s.Strength), rep})
	}
	dv.FactsJSON = factsJSON(f)
	return dv
}
