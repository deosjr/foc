package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/deosjr/foc/internal/config"
	"github.com/deosjr/foc/internal/game"
	"github.com/deosjr/foc/internal/model"
	dmock "github.com/deosjr/foc/internal/providers/decision/mock"
	lmock "github.com/deosjr/foc/internal/providers/llm/mock"
	"github.com/deosjr/foc/internal/runlog"
)

func newGame(t *testing.T) *game.Game {
	t.Helper()
	c := config.Default()
	c.Map, c.Ruleset, c.Generals = "../../maps/valley.json", "../../rulesets/ancient.yaml", "../../content/generals.yaml"
	c.Scenario, c.Prompts = "../../scenarios/poc.yaml", "../../prompts"
	g, err := game.New(game.Options{Config: c, Decision: dmock.New(), LLM: lmock.New(), Log: runlog.New(), RunDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func newServer(t *testing.T, debug bool) (*Server, *game.Game) {
	t.Helper()
	g := newGame(t)
	s, err := New(g, debug)
	if err != nil {
		t.Fatal(err)
	}
	return s, g
}

func do(s *Server, method, path string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if htmx {
		r.Header.Set("HX-Request", "true")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

// endTurn posts End turn and waits for the background turn to finish.
func endTurn(s *Server, form url.Values) *httptest.ResponseRecorder {
	w := do(s, "POST", "/turn", form, true)
	s.progress.wait()
	return w
}

func TestRoutesPageVersusFragment(t *testing.T) {
	s, _ := newServer(t, false)
	for _, path := range []string{"/", "/map", "/letters", "/inbox", "/province/marren", "/map?highlight=L1"} {
		page := do(s, "GET", path, nil, false)
		if page.Code != 200 || !strings.Contains(page.Body.String(), "<!doctype html>") {
			t.Errorf("GET %s without HX-Request: %d, full page=%v", path, page.Code, strings.Contains(page.Body.String(), "<!doctype"))
		}
		if path == "/" {
			continue
		}
		frag := do(s, "GET", path, nil, true)
		if frag.Code != 200 || strings.Contains(frag.Body.String(), "<!doctype html>") {
			t.Errorf("GET %s with HX-Request: %d, should be a fragment", path, frag.Code)
		}
	}
	if w := do(s, "GET", "/province/atlantis", nil, true); w.Code != 404 {
		t.Errorf("unknown province: %d", w.Code)
	}
	if w := do(s, "GET", "/letter/L99", nil, true); w.Code != 404 {
		t.Errorf("unknown letter: %d", w.Code)
	}
	if w := do(s, "GET", "/static/htmx.min.js", nil, false); w.Code != 200 {
		t.Errorf("static asset: %d", w.Code)
	}
	if w := do(s, "GET", "/", nil, false); w.Header().Get("Content-Security-Policy") == "" {
		t.Error("missing CSP header")
	}
}

func TestDraftsSurviveUntilTurnEnds(t *testing.T) {
	s, g := newServer(t, false)
	w := do(s, "PUT", "/drafts/velk", url.Values{"draft-velk": {"March on Hollow Wood and then Marrn."}}, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Hollow Wood") || !strings.Contains(w.Body.String(), "Marrn") {
		t.Fatalf("recognised line: %d %s", w.Code, w.Body.String())
	}
	if page := do(s, "GET", "/letters", nil, true).Body.String(); !strings.Contains(page, "March on Hollow Wood") {
		t.Error("draft not shown after reload")
	}
	do(s, "PUT", "/drafts/saris", url.Values{"draft-saris": {"Hold fast."}}, true)
	if w := do(s, "DELETE", "/drafts/saris", nil, true); w.Code != 200 || strings.Contains(w.Body.String(), "Hold fast.") {
		t.Errorf("discard: %d", w.Code)
	}
	if w := do(s, "PUT", "/drafts/nobody", url.Values{"draft-nobody": {"x"}}, true); w.Code != 400 {
		t.Errorf("unknown general: %d", w.Code)
	}
	// End turn includes the textareas; an unsaved edit wins over the saved draft.
	w = endTurn(s, url.Values{"draft-velk": {"March on Oros Ford."}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `sse-connect="/turn/events"`) {
		t.Fatalf("end turn: %d %s", w.Code, w.Body.String())
	}
	w = do(s, "GET", "/", nil, true)
	g.Lock()
	sent := g.Sent()
	g.Unlock()
	if len(sent) != 1 || sent[0].Body != "March on Oros Ford." {
		t.Errorf("sent = %+v", sent)
	}
	if strings.Contains(w.Body.String(), "March on Hollow Wood") {
		t.Error("draft should be gone after the turn ends")
	}
	if !strings.Contains(w.Body.String(), "Turn 2") {
		t.Error("turn did not advance")
	}
}

func TestConcurrentTurnRejected(t *testing.T) {
	s, g := newServer(t, false)
	g.Lock() // hold the game so the first turn cannot finish
	done := make(chan int)
	go func() { done <- do(s, "POST", "/turn", url.Values{}, true).Code }()
	for !s.running.Load() {
	}
	if w := do(s, "POST", "/turn", url.Values{}, true); w.Code != http.StatusConflict {
		t.Errorf("second POST /turn: %d, want 409", w.Code)
	}
	g.Unlock()
	if code := <-done; code != 200 {
		t.Errorf("first POST /turn: %d", code)
	}
	s.progress.wait()
}

func TestDebugRoutes(t *testing.T) {
	s, _ := newServer(t, false)
	if w := do(s, "GET", "/debug/truth", nil, false); w.Code != 404 {
		t.Errorf("/debug/truth without --debug: %d", w.Code)
	}
	endTurn(s, url.Values{"draft-velk": {"Hold."}})
	if strings.Contains(do(s, "GET", "/inbox", nil, true).Body.String(), "Debug:") {
		t.Error("debug drawer shown without --debug")
	}
	d, _ := newServer(t, true)
	if w := do(d, "GET", "/debug/truth", nil, false); w.Code != 200 || !strings.Contains(w.Body.String(), "e-1") {
		t.Errorf("/debug/truth with --debug: %d", w.Code)
	}
	endTurn(d, url.Values{"draft-velk": {"Hold."}})
	if !strings.Contains(do(d, "GET", "/inbox", nil, true).Body.String(), "Debug:") {
		t.Error("debug drawer missing with --debug")
	}
}

// The belief boundary: no PlayerView may contain an enemy position or
// strength that did not appear in a delivered report or the opening intel.
func TestBeliefBoundary(t *testing.T) {
	g := newGame(t)
	letters := []map[string]string{
		{"velk": "March on Hollow Wood.", "saris": "Hold Duna Hills."},
		{}, {"velk": "Take Marren."}, {"saris": "Advance to Hollow Wood."}, {}, {"velk": "Press on to Sarnos."},
	}
	known := map[string]map[int]bool{"marren": {30: true}} // opening intel
	for turn := 0; turn < 8 && !g.Over(); turn++ {
		g.Lock()
		if turn < len(letters) {
			for id, text := range letters[turn] {
				g.SetDraft(id, text)
			}
		}
		if err := g.EndTurn(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
		for _, l := range g.Inbox() {
			f := g.DebugLetter(l.ID).Facts
			for _, s := range f.Sightings {
				for _, p := range g.Map.Provinces {
					if p.Name == s.Province {
						if known[p.ID] == nil {
							known[p.ID] = map[int]bool{}
						}
						known[p.ID][s.EnemyStrength] = true
					}
				}
			}
		}
		v := BuildView(g, false, "", false)
		truth := g.DebugTruth()
		g.Unlock()
		if v.Debug != nil {
			t.Fatal("PlayerView carries debug data without --debug")
		}
		for id, e := range v.Belief.Provinces {
			if e.EnemyStrength != nil && !known[id][*e.EnemyStrength] {
				t.Errorf("turn %d: view shows %d enemy in %s, never reported", turn+1, *e.EnemyStrength, id)
			}
		}
		raw, _ := json.Marshal(v)
		for _, a := range truth.Armies {
			if a.Side == model.Enemy && (strings.Contains(string(raw), a.ID) || strings.Contains(string(raw), a.Commander)) {
				t.Errorf("turn %d: view leaks enemy army %s", turn+1, a.ID)
			}
		}
	}
}

func TestReview(t *testing.T) {
	s, g := newServer(t, false)
	if w := do(s, "GET", "/review", nil, false); w.Code != http.StatusForbidden {
		t.Errorf("review before the end without --debug: %d, want 403", w.Code)
	}
	for i := 0; i < 10 && !g.Over(); i++ {
		endTurn(s, url.Values{"draft-velk": {"March on Kethra."}, "draft-saris": {"Hold Duna Hills."}})
	}
	if !g.Over() {
		t.Fatal("game did not end")
	}
	w := do(s, "GET", "/review?turn=1", nil, false)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "What was true") || !strings.Contains(body, "March on Kethra.") {
		t.Fatalf("review turn 1: %d", w.Code)
	}
	// The truth map shows the enemy commander; the player's view never does.
	if !strings.Contains(body, "Orsk the Red") {
		t.Error("truth map should name the enemy commander")
	}
	if !strings.Contains(body, "rationale") {
		t.Error("review should show the general's rationale")
	}
	if w := do(s, "GET", "/review?turn=99", nil, false); w.Code != 200 {
		t.Errorf("out-of-range turn should clamp: %d", w.Code)
	}
	if w := do(s, "GET", "/review?turn=x", nil, false); w.Code != 400 {
		t.Errorf("bad turn: %d", w.Code)
	}
	d, _ := newServer(t, true)
	if w := do(d, "GET", "/review?turn=0", nil, false); w.Code != 200 {
		t.Errorf("review with --debug before the end: %d", w.Code)
	}
}

func TestRouteOverlay(t *testing.T) {
	s, _ := newServer(t, false)
	plain := do(s, "GET", "/map", nil, true).Body.String()
	routes := do(s, "GET", "/map?routes=1", nil, true).Body.String()
	if strings.Contains(plain, "<polyline") || !strings.Contains(routes, `class="route"`) {
		t.Error("route overlay should appear only with routes=1")
	}
	if !strings.Contains(routes, "to DV") || !strings.Contains(routes, "to IS") {
		t.Error("route labels missing")
	}
}

func TestTurnEventsStream(t *testing.T) {
	s, g := newServer(t, false)
	srv := httptest.NewServer(s)
	defer srv.Close()
	g.Lock() // keep the turn from finishing until the stream is listening
	go do(s, "POST", "/turn", url.Values{"draft-velk": {"March on Hollow Wood."}}, true)
	for !s.running.Load() {
	}
	resp, err := http.Get(srv.URL + "/turn/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type %q", ct)
	}
	g.Unlock()
	var got strings.Builder
	buf := make([]byte, 4096)
	for !strings.Contains(got.String(), "event: turn-done") {
		n, err := resp.Body.Read(buf)
		got.Write(buf[:n])
		if err != nil {
			break
		}
	}
	for _, want := range []string{"event: phase", "generals reading", "reports being written", "event: turn-done"} {
		if !strings.Contains(got.String(), want) {
			t.Errorf("stream missing %q:\n%s", want, got.String())
		}
	}
	s.progress.wait()
	if g.Turn() != 2 {
		t.Errorf("turn = %d after the stream ended", g.Turn())
	}
}
