// Package web serves the game as server-rendered HTML with htmx. Templates
// only ever receive a PlayerView (or, with --debug, truth pages).
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/deosjr/foc/internal/game"
	"github.com/deosjr/foc/internal/report"
	webfs "github.com/deosjr/foc/web"
)

// Server is the HTTP front end for one game.
type Server struct {
	g       *game.Game
	debug   bool
	tmpl    *template.Template
	running atomic.Bool
	mux     *http.ServeMux
	timeout time.Duration
}

// New builds the server. debug enables the truth routes and the debug drawer.
func New(g *game.Game, debug bool) (*Server, error) {
	t, err := template.New("").Funcs(funcs).ParseFS(webfs.FS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{g: g, debug: debug, tmpl: t, mux: http.NewServeMux(), timeout: 5 * time.Minute}
	static, err := fs.Sub(webfs.FS, "static")
	if err != nil {
		return nil, err
	}
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /{$}", s.index)
	s.mux.HandleFunc("GET /map", s.mapFragment)
	s.mux.HandleFunc("GET /province/{id}", s.province)
	s.mux.HandleFunc("PUT /drafts/{generalId}", s.putDraft)
	s.mux.HandleFunc("DELETE /drafts/{generalId}", s.deleteDraft)
	s.mux.HandleFunc("GET /letters", s.letters)
	s.mux.HandleFunc("GET /inbox", s.inbox)
	s.mux.HandleFunc("GET /letter/{id}", s.letter)
	s.mux.HandleFunc("POST /turn", s.endTurn)
	s.mux.HandleFunc("POST /save", s.save)
	if debug {
		s.mux.HandleFunc("GET /debug/truth", s.truth)
	}
	return s, nil
}

var funcs = template.FuncMap{
	"pct": func(p float64) string { return fmt.Sprintf("%.0f%%", p*100) },
	"f2":  func(p float64) string { return fmt.Sprintf("%.2f", p) },
	"sub": func(a, b float64) float64 { return a - b },
	"add": func(a, b float64) float64 { return a + b },
	"join": func(s []string) string {
		return strings.Join(s, ", ")
	},
	"men":   func(n int) string { return fmt.Sprint(n * report.MenPerStrength) },
	"deref": func(p *int) int { return *p },
	"card": func(l game.InboxLetter, debug map[string]*DebugView) letterData {
		return letterData{InboxLetter: l, Debug: debug[l.ID]}
	},
}

// ServeHTTP adds security headers and a body limit to every response.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; form-action 'self'; base-uri 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	s.mux.ServeHTTP(w, r)
}

// render writes a fragment for htmx requests, or the full page otherwise, so
// every view also works as a plain link.
func (s *Server) render(w http.ResponseWriter, r *http.Request, fragment string, data any) {
	name := fragment
	if r.Header.Get("HX-Request") != "true" {
		if _, ok := data.(pageData); !ok {
			// Fragments of a composer make no sense on their own.
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		name = "page"
	}
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

// pageData is what the page template gets: the view plus which fragment
// (if any) the non-htmx request asked for, and the province detail.
type pageData struct {
	PlayerView
	DebugMode bool
	Province  *ProvinceDetail
	Notice    string
}

func (s *Server) view(r *http.Request) pageData {
	s.g.Lock()
	defer s.g.Unlock()
	return pageData{PlayerView: BuildView(s.g, s.debug, r.URL.Query().Get("highlight")), DebugMode: s.debug}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "page", s.view(r))
}

func (s *Server) mapFragment(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "map", s.view(r))
}

func (s *Server) letters(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "letters", s.view(r))
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "inbox", s.view(r))
}

// ProvinceDetail is the belief entry for one province, for the detail box.
type ProvinceDetail struct {
	ID, Name, Terrain string
	Supply, Capital   bool
	Entry             report.BeliefEntry
	Source            string // who the news came from
	Generals          []string
}

func (s *Server) province(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d := s.view(r)
	p := s.g.Map.Province(id)
	if p == nil {
		http.NotFound(w, r)
		return
	}
	e := d.Belief.Provinces[id]
	pd := &ProvinceDetail{ID: id, Name: p.Name, Terrain: p.Terrain, Supply: p.Supply, Capital: p.Capital, Entry: *e}
	for _, gv := range d.Generals {
		if gv.ID == e.Source {
			pd.Source = gv.Name
		}
	}
	for _, gv := range d.Generals {
		if gv.Province == id && !gv.Destroyed {
			pd.Generals = append(pd.Generals, fmt.Sprintf("%s (as of turn %d)", gv.Name, gv.AsOfTurn))
		}
	}
	d.Province = pd
	s.render(w, r, "province", d)
}

func (s *Server) generalView(d pageData, id string) (GeneralView, bool) {
	for _, gv := range d.Generals {
		if gv.ID == id {
			return gv, true
		}
	}
	return GeneralView{}, false
}

func (s *Server) putDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("generalId")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.g.Lock()
	err := s.g.SetDraft(id, r.PostForm.Get("draft-"+id))
	s.g.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	gv, ok := s.generalView(s.view(r), id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, "recognised", gv)
}

func (s *Server) deleteDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("generalId")
	s.g.Lock()
	err := s.g.SetDraft(id, "")
	s.g.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	d := s.view(r)
	gv, ok := s.generalView(d, id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, "composer", gv)
}

func (s *Server) letter(w http.ResponseWriter, r *http.Request) {
	d := s.view(r)
	id := r.PathValue("id")
	for _, l := range d.Inbox {
		if l.ID == id {
			if r.Header.Get("HX-Request") != "true" {
				s.render(w, r, "page", d)
				return
			}
			s.render(w, r, "letter", letterData{InboxLetter: l, Debug: d.Debug[id]})
			return
		}
	}
	http.NotFound(w, r)
}

type letterData struct {
	game.InboxLetter
	Debug *DebugView
}

// endTurn seals the drafts (including any the browser had not saved yet)
// and runs the turn. A second call while a turn runs is rejected.
func (s *Server) endTurn(w http.ResponseWriter, r *http.Request) {
	if !s.running.CompareAndSwap(false, true) {
		http.Error(w, "a turn is already being resolved", http.StatusConflict)
		return
	}
	defer s.running.Store(false)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s.g.Lock()
	for _, id := range s.g.GeneralIDs() {
		if text, ok := r.PostForm["draft-"+id]; ok && len(text) > 0 {
			if err := s.g.SetDraft(id, text[0]); err != nil && err != game.ErrOver {
				s.g.Unlock()
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	err := s.g.EndTurn(ctx, nil)
	cancel()
	s.g.Unlock()
	if err != nil {
		code := http.StatusInternalServerError
		if err == game.ErrOver {
			code = http.StatusConflict
		}
		http.Error(w, err.Error(), code)
		return
	}
	s.render(w, r, "game", s.view(r))
}

func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	s.g.Lock()
	dir, err := s.g.Save()
	s.g.Unlock()
	d := s.view(r)
	if err != nil {
		d.Notice = "Could not save: " + err.Error()
	} else {
		d.Notice = "Saved to " + dir
	}
	s.render(w, r, "notice", d)
}

func (s *Server) truth(w http.ResponseWriter, r *http.Request) {
	s.g.Lock()
	b, err := json.MarshalIndent(s.g.DebugTruth(), "", "  ")
	s.g.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, "truth", string(b)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

func factsJSON(f report.ReportFacts) string {
	b, _ := json.MarshalIndent(f, "", "  ")
	return string(b)
}
