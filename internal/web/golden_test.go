package web

import (
	"bytes"
	"context"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Key fragments rendered from the fixed PlayerView after one scripted turn.
func TestGoldenFragments(t *testing.T) {
	s, g := newServer(t, false)
	g.Lock()
	g.SetDraft("velk", "March on Hollow Wood.")
	g.SetDraft("saris", "Hold Duna Hills.")
	if err := g.EndTurn(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	g.SetDraft("velk", "Press on to Marren and Sarnoss.")
	v := BuildView(g, false, "", false)
	g.Unlock()
	d := pageData{PlayerView: v}
	cases := map[string]struct {
		tmpl string
		data any
	}{
		"map":        {"map", d},
		"composer":   {"composer", v.Generals[1]},
		"recognised": {"recognised", v.Generals[1]},
		"inbox-card": {"letter", letterData{InboxLetter: v.Inbox[0]}},
	}
	for name, c := range cases {
		var buf bytes.Buffer
		if err := s.tmpl.ExecuteTemplate(&buf, c.tmpl, c.data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		path := "../../testdata/golden/" + name + ".html"
		if *update {
			os.WriteFile(path, buf.Bytes(), 0o644)
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (run with -update)", err)
		}
		if !bytes.Equal(buf.Bytes(), want) {
			t.Errorf("%s differs from %s", name, path)
		}
	}
}
