package report

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// The template letter used by the mock LLM and as the fallback when a real
// LLM fails validation twice. It states the facts plainly and nothing else.
var fallbackTmpl = template.Must(template.New("fallback").Funcs(template.FuncMap{
	"men":  men,
	"join": joinAnd,
}).Parse(`To my sovereign,

Your orders as I understood them: {{.OrderUnderstood}} ({{.OrderSource}}). This season: {{.OrderOutcome}}.
{{- range .Battles}} At {{.Place}} we fought{{if eq .Result "won"}} and carried the day{{else if eq .Result "lost"}} and were beaten{{else}} to a stand-off{{end}}; the enemy lost {{men .EnemyLosses}}.{{end}}
{{- if .ArmyDestroyed}} My army is broken and scattered; I write this from among the few who remain.
{{- else}} I have {{men .OwnStrength}} under my command{{if .OwnLosses}}, having lost {{men .OwnLosses}}{{end}}.{{end}}
{{- range .Sightings}} {{if eq .Certainty "certain"}}There are {{men .EnemyStrength}} of the enemy in {{.Province}}.{{else}}I judge there are some {{men .EnemyStrength}} of the enemy in {{.Province}}.{{end}}{{end}}
{{- if .NoEnemySeenIn}} I see no enemy in {{join .NoEnemySeenIn}}.{{end}}
{{- range .FriendlyContacts}} {{.General}} is in {{.Province}}.{{end}}
{{- if .WatchFired}} {{.WatchFired}}.{{end}}
{{- if .Watching}} As you bade me, {{.Watching}}.{{end}}
{{- if .Refused}} I did not {{.RefusedOrder}}; I judged it would cost us the army.{{end}}
{{- range .Concerns}} {{.}}{{end}}
{{- with .Clarification}} I could not make out {{join .Unclear}} from your letter sent turn {{.LetterSentTurn}}, so I hold here until you write again.{{end}}
I await your word.

{{.General}}`))

// Fallback renders the template letter for a set of facts.
func Fallback(f ReportFacts) string {
	var buf bytes.Buffer
	if err := fallbackTmpl.Execute(&buf, f); err != nil {
		return fmt.Sprintf("To my sovereign,\n\nI hold at %s with %s.\n\n%s", f.Location, men(f.OwnStrength), f.General)
	}
	return buf.String()
}

// MenPerStrength is set from the ruleset at startup.
var MenPerStrength = 100

// men renders a strength as a count of men: 26 -> "2,600 men".
func men(strength int) string {
	n := strength * MenPerStrength
	s := fmt.Sprint(n)
	if n >= 1000 {
		s = fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return s + " men"
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
