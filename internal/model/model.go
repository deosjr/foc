// Package model holds the value types shared by the engine, generals,
// messaging and interpretation packages, so that none of them need to
// import each other.
package model

import "fmt"

// Side is the allegiance of an army or the owner of a province.
type Side string

const (
	Player  Side = "player"
	Enemy   Side = "enemy"
	Neutral Side = "neutral"
)

// Opponent returns the other fighting side.
func (s Side) Opponent() Side {
	switch s {
	case Player:
		return Enemy
	case Enemy:
		return Player
	}
	return Neutral
}

// OrderType is one of the closed set of orders the engine accepts.
type OrderType string

const (
	Hold       OrderType = "Hold"
	MoveToward OrderType = "MoveToward"
	Retreat    OrderType = "Retreat"
	// Later milestones; the engine rejects these for now.
	Support  OrderType = "Support"
	Entrench OrderType = "Entrench"
	Scout    OrderType = "Scout"
)

// Order is a structured order for one army.
type Order struct {
	ArmyID        string    `json:"army"`
	Type          OrderType `json:"type"`
	Target        string    `json:"target,omitempty"` // province id, if any
	SupportArmyID string    `json:"support,omitempty"`
	// Stance is the army's attitude to battle. StanceRefuse: it keeps to a
	// fortified camp and will not offer battle; "" accepts battle.
	Stance string `json:"stance,omitempty"`
}

// StanceRefuse marks an army that avoids battle.
const StanceRefuse = "refuse"

func (o Order) String() string {
	if o.Target != "" {
		return fmt.Sprintf("%s %s", o.Type, o.Target)
	}
	return string(o.Type)
}

// Traits are a general's personality, each in [0, 1]. They are applied only
// by Go code and are never shown to a model.
type Traits struct {
	Aggression float64 `yaml:"aggression" json:"aggression"`
	Caution    float64 `yaml:"caution" json:"caution"`
	Initiative float64 `yaml:"initiative" json:"initiative"`
	Honesty    float64 `yaml:"honesty" json:"honesty"`
	Vanity     float64 `yaml:"vanity" json:"vanity"`
	Loyalty    float64 `yaml:"loyalty" json:"loyalty"`
}

// General is a player commander. Bio is shown to the player; Voice only to the LLM.
type General struct {
	ID     string `yaml:"id" json:"id"`
	Name   string `yaml:"name" json:"name"`
	Bio    string `yaml:"bio" json:"bio"`
	Voice  string `yaml:"voice" json:"voice"`
	Traits Traits `yaml:"traits" json:"traits"`
	ArmyID string `yaml:"-" json:"army"`
}

// Sovereign is the From/To address of the player in letters.
const Sovereign = "sovereign"

// LetterKind distinguishes letters to and from the field.
type LetterKind string

const (
	Dispatch      LetterKind = "dispatch"
	Report        LetterKind = "report"
	Clarification LetterKind = "clarification"
)

// Letter is anything a courier carries.
type Letter struct {
	ID          string     `json:"id"`
	Seq         int        `json:"seq"`
	Kind        LetterKind `json:"kind"`
	From        string     `json:"from"`
	To          string     `json:"to"`
	Body        string     `json:"body"`
	SentTurn    int        `json:"sent_turn"`
	ArriveTurn  int        `json:"arrive_turn"`
	Route       []string   `json:"route"`
	Intercepted bool       `json:"intercepted"` // truth only; never shown during play
}

// TriggerKind is what a conditional order waits for.
type TriggerKind string

const (
	EnemyAt     TriggerKind = "enemy_at"    // enemy seen in a place (or anywhere near, with no place)
	Attacked    TriggerKind = "attacked"    // the general's army was attacked or met the enemy in the field
	Outnumbered TriggerKind = "outnumbered" // more enemy seen nearby than he has men
	PlaceLost   TriggerKind = "place_lost"  // he knows a place has fallen to the enemy
)

// Trigger is a condition a general watches for, judged only from what he
// himself has seen.
type Trigger struct {
	Kind  TriggerKind `json:"kind"`
	Place string      `json:"place,omitempty"` // province id; "" means "near me" / "where I stand"
}

// Contingency is the "if X, then Y" half of a conditional order. It fires
// once, the turn after the general sees the condition met, and is replaced
// by any new letter that gives an order. The "then" order is rebuilt from
// Action/Target/Whom when it fires, from wherever the general is by then.
type Contingency struct {
	Trigger  Trigger `json:"trigger"`
	Action   string  `json:"action"`           // hold | move | support | entrench | scout | retreat
	Target   string  `json:"target,omitempty"` // province id
	Whom     string  `json:"whom,omitempty"`   // name of the general to support
	SetTurn  int     `json:"set_turn"`
	LetterID string  `json:"letter"`
}
