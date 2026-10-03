// Package interpret turns a dispatch into a decision-model request and the
// response into validated probabilities.
package interpret

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"text/template"

	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/providers/decision"
	"gopkg.in/yaml.v3"
)

// Special target answers besides province names.
const (
	None    = "none"
	Unclear = "unclear"
)

// Context is everything the state string may contain. It never holds traits,
// bios or true enemy positions.
type Context struct {
	General    *model.General
	Location   string            // province id
	Friendly   map[string]string // friendly general name -> province id
	Standing   model.Order
	SentTurn   int
	ArriveTurn int
	Letter     string
}

// FriendlyNames returns the friendly generals in sorted order.
func (c Context) friendlyNames() []string {
	names := make([]string, 0, len(c.Friendly))
	for n := range c.Friendly {
		names = append(names, n)
	}
	sortStrings(names)
	return names
}

// BuildState renders the plain labelled context for the decision model.
func BuildState(c Context, m *mapdata.Map) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A letter from the sovereign, who writes from %s, to one of the sovereign's field generals.\n\n", m.NameOf(m.Capital(model.Player)))
	fmt.Fprintf(&b, "GENERAL: %s\n", c.General.Name)
	fmt.Fprintf(&b, "CURRENT PROVINCE: %s\n", m.NameOf(c.Location))
	var adj []string
	for _, n := range m.Neighbours(c.Location) {
		adj = append(adj, m.NameOf(n))
	}
	fmt.Fprintf(&b, "NEIGHBOURING PROVINCES: %s\n", strings.Join(adj, ", "))
	var all []string
	for _, p := range m.Provinces {
		s := p.Name
		if len(p.Aliases) > 0 {
			s += " (also: " + strings.Join(p.Aliases, ", ") + ")"
		}
		all = append(all, s)
	}
	fmt.Fprintf(&b, "ALL PROVINCES: %s\n", strings.Join(all, "; "))
	var fr []string
	for _, n := range c.friendlyNames() {
		fr = append(fr, fmt.Sprintf("%s in %s", n, m.NameOf(c.Friendly[n])))
	}
	if len(fr) == 0 {
		fr = []string{"none"}
	}
	fmt.Fprintf(&b, "FRIENDLY GENERALS: %s\n", strings.Join(fr, "; "))
	fmt.Fprintf(&b, "STANDING ORDER: %s\n", DescribeOrder(c.Standing, m))
	fmt.Fprintf(&b, "LETTER SENT: turn %d\nLETTER ARRIVED: turn %d\n\n", c.SentTurn, c.ArriveTurn)
	fmt.Fprintf(&b, "LETTER:\n<<<\n%s\n>>>\n", strings.TrimSpace(c.Letter))
	return b.String()
}

// LetterFromState extracts the letter text from a state string.
func LetterFromState(state string) string {
	i := strings.Index(state, "LETTER:\n<<<\n")
	j := strings.LastIndex(state, "\n>>>")
	if i < 0 || j < i {
		return ""
	}
	return state[i+len("LETTER:\n<<<\n") : j]
}

// DescribeOrder renders an order in plain words with display names.
func DescribeOrder(o model.Order, m *mapdata.Map) string {
	switch o.Type {
	case model.MoveToward:
		return "march toward " + m.NameOf(o.Target)
	case model.Retreat:
		return "fall back to " + m.NameOf(o.Target)
	case "":
		return "none"
	}
	return "hold position"
}

// QuestionSpec is one entry of questions.yaml.
type QuestionSpec struct {
	ID          string        `yaml:"id"`
	Kind        decision.Kind `yaml:"kind"`
	Prompt      string        `yaml:"prompt"`
	Options     []string      `yaml:"options"`
	OptionsFrom string        `yaml:"options_from"` // "provinces"
	tmpl        *template.Template
}

// QuestionSet is the loaded question templates.
type QuestionSet struct {
	Questions []*QuestionSpec `yaml:"questions"`
}

// Required question ids the policy relies on.
var required = []string{"plausible", "addressed", "action", "target", "engagement"}

// LoadQuestions reads and checks questions.yaml.
func LoadQuestions(path string) (*QuestionSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var qs QuestionSet
	if err := yaml.Unmarshal(data, &qs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	have := map[string]bool{}
	for _, q := range qs.Questions {
		t, err := template.New(q.ID).Option("missingkey=error").Parse(q.Prompt)
		if err != nil {
			return nil, fmt.Errorf("%s: question %s: %w", path, q.ID, err)
		}
		q.tmpl = t
		have[q.ID] = true
		if q.Kind == decision.KindChoice && len(q.Options) == 0 && q.OptionsFrom != "provinces" {
			return nil, fmt.Errorf("%s: choice question %s has no options", path, q.ID)
		}
	}
	for _, id := range required {
		if !have[id] {
			return nil, fmt.Errorf("%s: missing required question %q", path, id)
		}
	}
	return &qs, nil
}

// ActionOptions returns the options of the action question.
func (qs *QuestionSet) ActionOptions() []string {
	for _, q := range qs.Questions {
		if q.ID == "action" {
			return q.Options
		}
	}
	return nil
}

// Build renders the questions for one general and location.
func (qs *QuestionSet) Build(general, location string, m *mapdata.Map) ([]decision.Question, error) {
	data := map[string]string{"General": general, "Location": location}
	var out []decision.Question
	for _, q := range qs.Questions {
		var buf bytes.Buffer
		if err := q.tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("question %s: %w", q.ID, err)
		}
		dq := decision.Question{ID: q.ID, Kind: q.Kind, Prompt: strings.TrimSpace(buf.String())}
		if q.Kind == decision.KindChoice {
			dq.Options = append([]string(nil), q.Options...)
			if q.OptionsFrom == "provinces" {
				for _, p := range m.Provinces {
					dq.Options = append(dq.Options, p.Name)
				}
				dq.Options = append(dq.Options, None, Unclear)
			}
		}
		out = append(out, dq)
	}
	return out, nil
}

// Answers are the validated, normalised answers the policy uses.
type Answers struct {
	Plausible  float64            `json:"plausible"`
	Addressed  float64            `json:"addressed"`
	Engagement float64            `json:"engagement"`
	Action     map[string]float64 `json:"action"`
	Target     map[string]float64 `json:"target"` // province id, "none" or "unclear"
}

// Parse validates a response against the questions. A missing answer is an
// error, never a default. Choice probabilities are normalised; province names
// are mapped back to ids.
func Parse(resp decision.Response, qs []decision.Question, m *mapdata.Map) (Answers, error) {
	byID := map[string]decision.Answer{}
	for _, a := range resp.Answers {
		byID[a.QuestionID] = a
	}
	var out Answers
	for _, q := range qs {
		a, ok := byID[q.ID]
		if !ok {
			return out, fmt.Errorf("decision response has no answer to %q", q.ID)
		}
		switch q.Kind {
		case decision.KindChoice:
			probs, err := Normalise(a.Probs, q.Options)
			if err != nil {
				return out, fmt.Errorf("question %q: %w", q.ID, err)
			}
			switch q.ID {
			case "action":
				out.Action = probs
			case "target":
				out.Target = map[string]float64{}
				for name, p := range probs {
					out.Target[provinceID(name, m)] += p
				}
			}
		case decision.KindScore:
			if math.IsNaN(a.Score) {
				return out, fmt.Errorf("question %q: score is NaN", q.ID)
			}
			v := clamp01(a.Score)
			switch q.ID {
			case "plausible":
				out.Plausible = v
			case "engagement":
				out.Engagement = v
			}
		case decision.KindBinary:
			if q.ID == "addressed" {
				out.Addressed = clamp01(a.PYes)
			}
		}
	}
	return out, nil
}

// Normalise checks choice probabilities against the allowed options and
// rescales them to sum to 1. Options with no probability get 0.
func Normalise(probs map[string]float64, options []string) (map[string]float64, error) {
	allowed := map[string]bool{}
	for _, o := range options {
		allowed[o] = true
	}
	for k, v := range probs {
		if !allowed[k] {
			return nil, fmt.Errorf("unknown option %q", k)
		}
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("bad probability %v for %q", v, k)
		}
	}
	// Sum in option order: float addition is not associative, and map order
	// would make the result differ between runs.
	sum := 0.0
	for _, o := range options {
		sum += probs[o]
	}
	if sum <= 0 {
		return nil, fmt.Errorf("probabilities sum to zero")
	}
	out := make(map[string]float64, len(options))
	for _, o := range options {
		out[o] = probs[o] / sum
	}
	return out, nil
}

func provinceID(name string, m *mapdata.Map) string {
	for _, p := range m.Provinces {
		if p.Name == name {
			return p.ID
		}
	}
	return name // "none" / "unclear"
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// Ask sends the question set for one dispatch and validates the answers.
func Ask(ctx context.Context, dm decision.Model, qs *QuestionSet, c Context, m *mapdata.Map) (Answers, decision.Response, error) {
	questions, err := qs.Build(c.General.Name, m.NameOf(c.Location), m)
	if err != nil {
		return Answers{}, decision.Response{}, err
	}
	req := decision.Request{State: BuildState(c, m), Questions: questions}
	resp, err := dm.Decide(ctx, req)
	if err != nil {
		return Answers{}, resp, err
	}
	ans, err := Parse(resp, questions, m)
	return ans, resp, err
}

// Top returns the highest-probability key, ties broken by the given order.
func Top(probs map[string]float64, order []string) (string, float64) {
	best, bp := "", -1.0
	for _, k := range order {
		if v, ok := probs[k]; ok && v > bp+1e-12 {
			best, bp = k, v
		}
	}
	return best, bp
}

func sortStrings(s []string) { sort.Strings(s) }

// Interpretation is the full record of how one dispatch was read. It is
// hidden from the player until the after-action review (or --debug).
type Interpretation struct {
	LetterID   string             `json:"letter"`
	GeneralID  string             `json:"general"`
	Turn       int                `json:"turn"`
	State      string             `json:"-"`
	Answers    decision.Response  `json:"answers"`
	Parsed     Answers            `json:"parsed"`
	Weights    map[string]float64 `json:"weights,omitempty"`
	Reweighted map[string]float64 `json:"reweighted,omitempty"`
	RNGDraw    float64            `json:"rng_draw"`
	Outcome    string             `json:"outcome"` // order | unclear | ignored | error
	Step       string             `json:"step"`
	Order      *model.Order       `json:"order,omitempty"`
	Rationale  string             `json:"rationale,omitempty"`
	Error      string             `json:"error,omitempty"`
}
