package web

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/report"
)

// MapView is a map laid out for the SVG template. All the logic lives here;
// the template only draws. The same view type renders the believed map, the
// courier-route overlay and (in the review) the true map.
type MapView struct {
	Provinces   []ProvinceMark
	Roads       []Road
	Tokens      []Token
	Routes      []RouteLine
	Highlight   string // letter id whose mentions are highlighted
	Interactive bool   // provinces load their detail box when clicked
	ShowRoutes  bool
	Truth       bool
}

type ProvinceMark struct {
	ID, Name  string
	X, Y      float64
	Owner     string // player | enemy | neutral
	Badge     string // "T5": the turn the knowledge dates from
	Supply    bool
	Capital   bool
	Enemy     string // enemy strength in men, "~" prefixed when an estimate
	Title     string
	Highlight bool
	Stale     bool
}

type Road struct{ X1, Y1, X2, Y2 float64 }

type Token struct {
	X, Y     float64
	Initials string
	Side     string // player | enemy
	Opacity  float64
	Title    string
}

// RouteLine is the road a courier takes from the capital to a general.
type RouteLine struct {
	Points string // SVG polyline points
	LabelX float64
	LabelY float64
	Label  string
}

// MapOptions tune a believed map.
type MapOptions struct {
	Marked      map[string]bool
	Highlight   string
	Interactive bool
	Routes      bool
}

// tokenSource is what the player believes about one general's position.
type tokenSource struct {
	Name      string
	Province  string
	AsOf      int
	Destroyed bool
	Delay     int
}

func roads(m *mapdata.Map) []Road {
	var out []Road
	for _, e := range m.Edges {
		a, b := m.Province(e[0]), m.Province(e[1])
		out = append(out, Road{a.Pos[0], a.Pos[1], b.Pos[0], b.Pos[1]})
	}
	return out
}

func men(n int) string { return fmt.Sprint(n * report.MenPerStrength) }

// BeliefMap draws what the player believes as of a turn.
func BeliefMap(m *mapdata.Map, b *report.Belief, turn int, gens []tokenSource, o MapOptions) MapView {
	mv := MapView{Roads: roads(m), Highlight: o.Highlight, Interactive: o.Interactive, ShowRoutes: o.Routes}
	for _, p := range m.Provinces {
		e := b.Provinces[p.ID]
		pm := ProvinceMark{
			ID: p.ID, Name: p.Name, X: p.Pos[0], Y: p.Pos[1],
			Owner: string(e.LastKnownOwner), Supply: p.Supply, Capital: p.Capital,
			Badge: fmt.Sprintf("T%d", e.AsOfTurn), Highlight: o.Marked[p.ID],
			Stale: turn-e.AsOfTurn > 3,
		}
		title := []string{fmt.Sprintf("%s (%s) — last known %s, as of turn %d", p.Name, p.Terrain, e.LastKnownOwner, e.AsOfTurn)}
		if e.EnemyStrength != nil {
			pm.Enemy = men(*e.EnemyStrength)
			if e.EnemyEstimate {
				pm.Enemy = "~" + pm.Enemy
			}
			title = append(title, "enemy reported: "+pm.Enemy+" men")
		}
		pm.Title = strings.Join(title, "\n")
		mv.Provinces = append(mv.Provinces, pm)
	}
	// General tokens at their last reported location, fading with age.
	byProvince := map[string][]tokenSource{}
	for _, g := range gens {
		if !g.Destroyed && g.Province != "" {
			byProvince[g.Province] = append(byProvince[g.Province], g)
		}
	}
	for pid, list := range byProvince {
		p := m.Province(pid)
		for i, g := range list {
			mv.Tokens = append(mv.Tokens, Token{
				X: spread(p.Pos[0], i, len(list)), Y: p.Pos[1], Initials: initials(g.Name), Side: "player",
				Opacity: math.Max(0.35, 1-0.15*float64(turn-g.AsOf-1)),
				Title:   fmt.Sprintf("%s, reported here as of turn %d", g.Name, g.AsOf),
			})
		}
	}
	sortTokens(mv.Tokens)
	if o.Routes {
		capital := m.Capital(model.Player)
		for _, g := range gens {
			if g.Destroyed || g.Province == "" || g.Province == capital {
				continue
			}
			path := m.Path(capital, g.Province)
			var pts []string
			for _, id := range path {
				pos := m.Province(id).Pos
				pts = append(pts, fmt.Sprintf("%g,%g", pos[0], pos[1]))
			}
			last, prev := m.Province(path[len(path)-1]).Pos, m.Province(path[len(path)-2]).Pos
			when := "arrives this turn"
			if g.Delay == 1 {
				when = "1 turn"
			} else if g.Delay > 1 {
				when = fmt.Sprintf("%d turns", g.Delay)
			}
			mv.Routes = append(mv.Routes, RouteLine{
				Points: strings.Join(pts, " "),
				LabelX: (last[0] + prev[0]) / 2, LabelY: (last[1]+prev[1])/2 - 8,
				Label: fmt.Sprintf("to %s: %s", initials(g.Name), when),
			})
		}
	}
	return mv
}

// TruthMap draws the true state, for the after-action review.
func TruthMap(m *mapdata.Map, s *engine.GameState, names map[string]string) MapView {
	mv := MapView{Roads: roads(m), Truth: true}
	for _, p := range m.Provinces {
		prov := s.Provinces[p.ID]
		pm := ProvinceMark{ID: p.ID, Name: p.Name, X: p.Pos[0], Y: p.Pos[1],
			Owner: string(prov.Owner), Supply: p.Supply, Capital: p.Capital}
		enemy := 0
		var lines []string
		for _, a := range s.ArmiesIn(p.ID) {
			who := a.Commander
			if a.GeneralID != "" {
				who = names[a.GeneralID]
			}
			lines = append(lines, fmt.Sprintf("%s: %s men", who, men(a.Strength)))
			if a.Side == model.Enemy {
				enemy += a.Strength
			}
		}
		if enemy > 0 {
			pm.Enemy = men(enemy)
		}
		pm.Title = strings.Join(append([]string{fmt.Sprintf("%s — truly %s", p.Name, prov.Owner)}, lines...), "\n")
		mv.Provinces = append(mv.Provinces, pm)
		armies := s.ArmiesIn(p.ID)
		for i, a := range armies {
			name, side := a.Commander, "enemy"
			if a.GeneralID != "" {
				name, side = names[a.GeneralID], "player"
			}
			mv.Tokens = append(mv.Tokens, Token{
				X: spread(p.Pos[0], i, len(armies)), Y: p.Pos[1], Initials: initials(name), Side: side, Opacity: 1,
				Title: fmt.Sprintf("%s: %s men", name, men(a.Strength)),
			})
		}
	}
	sortTokens(mv.Tokens)
	return mv
}

func spread(x float64, i, n int) float64 { return x - 13*float64(n-1) + 26*float64(i) }

func sortTokens(t []Token) {
	sort.Slice(t, func(i, j int) bool { return t[i].Title < t[j].Title })
}
