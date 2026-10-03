// Package contract runs every decision and LLM adapter through the same
// cases. Real-API runs live behind the "live" build tag.
package contract

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deosjr/foc/internal/interpret"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/decision"
	dllm "github.com/deosjr/foc/internal/providers/decision/llm"
	dmock "github.com/deosjr/foc/internal/providers/decision/mock"
	"github.com/deosjr/foc/internal/providers/llm"
	"github.com/deosjr/foc/internal/providers/llm/anthropic"
	"github.com/deosjr/foc/internal/providers/llm/openaicompat"
)

// --- LLM adapters ---------------------------------------------------------

type llmCase struct {
	name string
	// serve returns a fixture response body for a successful call.
	ok   string
	make func(t *testing.T, url string, cfg providers.ProviderConfig) llm.Model
}

var llmCases = []llmCase{
	{
		name: "anthropic",
		ok: `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5",
		      "content":[{"type":"thinking","thinking":""},{"type":"text","text":"To my lord in Karsa."}],
		      "stop_reason":"end_turn","usage":{"input_tokens":120,"output_tokens":9}}`,
		make: func(t *testing.T, url string, cfg providers.ProviderConfig) llm.Model {
			t.Setenv("TEST_ANTHROPIC_KEY", "sk-test")
			cfg.Provider, cfg.Endpoint, cfg.APIKeyEnv = "anthropic", url, "TEST_ANTHROPIC_KEY"
			if cfg.Model == "" {
				cfg.Model = "claude-sonnet-5"
			}
			m, err := anthropic.New(cfg, providers.Deps{})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	},
	{
		name: "openaicompat",
		ok: `{"id":"c1","model":"llama3.1","choices":[{"index":0,"message":{"role":"assistant","content":"To my lord in Karsa."},"finish_reason":"stop"}],
		      "usage":{"prompt_tokens":120,"completion_tokens":9}}`,
		make: func(t *testing.T, url string, cfg providers.ProviderConfig) llm.Model {
			t.Setenv("TEST_OPENAI_KEY", "sk-test")
			cfg.Provider, cfg.Endpoint, cfg.APIKeyEnv, cfg.Model = "openaicompat", url, "TEST_OPENAI_KEY", "llama3.1"
			m, err := openaicompat.New(cfg, providers.Deps{})
			if err != nil {
				t.Fatal(err)
			}
			return m
		},
	},
}

var sample = llm.Request{
	System:      "You are Damar Velk.",
	Messages:    []llm.Message{{Role: llm.RoleUser, Content: "FACTS: {}"}},
	MaxTokens:   400,
	Temperature: 0.8,
}

func TestLLMAdapters(t *testing.T) {
	for _, c := range llmCases {
		t.Run(c.name+"/success", func(t *testing.T) {
			var got map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				json.Unmarshal(b, &got)
				w.Write([]byte(c.ok))
			}))
			defer srv.Close()
			resp, err := c.make(t, srv.URL, providers.ProviderConfig{}).Complete(context.Background(), sample)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Text != "To my lord in Karsa." || resp.InputTok != 120 || resp.OutputTok != 9 {
				t.Errorf("response = %+v", resp)
			}
			if !strings.Contains(string(mustJSON(got)), "You are Damar Velk.") || !strings.Contains(string(mustJSON(got)), "FACTS: {}") {
				t.Errorf("request lost the system prompt or message: %s", mustJSON(got))
			}
		})
		for _, status := range []int{429, 500, 400} {
			t.Run(c.name+"/status", func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					w.Write([]byte(`{"type":"error","error":{"type":"x","message":"nope"}}`))
				}))
				defer srv.Close()
				_, err := c.make(t, srv.URL, providers.ProviderConfig{}).Complete(context.Background(), sample)
				var he *providers.HTTPError
				if !errors.As(err, &he) || he.Status != status || he.Retryable() != (status != 400) {
					t.Errorf("status %d: err = %v", status, err)
				}
			})
		}
	}
}

func TestAnthropicRequestShape(t *testing.T) {
	var headers http.Header
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		body = map[string]any{}
		json.Unmarshal(b, &body)
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(llmCases[0].ok))
	}))
	defer srv.Close()
	m := llmCases[0].make(t, srv.URL, providers.ProviderConfig{Model: "claude-sonnet-5", Effort: "low"})
	req := sample
	req.JSONSchema = []byte(`{"type":"object"}`)
	if _, err := m.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if headers.Get("x-api-key") != "sk-test" || headers.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("headers = %v", headers)
	}
	if _, ok := body["temperature"]; ok {
		t.Error("temperature sent to claude-sonnet-5, which rejects it")
	}
	oc, _ := body["output_config"].(map[string]any)
	if oc["effort"] != "low" || oc["format"] == nil {
		t.Errorf("output_config = %v", oc)
	}
	m = llmCases[0].make(t, srv.URL, providers.ProviderConfig{Model: "claude-sonnet-4-6"})
	if _, err := m.Complete(context.Background(), sample); err != nil {
		t.Fatal(err)
	}
	if body["temperature"] != 0.8 {
		t.Errorf("temperature not sent to claude-sonnet-4-6: %v", body["temperature"])
	}
}

func TestAnthropicRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"model":"claude-sonnet-5","content":[],"stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber"},"usage":{}}`))
	}))
	defer srv.Close()
	if _, err := llmCases[0].make(t, srv.URL, providers.ProviderConfig{}).Complete(context.Background(), sample); err == nil {
		t.Error("refusal should be an error")
	}
}

func TestOpenAICompatAuthHeader(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Write([]byte(llmCases[1].ok))
	}))
	defer srv.Close()
	if _, err := llmCases[1].make(t, srv.URL, providers.ProviderConfig{}).Complete(context.Background(), sample); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", auth)
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// --- Decision adapters ----------------------------------------------------

// fakeLLM replies with canned texts in order.
type fakeLLM struct{ replies []string }

func (f *fakeLLM) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	if len(f.replies) == 0 {
		return llm.Response{}, errors.New("no more replies")
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return llm.Response{Text: r, Provider: "fake", Model: "fake"}, nil
}

const goodJSON = "```json\n" + `{"plausible": 0.9, "addressed": 0.95, "engagement": 0.6,
  "action": {"hold": 0.1, "move": 0.85, "retreat": 0.0, "unclear": 0.05},
  "target": {"Oros Ford": 0.8, "none": 0.1, "unclear": 0.1},
  "support_whom": {"none": 1},
  "conditional": 0.05, "trigger": {"none": 1}, "trigger_place": {"none": 1},
  "then_action": {"none": 1}, "then_target": {"none": 1}}` + "\n```"

func decisionRequest(t *testing.T) (decision.Request, []decision.Question, *mapdata.Map) {
	t.Helper()
	m, err := mapdata.LoadMap("../../../maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	qs, err := interpret.LoadQuestions("../../../prompts/questions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	questions, err := qs.Build("Damar Velk", "Velia", m, []string{"Ione Saris"})
	if err != nil {
		t.Fatal(err)
	}
	c := interpret.Context{
		General: &model.General{Name: "Damar Velk"}, Location: "velia", Friendly: map[string]string{},
		Standing: model.Order{Type: model.Hold}, SentTurn: 1, ArriveTurn: 1, Letter: "March on Oros Ford at once.",
	}
	return decision.Request{State: interpret.BuildState(c, m), Questions: questions}, questions, m
}

func TestDecisionAdapters(t *testing.T) {
	adapters := map[string]func() decision.Model{
		"mock": func() decision.Model { return dmock.New() },
		"llm": func() decision.Model {
			d, _ := dllm.New(providers.ProviderConfig{}, providers.Deps{LLM: &fakeLLM{replies: []string{goodJSON}}})
			return d
		},
	}
	for name, mk := range adapters {
		t.Run(name, func(t *testing.T) {
			req, questions, m := decisionRequest(t)
			resp, err := mk().Decide(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			ans, err := interpret.Parse(resp, questions, m)
			if err != nil {
				t.Fatal(err)
			}
			sum := 0.0
			for _, p := range ans.Action {
				sum += p
			}
			if math.Abs(sum-1) > 1e-9 {
				t.Errorf("action probabilities sum to %v", sum)
			}
			if top, _ := interpret.Top(ans.Action, []string{"hold", "move", "retreat", "unclear"}); top != "move" {
				t.Errorf("top action = %s, want move", top)
			}
			if top, _ := interpret.Top(ans.Target, append(m.IDs(), "none", "unclear")); top != "oros" {
				t.Errorf("top target = %s, want oros", top)
			}
		})
	}
}

func TestDecisionLLMValidation(t *testing.T) {
	req, _, _ := decisionRequest(t)
	missing := `{"plausible": 0.9, "addressed": 0.9, "engagement": 0.5, "action": {"move": 1}}`
	unknown := `{"plausible": 0.9, "addressed": 0.9, "engagement": 0.5, "action": {"charge": 1}, "target": {"none": 1}, "support_whom": {"none": 1}, "conditional": 0, "trigger": {"none": 1}, "trigger_place": {"none": 1}, "then_action": {"none": 1}, "then_target": {"none": 1}}`
	for name, reply := range map[string]string{"missing answer": missing, "unknown option": unknown, "not json": "I would march."} {
		d, _ := dllm.New(providers.ProviderConfig{}, providers.Deps{LLM: &fakeLLM{replies: []string{reply, reply}}})
		if _, err := d.Decide(context.Background(), req); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// A bad first reply is retried once.
	d, _ := dllm.New(providers.ProviderConfig{}, providers.Deps{LLM: &fakeLLM{replies: []string{"oops", goodJSON}}})
	if _, err := d.Decide(context.Background(), req); err != nil {
		t.Errorf("retry after a bad reply failed: %v", err)
	}
}
