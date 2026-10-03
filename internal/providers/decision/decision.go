// Package decision is the interface to decision models: typed questions
// about a context string, answered with probabilities over closed sets.
package decision

import "context"

type Kind string

const (
	KindChoice Kind = "choice" // one of Options, probability per option
	KindScore  Kind = "score"  // a value in [0,1]
	KindBinary Kind = "binary" // probability that the answer is yes
)

type Question struct {
	ID      string   `json:"id"`
	Kind    Kind     `json:"kind"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options,omitempty"` // KindChoice only
}

type Request struct {
	State     string     `json:"state"` // plain text or JSON context
	Questions []Question `json:"questions"`
}

type Answer struct {
	QuestionID string             `json:"question"`
	Probs      map[string]float64 `json:"probs,omitempty"` // KindChoice; sums to 1 after normalisation
	Score      float64            `json:"score,omitempty"` // KindScore
	PYes       float64            `json:"p_yes,omitempty"` // KindBinary
	Confidence float64            `json:"confidence"`      // provider confidence if offered, else 1
}

type Response struct {
	Answers  []Answer `json:"answers"`
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
	Raw      []byte   `json:"raw,omitempty"` // raw provider payload, kept for the log
}

type Model interface {
	Decide(ctx context.Context, req Request) (Response, error)
}
