package generals

import (
	"fmt"

	"github.com/deosjr/foc/internal/engine"
	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

// Fires reports whether a general's last observation meets a trigger. It
// uses only what he saw, so a place out of his sight can never trigger.
func Fires(tr model.Trigger, obs *engine.Observation) bool {
	if obs == nil || obs.Disbanded {
		return false
	}
	switch tr.Kind {
	case model.EnemyAt:
		for _, s := range obs.Sightings {
			if tr.Place == "" || s.Province == tr.Place {
				return true
			}
		}
		for _, b := range obs.Battles {
			if tr.Place == "" || b.Place == tr.Place || b.Road == tr.Place {
				return true
			}
		}
	case model.Attacked:
		if obs.RetreatedTo != "" || obs.KeptCamp {
			return true
		}
		for _, b := range obs.Battles {
			// Defending where he stood, or meeting the enemy on the road.
			if b.Road != "" || b.Place == obs.Start {
				return true
			}
		}
	case model.Outnumbered:
		seen := 0
		for _, s := range obs.Sightings {
			seen += s.Strength
		}
		return seen > obs.Strength
	case model.PlaceLost:
		place := tr.Place
		if place == "" {
			place = obs.Start
		}
		if obs.RetreatedTo != "" && obs.Start == place {
			return true
		}
		for _, s := range obs.Sightings {
			if s.Province == place {
				return true
			}
		}
	}
	return false
}

// DescribeTrigger renders a trigger in plain words: "the enemy is seen at
// Oros Ford".
func DescribeTrigger(tr model.Trigger, m *mapdata.Map) string {
	place := m.NameOf(tr.Place)
	switch tr.Kind {
	case model.EnemyAt:
		if tr.Place == "" {
			return "the enemy comes near"
		}
		return "the enemy is seen at " + place
	case model.Attacked:
		return "we are attacked"
	case model.Outnumbered:
		return "the enemy comes in greater force than ours"
	case model.PlaceLost:
		if tr.Place == "" {
			return "our position falls"
		}
		return place + " falls to the enemy"
	}
	return string(tr.Kind)
}

// DescribeAction renders a watch's "then" action: "march toward Oros Ford".
func DescribeAction(w *model.Contingency, m *mapdata.Map) string {
	switch w.Action {
	case ActMove:
		return "march toward " + m.NameOf(w.Target)
	case ActRetreat:
		if w.Target == "" {
			return "fall back"
		}
		return "fall back toward " + m.NameOf(w.Target)
	case ActScout:
		return "scout " + m.NameOf(w.Target)
	case ActSupport:
		return "support " + w.Whom
	case ActEntrench:
		return "dig in"
	}
	return "hold position"
}

// DescribeWatch renders a whole watch: "if the enemy is seen at Oros Ford,
// march toward Oros Ford".
func DescribeWatch(w *model.Contingency, m *mapdata.Map) string {
	return fmt.Sprintf("if %s, %s", DescribeTrigger(w.Trigger, m), DescribeAction(w, m))
}
