package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/deosjr/foc/internal/game"
	"github.com/deosjr/foc/internal/model"
)

// ReviewView is one turn of the after-action review: what the player
// believed beside what was true, and how each general got from letter to
// report.
type ReviewView struct {
	Turn, Last    int
	Prev, Next    int
	HasPrev       bool
	HasNext       bool
	Season        string
	Over          bool
	Outcome       string
	Belief, Truth MapView
	Generals      []ReviewGeneral
}

// ReviewGeneral is one general's turn.
type ReviewGeneral struct {
	Name        string
	Sent        []ReviewLetter // letters the player sent him this turn
	Debug       *DebugView     // how the letters he read were interpreted, and what he saw and said
	TrueOutcome string
	Report      string
	ReportFate  string
}

type ReviewLetter struct {
	Body string
	Fate string
}

func buildReview(g *game.Game, turn int) ReviewView {
	snaps := g.Snapshots()
	last := snaps[len(snaps)-1].Turn
	if turn < 0 {
		turn = 0
	}
	if turn > last {
		turn = last
	}
	snap := snaps[turn]
	v := ReviewView{
		Turn: turn, Last: last, Prev: turn - 1, Next: turn + 1, HasPrev: turn > 0, HasNext: turn < last,
		Season: g.Rules.Season(max(turn, 1)), Over: g.Over(), Outcome: g.Outcome(),
	}
	names := map[string]string{}
	var gens []tokenSource
	for _, id := range g.GeneralIDs() {
		names[id] = g.GeneralName(id)
		if b := snap.Belief.Generals[id]; b != nil {
			gens = append(gens, tokenSource{Name: names[id], Province: b.Province, AsOf: b.AsOfTurn, Destroyed: b.Destroyed})
		}
	}
	v.Belief = BeliefMap(g.Map, snap.Belief, turn, gens, MapOptions{})
	v.Truth = TruthMap(g.Map, snap.Truth, names)
	if turn == 0 {
		return v
	}
	for _, id := range g.GeneralIDs() {
		rg := ReviewGeneral{Name: names[id]}
		for _, l := range g.Letters() {
			if l.Kind != model.Dispatch || l.To != id || l.SentTurn != turn {
				continue
			}
			fate := fmt.Sprintf("reached him in turn %d", l.ArriveTurn)
			switch {
			case l.Intercepted:
				fate = "intercepted on the road; he never received it"
			case l.Delivered && l.Interpretation == nil:
				fate = fmt.Sprintf("arrived in turn %d, but nobody was left to read it", l.ArriveTurn)
			case !l.Delivered:
				fate = fmt.Sprintf("still on the road (due turn %d)", l.ArriveTurn)
			}
			rg.Sent = append(rg.Sent, ReviewLetter{Body: l.Body, Fate: fate})
		}
		if d := g.DebugTurn(id, turn); d != nil {
			rg.Debug = buildDebug(g, d)
			o := d.Observation
			rg.TrueOutcome = fmt.Sprintf("%s. Ended the turn with %s men, having lost %s.",
				capitalise(d.Facts.OrderOutcome), men(o.Strength), men(o.Losses))
			if o.Disbanded {
				rg.TrueOutcome = "The army was destroyed."
			}
		}
		for _, l := range g.Letters() {
			if l.Kind == model.Dispatch || l.From != id || l.SentTurn != turn {
				continue
			}
			rg.Report = l.Body
			switch {
			case l.Intercepted:
				rg.ReportFate = "intercepted; you never received it"
			case l.Delivered:
				rg.ReportFate = fmt.Sprintf("written turn %d, reached you in turn %d", l.SentTurn, l.DeliveredTurn)
			default:
				rg.ReportFate = fmt.Sprintf("written turn %d, still on the road", l.SentTurn)
			}
		}
		v.Generals = append(v.Generals, rg)
	}
	return v
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

// review serves the after-action review. It is closed until the game is
// over, unless the server runs with --debug.
func (s *Server) review(w http.ResponseWriter, r *http.Request) {
	s.g.Lock()
	over := s.g.Over()
	s.g.Unlock()
	if !over && !s.debug {
		http.Error(w, "The after-action review opens when the campaign is over.", http.StatusForbidden)
		return
	}
	turn := 1
	if t := r.URL.Query().Get("turn"); t != "" {
		n, err := strconv.Atoi(t)
		if err != nil {
			http.Error(w, "bad turn", http.StatusBadRequest)
			return
		}
		turn = n
	}
	s.g.Lock()
	v := buildReview(s.g, turn)
	s.g.Unlock()
	s.renderPage(w, "review", v)
}
