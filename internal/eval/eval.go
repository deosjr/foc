// Package eval scores a decision provider against hand-labelled letters:
// agreement on clear letters, the probability split on ambiguous ones (and
// whether Velk and Saris diverge as their traits predict), and errors.
package eval

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/deosjr/foc/internal/generals"
	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/providers/decision"
	"gopkg.in/yaml.v3"
)

// Letter is one labelled case.
type Letter struct {
	ID       string            `yaml:"id"`
	Class    string            `yaml:"class"` // clear | ambiguous | conditional | unclear | none
	General  string            `yaml:"general"`
	Location string            `yaml:"location"`
	Friendly map[string]string `yaml:"friendly"` // general id -> province id
	Standing string            `yaml:"standing"` // "hold" or "move:<province>"
	Letter   string            `yaml:"letter"`
	Note     string            `yaml:"note"`
	Expect   struct {
		Action string `yaml:"action"`
		Target string `yaml:"target"`
		Whom   string `yaml:"whom"` // general id a support letter means
		// Conditional letters: the triggers that would be a fair reading,
		// and what to do when one fires.
		Trigger []string `yaml:"trigger"`
		Then    string   `yaml:"then"`
	} `yaml:"expect"`
	Accept   []string `yaml:"accept"`
	Readings []string `yaml:"readings"`
	Target   string   `yaml:"target"`
}

// Load reads and checks a letters file against the map and generals.
func Load(path string, m *mapdata.Map, gens map[string]*model.General) ([]Letter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f struct {
		Letters []Letter `yaml:"letters"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[string]bool{}
	for _, l := range f.Letters {
		switch {
		case seen[l.ID]:
			return nil, fmt.Errorf("%s: duplicate id %s", path, l.ID)
		case gens[l.General] == nil:
			return nil, fmt.Errorf("%s: %s: unknown general %q", path, l.ID, l.General)
		case m.Province(l.Location) == nil:
			return nil, fmt.Errorf("%s: %s: unknown location %q", path, l.ID, l.Location)
		case l.Expect.Whom != "" && gens[l.Expect.Whom] == nil:
			return nil, fmt.Errorf("%s: %s: unknown general %q in expect.whom", path, l.ID, l.Expect.Whom)
		case l.Class == "clear" && l.Expect.Action == "":
			return nil, fmt.Errorf("%s: %s: clear letter without expect.action", path, l.ID)
		case l.Class == "ambiguous" && len(l.Readings) < 2:
			return nil, fmt.Errorf("%s: %s: ambiguous letter needs at least two readings", path, l.ID)
		case l.Class == "conditional" && (len(l.Expect.Trigger) == 0 || l.Expect.Then == "" || l.Expect.Action == ""):
			return nil, fmt.Errorf("%s: %s: conditional letter needs expect.action, expect.trigger and expect.then", path, l.ID)
		case l.Class != "clear" && l.Class != "ambiguous" && l.Class != "conditional" && l.Class != "unclear" && l.Class != "none":
			return nil, fmt.Errorf("%s: %s: bad class %q", path, l.ID, l.Class)
		}
		for _, t := range []string{l.Expect.Target, l.Target} {
			if t != "" && m.Province(t) == nil {
				return nil, fmt.Errorf("%s: %s: unknown target %q", path, l.ID, t)
			}
		}
		seen[l.ID] = true
	}
	return f.Letters, nil
}

// Result is how the provider (and the policy) read one letter.
type Result struct {
	Letter     Letter
	Answers    interpret.Answers
	Error      string
	TopAction  string
	PAction    float64
	TopTarget  string
	PTarget    float64
	Outcome    string // policy outcome for the letter's own general
	Agree      bool
	Confident  bool               // clear letters: top action reached the clear threshold
	PMove      map[string]float64 // ambiguous: P(move) per general after reweighting
	Diverge    string             // ambiguous: "as traits predict" | "same" | "against traits"
	Watch      string             // the condition the general would watch for, in words
	Annotation string
}

// Summary aggregates the results.
type Summary struct {
	Clear, ClearAgree, ClearConfident int
	Ambiguous, AmbInReadings          int
	AmbSplit, AmbDiverge, AmbAgainst  int // AmbSplit: letters that reached the ambiguous branch
	Conditional, CondAgree            int
	ClearWithWatch                    int // clear letters wrongly read as conditional
	Unclear, UnclearAgree             int
	None, NoneAgree                   int
	Errors                            int
}

// Setup is everything Run needs besides the provider.
type Setup struct {
	Map         *mapdata.Map
	Generals    map[string]*model.General
	Questions   *interpret.QuestionSet
	Thresholds  generals.Thresholds
	Concurrency int
}

// Run asks the provider about every letter and scores the answers.
func Run(ctx context.Context, dm decision.Model, letters []Letter, s Setup) ([]Result, Summary) {
	results := make([]Result, len(letters))
	conc := s.Concurrency
	if conc < 1 {
		conc = 4
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i, l := range letters {
		wg.Add(1)
		go func(i int, l Letter) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = one(ctx, dm, l, s)
		}(i, l)
	}
	wg.Wait()
	var sum Summary
	for _, r := range results {
		if r.Error != "" {
			sum.Errors++
		}
		switch r.Letter.Class {
		case "clear":
			sum.Clear++
			if r.Agree {
				sum.ClearAgree++
			}
			if r.Confident {
				sum.ClearConfident++
			}
			if r.Watch != "" {
				sum.ClearWithWatch++
			}
		case "ambiguous":
			sum.Ambiguous++
			if r.Agree {
				sum.AmbInReadings++
			}
			if r.Diverge != "" && r.Diverge != "not ambiguous" && r.Diverge != "no aggressive reading" {
				sum.AmbSplit++
			}
			switch r.Diverge {
			case "as traits predict":
				sum.AmbDiverge++
			case "against traits":
				sum.AmbAgainst++
			}
		case "conditional":
			sum.Conditional++
			if r.Agree {
				sum.CondAgree++
			}
		case "unclear":
			sum.Unclear++
			if r.Agree {
				sum.UnclearAgree++
			}
		case "none":
			sum.None++
			if r.Agree {
				sum.NoneAgree++
			}
		}
	}
	return results, sum
}

func one(ctx context.Context, dm decision.Model, l Letter, s Setup) Result {
	r := Result{Letter: l}
	gen := s.Generals[l.General]
	c := interpret.Context{
		General: gen, Location: l.Location, Friendly: map[string]string{},
		Standing: model.Order{Type: model.Hold}, SentTurn: 1, ArriveTurn: 1, Letter: l.Letter,
	}
	for id, p := range l.Friendly {
		if g := s.Generals[id]; g != nil {
			c.Friendly[g.Name] = p
		}
	}
	if strings.HasPrefix(l.Standing, "move:") {
		c.Standing = model.Order{Type: model.MoveToward, Target: strings.TrimPrefix(l.Standing, "move:")}
	}
	ans, _, err := interpret.Ask(ctx, dm, s.Questions, c, s.Map)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.Answers = ans
	actions := s.Questions.ActionOptions()
	r.TopAction, r.PAction = interpret.Top(ans.Action, actions)
	r.TopTarget, r.PTarget = interpret.Top(ans.Target, append(s.Map.IDs(), interpret.None, interpret.Unclear))
	r.Confident = r.PAction >= s.Thresholds.Clear

	owners := map[string]model.Side{}
	for _, p := range s.Map.Provinces {
		owners[p.ID] = p.Owner
	}
	sit := generals.Situation{ArmyID: "a", Location: l.Location, Side: model.Player, Map: s.Map,
		Owner: func(p string) model.Side { return owners[p] }, Friends: map[string]string{}}
	for id := range l.Friendly {
		if g := s.Generals[id]; g != nil {
			sit.Friends[g.Name] = id
		}
	}
	decide := func(g *model.General) generals.Decision {
		// A fixed middle draw for sampling; never own judgement or refusal,
		// so unclear letters show up as requests for clarification.
		return generals.Interpret(ans, g.Traits, sit, s.Thresholds, actions, generals.Draws{Sample: 0.5, Initiative: 1, Refuse: 1, Then: 0.5})
	}
	own := decide(gen)
	r.Outcome = own.Outcome
	if own.Watch != nil {
		r.Watch = generals.DescribeWatch(own.Watch, s.Map)
	}

	switch l.Class {
	case "clear":
		ok := r.TopAction == l.Expect.Action
		for _, a := range l.Accept {
			ok = ok || r.TopAction == a
		}
		if ok && l.Expect.Target != "" && r.TopAction != generals.ActHold && r.TopTarget != l.Expect.Target {
			ok = false
			r.Annotation = fmt.Sprintf("target %s, expected %s", r.TopTarget, l.Expect.Target)
		}
		if ok && l.Expect.Whom != "" && r.TopAction == generals.ActSupport {
			names := make([]string, 0, len(ans.Whom))
			for n := range ans.Whom {
				names = append(names, n)
			}
			sort.Strings(names)
			if whom, _ := interpret.Top(ans.Whom, names); whom != s.Generals[l.Expect.Whom].Name {
				ok = false
				r.Annotation = fmt.Sprintf("supports %s, expected %s", whom, s.Generals[l.Expect.Whom].Name)
			}
		}
		r.Agree = ok
	case "ambiguous":
		for _, a := range l.Readings {
			r.Agree = r.Agree || r.TopAction == a
		}
		r.PMove = map[string]float64{}
		split := false
		for _, id := range []string{"velk", "saris"} {
			if g := s.Generals[id]; g != nil {
				d := decide(g)
				r.PMove[id] = d.Expected()[generals.ActMove]
				split = split || d.Step == "ambiguous"
			}
		}
		aggressive := false
		for _, a := range l.Readings {
			aggressive = aggressive || a == generals.ActMove
		}
		switch {
		case !split:
			r.Diverge = "not ambiguous"
		case !aggressive:
			r.Diverge = "no aggressive reading"
		case r.PMove["velk"] > r.PMove["saris"]+1e-9:
			r.Diverge = "as traits predict"
		case r.PMove["saris"] > r.PMove["velk"]+1e-9:
			r.Diverge = "against traits"
		default:
			r.Diverge = "same"
		}
	case "conditional":
		w := own.Watch
		okNow := r.TopAction == l.Expect.Action
		for _, a := range l.Accept {
			okNow = okNow || r.TopAction == a
		}
		okTrig := false
		if w != nil {
			for _, tr := range l.Expect.Trigger {
				okTrig = okTrig || string(w.Trigger.Kind) == tr
			}
		}
		r.Agree = okNow && okTrig && w.Action == l.Expect.Then
		switch {
		case w == nil:
			r.Annotation = "no condition read"
		case !r.Agree:
			r.Annotation = fmt.Sprintf("now %s, watch %q", r.TopAction, r.Watch)
		}
	case "unclear":
		r.Agree = own.Unclear()
	case "none":
		r.Agree = r.Outcome == generals.OutcomeIgnored
	}
	return r
}

func pct(p float64) string { return fmt.Sprintf("%3.0f%%", p*100) }

func ratio(a, b int) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", a, b, 100*float64(a)/float64(b))
}

// Report writes a human-readable table and summary.
func Report(w io.Writer, provider string, results []Result, sum Summary, verbose bool) {
	fmt.Fprintf(w, "Decision provider: %s\n\n", provider)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "id\tclass\tgen\taction\ttarget\tpolicy\tverdict")
	for _, r := range results {
		l := r.Letter
		if r.Error != "" {
			fmt.Fprintf(tw, "%s\t%s\t%s\t\t\t\tERROR %s\n", l.ID, l.Class, l.General, r.Error)
			continue
		}
		verdict := map[bool]string{true: "ok", false: "MISS"}[r.Agree]
		switch l.Class {
		case "clear":
			if !r.Confident {
				verdict += " (below clear threshold)"
			}
		case "conditional":
			if r.Watch != "" && r.Agree {
				verdict += "  " + r.Watch
			}
		case "ambiguous":
			verdict += fmt.Sprintf("  P(move) Velk %s Saris %s: %s", pct(r.PMove["velk"]), pct(r.PMove["saris"]), r.Diverge)
		}
		if r.Annotation != "" {
			verdict += "  " + r.Annotation
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s %s\t%s %s\t%s\t%s\n", l.ID, l.Class, l.General,
			r.TopAction, pct(r.PAction), r.TopTarget, pct(r.PTarget), r.Outcome, verdict)
		if verbose {
			fmt.Fprintf(tw, "\t\t\t%q\n", l.Letter)
			fmt.Fprintf(tw, "\t\t\taction %s  engagement %.2f  addressed %.2f  plausible %.2f\n",
				dist(r.Answers.Action), r.Answers.Engagement, r.Answers.Addressed, r.Answers.Plausible)
			if r.Answers.Conditional >= 0.2 {
				fmt.Fprintf(tw, "\t\t\tconditional %.2f  trigger %s  at %s  then %s  then-target %s\n",
					r.Answers.Conditional, dist(r.Answers.Trigger), top(r.Answers.TriggerPlace),
					dist(r.Answers.ThenAction), top(r.Answers.ThenTarget))
			}
		}
	}
	tw.Flush()
	fmt.Fprintf(w, "\nClear letters:     %s agree with the expected action; %s confident (>= clear threshold)\n",
		ratio(sum.ClearAgree, sum.Clear), ratio(sum.ClearConfident, sum.Clear))
	fmt.Fprintf(w, "Ambiguous letters: %s top reading among the defensible ones; %s with an aggressive reading reached the ambiguous branch,\n",
		ratio(sum.AmbInReadings, sum.Ambiguous), ratio(sum.AmbSplit, sum.Ambiguous))
	fmt.Fprintf(w, "                   of which Velk is likelier to march than Saris in %s, the reverse in %d\n",
		ratio(sum.AmbDiverge, sum.AmbSplit), sum.AmbAgainst)
	fmt.Fprintf(w, "Conditional:       %s read with the expected condition and actions; %d clear letters wrongly read as conditional\n",
		ratio(sum.CondAgree, sum.Conditional), sum.ClearWithWatch)
	fmt.Fprintf(w, "Unclear letters:   %s read as unclear (the general asks, or uses his own judgement)\n", ratio(sum.UnclearAgree, sum.Unclear))
	fmt.Fprintf(w, "No instruction:    %s ignored\n", ratio(sum.NoneAgree, sum.None))
	fmt.Fprintf(w, "Errors:            %d of %d\n", sum.Errors, len(results))
}

func top(p map[string]float64) string {
	best, bp := "-", -1.0
	for k, v := range p {
		if v > bp || (v == bp && k < best) {
			best, bp = k, v
		}
	}
	return fmt.Sprintf("%s %.2f", best, bp)
}

func dist(p map[string]float64) string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return p[keys[i]] > p[keys[j]] })
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %.2f", k, p[k]))
	}
	return strings.Join(parts, ", ")
}
