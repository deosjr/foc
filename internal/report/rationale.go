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

// RationaleFacts is everything the LLM may use for a private rationale:
// the letter and what the general decided, never the truth of the world.
type RationaleFacts struct {
	General  string `json:"general"`
	Location string `json:"location"`
	Letter   string `json:"letter"`
	SentTurn int    `json:"letter_sent_turn"`
	Decision string `json:"decision"`
	Reading  string `json:"reading"`
}

// RationaleMarker precedes the note JSON, so the mock LLM can find it.
const RationaleMarker = "NOTE:\n```json\n"

// Readings explain, for the LLM, which policy step decided.
func Reading(outcome, step string) string {
	switch {
	case outcome == "ignored":
		return "the letter gave no instruction, so you kept to your standing orders"
	case step == "clear":
		return "the letter was plain, and you did as it said"
	case step == "ambiguous":
		return "the letter could be read more than one way, and you chose the reading that suited your judgement and temperament"
	}
	return "you could not make out what the sovereign wanted, so you held your position"
}

// RationaleWriter writes rationales.
type RationaleWriter struct {
	LLM         llm.Model
	Prompts     *template.Template
	MaxTokens   int
	Temperature float64
}

// Write returns a short private note. Rationales never affect the game, so
// failures return an empty note and the error for the log.
func (w *RationaleWriter) Write(ctx context.Context, g *model.General, f RationaleFacts) (string, error) {
	note, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return "", err
	}
	data := map[string]any{"General": g, "NoteJSON": string(note)}
	var sys, user bytes.Buffer
	if err := w.Prompts.ExecuteTemplate(&sys, "system", data); err != nil {
		return "", err
	}
	if err := w.Prompts.ExecuteTemplate(&user, "user", data); err != nil {
		return "", err
	}
	resp, err := w.LLM.Complete(ctx, llm.Request{
		System:      strings.TrimSpace(sys.String()),
		Messages:    []llm.Message{{Role: llm.RoleUser, Content: strings.TrimSpace(user.String())}},
		MaxTokens:   w.MaxTokens,
		Temperature: w.Temperature,
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(resp.Text)
	if words := strings.Fields(text); len(words) > 80 {
		text = strings.Join(words[:80], " ") + "…"
	}
	return text, nil
}

// RationaleFromPrompt extracts the note JSON from a rationale request.
func RationaleFromPrompt(content string) (RationaleFacts, bool) {
	i := strings.Index(content, RationaleMarker)
	if i < 0 {
		return RationaleFacts{}, false
	}
	rest := content[i+len(RationaleMarker):]
	j := strings.Index(rest, "```")
	if j < 0 {
		return RationaleFacts{}, false
	}
	var f RationaleFacts
	if err := json.Unmarshal([]byte(rest[:j]), &f); err != nil {
		return RationaleFacts{}, false
	}
	return f, true
}

// FallbackRationale is the template note used by the mock LLM.
func FallbackRationale(f RationaleFacts) string {
	return fmt.Sprintf("Read the letter sent in turn %d: %s. So: %s.", f.SentTurn, f.Reading, f.Decision)
}
