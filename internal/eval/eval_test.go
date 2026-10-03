package eval

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/deosjr/foc/internal/generals"
	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	dmock "github.com/deosjr/foc/internal/providers/decision/mock"
)

func TestEvalRunsOnShippedLetters(t *testing.T) {
	m, _ := mapdata.LoadMap("../../maps/valley.json")
	gens, _ := mapdata.LoadGenerals("../../content/generals.yaml")
	qs, err := interpret.LoadQuestions("../../prompts/questions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	letters, err := Load("../../testdata/eval/letters.yaml", m, gens)
	if err != nil {
		t.Fatal(err)
	}
	if len(letters) < 30 || len(letters) > 50 {
		t.Errorf("%d letters; the spec asks for 30-50", len(letters))
	}
	results, sum := Run(context.Background(), dmock.New(), letters, Setup{
		Map: m, Generals: gens, Questions: qs, Thresholds: generals.Thresholds{Clear: 0.8, Unclear: 0.4},
	})
	if sum.Errors != 0 {
		t.Errorf("%d errors", sum.Errors)
	}
	if sum.Clear+sum.Ambiguous+sum.Conditional+sum.Unclear+sum.None != len(letters) {
		t.Errorf("summary does not cover every letter: %+v", sum)
	}
	var buf bytes.Buffer
	Report(&buf, "mock", results, sum, true)
	if !strings.Contains(buf.String(), "Clear letters:") || !strings.Contains(buf.String(), "c01") {
		t.Errorf("report:\n%s", buf.String())
	}
}
