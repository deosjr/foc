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
	OrderSource  string
	Concerns     []string
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
//
// Honesty-driven omissions come in a later milestone.
func Distort(obs engine.Observation, g *model.General, rc ReportContext) report.ReportFacts {
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
	f.OrderOutcome = outcome(obs, rc.Map)
	for _, b := range obs.Battles {
		f.Battles = append(f.Battles, report.BattleFacts{
			Place:       name(b.Place),
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
	return f
}

func outcome(obs engine.Observation, m *mapdata.Map) string {
	name := m.NameOf
	switch {
	case obs.Disbanded:
		return "the army was destroyed"
	case obs.RetreatedTo != "":
		return fmt.Sprintf("driven out of %s, fell back to %s", name(obs.Start), name(obs.RetreatedTo))
	case obs.Blocked != nil:
		switch obs.Blocked.Reason {
		case "retreat-blocked":
			return fmt.Sprintf("could not fall back to %s: the enemy holds it", name(obs.Blocked.Target))
		case "standoff":
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
	}
	return "held " + name(obs.Location)
}
