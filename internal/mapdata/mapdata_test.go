package mapdata

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadShippedFiles(t *testing.T) {
	m, err := LoadMap("../../maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuleset("../../rulesets/ancient.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadScenario("../../scenarios/poc.yaml", m); err != nil {
		t.Fatal(err)
	}
	gens, err := LoadGenerals("../../content/generals.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if gens["saris"].Traits.Caution != 0.85 {
		t.Errorf("saris caution = %v", gens["saris"].Traits.Caution)
	}
}

func TestPaths(t *testing.T) {
	m, err := LoadMap("../../maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Path("karsa", "kethra"); !reflect.DeepEqual(got, []string{"karsa", "duna", "lyde", "ilth", "kethra"}) {
		t.Errorf("path karsa->kethra = %v", got)
	}
	if d := m.Distance("velia", "marren"); d != 2 {
		t.Errorf("distance velia->marren = %d", d)
	}
	if n := m.NextStep("velia", "velia"); n != "velia" {
		t.Errorf("next step to self = %s", n)
	}
	if m.Capital("player") != "karsa" || m.Capital("enemy") != "kethra" {
		t.Error("capitals wrong")
	}
}

func TestMapValidation(t *testing.T) {
	bad := `{"terrain":{"plain":1},"provinces":[
	  {"id":"a","name":"A","terrain":"plain","owner":"player","capital":true},
	  {"id":"b","name":"B","terrain":"plain","owner":"enemy","capital":true}],"edges":[]}`
	path := filepath.Join(t.TempDir(), "m.json")
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMap(path); err == nil {
		t.Error("disconnected map accepted")
	}
}
