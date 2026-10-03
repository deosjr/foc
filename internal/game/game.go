// Package game orchestrates a turn (phases 1–10), owns the truth, the
// player's belief and the letters, and writes the run directory.
package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	stdlog "log"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/deosjr/foc/internal/config"
	"github.com/deosjr/foc/internal/enemy"
	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/generals"
	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/messaging"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/providers/decision"
	"github.com/deosjr/foc/internal/providers/llm"
	"github.com/deosjr/foc/internal/report"
	"github.com/deosjr/foc/internal/runlog"
	"gopkg.in/yaml.v3"
)

// Options are what New needs besides the config.
type Options struct {
	Config   *config.Config
	Decision decision.Model
	LLM      llm.Model
	Log      *runlog.Log
	RunDir   string
	// Recorder holds every model response of the run; Save writes it as
	// responses.jsonl so the run can be replayed exactly. May be nil.
	Recorder interface{ WriteFile(path string) error }
}

// LetterRecord is a letter plus everything the game knows about it.
type LetterRecord struct {
	model.Letter
	Delivered      bool                      `json:"delivered"`
	DeliveredTurn  int                       `json:"delivered_turn,omitempty"`
	Facts          *report.ReportFacts       `json:"facts,omitempty"`
	Written        *report.Written           `json:"written,omitempty"`
	Interpretation *interpret.Interpretation `json:"interpretation,omitempty"`
}

// TurnDebug is the per-general truth behind one turn's report, for the
// debug drawer and the review.
type TurnDebug struct {
	Turn            int                         `json:"turn"`
	GeneralID       string                      `json:"general"`
	Interpretations []*interpret.Interpretation `json:"interpretations"`
	Order           model.Order                 `json:"order"`
	OrderSource     string                      `json:"order_source"`
	Observation     engine.Observation          `json:"observation"`
	Facts           report.ReportFacts          `json:"facts"`
	Omitted         []string                    `json:"omitted,omitempty"`
	Written         report.Written              `json:"written"`
}

// Game is one campaign.
type Game struct {
	mu sync.Mutex

	cfg      *config.Config
	Map      *mapdata.Map
	Rules    *mapdata.Ruleset
	Scenario *mapdata.Scenario
	eng      *engine.Engine
	couriers messaging.Couriers

	state        *engine.GameState
	generals     map[string]*model.General
	generalOrder []string
	questions    *interpret.QuestionSet
	writer       *report.Writer
	rationales   *report.RationaleWriter
	enemy        enemy.AI
	decision     decision.Model
	log          *runlog.Log
	runDir       string
	recorder     interface{ WriteFile(path string) error }

	rngMessaging, rngInterp, rngPerception, rngReporting *rand.Rand

	belief         *report.Belief
	drafts         map[string]string
	letters        []*LetterRecord
	standingSource map[string]string
	gone           map[string]bool // generals whose army has been destroyed (truth)
	debug          map[string]*TurnDebug
	snapshots      []Snapshot                // end of each turn; index 0 is the start
	seen           map[string]map[string]int // general id -> province -> enemy strength he last saw
}

// Snapshot is belief and truth as they stood at the end of a turn.
type Snapshot struct {
	Turn   int
	Belief *report.Belief
	Truth  *engine.GameState
}

func (g *Game) snapshot(turn int) {
	g.snapshots = append(g.snapshots, Snapshot{Turn: turn, Belief: g.belief.Clone(), Truth: g.state.Clone()})
}

func stream(seed uint64, name string) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(name))
	return rand.New(rand.NewPCG(seed, h.Sum64()))
}

// New loads the content files and starts a game at turn 1.
func New(o Options) (*Game, error) {
	cfg := o.Config
	m, err := mapdata.LoadMap(cfg.Map)
	if err != nil {
		return nil, err
	}
	rules, err := mapdata.LoadRuleset(cfg.Ruleset)
	if err != nil {
		return nil, err
	}
	scn, err := mapdata.LoadScenario(cfg.Scenario, m)
	if err != nil {
		return nil, err
	}
	all, err := mapdata.LoadGenerals(cfg.Generals)
	if err != nil {
		return nil, err
	}
	qs, err := interpret.LoadQuestions(filepath.Join(cfg.Prompts, "questions.yaml"))
	if err != nil {
		return nil, err
	}
	prompts, err := report.LoadPrompts(filepath.Join(cfg.Prompts, "report.tmpl"))
	if err != nil {
		return nil, err
	}
	clarify, err := report.LoadPrompts(filepath.Join(cfg.Prompts, "clarification.tmpl"))
	if err != nil {
		return nil, err
	}
	var rationales *report.RationaleWriter
	if cfg.Interpretation.Rationale {
		rp, err := report.LoadPrompts(filepath.Join(cfg.Prompts, "rationale.tmpl"))
		if err != nil {
			return nil, err
		}
		rationales = &report.RationaleWriter{LLM: o.LLM, Prompts: rp, MaxTokens: cfg.LLM.MaxTokens, Temperature: cfg.LLM.Temperature}
	}
	if o.Log == nil {
		o.Log = runlog.New()
	}
	report.MenPerStrength = rules.MenPerStrength

	eng := &engine.Engine{Map: m, Rules: rules}
	g := &Game{
		cfg: cfg, Map: m, Rules: rules, Scenario: scn, eng: eng,
		couriers:       messaging.Couriers{Map: m, Rules: rules},
		state:          eng.NewState(scn),
		generals:       map[string]*model.General{},
		questions:      qs,
		enemy:          newEnemy(scn, m),
		decision:       o.Decision,
		rationales:     rationales,
		log:            o.Log,
		runDir:         o.RunDir,
		recorder:       o.Recorder,
		rngMessaging:   stream(cfg.Seed, "messaging"),
		rngInterp:      stream(cfg.Seed, "interpretation"),
		rngPerception:  stream(cfg.Seed, "perception"),
		rngReporting:   stream(cfg.Seed, "reporting"),
		belief:         report.NewBelief(m, scn),
		drafts:         map[string]string{},
		standingSource: map[string]string{},
		gone:           map[string]bool{},
		debug:          map[string]*TurnDebug{},
		seen:           map[string]map[string]int{},
	}
	var names []string
	for _, a := range scn.Armies {
		if a.General == "" {
			continue
		}
		gen, ok := all[a.General]
		if !ok {
			return nil, fmt.Errorf("scenario army %s: unknown general %q", a.ID, a.General)
		}
		cp := *gen
		cp.ArmyID = a.ID
		g.generals[cp.ID] = &cp
		g.generalOrder = append(g.generalOrder, cp.ID)
		g.standingSource[cp.ID] = "no orders received yet"
		// Generals start out knowing the court's intelligence.
		g.seen[cp.ID] = map[string]int{}
		for _, in := range scn.Intel {
			g.seen[cp.ID][in.Province] = in.EnemyStrength
		}
		names = append(names, cp.Name)
	}
	for _, gen := range all {
		if g.generals[gen.ID] == nil {
			names = append(names, gen.Name)
		}
	}
	sort.Strings(g.generalOrder)
	sort.Strings(names)
	g.writer = &report.Writer{
		LLM:     o.LLM,
		Prompts: prompts,
		Clarify: clarify,
		Validator: report.Validator{Map: m, MenPerStrength: rules.MenPerStrength, GeneralNames: names,
			MinWords: cfg.Report.MinWords, MaxWords: cfg.Report.MaxWords},
		Capital:     m.NameOf(m.Capital(model.Player)),
		MaxTokens:   cfg.LLM.MaxTokens,
		Temperature: cfg.LLM.Temperature,
	}
	g.snapshot(0)
	g.log.SetPhase(0, "setup")
	g.log.Add("start", map[string]any{"seed": cfg.Seed, "scenario": scn.Name, "state": g.state})
	return g, nil
}

// Lock and Unlock guard the game for callers that read several accessors
// together (the web server). EndTurn holds the lock for the whole turn.
func (g *Game) Lock()   { g.mu.Lock() }
func (g *Game) Unlock() { g.mu.Unlock() }

// ErrOver is returned when acting after the game has ended.
var ErrOver = errors.New("the game is over")

// SetDraft saves the draft letter to a general. Call with the lock held.
func (g *Game) SetDraft(generalID, text string) error {
	if g.generals[generalID] == nil {
		return fmt.Errorf("unknown general %q", generalID)
	}
	if g.state.Over {
		return ErrOver
	}
	if strings.TrimSpace(text) == "" {
		delete(g.drafts, generalID)
		return nil
	}
	g.drafts[generalID] = text
	return nil
}

// Draft returns the saved draft for a general.
func (g *Game) Draft(generalID string) string { return g.drafts[generalID] }

func (g *Game) nextSeq() int { return len(g.letters) + 1 }

func (g *Game) generalLocation(id string) string {
	if a := g.state.Armies[g.generals[id].ArmyID]; a != nil {
		return a.Location
	}
	// A destroyed army's general is found where it fell.
	for i := len(g.letters) - 1; i >= 0; i-- {
		if l := g.letters[i]; l.From == id && l.Facts != nil {
			for _, p := range g.Map.Provinces {
				if p.Name == l.Facts.Location {
					return p.ID
				}
			}
		}
	}
	return g.Map.Capital(model.Player)
}

// EndTurn seals the drafts and runs phases 2–10. progress, if not nil, is
// told the name of each phase as it starts. Call with the lock held.
func (g *Game) EndTurn(ctx context.Context, progress func(string)) error {
	if g.state.Over {
		return ErrOver
	}
	if progress == nil {
		progress = func(string) {}
	}
	T := g.state.Turn
	capital := g.Map.Capital(model.Player)

	// 1. Compose: seal drafts into dispatches.
	g.log.SetPhase(T, "compose")
	for _, gid := range g.generalOrder {
		text, ok := g.drafts[gid]
		if !ok {
			continue
		}
		seq := g.nextSeq()
		l := &LetterRecord{Letter: model.Letter{
			ID: fmt.Sprintf("L%d", seq), Seq: seq, Kind: model.Dispatch,
			From: model.Sovereign, To: gid, Body: text, SentTurn: T,
		}}
		g.couriers.Send(&l.Letter, capital, g.generalLocation(gid))
		g.letters = append(g.letters, l)
		g.log.Add("dispatch", l.Letter)
		g.intercept(l)
	}
	g.drafts = map[string]string{}

	// 2. Transit: dispatches arriving this turn, by general then sequence.
	progress("couriers riding")
	g.log.SetPhase(T, "transit")
	var arriving []*LetterRecord
	for _, l := range g.letters {
		if l.Kind == model.Dispatch && !l.Delivered && !l.Intercepted && l.ArriveTurn <= T {
			l.Delivered, l.DeliveredTurn = true, T
			if g.gone[l.To] {
				g.log.Add("undeliverable", map[string]any{"letter": l.ID, "reason": "army destroyed"})
				continue
			}
			arriving = append(arriving, l)
			g.log.Add("arrive", map[string]any{"letter": l.ID, "general": l.To})
		}
	}
	sort.SliceStable(arriving, func(i, j int) bool {
		if arriving[i].To != arriving[j].To {
			return arriving[i].To < arriving[j].To
		}
		return arriving[i].Seq < arriving[j].Seq
	})

	// 3. Interpretation: model calls concurrently, RNG draws afterwards in
	// a fixed order, so concurrency never changes outcomes.
	progress("generals reading")
	g.log.SetPhase(T, "interpretation")
	interps := g.interpret(ctx, T, arriving)
	g.log.SetPhase(T, "interpretation")
	orders := map[string]model.Order{}
	sources := map[string]string{}
	effective := map[string][]string{} // concerns from the letter that decided the order
	doubts := map[string][]string{}    // concerns about letters set aside as not genuine
	refused := map[string]*model.Order{}
	clarify := map[string]*report.ClarifyFacts{}
	byGeneral := map[string][]*interpret.Interpretation{}
	for i, l := range arriving {
		in := interps[i]
		gen := g.generals[l.To]
		// Always three draws per letter, in this order, whichever step decides.
		dr := generals.Draws{Sample: g.rngInterp.Float64(), Initiative: g.rngInterp.Float64(), Refuse: g.rngInterp.Float64()}
		in.RNGDraw, in.Initiative, in.RefuseDraw = dr.Sample, dr.Initiative, dr.Refuse
		if in.Error == "" {
			d := generals.Interpret(in.Parsed, gen.Traits, g.situation(l.To), g.cfg.Interpretation.Thresholds, g.questions.ActionOptions(), dr)
			in.Outcome, in.Step, in.Order, in.Refused = d.Outcome, d.Step, d.Order, d.Refused
			in.Weights, in.Reweighted = d.Weights, d.Reweighted
		} else {
			// The general could not make sense of the letter at all.
			in.Outcome, in.Step = generals.OutcomeClarify, "error"
			in.Order = &model.Order{ArmyID: gen.ArmyID, Type: model.Hold}
		}
		l.Interpretation = in
		byGeneral[l.To] = append(byGeneral[l.To], in)
		g.log.Add("interpretation", in)
		source := fmt.Sprintf("your letter sent turn %d", l.SentTurn)
		switch in.Outcome {
		case generals.OutcomeOrder, generals.OutcomeClarify, generals.OutcomeRefuse:
			// The later letter wins.
			orders[l.To], sources[l.To] = *in.Order, source
			effective[l.To], refused[l.To], clarify[l.To] = nil, nil, nil
		}
		switch in.Outcome {
		case generals.OutcomeOrder:
			if strings.HasPrefix(in.Step, "own-judgement") {
				effective[l.To] = []string{fmt.Sprintf(
					"I could not make out what you wished me to do from your letter sent turn %d, so I acted on my own judgement and chose to %s.",
					l.SentTurn, interpret.DescribeOrder(*in.Order, g.Map, g.armyNames()))}
			}
		case generals.OutcomeClarify:
			what := map[string]string{
				"no-target":         "where you would have me go",
				"no-support-target": "whom you would have me support",
			}[in.Step]
			if what == "" {
				what = "what you would have me do"
			}
			clarify[l.To] = &report.ClarifyFacts{LetterSentTurn: l.SentTurn, YourLetter: l.Body, Unclear: []string{what}}
		case generals.OutcomeRefuse:
			refused[l.To] = in.Refused
		case generals.OutcomeDoubted:
			before := gen.Traits.Loyalty
			gen.Traits.Loyalty = math.Max(0, gen.Traits.Loyalty-0.1)
			g.log.Add("loyalty", map[string]any{"general": l.To, "from": before, "to": gen.Traits.Loyalty, "letter": l.ID})
			doubts[l.To] = append(doubts[l.To], fmt.Sprintf(
				"Your letter sent turn %d did not read as if it came from your hand; I have set it aside and keep to my previous orders.", l.SentTurn))
		}
	}

	// 4. Standing orders for generals with no new order.
	g.log.SetPhase(T, "standing")
	var all []model.Order
	active := []string{}
	for _, gid := range g.generalOrder {
		gen := g.generals[gid]
		if g.state.Armies[gen.ArmyID] == nil {
			continue
		}
		active = append(active, gid)
		if _, ok := orders[gid]; !ok {
			orders[gid] = g.state.Standing[gid]
			sources[gid] = g.standingSource[gid]
		}
		all = append(all, orders[gid])
		g.log.Add("order", map[string]any{"general": gid, "order": orders[gid], "source": sources[gid]})
	}

	// 5. Enemy orders, issued directly with full knowledge.
	g.log.SetPhase(T, "enemy")
	eo := g.enemy.Orders(g.state)
	g.log.Add("enemy-orders", eo)
	all = append(all, eo...)

	// 6. Resolution.
	progress("battle")
	g.log.SetPhase(T, "resolution")
	res, err := g.eng.Resolve(g.state, all)
	if err != nil {
		return fmt.Errorf("turn %d: %w", T, err)
	}
	g.log.Add("resolution", res)
	for _, gid := range active {
		o := orders[gid]
		next := messaging.NextStanding(o, res, g.state)
		g.state.Standing[gid] = next
		switch {
		case next == o && strings.HasPrefix(sources[gid], "your letter"):
			g.standingSource[gid] = "standing order from " + sources[gid]
		case next == o:
			g.standingSource[gid] = sources[gid]
		default:
			g.standingSource[gid] = "no new orders; holding"
		}
	}

	// 7–8. Perception and distortion, in general order.
	g.log.SetPhase(T, "perception")
	names := map[string]string{}
	for id, gen := range g.generals {
		names[id] = gen.Name
	}
	facts := map[string]report.ReportFacts{}
	obs := map[string]engine.Observation{}
	for _, gid := range active {
		gen := g.generals[gid]
		o := g.eng.Perceive(g.state, res, gid, gen.ArmyID, g.rngPerception)
		obs[gid] = o
		g.log.Add("observation", o)
		if o.Disbanded {
			g.gone[gid] = true
		}
		// What he saw this turn replaces what he saw nearby before.
		seen := g.seen[gid]
		for _, p := range append([]string{o.Location}, o.Quiet...) {
			delete(seen, p)
		}
		for _, sgt := range o.Sightings {
			seen[sgt.Province] = sgt.Strength
		}
	}
	g.log.SetPhase(T, "distortion")
	omitted := map[string][]string{}
	for _, gid := range active {
		f, om := generals.Distort(obs[gid], g.generals[gid], generals.ReportContext{
			Map: g.Map, Rules: g.Rules, GeneralNames: names, ArmyNames: g.armyNames(),
			OrderSource: sources[gid], Concerns: append(append([]string{}, doubts[gid]...), effective[gid]...),
			Refused: refused[gid], Clarify: clarify[gid], Draw: g.rngReporting.Float64,
		})
		facts[gid], omitted[gid] = f, om
		g.log.Add("report-facts", map[string]any{"general": gid, "facts": f, "omitted": om})
	}

	// 9. Reporting: letters written concurrently, sent in general order.
	progress("reports being written")
	g.log.SetPhase(T, "reporting")
	written := make([]report.Written, len(active))
	var wg sync.WaitGroup
	for i, gid := range active {
		wg.Add(1)
		go func(i int, gid string) {
			defer wg.Done()
			written[i] = g.writer.Write(ctx, g.generals[gid], facts[gid])
		}(i, gid)
	}
	// Private rationales for the review, written alongside the reports.
	rationaleErrs := make([]error, len(arriving))
	if g.rationales != nil {
		for i, l := range arriving {
			in := l.Interpretation
			if in == nil || in.Error != "" {
				continue
			}
			order := orders[l.To] // an ignored letter: he carried on as before
			if in.Order != nil {
				order = *in.Order
			}
			rf := report.RationaleFacts{
				General: g.generals[l.To].Name, Location: g.Map.NameOf(obs[l.To].Start), Letter: l.Body,
				SentTurn: l.SentTurn, Decision: interpret.DescribeOrder(order, g.Map, g.armyNames()),
				Reading: report.Reading(in.Outcome, in.Step),
			}
			wg.Add(1)
			go func(i int, in *interpret.Interpretation, gen *model.General, rf report.RationaleFacts) {
				defer wg.Done()
				in.Rationale, rationaleErrs[i] = g.rationales.Write(ctx, gen, rf)
			}(i, in, g.generals[l.To], rf)
		}
	}
	wg.Wait()
	g.log.Flush()
	for i, l := range arriving {
		if in := l.Interpretation; in != nil && (in.Rationale != "" || rationaleErrs[i] != nil) {
			rec := map[string]any{"letter": l.ID, "general": l.To, "rationale": in.Rationale}
			if rationaleErrs[i] != nil {
				rec["error"] = rationaleErrs[i].Error()
			}
			g.log.Add("rationale", rec)
		}
	}
	for i, gid := range active {
		f, w := facts[gid], written[i]
		seq := g.nextSeq()
		kind := model.Report
		if f.Clarification != nil {
			kind = model.Clarification
		}
		l := &LetterRecord{Letter: model.Letter{
			ID: fmt.Sprintf("L%d", seq), Seq: seq, Kind: kind,
			From: gid, To: model.Sovereign, Body: w.Text, SentTurn: T,
		}, Facts: &f, Written: &w}
		g.couriers.Send(&l.Letter, obs[gid].Location, capital)
		g.letters = append(g.letters, l)
		g.log.Add("report", map[string]any{"letter": l.Letter, "written": w})
		g.intercept(l)
		g.debug[debugKey(gid, T)] = &TurnDebug{
			Turn: T, GeneralID: gid, Interpretations: byGeneral[gid],
			Order: orders[gid], OrderSource: sources[gid], Observation: obs[gid], Facts: f, Omitted: omitted[gid], Written: w,
		}
	}

	// Victory is decided by the truth at the end of resolution.
	g.eng.Evaluate(g.state, g.Scenario.TurnCap)

	// 10. Delivery. At the end of the game every report still on the road
	// is delivered, so the player hears the last of the campaign.
	g.log.SetPhase(T, "delivery")
	var due []*LetterRecord
	for _, l := range g.letters {
		if l.Kind != model.Dispatch && !l.Delivered && !l.Intercepted && (l.ArriveTurn <= T || g.state.Over) {
			due = append(due, l)
		}
	}
	sort.SliceStable(due, func(i, j int) bool {
		if due[i].SentTurn != due[j].SentTurn {
			return due[i].SentTurn < due[j].SentTurn
		}
		return due[i].Seq < due[j].Seq
	})
	ids := map[string]string{}
	for id, gen := range g.generals {
		ids[gen.Name] = id
	}
	for _, l := range due {
		l.Delivered, l.DeliveredTurn = true, T
		g.belief.Apply(*l.Facts, l.From, g.Map, ids)
		g.log.Add("deliver", map[string]any{"letter": l.ID, "from": l.From, "written": l.SentTurn})
	}

	g.snapshot(T)
	if g.state.Over {
		g.log.Add("game-over", map[string]any{"winner": g.state.Winner, "outcome": g.state.Outcome})
		g.autosave()
		progress("done")
		return nil
	}
	g.state.Turn++
	g.autosave()
	progress("done")
	return nil
}

// autosave writes the run directory after every turn, so any game can be
// inspected and replayed afterwards. A failed save never stops the game.
func (g *Game) autosave() {
	if g.runDir == "" {
		return
	}
	if _, err := g.Save(); err != nil {
		stdlog.Printf("autosave to %s failed: %v", g.runDir, err)
	}
}

// intercept rolls for a letter on the road, against the true positions of
// the enemy at the time it travels. A captured letter is lost silently.
func (g *Game) intercept(l *LetterRecord) {
	near := func(p string) bool {
		for _, n := range append([]string{p}, g.Map.Neighbours(p)...) {
			for _, a := range g.state.ArmiesIn(n) {
				if a.Side == model.Enemy {
					return true
				}
			}
		}
		return false
	}
	if where := g.couriers.Intercept(l.Route, near, g.rngMessaging.Float64); where != "" {
		l.Intercepted = true
		g.log.Add("intercepted", map[string]any{"letter": l.ID, "kind": l.Kind, "at": where})
	}
}

func newEnemy(scn *mapdata.Scenario, m *mapdata.Map) enemy.AI {
	if scn.EnemyAI == "heuristic" {
		return &enemy.Heuristic{Map: m}
	}
	return enemy.NewScripted(scn.EnemyRoutes)
}

// armyNames maps army ids to their generals' names.
func (g *Game) armyNames() map[string]string {
	out := map[string]string{}
	for _, gen := range g.generals {
		out[gen.ArmyID] = gen.Name
	}
	return out
}

// situation is what a general knows of his own position and the enemy.
func (g *Game) situation(gid string) generals.Situation {
	gen := g.generals[gid]
	army := g.state.Armies[gen.ArmyID]
	seen := g.seen[gid]
	sit := generals.Situation{
		ArmyID: gen.ArmyID, Location: army.Location, Side: model.Player, Map: g.Map,
		Owner:     func(p string) model.Side { return g.state.Provinces[p].Owner },
		Strength:  army.Strength,
		EnemySeen: func(p string) int { return seen[p] },
		Friends:   map[string]string{},
	}
	for _, other := range g.generalOrder {
		if a := g.state.Armies[g.generals[other].ArmyID]; other != gid && a != nil {
			sit.Friends[g.generals[other].Name] = a.ID
		}
	}
	best := -1
	for _, p := range g.Map.IDs() { // sorted, so ties go to the lowest id
		if seen[p] > 0 {
			if d := g.Map.Distance(army.Location, p); best < 0 || d < best {
				best, sit.NearestEnemy = d, p
			}
		}
	}
	return sit
}

// interpret asks the decision model about every arriving letter at once.
func (g *Game) interpret(ctx context.Context, T int, arriving []*LetterRecord) []*interpret.Interpretation {
	out := make([]*interpret.Interpretation, len(arriving))
	var wg sync.WaitGroup
	for i, l := range arriving {
		gen := g.generals[l.To]
		c := interpret.Context{
			General:    gen,
			Location:   g.state.Armies[gen.ArmyID].Location,
			Friendly:   map[string]string{},
			Standing:   g.state.Standing[l.To],
			SentTurn:   l.SentTurn,
			ArriveTurn: T,
			Letter:     l.Body,
		}
		for _, other := range g.generalOrder {
			if a := g.state.Armies[g.generals[other].ArmyID]; other != l.To && a != nil {
				c.Friendly[g.generals[other].Name] = a.Location
			}
		}
		out[i] = &interpret.Interpretation{LetterID: l.ID, GeneralID: l.To, Turn: T, State: interpret.BuildState(c, g.Map)}
		wg.Add(1)
		go func(in *interpret.Interpretation, c interpret.Context) {
			defer wg.Done()
			ans, resp, err := interpret.Ask(ctx, g.decision, g.questions, c, g.Map)
			in.Answers, in.Parsed = resp, ans
			if err != nil {
				in.Error = err.Error()
			}
		}(out[i], c)
	}
	wg.Wait()
	return out
}

func debugKey(gid string, turn int) string { return fmt.Sprintf("%s/%d", gid, turn) }

// Save writes the run directory: config, turn log, state and the recorded
// model responses.
func (g *Game) Save() (string, error) {
	if g.runDir == "" {
		return "", errors.New("no run directory configured")
	}
	if err := os.MkdirAll(g.runDir, 0o755); err != nil {
		return "", err
	}
	cfg, err := yaml.Marshal(g.cfg)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(g.runDir, "config.yaml"), cfg, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(g.runDir, "turns.jsonl"), g.log.Bytes(), 0o644); err != nil {
		return "", err
	}
	st, err := json.MarshalIndent(g.state, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(g.runDir, "state.json"), st, 0o644); err != nil {
		return "", err
	}
	if g.recorder != nil {
		if err := g.recorder.WriteFile(filepath.Join(g.runDir, "responses.jsonl")); err != nil {
			return "", err
		}
	}
	return g.runDir, nil
}
