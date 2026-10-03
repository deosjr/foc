package report

import (
	"reflect"
	"strings"
	"testing"

	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

func validator(t *testing.T) Validator {
	t.Helper()
	m, err := mapdata.LoadMap("../../maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	return Validator{Map: m, MenPerStrength: 100, MinWords: 30, MaxWords: 260,
		GeneralNames: []string{"Damar Velk", "Ione Saris", "Hesk the Elder"}}
}

// The example facts and letter from the spec.
var specFacts = ReportFacts{
	General: "Damar Velk", WrittenTurn: 6, Location: "Oros Ford",
	OrderUnderstood: "march toward Oros Ford", OrderSource: "letter sent turn 5",
	OwnStrength: 26, OwnLosses: 2,
	Battles:          []BattleFacts{{Place: "Oros Ford", Result: "won", EnemyLosses: 12}},
	Sightings:        []SightingFacts{{Province: "Marren", EnemyStrength: 15, Certainty: "estimate"}},
	FriendlyContacts: []ContactFacts{{General: "Ione Saris", Province: "Velia"}},
}

const specLetter = `To my lord in Karsa. As you commanded, I took the ford. They stood against us, and we broke them; twelve hundred of theirs lie in the shallows, and I lost scarcely two hundred. I hold Oros now with 2,600 good men. Riders say perhaps fifteen hundred wait in Marren, though I put little stock in it. Lady Saris keeps to Velia. Send word and I will push on. — Damar Velk`

func TestNumbersIn(t *testing.T) {
	cases := map[string][]int{
		"twelve hundred of theirs":             {1200},
		"2,600 good men and 3 riders":          {2600, 3},
		"two thousand six hundred":             {2600},
		"twenty-six hundred":                   {2600},
		"a hundred and fifty":                  {150},
		"some 12 hundred":                      {1200},
		"no numbers here, only hundreds of men": nil,
	}
	for text, want := range cases {
		if got := NumbersIn(text); !reflect.DeepEqual(got, want) {
			t.Errorf("NumbersIn(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	v := validator(t)
	if got := v.Validate(specLetter, specFacts); len(got) != 0 {
		t.Errorf("spec example letter rejected: %v", got)
	}
	invented := strings.Replace(specLetter, "twelve hundred", "three thousand", 1)
	if got := v.Validate(invented, specFacts); len(got) != 1 || !strings.Contains(got[0], "3000") {
		t.Errorf("invented number: %v", got)
	}
	place := strings.Replace(specLetter, "wait in Marren", "wait in Sarnos and Marren", 1)
	if got := v.Validate(place, specFacts); len(got) != 1 || !strings.Contains(got[0], "Sarnos") {
		t.Errorf("invented province: %v", got)
	}
	person := strings.Replace(specLetter, "Send word", "Hesk sends greetings. Send word", 1)
	if got := v.Validate(person, specFacts); len(got) != 1 || !strings.Contains(got[0], "Hesk") {
		t.Errorf("invented general: %v", got)
	}
}

func TestFallbackPassesValidation(t *testing.T) {
	v := validator(t)
	f := specFacts
	f.NoEnemySeenIn = []string{"Velia"}
	f.Concerns = []string{"I could not make out your letter sent in turn 14."}
	text := Fallback(f)
	if got := v.Validate(text, f); len(got) != 0 {
		t.Errorf("fallback letter rejected: %v\n%s", got, text)
	}
	for _, want := range []string{"2,600 men", "1,200 men", "Marren", "Ione Saris is in Velia", "Damar Velk"} {
		if !strings.Contains(text, want) {
			t.Errorf("fallback letter missing %q:\n%s", want, text)
		}
	}
}

func TestBelief(t *testing.T) {
	m, _ := mapdata.LoadMap("../../maps/valley.json")
	scn, err := mapdata.LoadScenario("../../scenarios/poc.yaml", m)
	if err != nil {
		t.Fatal(err)
	}
	b := NewBelief(m, scn)
	if b.Provinces["marren"].EnemyStrength == nil || *b.Provinces["marren"].EnemyStrength != 30 {
		t.Fatal("opening intel missing")
	}
	ids := map[string]string{"Ione Saris": "saris", "Damar Velk": "velk"}
	b.Apply(specFacts, "velk", m, ids)
	if b.Generals["velk"].Province != "oros" || b.Generals["saris"].Province != "velia" {
		t.Errorf("generals: velk %+v saris %+v", b.Generals["velk"], b.Generals["saris"])
	}
	if e := b.Provinces["marren"]; *e.EnemyStrength != 15 || e.AsOfTurn != 6 || e.LastKnownOwner != model.Enemy {
		t.Errorf("marren: %+v", e)
	}
	if e := b.Provinces["oros"]; e.LastKnownOwner != model.Player {
		t.Errorf("oros: %+v", e)
	}
	// Older news does not overwrite newer.
	stale := ReportFacts{General: "Ione Saris", WrittenTurn: 4, Location: "Velia",
		Sightings: []SightingFacts{{Province: "Marren", EnemyStrength: 40}}}
	b.Apply(stale, "saris", m, ids)
	if *b.Provinces["marren"].EnemyStrength != 15 {
		t.Errorf("stale report overwrote marren: %d", *b.Provinces["marren"].EnemyStrength)
	}
}
