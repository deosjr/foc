// Package llm adapts any LLM into a decision model: it asks for JSON with a
// probability per option, validates it against the question set, and
// normalises. This lets the game run with a single API key.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/decision"
	llmapi "github.com/deosjr/foc/internal/providers/llm"
)

func init() {
	providers.RegisterDecision("llm", New)
}

type Model struct {
	llm       llmapi.Model
	cfg       providers.ProviderConfig
	maxTokens int
}

func New(cfg providers.ProviderConfig, deps providers.Deps) (decision.Model, error) {
	if deps.LLM == nil {
		return nil, errors.New("decision provider llm needs an llm provider")
	}
	mt := cfg.MaxTokens
	if mt <= 0 {
		mt = 2000
	}
	return &Model{llm: deps.LLM, cfg: cfg, maxTokens: mt}, nil
}

const system = `You read letters sent by a sovereign to a general in the field and judge, as a neutral and careful reader, what the letter most plausibly asks. You are not the general and have no personality: give the reading most people would agree on, and spread probability across readings when the letter genuinely allows several.

Answer every question with calibrated probabilities. Reply with a single JSON object and nothing else.`

// Prompt renders the user message for a request.
func Prompt(req decision.Request) string {
	var b strings.Builder
	b.WriteString(req.State)
	b.WriteString("\nQUESTIONS\n")
	for _, q := range req.Questions {
		switch q.Kind {
		case decision.KindScore:
			fmt.Fprintf(&b, "- %q (a number from 0 to 1): %s\n", q.ID, q.Prompt)
		case decision.KindBinary:
			fmt.Fprintf(&b, "- %q (the probability, 0 to 1, that the answer is yes): %s\n", q.ID, q.Prompt)
		case decision.KindChoice:
			opts := make([]string, len(q.Options))
			for i, o := range q.Options {
				opts[i] = fmt.Sprintf("%q", o)
			}
			fmt.Fprintf(&b, "- %q (an object giving a probability to every option: %s; they must sum to 1): %s\n",
				q.ID, strings.Join(opts, ", "), q.Prompt)
		}
	}
	b.WriteString("\nReply with one JSON object whose keys are exactly the question ids above.")
	return b.String()
}

// Schema builds a JSON schema for the answers, for providers that support
// constrained output.
func Schema(qs []decision.Question) []byte {
	props := map[string]any{}
	var required []string
	num := map[string]any{"type": "number"}
	for _, q := range qs {
		required = append(required, q.ID)
		if q.Kind != decision.KindChoice {
			props[q.ID] = num
			continue
		}
		opts := map[string]any{}
		for _, o := range q.Options {
			opts[o] = num
		}
		props[q.ID] = map[string]any{
			"type": "object", "properties": opts, "required": q.Options, "additionalProperties": false,
		}
	}
	b, _ := json.Marshal(map[string]any{
		"type": "object", "properties": props, "required": required, "additionalProperties": false,
	})
	return b
}

func (m *Model) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	lreq := llmapi.Request{
		System:      system,
		Messages:    []llmapi.Message{{Role: llmapi.RoleUser, Content: Prompt(req)}},
		MaxTokens:   m.maxTokens,
		Temperature: m.cfg.Temperature,
		JSONSchema:  Schema(req.Questions),
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := m.llm.Complete(ctx, lreq)
		if err != nil {
			return decision.Response{}, err
		}
		answers, err := Parse(resp.Text, req.Questions)
		if err == nil {
			return decision.Response{Answers: answers, Provider: "llm/" + resp.Provider, Model: resp.Model, Raw: []byte(resp.Text)}, nil
		}
		lastErr = err
		lreq.Messages = append(lreq.Messages,
			llmapi.Message{Role: llmapi.RoleAssistant, Content: resp.Text},
			llmapi.Message{Role: llmapi.RoleUser, Content: "That reply was not valid: " + err.Error() + ". Reply again with only the JSON object."},
		)
	}
	return decision.Response{}, fmt.Errorf("decision llm: %w", lastErr)
}

// Parse extracts and validates the JSON answers from a model's text.
func Parse(text string, qs []decision.Question) ([]decision.Answer, error) {
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j < i {
		return nil, errors.New("no JSON object found")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text[i:j+1]), &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %v", err)
	}
	var out []decision.Answer
	for _, q := range qs {
		v, ok := raw[q.ID]
		if !ok {
			return nil, fmt.Errorf("missing answer to %q", q.ID)
		}
		a := decision.Answer{QuestionID: q.ID, Confidence: 1}
		switch q.Kind {
		case decision.KindScore, decision.KindBinary:
			var x float64
			if err := json.Unmarshal(v, &x); err != nil || math.IsNaN(x) || x < 0 || x > 1 {
				return nil, fmt.Errorf("answer to %q must be a number from 0 to 1", q.ID)
			}
			if q.Kind == decision.KindScore {
				a.Score = x
			} else {
				a.PYes = x
			}
		case decision.KindChoice:
			var probs map[string]float64
			if err := json.Unmarshal(v, &probs); err != nil {
				return nil, fmt.Errorf("answer to %q must be an object of probabilities", q.ID)
			}
			a.Probs = map[string]float64{}
			sum := 0.0
			for k, p := range probs {
				opt, ok := matchOption(k, q.Options)
				if !ok {
					return nil, fmt.Errorf("answer to %q has unknown option %q", q.ID, k)
				}
				if p < 0 || math.IsNaN(p) {
					return nil, fmt.Errorf("answer to %q has a bad probability for %q", q.ID, k)
				}
				a.Probs[opt] += p
			}
			for _, o := range q.Options {
				sum += a.Probs[o]
			}
			if sum <= 0 {
				return nil, fmt.Errorf("answer to %q gives no probability to any option", q.ID)
			}
			for _, o := range q.Options {
				a.Probs[o] /= sum
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func matchOption(k string, options []string) (string, bool) {
	for _, o := range options {
		if o == k {
			return o, true
		}
	}
	for _, o := range options {
		if strings.EqualFold(strings.TrimSpace(o), strings.TrimSpace(k)) {
			return o, true
		}
	}
	return "", false
}
