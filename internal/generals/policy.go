// Package generals applies personality: the interpretation policy that turns
// decision-model probabilities into an order, and the distortion that turns
// what a general saw into what he reports.
package generals

import (
	"sort"
	"strings"

	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// Thresholds are the interpretation config values.
type Thresholds struct {
	Clear        float64 `yaml:"clear_threshold"`
	Unclear      float64 `yaml:"unclear_threshold"`
	Plausibility float64 `yaml:"plausibility_threshold"`
}

// Outcomes of interpretation.
const (
	OutcomeOrder   = "order"   // the general carries out an order (perhaps of his own devising)
	OutcomeClarify = "clarify" // he holds and writes back asking what was meant
	OutcomeRefuse  = "refuse"  // he quietly refuses a risky order and holds
	OutcomeIgnored = "ignored" // the letter carried no instruction
	OutcomeDoubted = "doubted" // the letter did not read as genuine; set aside
)

// Action options the policy understands.
const (
	ActHold    = "hold"
	ActMove    = "move"
	ActRetreat = "retreat"
	ActUnclear = "unclear"
)

// Draws are the uniform [0,1) numbers the policy may use, all taken from the
// interpretation stream for every letter whichever step decides, so the
// streams stay aligned.
type Draws struct {
	Sample     float64 `json:"sample"`     // step 5: which reading of an ambiguous letter
	Initiative float64 `json:"initiative"` // step 4: own judgement or ask
	Refuse     float64 `json:"refuse"`     // step 8: quiet refusal
}

// Situation is what the policy needs to know about the general's position
// and what he believes about the enemy.
type Situation struct {
	ArmyID   string
	Location string
	Side     model.Side
	Map      *mapdata.Map
	Owner    func(province string) model.Side
	Strength int
	// EnemySeen is the enemy strength the general last saw in a province
	// (0 if none). NearestEnemy is the closest such province, or "".
	EnemySeen    func(province string) int
	NearestEnemy string
}

// Decision is the policy's output plus everything needed to explain it.
type Decision struct {
	Outcome    string             `json:"outcome"`
	Step       string             `json:"step"` // which policy step decided
	Action     string             `json:"action,omitempty"`
	Target     string             `json:"target,omitempty"`
	Order      *model.Order       `json:"order,omitempty"`
	Refused    *model.Order       `json:"refused,omitempty"` // the order he would not carry out
	Weights    map[string]float64 `json:"weights,omitempty"` // per-action multipliers in the ambiguous case
	Reweighted map[string]float64 `json:"reweighted,omitempty"`
	Draws      Draws              `json:"draws"`
}

// Unclear reports whether the general could not make out the letter, whether
// he then asked or acted on his own judgement.
func (d Decision) Unclear() bool {
	return d.Outcome == OutcomeClarify || strings.HasPrefix(d.Step, "own-judgement")
}

// Interpret applies the interpretation policy, steps 1 to 8.
func Interpret(a interpret.Answers, t model.Traits, sit Situation, th Thresholds, actionOrder []string, dr Draws) Decision {
	d := Decision{Draws: dr}

	// Step 1: a letter that does not read as genuine is set aside. The
	// caller lowers loyalty and has the next report say so.
	if a.Plausible < th.Plausibility {
		d.Outcome, d.Step = OutcomeDoubted, "implausible"
		return d
	}
	// Step 2: a letter with no instruction leaves the standing order alone.
	if a.Addressed < 0.5 {
		d.Outcome, d.Step = OutcomeIgnored, "not-addressed"
		return d
	}

	top, pTop := interpret.Top(a.Action, actionOrder)
	switch {
	case top == ActUnclear || pTop < th.Unclear:
		return unclear(d, t, sit, "unclear")
	case pTop >= th.Clear:
		// Step 3: a clear order; personality does not matter.
		d.Action, d.Step = top, "clear"
	default:
		// Step 5: ambiguous. Reweight by temperament and the letter's
		// engagement, drop "unclear", renormalise and sample.
		d.Step = "ambiguous"
		d.Weights = map[string]float64{}
		d.Reweighted = map[string]float64{}
		e := a.Engagement
		sum := 0.0
		for _, act := range actionOrder {
			if act == ActUnclear {
				continue
			}
			w := 1.0
			switch category(act, a, sit) {
			case aggressive:
				w = (0.5 + t.Aggression) * (0.5 + e)
			case defensive:
				w = (0.5 + t.Caution) * (1.5 - e)
			}
			d.Weights[act] = w
			d.Reweighted[act] = a.Action[act] * w
			sum += d.Reweighted[act]
		}
		if sum <= 0 {
			return unclear(d, t, sit, "ambiguous-empty")
		}
		for k := range d.Reweighted {
			d.Reweighted[k] /= sum
		}
		d.Action = sample(d.Reweighted, actionOrder, dr.Sample)
	}

	// Step 7: parameters from the target question.
	order := model.Order{ArmyID: sit.ArmyID, Type: model.Hold}
	switch d.Action {
	case ActHold:
	case ActMove:
		target := topTarget(a, sit.Map)
		if target == interpret.None || target == interpret.Unclear {
			return unclear(d, t, sit, "no-target")
		}
		d.Target = target
		if target != sit.Location {
			order = model.Order{ArmyID: sit.ArmyID, Type: model.MoveToward, Target: target}
		}
	case ActRetreat:
		target := topTarget(a, sit.Map)
		capital := sit.Map.Capital(sit.Side)
		if target == interpret.None || target == interpret.Unclear ||
			sit.Map.Distance(target, capital) >= sit.Map.Distance(sit.Location, capital) {
			// "Fall back!" names no place, or names one that is not
			// homeward (usually the place the letter mentions as the
			// danger): fall back toward home.
			target = capital
		}
		d.Target = target
		if target != sit.Location {
			step := target
			if !sit.Map.Adjacent(sit.Location, target) {
				step = sit.Map.NextStep(sit.Location, target)
			}
			order = model.Order{ArmyID: sit.ArmyID, Type: model.Retreat, Target: step}
		}
	default:
		return unclear(d, t, sit, "unknown-action")
	}

	// Step 8: a disloyal general may quietly refuse to attack a place he
	// believes is stronger than his own army.
	if t.Loyalty < 0.3 && order.Type == model.MoveToward && sit.EnemySeen != nil {
		next := sit.Map.NextStep(sit.Location, order.Target)
		if sit.EnemySeen(next) > sit.Strength && dr.Refuse < (0.3-t.Loyalty)*3 {
			refused := order
			d.Outcome, d.Step = OutcomeRefuse, "refused"
			d.Refused = &refused
			d.Order = &model.Order{ArmyID: sit.ArmyID, Type: model.Hold}
			return d
		}
	}
	d.Outcome = OutcomeOrder
	d.Order = &order
	return d
}

// unclear is step 4: with probability Initiative the general acts on his own
// judgement (step 6); otherwise he holds and asks for clarification.
func unclear(d Decision, t model.Traits, sit Situation, why string) Decision {
	if d.Draws.Initiative < t.Initiative {
		d.Outcome, d.Step = OutcomeOrder, "own-judgement:"+why
		d.Order = OwnJudgement(t, sit)
		return d
	}
	d.Outcome, d.Step = OutcomeClarify, why
	d.Order = &model.Order{ArmyID: sit.ArmyID, Type: model.Hold}
	return d
}

// OwnJudgement is step 6, a tiny per-general heuristic: an aggressive
// general marches on the nearest enemy he has seen; anyone else holds.
// (Cautious generals will entrench once Entrench exists.)
func OwnJudgement(t model.Traits, sit Situation) *model.Order {
	if t.Aggression > t.Caution && t.Aggression >= 0.5 && sit.NearestEnemy != "" {
		return &model.Order{ArmyID: sit.ArmyID, Type: model.MoveToward, Target: sit.NearestEnemy}
	}
	return &model.Order{ArmyID: sit.ArmyID, Type: model.Hold}
}

type cat int

const (
	neutral cat = iota
	aggressive
	defensive
)

// category classifies an action for the ambiguity reweighting. Marching on a
// province the player does not own (or on an unnamed one) is aggressive;
// marching within the player's own lands is neither.
func category(act string, a interpret.Answers, sit Situation) cat {
	switch act {
	case ActHold, ActRetreat, "entrench":
		return defensive
	case "support":
		return aggressive
	case ActMove:
		t := topTarget(a, sit.Map)
		if sit.Map.Province(t) != nil && sit.Owner(t) == sit.Side {
			return neutral
		}
		return aggressive
	}
	return neutral
}

func topTarget(a interpret.Answers, m *mapdata.Map) string {
	order := append(m.IDs(), interpret.None, interpret.Unclear)
	t, _ := interpret.Top(a.Target, order)
	return t
}

// sample picks from a distribution using a uniform draw, walking the keys in
// a fixed order.
func sample(p map[string]float64, order []string, draw float64) string {
	keys := make([]string, 0, len(p))
	seen := map[string]bool{}
	for _, k := range order {
		if _, ok := p[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	var rest []string
	for k := range p {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	keys = append(keys, rest...)
	acc := 0.0
	last := ""
	for _, k := range keys {
		if p[k] <= 0 {
			continue
		}
		acc += p[k]
		last = k
		if draw < acc {
			return k
		}
	}
	return last
}

// Expected returns the probability, before the draw, of each action being
// chosen in the ambiguous branch. Used by the eval harness.
func (d Decision) Expected() map[string]float64 {
	if d.Reweighted != nil {
		return d.Reweighted
	}
	if d.Action != "" {
		return map[string]float64{d.Action: 1}
	}
	return map[string]float64{ActHold: 1}
}
