package middleware

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/llm"
	"github.com/deosjr/foc/internal/runlog"
)

type counting struct {
	calls int
	errs  []error
}

func (c *counting) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	c.calls++
	if len(c.errs) > 0 {
		err := c.errs[0]
		c.errs = c.errs[1:]
		if err != nil {
			return llm.Response{}, err
		}
	}
	return llm.Response{Text: "reply to " + req.Messages[0].Content}, nil
}

var req = llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}}

func TestCacheAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "responses.jsonl")
	store, err := OpenStore(path, false)
	if err != nil {
		t.Fatal(err)
	}
	inner := &counting{}
	log := runlog.New()
	m := &LLM{Inner: inner, Opts: Options{Name: "x", Model: "y", Cache: store, Log: log}}
	for i := 0; i < 3; i++ {
		if r, err := m.Complete(context.Background(), req); err != nil || r.Text != "reply to hi" {
			t.Fatal(r, err)
		}
	}
	if inner.calls != 1 {
		t.Errorf("cache: inner called %d times, want 1", inner.calls)
	}
	if n := len(log.Records()); n != 3 {
		t.Errorf("every call should be logged, cached or not: %d records", n)
	}
	replay, err := OpenStore(path, true)
	if err != nil {
		t.Fatal(err)
	}
	r := &LLM{Inner: &counting{errs: []error{errors.New("must not be called")}}, Opts: Options{Name: "x", Model: "y", Replay: replay}}
	if resp, err := r.Complete(context.Background(), req); err != nil || resp.Text != "reply to hi" {
		t.Errorf("replay: %v %v", resp, err)
	}
	other := llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: "bye"}}}
	if _, err := r.Complete(context.Background(), other); err == nil {
		t.Error("replay should error on a miss")
	}
}

func TestRetry(t *testing.T) {
	inner := &counting{errs: []error{&providers.HTTPError{Status: 429}, &providers.HTTPError{Status: 503}, nil}}
	m := &LLM{Inner: inner, Opts: Options{Backoff: time.Millisecond}}
	if _, err := m.Complete(context.Background(), req); err != nil || inner.calls != 3 {
		t.Errorf("retry: err %v after %d calls", err, inner.calls)
	}
	inner = &counting{errs: []error{&providers.HTTPError{Status: 400}, nil}}
	m = &LLM{Inner: inner, Opts: Options{Backoff: time.Millisecond}}
	if _, err := m.Complete(context.Background(), req); err == nil || inner.calls != 1 {
		t.Errorf("400 must not be retried: err %v after %d calls", err, inner.calls)
	}
}
