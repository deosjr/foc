// Package mock is an offline decision model built on keyword rules, so the
// game and tests run with no network. It is deliberately simple: a letter
// with one kind of instruction reads as clear, a letter that mixes kinds
// reads as ambiguous, and a letter with none reads as unclear.
package mock

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/decision"
)

func init() {
	providers.RegisterDecision("mock", func(cfg providers.ProviderConfig, _ providers.Deps) (decision.Model, error) {
		return New(), nil
	})
}

type Model struct{}

func New() *Model { return &Model{} }

var keywords = map[string][]string{
	"move": {"march", "move", "advance", "attack", "take", "seize", "capture", "go", "proceed", "push",
		"press", "strike", "assault", "occupy", "relieve", "join", "cross", "ride", "head", "invade", "drive",
		"make for", "return to"},
	"hold": {"hold", "stay", "remain", "defend", "keep", "guard", "stand fast", "stand firm", "wait",
		"garrison", "sit tight"},
	"retreat":  {"retreat", "withdraw", "fall back", "pull back", "abandon", "come home", "return home", "pull"},
	"support":  {"support", "aid", "assist", "help", "back up", "reinforce", "lend"},
	"entrench": {"entrench", "dig in", "fortify", "earthworks", "dig"},
	"scout":    {"scout", "reconnoitre", "reconnoiter", "spy", "send riders", "find out"},
}

var actions = []string{"move", "hold", "support", "entrench", "scout", "retreat"}

var hawkish = []string{"attack", "crush", "destroy", "battle", "fight", "strike", "assault", "seize", "smash", "at once", "boldly", "drive"}
var dovish = []string{"avoid", "careful", "caution", "cautious", "do not engage", "don't engage", "preserve", "safe", "without a fight", "carefully", "no risk", "spare"}

var negation = regexp.MustCompile(`\b(not|don't|dont|never|nor)\s+(\w+\s+)?$`)

type mention struct {
	pos  int
	name string
}

func find(text string, phrase string) []int {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(phrase) + `\b`)
	var out []int
	for _, loc := range re.FindAllStringIndex(text, -1) {
		out = append(out, loc[0])
	}
	return out
}

var condRe = regexp.MustCompile(`\b(if|should|when|unless)\b`)
var hedgeRe = regexp.MustCompile(`^(if|when) (you can|you must|possible|need be|necessary)`)

// splitCondition finds the last real condition in a letter ("if they cross
// the ford") and the clause that says what to do then. It returns the rest
// of the letter, the condition and the then-clause.
func splitCondition(letter string) (rest, cond, then string) {
	locs := condRe.FindAllStringIndex(letter, -1)
	for i := len(locs) - 1; i >= 0; i-- {
		start := locs[i][0]
		if hedgeRe.MatchString(letter[start:]) {
			continue
		}
		end := len(letter)
		if j := strings.IndexAny(letter[start:], ",.;!?"); j >= 0 {
			end = start + j
		}
		cond = letter[start:end]
		// The then-clause: before the condition within its sentence, or
		// after it when the condition opens the sentence.
		sentStart := strings.LastIndexAny(letter[:start], ",.;!?") + 1
		before := strings.TrimSpace(letter[sentStart:start])
		if before != "" {
			then = before
			rest = letter[:sentStart] + letter[end:]
		} else {
			stop := len(letter)
			if j := strings.IndexAny(letter[end+1:], ".;!?"); j >= 0 && end+1 <= len(letter) {
				stop = end + 1 + j
			}
			if end < len(letter) {
				then = letter[end:stop]
			}
			rest = letter[:start] + letter[stop:]
		}
		return rest, cond, then
	}
	return letter, "", ""
}

type condAnswers struct {
	p                    float64
	trigger, place, then map[string]float64
	target               map[string]float64
}

// conditional reads the condition and then-clause with the same keyword rules.
func conditional(cond, then, state string) condAnswers {
	none := map[string]float64{interpret.None: 1}
	if cond == "" {
		return condAnswers{p: 0.05, trigger: none, place: none, then: none, target: none}
	}
	c := condAnswers{p: 0.9}
	switch {
	case strings.Contains(cond, "force") || strings.Contains(cond, "outnumber") || strings.Contains(cond, "too many") || strings.Contains(cond, "strong"):
		c.trigger = map[string]float64{"outnumbered": 0.85, "attacked": 0.1, "unclear": 0.05}
	case strings.Contains(cond, "fall") || strings.Contains(cond, "lost") || strings.Contains(cond, "taken"):
		c.trigger = map[string]float64{"place_lost": 0.85, "enemy_at": 0.1, "unclear": 0.05}
	case strings.Contains(cond, "for you") || strings.Contains(cond, "attack") || strings.Contains(cond, "assault"):
		c.trigger = map[string]float64{"attacked": 0.85, "enemy_at": 0.1, "unclear": 0.05}
	default:
		c.trigger = map[string]float64{"enemy_at": 0.85, "attacked": 0.1, "unclear": 0.05}
	}
	place := func(text string) map[string]float64 {
		for _, m := range mentionsIn(text, state) {
			return map[string]float64{m: 0.9, interpret.None: 0.1}
		}
		return map[string]float64{interpret.None: 0.9, interpret.Unclear: 0.1}
	}
	c.place = place(cond)
	c.then = map[string]float64{"unclear": 1}
	for _, k := range actions {
		for _, w := range keywords[k] {
			if len(find(then, w)) > 0 {
				c.then = map[string]float64{k: 0.85, "hold": 0.1, "unclear": 0.05}
				if k == "hold" {
					c.then = map[string]float64{"hold": 0.9, "unclear": 0.1}
				}
			}
		}
	}
	c.target = place(then)
	if _, ok := c.target[interpret.None]; ok {
		c.target = place(cond)
	}
	return c
}

// mentionsIn lists the provinces named in a text, in order of appearance.
func mentionsIn(text, state string) []string {
	var ms []mention
	for name, names := range provinces(state) {
		best := -1
		for _, n := range names {
			for _, pos := range find(text, strings.ToLower(n)) {
				if best < 0 || pos < best {
					best = pos
				}
			}
		}
		if best >= 0 {
			ms = append(ms, mention{best, name})
		}
	}
	sort.Slice(ms, func(i, j int) bool {
		if ms[i].pos != ms[j].pos {
			return ms[i].pos < ms[j].pos
		}
		return ms[i].name < ms[j].name
	})
	var out []string
	for _, m := range ms {
		out = append(out, m.name)
	}
	return out
}

// friends parses "FRIENDLY GENERALS: A in X; B in Y" from the state.
func friends(state string) []string {
	var out []string
	for _, line := range strings.Split(state, "\n") {
		if !strings.HasPrefix(line, "FRIENDLY GENERALS: ") {
			continue
		}
		for _, item := range strings.Split(strings.TrimPrefix(line, "FRIENDLY GENERALS: "), "; ") {
			if i := strings.Index(item, " in "); i > 0 {
				out = append(out, item[:i])
			}
		}
	}
	sort.Strings(out)
	return out
}

// provinces parses "ALL PROVINCES: A; B (also: x, y); ..." from the state.
func provinces(state string) map[string][]string {
	out := map[string][]string{}
	for _, line := range strings.Split(state, "\n") {
		if !strings.HasPrefix(line, "ALL PROVINCES: ") {
			continue
		}
		for _, item := range strings.Split(strings.TrimPrefix(line, "ALL PROVINCES: "), "; ") {
			name, aliases := item, ""
			if i := strings.Index(item, " (also: "); i >= 0 {
				name, aliases = item[:i], strings.TrimSuffix(item[i+len(" (also: "):], ")")
			}
			names := []string{name}
			if aliases != "" {
				names = append(names, strings.Split(aliases, ", ")...)
			}
			out[name] = names
		}
	}
	return out
}

func (m *Model) Decide(_ context.Context, req decision.Request) (decision.Response, error) {
	full := strings.ToLower(interpret.LetterFromState(req.State))
	letter, cond, then := splitCondition(full)
	counts := map[string]int{}
	first := -1 // position of the first move/retreat keyword
	for kind, words := range keywords {
		for _, w := range words {
			for _, pos := range find(letter, w) {
				k := kind
				if kind != "hold" && negation.MatchString(letter[:pos]) {
					k = "hold" // "do not attack" is an instruction to hold
				}
				counts[k]++
				if k != "hold" && (first < 0 || pos < first) {
					first = pos
				}
			}
		}
	}
	action := map[string]float64{}
	total, kinds := 0, 0
	for _, k := range actions {
		total += counts[k]
		if counts[k] > 0 {
			kinds++
		}
	}
	switch {
	case total == 0 && cond != "":
		// Only a condition: hold until it happens.
		action = map[string]float64{"hold": 0.85, "unclear": 0.05, "move": 0.05, "retreat": 0.05}
	case total == 0:
		action = map[string]float64{"unclear": 0.7, "hold": 0.15, "move": 0.1, "retreat": 0.05}
	case kinds == 1:
		for _, k := range actions {
			if counts[k] > 0 {
				action[k] = 0.88
			} else {
				action[k] = 0.02
			}
		}
		action["unclear"] = 0.02
	default:
		for _, k := range actions {
			action[k] = 0.9 * float64(counts[k]) / float64(total)
		}
		action["unclear"] = 0.1
	}

	// Target: the first province named after the first move or retreat
	// word, else the first province named at all.
	var mentions []mention
	for name, names := range provinces(req.State) {
		best := -1
		for _, n := range names {
			for _, pos := range find(letter, strings.ToLower(n)) {
				if best < 0 || pos < best {
					best = pos
				}
			}
		}
		if best >= 0 {
			mentions = append(mentions, mention{best, name})
		}
	}
	sort.Slice(mentions, func(i, j int) bool {
		if mentions[i].pos != mentions[j].pos {
			return mentions[i].pos < mentions[j].pos
		}
		return mentions[i].name < mentions[j].name
	})
	target := map[string]float64{}
	switch len(mentions) {
	case 0:
		target = map[string]float64{interpret.None: 0.85, interpret.Unclear: 0.15}
	case 1:
		target = map[string]float64{mentions[0].name: 0.88, interpret.None: 0.06, interpret.Unclear: 0.06}
	default:
		chosen := mentions[0].name
		for _, mt := range mentions {
			if first >= 0 && mt.pos > first {
				chosen = mt.name
				break
			}
		}
		target[chosen] = 0.7
		for _, mt := range mentions {
			if mt.name != chosen {
				target[mt.name] += 0.2 / float64(len(mentions)-1)
			}
		}
		target[interpret.Unclear] = 0.1
	}

	// Whom to support: the first fellow general named, by any part of his name.
	whom := map[string]float64{interpret.None: 0.85, interpret.Unclear: 0.15}
	firstName := -1
	for _, name := range friends(req.State) {
		for _, part := range strings.Fields(name) {
			if len(part) < 4 {
				continue
			}
			for _, pos := range find(letter, strings.ToLower(part)) {
				if firstName < 0 || pos < firstName {
					firstName = pos
					whom = map[string]float64{name: 0.88, interpret.None: 0.06, interpret.Unclear: 0.06}
				}
			}
		}
	}

	engagement := 0.5
	for _, w := range hawkish {
		engagement += 0.15 * float64(len(find(letter, w)))
	}
	for _, w := range dovish {
		engagement -= 0.15 * float64(len(find(letter, w)))
	}
	engagement = clamp(engagement)
	plausible := 0.9
	if len(strings.Fields(letter)) < 3 {
		plausible = 0.4
	}
	addressed := 0.3
	if total > 0 || cond != "" {
		addressed = 0.9
	}
	cq := conditional(cond, then, req.State)

	var resp decision.Response
	resp.Provider, resp.Model = "mock", "keywords"
	for _, q := range req.Questions {
		a := decision.Answer{QuestionID: q.ID, Confidence: 1}
		switch q.ID {
		case "plausible":
			a.Score = plausible
		case "addressed":
			a.PYes = addressed
		case "engagement":
			a.Score = engagement
		case "action":
			a.Probs = restrict(action, q.Options)
		case "target":
			a.Probs = restrict(target, q.Options)
		case "support_whom":
			a.Probs = restrict(whom, q.Options)
		case "conditional":
			a.PYes = cq.p
		case "trigger":
			a.Probs = restrict(cq.trigger, q.Options)
		case "trigger_place":
			a.Probs = restrict(cq.place, q.Options)
		case "then_action":
			a.Probs = restrict(cq.then, q.Options)
		case "then_target":
			a.Probs = restrict(cq.target, q.Options)
		default:
			switch q.Kind {
			case decision.KindChoice:
				a.Probs = map[string]float64{}
				for _, o := range q.Options {
					a.Probs[o] = 1 / float64(len(q.Options))
				}
			case decision.KindScore:
				a.Score = 0.5
			case decision.KindBinary:
				a.PYes = 0.5
			}
		}
		resp.Answers = append(resp.Answers, a)
	}
	resp.Raw, _ = json.Marshal(resp.Answers)
	return resp, nil
}

// restrict keeps only the options the question offers.
func restrict(p map[string]float64, options []string) map[string]float64 {
	out := map[string]float64{}
	for _, o := range options {
		if v, ok := p[o]; ok {
			out[o] = v
		}
	}
	if len(out) == 0 && len(options) > 0 {
		out[options[len(options)-1]] = 1
	}
	return out
}

func clamp(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
