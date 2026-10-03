package generals

import (
	"fmt"
	"math"

	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/report"
)

// ReportContext is what distortion needs besides the observation.
type ReportContext struct {
	Map          *mapdata.Map
	Rules        *mapdata.Ruleset
	GeneralNames map[string]string // id -> display name
	ArmyNames    map[string]string // army id -> its general's display name
	OrderSource  string
	Concerns     []string
	Refused      *model.Order         // an order he quietly refused this turn
	Clarify      *report.ClarifyFacts // what he could not make out, if he is asking
	// Draw returns a uniform [0,1) number from the reporting stream, for
	// omissions. Nil means nothing is ever omitted.
	Draw func() float64
}

func roundHalfUp(x float64) int { return int(math.Floor(x + 0.5)) }

func roundTo(x float64, step int) int {
	if step <= 1 {
		return roundHalfUp(x)
	}
	return roundHalfUp(x/float64(step)) * step
}

// Distort turns an observation into the facts a general will report,
// applying his biases deterministically:
//   - reported enemy strength = observed × (base + scale × Caution), to the nearest step
//   - reported own losses = true × (1 − vanity_own × Vanity)
//   - reported enemy losses = observed × (1 + vanity_enemy × Vanity)
//   - each negative fact (a lost battle, being driven out, a refused order)
//     is left out with probability omission × (1 − Honesty)
//
// It returns the facts and a description of every fact left out.
func Distort(obs engine.Observation, g *model.General, rc ReportContext) (report.ReportFacts, []string) {
	name := rc.Map.NameOf
	d := rc.Rules.Distortion
	tr := g.Traits
	f := report.ReportFacts{
		General:          g.Name,
		WrittenTurn:      obs.Turn,
		Location:         name(obs.Location),
		OrderUnderstood:  interpret.DescribeOrder(obs.Order, rc.Map),
		OrderSource:      rc.OrderSource,
		OwnStrength:      obs.Strength,
		OwnLosses:        roundHalfUp(float64(obs.Losses) * (1 - d.VanityOwn*tr.Vanity)),
		ArmyDestroyed:    obs.Disbanded,
		Sightings:        []report.SightingFacts{},
		NoEnemySeenIn:    []string{},
		FriendlyContacts: []report.ContactFacts{},
		Concerns:         append([]string{}, rc.Concerns...),
	}
	if obs.Order.Type == model.Retreat {
		// The retreat order names the step actually taken.
		f.OrderUnderstood = "fall back to " + name(obs.Order.Target)
	}
	if obs.Start != obs.Location {
		f.MovedFrom = name(obs.Start)
	}
	if obs.RetreatedTo != "" {
		f.RetreatedTo = name(obs.RetreatedTo)
	}
	f.OrderOutcome = outcome(obs, rc)
	if obs.Entrenched > 0 {
		f.EntrenchedTurns = obs.Entrenched
	}
	if obs.Scouted != "" {
		f.Scouted = name(obs.Scouted)
	}
	if sp := obs.Support; sp != nil {
		result := map[string]string{"given": "given", "cut": "cut by an enemy attack",
			"too-far": "too far away", "same-province": "given"}[sp.Status]
		f.Support = &report.SupportFacts{General: rc.ArmyNames[sp.Supported], At: name(sp.Into), Result: result}
	}
	for _, b := range obs.Battles {
		place := name(b.Place)
		if b.Road != "" {
			place = fmt.Sprintf("the road between %s and %s", name(b.Place), name(b.Road))
		}
		f.Battles = append(f.Battles, report.BattleFacts{
			Place:       place,
			Result:      b.Result,
			EnemyLosses: roundHalfUp(float64(b.EnemyLosses) * (1 + d.VanityEnemy*tr.Vanity)),
		})
	}
	step := rc.Rules.Perception.StrengthRounding
	for _, s := range obs.Sightings {
		str := roundTo(float64(s.Strength)*(d.CautionBase+d.CautionScale*tr.Caution), step)
		if str < step {
			str = step
		}
		certainty := "estimate"
		if s.Exact {
			certainty = "certain"
		}
		f.Sightings = append(f.Sightings, report.SightingFacts{Province: name(s.Province), EnemyStrength: str, Certainty: certainty})
	}
	for _, q := range obs.Quiet {
		f.NoEnemySeenIn = append(f.NoEnemySeenIn, name(q))
	}
	for _, c := range obs.Friendly {
		f.FriendlyContacts = append(f.FriendlyContacts, report.ContactFacts{General: rc.GeneralNames[c.GeneralID], Province: name(c.Province)})
	}
	if rc.Refused != nil {
		f.Refused, f.RefusedOrder = true, interpret.DescribeOrder(*rc.Refused, rc.Map)
	}
	f.Clarification = rc.Clarify
	return f, omit(&f, obs, tr, rc)
}

// omit leaves out bad news, drawing in a fixed order: lost battles, being
// driven out, a refused order. Draws are taken only for facts that exist.
func omit(f *report.ReportFacts, obs engine.Observation, tr model.Traits, rc ReportContext) []string {
	if rc.Draw == nil || obs.Disbanded {
		return nil // a destroyed army cannot be hidden
	}
	p := rc.Rules.Distortion.Omission * (1 - tr.Honesty)
	var omitted []string
	var kept []report.BattleFacts
	hidLoss := false
	for _, b := range f.Battles {
		if b.Result == "lost" && rc.Draw() < p {
			omitted = append(omitted, "the lost battle at "+b.Place)
			hidLoss = true
			continue
		}
		kept = append(kept, b)
	}
	f.Battles = kept
	if hidLoss {
		// Hiding a defeat means not counting its dead either.
		f.OwnLosses = 0
		if obs.Blocked != nil && obs.Blocked.Reason == "lost" {
			f.OrderOutcome = "held " + f.Location
		}
	}
	if f.RetreatedTo != "" && rc.Draw() < p {
		omitted = append(omitted, "being driven out of "+rc.Map.NameOf(obs.Start))
		f.OrderOutcome = fmt.Sprintf("moved from %s to %s", rc.Map.NameOf(obs.Start), f.RetreatedTo)
		f.RetreatedTo = ""
	}
	if f.Refused && rc.Draw() < p {
		omitted = append(omitted, "refusing the order to "+f.RefusedOrder)
		f.Refused, f.RefusedOrder = false, ""
	}
	if f.Support != nil && f.Support.Result != "given" && rc.Draw() < p {
		omitted = append(omitted, "failing to support "+f.Support.General)
		f.Support = nil
		f.OrderOutcome = "held " + f.Location
	}
	return omitted
}

func outcome(obs engine.Observation, rc ReportContext) string {
	name := rc.Map.NameOf
	field := false
	for _, b := range obs.Battles {
		field = field || b.Road != ""
	}
	switch {
	case obs.Disbanded:
		return "the army was destroyed"
	case obs.RetreatedTo != "":
		return fmt.Sprintf("driven out of %s, fell back to %s", name(obs.Start), name(obs.RetreatedTo))
	case obs.Blocked != nil:
		switch {
		case obs.Blocked.Reason == "retreat-blocked":
			return fmt.Sprintf("could not fall back to %s: the enemy holds it", name(obs.Blocked.Target))
		case obs.Blocked.Reason == "field":
			return fmt.Sprintf("met the enemy on the road to %s and drove them back; still in %s", name(obs.Blocked.Target), name(obs.Location))
		case field && obs.Blocked.Reason == "lost":
			return fmt.Sprintf("met the enemy on the road to %s and was driven back; still in %s", name(obs.Blocked.Target), name(obs.Location))
		case obs.Blocked.Reason == "standoff":
			return fmt.Sprintf("the advance into %s was halted in a stand-off; still in %s", name(obs.Blocked.Target), name(obs.Location))
		default:
			return fmt.Sprintf("the advance into %s was thrown back; still in %s", name(obs.Blocked.Target), name(obs.Location))
		}
	case obs.Start != obs.Location && obs.Order.Type == model.Retreat:
		return fmt.Sprintf("fell back from %s to %s", name(obs.Start), name(obs.Location))
	case obs.Start != obs.Location && obs.Order.Type == model.MoveToward && obs.Location == obs.Order.Target:
		return fmt.Sprintf("marched from %s and reached %s", name(obs.Start), name(obs.Location))
	case obs.Start != obs.Location:
		return fmt.Sprintf("marched from %s to %s, on the way to %s", name(obs.Start), name(obs.Location), name(obs.Order.Target))
	case obs.Order.Type == model.Entrench && obs.Entrenched >= 2:
		return "held " + name(obs.Location) + ", dug in"
	case obs.Order.Type == model.Entrench:
		return "began to dig in at " + name(obs.Location)
	case obs.Order.Type == model.Scout:
		return fmt.Sprintf("held %s and sent riders into %s", name(obs.Location), name(obs.Scouted))
	case obs.Support != nil:
		who := rc.ArmyNames[obs.Support.Supported]
		switch obs.Support.Status {
		case "cut":
			return fmt.Sprintf("held %s; could not support %s, for the enemy came at us", name(obs.Location), who)
		case "too-far":
			return fmt.Sprintf("held %s; %s was too far away to support %s", name(obs.Location), name(obs.Support.Into), who)
		}
		return fmt.Sprintf("held %s and lent strength to %s at %s", name(obs.Location), who, name(obs.Support.Into))
	}
	return "held " + name(obs.Location)
}
