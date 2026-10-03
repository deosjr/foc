// Package middleware wraps every provider with the same layers, outermost
// first: logging, record/replay cache, retry, timeout.
//
// Logging sits outside the cache (the spec lists the cache first) so that a
// replayed run logs exactly the same model calls as the original, which is
// what lets turns.jsonl match byte for byte.
package middleware

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/deosjr/foc/internal/providers"
	"github.com/deosjr/foc/internal/providers/decision"
	"github.com/deosjr/foc/internal/providers/llm"
	"github.com/deosjr/foc/internal/runlog"
)

// Options configures the wrappers for one provider.
type Options struct {
	Name    string        // provider name, part of the cache key
	Model   string        // model id, part of the cache key
	Timeout time.Duration // per call; 0 means none
	Retries int           // total attempts; default 3
	Backoff time.Duration // first retry delay; doubles each time
	Log     *runlog.Log   // may be nil
	Cache   *Store        // read-write development cache; may be nil
	Replay  *Store        // replay mode: serve only from here, error on a miss
	Record  *Store        // record mode: store every response here
}

func (o Options) attempts() int {
	if o.Retries <= 0 {
		return 3
	}
	return o.Retries
}

// Key hashes the provider, model and request.
func Key(provider, model string, req any) string {
	b, _ := json.Marshal(req)
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", provider, model)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// call runs one request through the layers. do performs the actual provider
// call; responses are passed around as JSON so one implementation serves
// both interfaces.
func call[Req, Resp any](ctx context.Context, o Options, kind string, req Req, do func(context.Context, Req) (Resp, error)) (Resp, error) {
	var zero Resp
	key := Key(o.Name, o.Model, req)
	start := time.Now()
	resp, err := cached(ctx, o, key, req, do)
	if o.Log != nil {
		rec := map[string]any{
			"provider": o.Name, "model": o.Model, "key": key,
			"request": req, "latency_ms": time.Since(start).Milliseconds(),
		}
		if err != nil {
			rec["error"] = err.Error()
		} else {
			rec["response"] = resp
		}
		o.Log.AddUnordered(key, kind, rec)
	}
	if err != nil {
		return zero, err
	}
	return resp, nil
}

func cached[Req, Resp any](ctx context.Context, o Options, key string, req Req, do func(context.Context, Req) (Resp, error)) (Resp, error) {
	var resp Resp
	if o.Replay != nil {
		raw, ok := o.Replay.Get(key)
		if !ok {
			return resp, fmt.Errorf("replay: no recorded response for %s request %s", o.Name, key[:12])
		}
		err := json.Unmarshal(raw, &resp)
		return resp, err
	}
	if o.Cache != nil {
		if raw, ok := o.Cache.Get(key); ok && json.Unmarshal(raw, &resp) == nil {
			o.record(key, raw)
			return resp, nil
		}
	}
	resp, err := retry(ctx, o, req, do)
	if err != nil {
		return resp, err
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		return resp, err
	}
	if o.Cache != nil {
		if err := o.Cache.Put(key, raw); err != nil {
			return resp, err
		}
	}
	o.record(key, raw)
	return resp, nil
}

func (o Options) record(key string, raw json.RawMessage) {
	if o.Record != nil {
		o.Record.Put(key, raw)
	}
}

func retry[Req, Resp any](ctx context.Context, o Options, req Req, do func(context.Context, Req) (Resp, error)) (Resp, error) {
	var resp Resp
	var err error
	delay := o.Backoff
	if delay == 0 {
		delay = 500 * time.Millisecond
	}
	for attempt := 1; attempt <= o.attempts(); attempt++ {
		resp, err = withTimeout(ctx, o.Timeout, req, do)
		if err == nil || !Retryable(err) || ctx.Err() != nil || attempt == o.attempts() {
			return resp, err
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return resp, ctx.Err()
		}
		delay *= 2
	}
	return resp, err
}

func withTimeout[Req, Resp any](ctx context.Context, d time.Duration, req Req, do func(context.Context, Req) (Resp, error)) (Resp, error) {
	if d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	return do(ctx, req)
}

// Retryable reports whether an error is worth another attempt: network
// errors, per-call timeouts, 429 and 5xx.
func Retryable(err error) bool {
	var he *providers.HTTPError
	if errors.As(err, &he) {
		return he.Retryable()
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

// Decision wraps a decision model.
type Decision struct {
	Inner decision.Model
	Opts  Options
}

func (d *Decision) Decide(ctx context.Context, req decision.Request) (decision.Response, error) {
	return call(ctx, d.Opts, "decision-call", req, d.Inner.Decide)
}

// LLM wraps an LLM.
type LLM struct {
	Inner llm.Model
	Opts  Options
}

func (l *LLM) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	return call(ctx, l.Opts, "llm-call", req, l.Inner.Complete)
}

// Store is a JSON-lines file of recorded responses keyed by request hash.
type Store struct {
	mu       sync.Mutex
	path     string
	entries  map[string]json.RawMessage
	readOnly bool
}

type storeLine struct {
	Key      string          `json:"key"`
	Response json.RawMessage `json:"response"`
}

// OpenStore loads a store, creating its directory if it is writable.
func OpenStore(path string, readOnly bool) (*Store, error) {
	s := &Store{path: path, entries: map[string]json.RawMessage{}, readOnly: readOnly}
	f, err := os.Open(path)
	switch {
	case err == nil:
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var l storeLine
			if json.Unmarshal(sc.Bytes(), &l) == nil {
				s.entries[l.Key] = l.Response
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	case os.IsNotExist(err) && !readOnly:
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	return s, nil
}

func (s *Store) Get(key string) (json.RawMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.entries[key]
	return v, ok
}

func (s *Store) Put(key string, v json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readOnly {
		return nil
	}
	if _, ok := s.entries[key]; ok {
		return nil
	}
	s.entries[key] = v
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, _ := json.Marshal(storeLine{Key: key, Response: v})
	_, err = f.Write(append(line, '\n'))
	return err
}

// Len is the number of stored responses.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}
