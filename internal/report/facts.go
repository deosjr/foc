// Package report turns distorted facts into letters (via the LLM, with
// validation and a template fallback), and keeps the player's belief map,
// which is built only from delivered letters.
package report

import "strings"

// ReportFacts is the only input about the world the LLM receives. It is the
// distorted output of the personality policy.
type ReportFacts struct {
	General          string          `json:"general"`
	WrittenTurn      int             `json:"written_turn"`
	Location         string          `json:"location"`
	MovedFrom        string          `json:"moved_from,omitempty"`
	OrderUnderstood  string          `json:"order_understood"`
	OrderSource      string          `json:"order_source"`
	OrderOutcome     string          `json:"order_outcome"`
	OwnStrength      int             `json:"own_strength"`
	OwnLosses        int             `json:"own_losses"`
	Battles          []BattleFacts   `json:"battles,omitempty"`
	RetreatedTo      string          `json:"retreated_to,omitempty"`
	ArmyDestroyed    bool            `json:"army_destroyed,omitempty"`
	Sightings        []SightingFacts `json:"sightings"`
	NoEnemySeenIn    []string        `json:"no_enemy_seen_in"`
	FriendlyContacts []ContactFacts  `json:"friendly_contacts"`
	Concerns         []string        `json:"concerns"`
	Refused          bool            `json:"refused"`
	RefusedOrder     string          `json:"refused_order,omitempty"`
	Support          *SupportFacts   `json:"support,omitempty"`
	EntrenchedTurns  int             `json:"entrenched_turns,omitempty"` // 2 or more: fully dug in
	Scouted          string          `json:"scouted,omitempty"`
	Clarification    *ClarifyFacts   `json:"clarification_needed,omitempty"`
}

// SupportFacts is what became of the support a general was ordered to give.
type SupportFacts struct {
	General string `json:"general"`
	At      string `json:"at"`
	Result  string `json:"result"` // given | cut by an enemy attack | too far away
}

// ClarifyFacts is what a general could not make out of a letter.
type ClarifyFacts struct {
	LetterSentTurn int      `json:"letter_sent_turn"`
	YourLetter     string   `json:"your_letter"`
	Unclear        []string `json:"unclear"` // e.g. "where you would have me go"
}

// BattleFacts is one battle as reported.
type BattleFacts struct {
	Place       string `json:"place"`
	Result      string `json:"result"` // won | lost | stand-off
	EnemyLosses int    `json:"enemy_losses"`
}

// SightingFacts is reported enemy strength in a province.
type SightingFacts struct {
	Province      string `json:"province"`
	EnemyStrength int    `json:"enemy_strength"`
	Certainty     string `json:"certainty"` // estimate | certain
}

// ContactFacts is a friendly general seen nearby.
type ContactFacts struct {
	General  string `json:"general"`
	Province string `json:"province"`
}

// Numbers returns every integer stated in the facts.
func (f ReportFacts) Numbers() []int {
	nums := []int{f.WrittenTurn, f.OwnStrength, f.OwnLosses}
	if f.Clarification != nil {
		nums = append(nums, f.Clarification.LetterSentTurn)
	}
	for _, b := range f.Battles {
		nums = append(nums, b.EnemyLosses)
	}
	for _, s := range f.Sightings {
		nums = append(nums, s.EnemyStrength)
	}
	return nums
}

// Text returns all the free text in the facts, for finding mentioned names.
func (f ReportFacts) Text() string {
	parts := []string{f.General, f.Location, f.MovedFrom, f.OrderUnderstood, f.OrderSource, f.OrderOutcome, f.RetreatedTo}
	for _, b := range f.Battles {
		parts = append(parts, b.Place)
	}
	for _, s := range f.Sightings {
		parts = append(parts, s.Province)
	}
	parts = append(parts, f.NoEnemySeenIn...)
	for _, c := range f.FriendlyContacts {
		parts = append(parts, c.General, c.Province)
	}
	parts = append(parts, f.Concerns...)
	parts = append(parts, f.RefusedOrder, f.Scouted)
	if f.Support != nil {
		parts = append(parts, f.Support.General, f.Support.At)
	}
	if c := f.Clarification; c != nil {
		parts = append(parts, c.YourLetter)
		parts = append(parts, c.Unclear...)
	}
	return strings.Join(parts, "\n")
}
