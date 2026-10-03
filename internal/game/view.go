package game

import (
	"regexp"
	"sort"

	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/report"
)

// Everything in this file except the Debug* accessors is player-safe: it is
// built from the belief, the player's own letters and delivered letters only.

// GeneralInfo is what the player knows about one of their generals.
type GeneralInfo struct {
	ID             string
	Name           string
	Bio            string
	Province       string // believed location (province id)
	AsOfTurn       int
	Destroyed      bool
	LastReportTurn int // written turn of the latest delivered report; 0 if none
	CourierDelay   int // expected turns for a letter to reach the believed location
}

// SentLetter is one of the player's own dispatches.
type SentLetter struct {
	ID       string
	To       string
	Body     string
	SentTurn int
	ReplyID  string // the report written on the turn this letter was read, once delivered
}

// InboxLetter is a delivered report or clarification.
type InboxLetter struct {
	ID          string
	From        string
	FromName    string
	Kind        model.LetterKind
	Body        string
	WrittenTurn int
	ArrivedTurn int
	Mentions    []string // province ids named in the letter text
}

func (g *Game) Turn() int          { return g.state.Turn }
func (g *Game) Season() string     { return g.Rules.Season(g.state.Turn) }
func (g *Game) Over() bool         { return g.state.Over }
func (g *Game) Outcome() string    { return g.state.Outcome }
func (g *Game) Winner() model.Side { return g.state.Winner }

// Belief returns the player's map. Callers must not modify it.
func (g *Game) Belief() *report.Belief { return g.belief }

// GeneralIDs returns the generals in a stable order.
func (g *Game) GeneralIDs() []string { return append([]string(nil), g.generalOrder...) }

// Generals returns what the player knows about each general.
func (g *Game) Generals() []GeneralInfo {
	capital := g.Map.Capital(model.Player)
	var out []GeneralInfo
	for _, id := range g.generalOrder {
		gen := g.generals[id]
		info := GeneralInfo{ID: id, Name: gen.Name, Bio: gen.Bio}
		if b := g.belief.Generals[id]; b != nil {
			info.Province, info.AsOfTurn, info.Destroyed = b.Province, b.AsOfTurn, b.Destroyed
			info.CourierDelay = g.couriers.Delay(g.couriers.Route(capital, b.Province))
		}
		for _, l := range g.letters {
			if l.Delivered && l.Kind != model.Dispatch && l.From == id && l.SentTurn > info.LastReportTurn {
				info.LastReportTurn = l.SentTurn
			}
		}
		out = append(out, info)
	}
	return out
}

// Sent returns the player's dispatches, oldest first.
func (g *Game) Sent() []SentLetter {
	var out []SentLetter
	for _, l := range g.letters {
		if l.Kind != model.Dispatch {
			continue
		}
		s := SentLetter{ID: l.ID, To: l.To, Body: l.Body, SentTurn: l.SentTurn}
		if l.Interpretation != nil {
			for _, r := range g.letters {
				if r.Kind != model.Dispatch && r.Delivered && r.From == l.To && r.SentTurn == l.Interpretation.Turn {
					s.ReplyID = r.ID
				}
			}
		}
		out = append(out, s)
	}
	return out
}

// Inbox returns delivered letters, newest first.
func (g *Game) Inbox() []InboxLetter {
	var out []InboxLetter
	for _, l := range g.letters {
		if l.Kind == model.Dispatch || !l.Delivered {
			continue
		}
		out = append(out, InboxLetter{
			ID: l.ID, From: l.From, FromName: g.generals[l.From].Name, Kind: l.Kind, Body: l.Body,
			WrittenTurn: l.SentTurn, ArrivedTurn: l.DeliveredTurn, Mentions: g.mentions(l.Body),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ArrivedTurn != out[j].ArrivedTurn {
			return out[i].ArrivedTurn > out[j].ArrivedTurn
		}
		if out[i].WrittenTurn != out[j].WrittenTurn {
			return out[i].WrittenTurn > out[j].WrittenTurn
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// InboxLetter returns one delivered letter by id.
func (g *Game) InboxLetter(id string) (InboxLetter, bool) {
	for _, l := range g.Inbox() {
		if l.ID == id {
			return l, true
		}
	}
	return InboxLetter{}, false
}

// Mentions returns the province ids named in a text, by name or capitalised
// alias.
func (g *Game) mentions(text string) []string {
	var out []string
	for _, p := range g.Map.Provinces {
		for _, n := range append([]string{p.Name}, p.Aliases...) {
			if n == "" || n[0] < 'A' || n[0] > 'Z' {
				continue
			}
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(n) + `\b`).MatchString(text) {
				out = append(out, p.ID)
				break
			}
		}
	}
	return out
}

// Recognised returns the province names found in a draft, and capitalised
// words that match nothing (likely typos of names).
func (g *Game) Recognised(text string) (found, unknown []string) {
	ids := g.mentions(text)
	known := map[string]bool{}
	for _, id := range ids {
		found = append(found, g.Map.NameOf(id))
	}
	for _, p := range g.Map.Provinces {
		for _, w := range regexp.MustCompile(`[A-Za-z]+`).FindAllString(p.Name+" "+joinAliases(p.Aliases), -1) {
			known[w] = true
		}
	}
	for _, gen := range g.generals {
		for _, w := range regexp.MustCompile(`[A-Za-z]+`).FindAllString(gen.Name, -1) {
			known[w] = true
		}
	}
	seen := map[string]bool{}
	// Capitalised words not at the start of a sentence.
	for _, m := range regexp.MustCompile(`[^.!?\n]\s+([A-Z][a-z]{2,})`).FindAllStringSubmatch(text, -1) {
		w := m[1]
		if !known[w] && !commonWords[w] && !seen[w] {
			unknown = append(unknown, w)
			seen[w] = true
		}
	}
	return found, unknown
}

var commonWords = map[string]bool{"The": true, "You": true, "Your": true, "Lord": true, "Lady": true,
	"General": true, "Majesty": true, "King": true, "Queen": true, "God": true, "Gods": true, "Sovereign": true}

func joinAliases(a []string) string {
	s := ""
	for _, x := range a {
		s += " " + x
	}
	return s
}

// DebugTruth returns the true state. For --debug only.
func (g *Game) DebugTruth() *engine.GameState { return g.state }

// DebugFor returns the truth behind a delivered report. For --debug only.
func (g *Game) DebugFor(letterID string) *TurnDebug {
	for _, l := range g.letters {
		if l.ID == letterID && l.Kind != model.Dispatch {
			return g.debug[debugKey(l.From, l.SentTurn)]
		}
	}
	return nil
}

// DebugLetter returns a letter record (dispatch or report) by id. For --debug only.
func (g *Game) DebugLetter(id string) *LetterRecord {
	for _, l := range g.letters {
		if l.ID == id {
			return l
		}
	}
	return nil
}

// Snapshots returns belief and truth at the start and after every turn.
// For the after-action review only.
func (g *Game) Snapshots() []Snapshot { return g.snapshots }

// DebugTurn returns the truth behind a general's turn. For review/debug only.
func (g *Game) DebugTurn(generalID string, turn int) *TurnDebug {
	return g.debug[debugKey(generalID, turn)]
}

// Letters returns every letter record, including undelivered and
// intercepted ones. For review/debug only.
func (g *Game) Letters() []*LetterRecord { return g.letters }

// GeneralName returns a general's display name.
func (g *Game) GeneralName(id string) string {
	if gen := g.generals[id]; gen != nil {
		return gen.Name
	}
	return id
}
