package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/providers/llm"
)

// Writer turns facts into a letter in the general's voice.
type Writer struct {
	LLM         llm.Model
	Prompts     *template.Template // defines "system" and "user"
	Clarify     *template.Template // the same, for reports that ask for clarification
	Validator   Validator
	Capital     string // display name of the sovereign's seat
	MaxTokens   int
	Temperature float64
}

// Written is a finished letter plus how it was produced, for the log.
type Written struct {
	Text       string     `json:"text"`
	Attempts   int        `json:"attempts"`
	Violations [][]string `json:"violations,omitempty"`
	FellBack   bool       `json:"fell_back"`
	Error      string     `json:"error,omitempty"`
}

// LoadPrompts parses a prompt template file.
func LoadPrompts(path string) (*template.Template, error) {
	t, err := template.New("").Option("missingkey=error").ParseFiles(path)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"system", "user"} {
		if t.Lookup(name) == nil {
			return nil, fmt.Errorf("%s: no %q template", path, name)
		}
	}
	return t, nil
}

// FactsMarker precedes the facts JSON in the user message, so the mock LLM
// can find it.
const FactsMarker = "FACTS:\n```json\n"

// Request builds the LLM request for a report.
func (w *Writer) Request(g *model.General, f ReportFacts) (llm.Request, error) {
	factsJSON, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return llm.Request{}, err
	}
	data := map[string]any{
		"General":   g,
		"Capital":   w.Capital,
		"FactsJSON": string(factsJSON),
		"MenNotes":  menNotes(f, w.Validator.MenPerStrength),
		"MinWords":  w.Validator.MinWords,
		"MaxWords":  w.Validator.MaxWords,
	}
	var sys, user bytes.Buffer
	t := w.Prompts
	if f.Clarification != nil && w.Clarify != nil {
		t = w.Clarify
	}
	if err := t.ExecuteTemplate(&sys, "system", data); err != nil {
		return llm.Request{}, err
	}
	if err := t.ExecuteTemplate(&user, "user", data); err != nil {
		return llm.Request{}, err
	}
	return llm.Request{
		System:      strings.TrimSpace(sys.String()),
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: strings.TrimSpace(user.String())}},
		MaxTokens:   w.MaxTokens,
		Temperature: w.Temperature,
	}, nil
}

// Write asks the LLM for a letter, validates it, retries once with the
// violations appended, and falls back to the template letter on a second
// failure or any error. It never blocks the game on the LLM.
func (w *Writer) Write(ctx context.Context, g *model.General, f ReportFacts) Written {
	var out Written
	req, err := w.Request(g, f)
	if err != nil {
		out.Error = err.Error()
		out.Text, out.FellBack = Fallback(f), true
		return out
	}
	for attempt := 1; attempt <= 2; attempt++ {
		out.Attempts = attempt
		resp, err := w.LLM.Complete(ctx, req)
		if err != nil {
			out.Error = err.Error()
			break
		}
		text := strings.TrimSpace(resp.Text)
		violations := w.Validator.Validate(text, f)
		if len(violations) == 0 {
			out.Text = text
			return out
		}
		out.Violations = append(out.Violations, violations)
		req.Messages = append(req.Messages,
			llm.Message{Role: llm.RoleAssistant, Content: text},
			llm.Message{Role: llm.RoleUser, Content: "Your letter broke these rules:\n- " +
				strings.Join(violations, "\n- ") + "\nRewrite the whole letter, keeping strictly to FACTS."},
		)
	}
	out.Text, out.FellBack = Fallback(f), true
	return out
}

// FactsFromPrompt extracts the facts JSON from a report request's user
// message. Used by the mock LLM.
func FactsFromPrompt(content string) (ReportFacts, bool) {
	i := strings.Index(content, FactsMarker)
	if i < 0 {
		return ReportFacts{}, false
	}
	rest := content[i+len(FactsMarker):]
	j := strings.Index(rest, "```")
	if j < 0 {
		return ReportFacts{}, false
	}
	var f ReportFacts
	if err := json.Unmarshal([]byte(rest[:j]), &f); err != nil {
		return ReportFacts{}, false
	}
	return f, true
}

// menNotes spells out every count in FACTS as a number of men, so the model
// does not have to do the conversion itself.
func menNotes(f ReportFacts, per int) []string {
	if per <= 1 {
		return nil
	}
	say := func(what string, n int) string { return fmt.Sprintf("%s %d = %s", what, n, men(n)) }
	notes := []string{say("own_strength", f.OwnStrength), say("own_losses", f.OwnLosses)}
	for _, b := range f.Battles {
		notes = append(notes, say("enemy_losses at "+b.Place, b.EnemyLosses))
	}
	for _, s := range f.Sightings {
		notes = append(notes, say("enemy_strength in "+s.Province, s.EnemyStrength))
	}
	return notes
}
