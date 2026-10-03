package generals

import (
	"math"
	"testing"

	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

var (
	velk  = model.Traits{Aggression: 0.85, Caution: 0.15, Initiative: 0.6, Honesty: 0.5, Vanity: 0.8, Loyalty: 0.7}
	saris = model.Traits{Aggression: 0.2, Caution: 0.85, Initiative: 0.25, Honesty: 0.9, Vanity: 0.2, Loyalty: 0.9}
	th    = Thresholds{Clear: 0.8, Unclear: 0.4, Plausibility: 0.35}
	poc   = []string{"hold", "move", "retreat", "unclear"}
)

func loadMap(t *testing.T) *mapdata.Map {
	t.Helper()
	m, err := mapdata.LoadMap("../../maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func sit(m *mapdata.Map, loc string) Situation {
	owners := map[string]model.Side{}
	for _, p := range m.Provinces {
		owners[p.ID] = p.Owner
	}
	return Situation{ArmyID: "a", Location: loc, Side: model.Player, Map: m, Owner: func(p string) model.Side { return owners[p] }}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// The worked example from the spec, with the full action set.
func TestSpecWorkedExample(t *testing.T) {
	m := loadMap(t)
	full := []string{"hold", "move", "support", "entrench", "scout", "retreat", "unclear"}
	a := interpret.Answers{
		Plausible: 0.93, Addressed: 0.95, Engagement: 0.55,
		Action: map[string]float64{"hold": 0.30, "move": 0.38, "entrench": 0.18, "support": 0.02, "scout": 0.07, "retreat": 0.0, "unclear": 0.05},
		Target: map[string]float64{"oros": 0.86, "velia": 0.09, "unclear": 0.05},
	}
	// The example's top action is 0.38, below the configured unclear
	// threshold of 0.4, so under the spec's own step 4 it would read as
	// unclear. Use a lower threshold here to check the reweighting maths.
	th := th
	th.Unclear = 0.35
	ds := Interpret(a, saris, sit(m, "velia"), th, full, 0.5)
	if ds.Step != "ambiguous" {
		t.Fatalf("step = %s", ds.Step)
	}
	if hold := ds.Reweighted["hold"] + ds.Reweighted["entrench"]; !near(hold, 0.63) {
		t.Errorf("Saris holds or entrenches with p=%.3f, spec says ~0.63", hold)
	}
	if !near(ds.Reweighted["move"], 0.285) {
		t.Errorf("Saris marches with p=%.3f, spec says ~0.28", ds.Reweighted["move"])
	}
	dv := Interpret(a, velk, sit(m, "velia"), th, full, 0.5)
	if !near(dv.Reweighted["move"], 0.577) {
		t.Errorf("Velk marches with p=%.3f, spec says ~0.58", dv.Reweighted["move"])
	}
}

func TestInterpretSteps(t *testing.T) {
	m := loadMap(t)
	base := func(action map[string]float64, target map[string]float64) interpret.Answers {
		return interpret.Answers{Plausible: 0.9, Addressed: 0.9, Engagement: 0.5, Action: action, Target: target}
	}
	cases := []struct {
		name    string
		a       interpret.Answers
		traits  model.Traits
		loc     string
		draw    float64
		outcome string
		step    string
		order   model.Order
	}{
		{
			name:   "not addressed keeps standing order",
			a:      interpret.Answers{Addressed: 0.2, Action: map[string]float64{"hold": 1}},
			traits: velk, loc: "velia", outcome: OutcomeIgnored, step: "not-addressed",
		},
		{
			name:   "clear move",
			a:      base(map[string]float64{"move": 0.9, "hold": 0.1}, map[string]float64{"marren": 0.9, "none": 0.1}),
			traits: saris, loc: "velia", outcome: OutcomeOrder, step: "clear",
			order: model.Order{ArmyID: "a", Type: model.MoveToward, Target: "marren"},
		},
		{
			name:   "clear hold ignores temperament",
			a:      base(map[string]float64{"hold": 0.85, "move": 0.15}, map[string]float64{"none": 1}),
			traits: velk, loc: "velia", outcome: OutcomeOrder, step: "clear",
			order: model.Order{ArmyID: "a", Type: model.Hold},
		},
		{
			name:   "unclear top answer holds",
			a:      base(map[string]float64{"unclear": 0.6, "hold": 0.4}, map[string]float64{"none": 1}),
			traits: velk, loc: "velia", outcome: OutcomeUnclear, step: "unclear",
			order: model.Order{ArmyID: "a", Type: model.Hold},
		},
		{
			name:   "flat distribution is unclear",
			a:      base(map[string]float64{"hold": 0.35, "move": 0.35, "retreat": 0.3}, map[string]float64{"oros": 1}),
			traits: velk, loc: "velia", outcome: OutcomeUnclear, step: "unclear",
			order: model.Order{ArmyID: "a", Type: model.Hold},
		},
		{
			name:   "move without a target falls back to unclear",
			a:      base(map[string]float64{"move": 0.95, "hold": 0.05}, map[string]float64{"unclear": 0.7, "oros": 0.3}),
			traits: velk, loc: "velia", outcome: OutcomeUnclear, step: "no-target",
			order: model.Order{ArmyID: "a", Type: model.Hold},
		},
		{
			name:   "ambiguous: low draw picks the first weighted action",
			a:      base(map[string]float64{"hold": 0.5, "move": 0.5}, map[string]float64{"oros": 1}),
			traits: saris, loc: "velia", draw: 0.1, outcome: OutcomeOrder, step: "ambiguous",
			order: model.Order{ArmyID: "a", Type: model.Hold},
		},
		{
			name:   "ambiguous: high draw picks the move",
			a:      base(map[string]float64{"hold": 0.5, "move": 0.5}, map[string]float64{"oros": 1}),
			traits: saris, loc: "velia", draw: 0.95, outcome: OutcomeOrder, step: "ambiguous",
			order: model.Order{ArmyID: "a", Type: model.MoveToward, Target: "oros"},
		},
		{
			name:   "retreat with no place falls back toward the capital",
			a:      base(map[string]float64{"retreat": 0.9, "hold": 0.1}, map[string]float64{"none": 1}),
			traits: saris, loc: "hollow", outcome: OutcomeOrder, step: "clear",
			order: model.Order{ArmyID: "a", Type: model.Retreat, Target: "duna"},
		},
		{
			name:   "retreat to a distant place takes the first step",
			a:      base(map[string]float64{"retreat": 0.9, "hold": 0.1}, map[string]float64{"karsa": 1}),
			traits: saris, loc: "marren", outcome: OutcomeOrder, step: "clear",
			order: model.Order{ArmyID: "a", Type: model.Retreat, Target: "hollow"},
		},
		{
			name:   "retreat toward a place that is not homeward falls back toward the capital",
			a:      base(map[string]float64{"retreat": 0.9, "hold": 0.1}, map[string]float64{"hollow": 1}),
			traits: saris, loc: "duna", outcome: OutcomeOrder, step: "clear",
			order: model.Order{ArmyID: "a", Type: model.Retreat, Target: "karsa"},
		},
		{
			name:   "move to own province is a hold",
			a:      base(map[string]float64{"move": 0.9, "hold": 0.1}, map[string]float64{"velia": 1}),
			traits: saris, loc: "velia", outcome: OutcomeOrder, step: "clear",
			order: model.Order{ArmyID: "a", Type: model.Hold},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Interpret(c.a, c.traits, sit(m, c.loc), th, poc, c.draw)
			if d.Outcome != c.outcome || d.Step != c.step {
				t.Fatalf("got %s/%s, want %s/%s", d.Outcome, d.Step, c.outcome, c.step)
			}
			if c.outcome == OutcomeIgnored {
				if d.Order != nil {
					t.Errorf("ignored letter produced order %v", d.Order)
				}
				return
			}
			if d.Order == nil || *d.Order != c.order {
				t.Errorf("order = %v, want %v", d.Order, c.order)
			}
		})
	}
}

// Moving within the player's own lands is not an aggressive act.
func TestMoveHomeIsNeutral(t *testing.T) {
	m := loadMap(t)
	a := interpret.Answers{Addressed: 1, Engagement: 0.5,
		Action: map[string]float64{"hold": 0.5, "move": 0.5}, Target: map[string]float64{"karsa": 1}}
	d := Interpret(a, velk, sit(m, "velia"), th, poc, 0)
	if d.Weights["move"] != 1 {
		t.Errorf("move home weighted %v, want 1", d.Weights["move"])
	}
}

func TestDistort(t *testing.T) {
	m := loadMap(t)
	r, err := mapdata.LoadRuleset("../../rulesets/ancient.yaml")
	if err != nil {
		t.Fatal(err)
	}
	obs := engine.Observation{
		Turn: 4, Start: "velia", Location: "oros", Strength: 26, Losses: 4,
		Order:     model.Order{ArmyID: "a", Type: model.MoveToward, Target: "oros"},
		Battles:   []engine.ObservedBattle{{Place: "oros", Result: "won", EnemyLosses: 10}},
		Sightings: []engine.Sighting{{Province: "marren", Strength: 20}},
		Quiet:     []string{"velia"},
		Friendly:  []engine.Contact{{GeneralID: "saris", Province: "velia"}},
	}
	rc := ReportContext{Map: m, Rules: r, GeneralNames: map[string]string{"saris": "Ione Saris"}, OrderSource: "letter sent turn 4"}
	fv := Distort(obs, &model.General{Name: "Damar Velk", Traits: velk}, rc)
	fs := Distort(obs, &model.General{Name: "Ione Saris", Traits: saris}, rc)
	// Velk: own losses 4*(1-0.48)=2.08 -> 2; enemy losses 10*1.48 -> 15; sighting 20*0.72=14.4 -> 15.
	if fv.OwnLosses != 2 || fv.Battles[0].EnemyLosses != 15 || fv.Sightings[0].EnemyStrength != 15 {
		t.Errorf("velk facts: losses %d, enemy losses %d, sighting %d", fv.OwnLosses, fv.Battles[0].EnemyLosses, fv.Sightings[0].EnemyStrength)
	}
	// Saris: own losses 4*0.88=3.52 -> 4; enemy losses 10*1.12 -> 11; sighting 20*1.28=25.6 -> 25.
	if fs.OwnLosses != 4 || fs.Battles[0].EnemyLosses != 11 || fs.Sightings[0].EnemyStrength != 25 {
		t.Errorf("saris facts: losses %d, enemy losses %d, sighting %d", fs.OwnLosses, fs.Battles[0].EnemyLosses, fs.Sightings[0].EnemyStrength)
	}
	if fv.Location != "Oros Ford" || fv.MovedFrom != "Velia" || fv.OrderOutcome != "marched from Velia and reached Oros Ford" {
		t.Errorf("velk text facts: %+v", fv)
	}
	if fv.FriendlyContacts[0].General != "Ione Saris" {
		t.Errorf("contacts: %+v", fv.FriendlyContacts)
	}
}
