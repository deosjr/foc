package web

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"
)

// progress tracks the turn being resolved, for the SSE progress line.
type progress struct {
	mu    sync.Mutex
	run   int    // increments with every turn started
	phase string // human-readable phase name
	done  bool
	err   string
	subs  map[chan struct{}]struct{}
	idle  chan struct{} // closed when no turn is running
}

func newProgress() *progress {
	idle := make(chan struct{})
	close(idle)
	return &progress{subs: map[chan struct{}]struct{}{}, done: true, idle: idle}
}

func (p *progress) start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.run++
	p.phase, p.done, p.err = "sealing the letters", false, ""
	p.idle = make(chan struct{})
	p.notify()
}

func (p *progress) set(phase string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if phase == "done" {
		return // finish reports the end
	}
	p.phase = phase
	p.notify()
}

func (p *progress) finish(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done = true
	if err != nil {
		p.err = err.Error()
	}
	close(p.idle)
	p.notify()
}

// notify wakes every subscriber. Call with the lock held.
func (p *progress) notify() {
	for ch := range p.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (p *progress) subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	p.mu.Lock()
	p.subs[ch] = struct{}{}
	p.mu.Unlock()
	return ch, func() {
		p.mu.Lock()
		delete(p.subs, ch)
		p.mu.Unlock()
	}
}

func (p *progress) state() (run int, phase string, done bool, err string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.run, p.phase, p.done, p.err
}

// wait blocks until no turn is running.
func (p *progress) wait() {
	p.mu.Lock()
	idle := p.idle
	p.mu.Unlock()
	<-idle
}

// events streams the phases of the running turn as server-sent events:
// "phase" carries the phase name as HTML, and "turn-done" tells htmx to
// reload the game. After turn-done the stream stays open until the browser
// drops it (when htmx swaps the progress line out), so the extension never
// reconnects and replays the end.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	ch, unsubscribe := s.progress.subscribe()
	defer unsubscribe()

	sent := ""
	for {
		_, phase, done, errText := s.progress.state()
		if phase != sent {
			fmt.Fprintf(w, "event: phase\ndata: %s…\n\n", html.EscapeString(phase))
			sent = phase
		}
		if done {
			if errText != "" {
				fmt.Fprintf(w, "event: phase\ndata: <span class=\"warn\">The turn failed: %s</span>\n\n", html.EscapeString(strings.ReplaceAll(errText, "\n", " ")))
			}
			fmt.Fprint(w, "event: turn-done\ndata: done\n\n")
			flusher.Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(time.Minute):
			}
			return
		}
		flusher.Flush()
		select {
		case <-ch:
		case <-r.Context().Done():
			return
		}
	}
}
