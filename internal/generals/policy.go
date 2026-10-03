// Package generals applies personality: the interpretation policy that turns
// decision-model probabilities into an order, and the distortion that turns
// what a general saw into what he reports.
package generals

import (
	"sort"

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
	OutcomeOrder   = "order"
	OutcomeUnclear = "unclear" // PoC: the general holds; clarification letters come later
	OutcomeIgnored = "ignored" // the letter carried no instruction
)

// Action options the policy understands.
const (
	ActHold    = "hold"
	ActMove    = "move"
	ActRetreat = "retreat"
	ActUnclear = "unclear"
)

// Situation is what the policy needs to know about the general's position.
type Situation struct {
	ArmyID   string
	Location string
	Side     model.Side
	Map      *mapdata.Map
	Owner    func(province string) model.Side
}

// Decision is the policy's output plus everything needed to explain it.
type Decision struct {
	Outcome    string             `json:"outcome"`
	Step       string             `json:"step"` // which policy step decided
	Action     string             `json:"action,omitempty"`
	Target     string             `json:"target,omitempty"`
	Order      *model.Order       `json:"order,omitempty"`
	Weights    map[string]float64 `json:"weights,omitempty"` // per-action multipliers in the ambiguous case
	Reweighted map[string]float64 `json:"reweighted,omitempty"`
	Draw       float64            `json:"draw"`
}

// Interpret applies the PoC interpretation policy (steps 2, 3, 4, 5 and 7).
// draw is a uniform [0,1) number from the interpretation RNG stream; the
// caller always draws one per letter so streams stay aligned whichever step
// decides.
func Interpret(a interpret.Answers, t model.Traits, sit Situation, th Thresholds, actionOrder []string, draw float64) Decision {
	d := Decision{Draw: draw}

	// Step 2: a letter with no instruction leaves the standing order alone.
	if a.Addressed < 0.5 {
		d.Outcome, d.Step = OutcomeIgnored, "not-addressed"
		return d
	}

	top, pTop := interpret.Top(a.Action, actionOrder)
	switch {
	case top == ActUnclear || pTop < th.Unclear:
		// Step 4, PoC: an unclear letter means hold.
		return unclear(d, sit, "unclear")
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
			return unclear(d, sit, "ambiguous-empty")
		}
		for k := range d.Reweighted {
			d.Reweighted[k] /= sum
		}
		d.Action = sample(d.Reweighted, actionOrder, draw)
	}

	// Step 7: parameters from the target question.
	order := model.Order{ArmyID: sit.ArmyID, Type: model.Hold}
	switch d.Action {
	case ActHold:
	case ActMove:
		target := topTarget(a, sit.Map)
		if target == interpret.None || target == interpret.Unclear {
			return unclear(d, sit, "no-target")
		}
		d.Target = target
		if target != sit.Location {
			order = model.Order{ArmyID: sit.ArmyID, Type: model.MoveToward, Target: target}
		}
	case ActRetreat:
		target := topTarget(a, sit.Map)
		if target == interpret.None || target == interpret.Unclear {
			// "Fall back!" names no place: fall back toward home.
			target = sit.Map.Capital(sit.Side)
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
		return unclear(d, sit, "unknown-action")
	}
	d.Outcome = OutcomeOrder
	d.Order = &order
	return d
}

func unclear(d Decision, sit Situation, step string) Decision {
	d.Outcome, d.Step = OutcomeUnclear, step
	d.Order = &model.Order{ArmyID: sit.ArmyID, Type: model.Hold}
	return d
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
