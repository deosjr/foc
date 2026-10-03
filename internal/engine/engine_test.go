package engine

import (
	"math/rand/v2"
	"testing"

	"github.com/deosjr/foc/internal/mapdata"
	"github.com/deosjr/foc/internal/model"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	m, err := mapdata.LoadMap("../../maps/valley.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := mapdata.LoadRuleset("../../rulesets/ancient.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return &Engine{Map: m, Rules: r}
}

// state builds a state on the valley map with only the given armies.
func state(e *Engine, armies ...*Army) *GameState {
	s := e.NewState(&mapdata.Scenario{TurnCap: 10})
	for _, a := range armies {
		s.Armies[a.ID] = a
	}
	return s
}

func p(id, loc string, str int) *Army {
	return &Army{ID: id, Side: model.Player, Location: loc, Strength: str}
}
func en(id, loc string, str int) *Army {
	return &Army{ID: id, Side: model.Enemy, Location: loc, Strength: str}
}
func move(id, to string) model.Order {
	return model.Order{ArmyID: id, Type: model.MoveToward, Target: to}
}
func retreat(id, to string) model.Order {
	return model.Order{ArmyID: id, Type: model.Retreat, Target: to}
}

type want struct {
	loc      string
	strength int
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name      string
		armies    []*Army
		orders    []model.Order
		want      map[string]want // army id -> expected; missing army means disbanded
		disbanded []string
		battles   int
		owners    map[string]model.Side
	}{
		{
			name:   "uncontested move captures",
			armies: []*Army{p("a", "velia", 30)},
			orders: []model.Order{move("a", "oros")},
			want:   map[string]want{"a": {"oros", 30}},
			owners: map[string]model.Side{"oros": model.Player},
		},
		{
			name:   "move toward far target takes one step along the lowest-id shortest path",
			armies: []*Army{p("a", "velia", 30)},
			orders: []model.Order{move("a", "marren")},
			want:   map[string]want{"a": {"hollow", 30}},
		},
		{
			name:    "tie into empty province is a stand-off",
			armies:  []*Army{p("a", "velia", 20), en("e", "marren", 20)},
			orders:  []model.Order{move("a", "hollow"), move("e", "hollow")},
			want:    map[string]want{"a": {"velia", 18}, "e": {"marren", 18}},
			battles: 1,
			owners:  map[string]model.Side{"hollow": model.Neutral},
		},
		{
			name:    "terrain multiplier makes a weaker defender hold",
			armies:  []*Army{p("a", "velia", 30), en("e", "hollow", 26)},
			orders:  []model.Order{move("a", "hollow")},
			want:    map[string]want{"a": {"velia", 21}, "e": {"hollow", 23}},
			battles: 1,
		},
		{
			name:    "terrain tie is a stand-off and the defender stays",
			armies:  []*Army{p("a", "velia", 30), en("e", "hollow", 25)},
			orders:  []model.Order{move("a", "hollow")},
			want:    map[string]want{"a": {"velia", 27}, "e": {"hollow", 22}},
			battles: 1,
		},
		{
			name:    "dislodged defender retreats toward its capital",
			armies:  []*Army{p("a", "velia", 30), en("e", "hollow", 24)},
			orders:  []model.Order{move("a", "hollow")},
			want:    map[string]want{"a": {"hollow", 27}, "e": {"marren", 17}},
			battles: 1,
			owners:  map[string]model.Side{"hollow": model.Player, "marren": model.Enemy},
		},
		{
			name: "dislodged defender with no free neighbour disbands",
			armies: []*Army{
				p("a", "velia", 30), p("b", "velia", 10), p("c", "marren", 10), en("e", "oros", 10),
			},
			orders:    []model.Order{move("a", "oros")},
			want:      map[string]want{"a": {"oros", 27}, "b": {"velia", 10}, "c": {"marren", 10}},
			disbanded: []string{"e"},
			battles:   1,
		},
		{
			name:      "army falling below 5 disbands",
			armies:    []*Army{p("a", "velia", 30), en("e", "hollow", 6)},
			orders:    []model.Order{move("a", "hollow")},
			want:      map[string]want{"a": {"hollow", 27}},
			disbanded: []string{"e"},
			battles:   1,
		},
		{
			name:   "head-to-head: one field battle on the road; nobody advances",
			armies: []*Army{p("a", "velia", 30), en("e", "hollow", 20)},
			orders: []model.Order{move("a", "hollow"), move("e", "velia")},
			// Field: 30 v 20, a -> 27 (winner, holds the road), e -> 14 (falls back).
			want:    map[string]want{"a": {"velia", 27}, "e": {"hollow", 14}},
			battles: 1,
		},
		{
			name:   "marching out of the capital to meet the enemy does not lose it",
			armies: []*Army{p("a", "karsa", 25), en("e", "duna", 30)},
			orders: []model.Order{move("a", "hollow"), move("e", "karsa")},
			// They meet on the road between Karsa and Duna Hills: 25 v 30.
			want:    map[string]want{"a": {"karsa", 17}, "e": {"duna", 27}},
			battles: 1,
			owners:  map[string]model.Side{"karsa": model.Player},
		},
		{
			name:   "friendly swap passes freely",
			armies: []*Army{p("a", "velia", 30), p("b", "karsa", 20)},
			orders: []model.Order{move("a", "karsa"), move("b", "velia")},
			want:   map[string]want{"a": {"karsa", 30}, "b": {"velia", 20}},
		},
		{
			name:   "retreat blocked by an enemy-held target",
			armies: []*Army{p("a", "velia", 30), en("e", "hollow", 10)},
			orders: []model.Order{retreat("a", "hollow")},
			want:   map[string]want{"a": {"velia", 30}, "e": {"hollow", 10}},
		},
		{
			name:   "retreat to a free province",
			armies: []*Army{p("a", "duna", 30), en("e", "hollow", 30)},
			orders: []model.Order{retreat("a", "karsa"), move("e", "duna")},
			want:   map[string]want{"a": {"karsa", 30}, "e": {"duna", 30}},
			owners: map[string]model.Side{"duna": model.Enemy},
		},
		{
			name: "a bounced attacker defends its home against a second attack",
			armies: []*Army{
				p("a", "hollow", 10), en("e1", "marren", 30), en("e2", "velia", 10),
			},
			orders: []model.Order{move("a", "marren"), move("e2", "hollow")},
			// Marren: 10 v 30*1.0 -> a bounces (a -> 7). Hollow: a defends 10*1.2 v 10 -> e2 bounces.
			want:    map[string]want{"a": {"hollow", 6}, "e1": {"marren", 27}, "e2": {"velia", 7}},
			battles: 2,
		},
		{
			name:   "support adds half the supporter's strength to a move",
			armies: []*Army{p("a", "velia", 20), p("b", "duna", 20), en("e", "hollow", 25)},
			orders: []model.Order{move("a", "hollow"), supp("b", "a")},
			// 20 + 10 = 30 v 25*1.2 = 30: a stand-off, where alone it would lose.
			want:    map[string]want{"a": {"velia", 18}, "b": {"duna", 20}, "e": {"hollow", 22}},
			battles: 1,
		},
		{
			name:   "supported attack dislodges",
			armies: []*Army{p("a", "velia", 20), p("b", "duna", 30), en("e", "hollow", 25)},
			orders: []model.Order{move("a", "hollow"), supp("b", "a")},
			// 20 + 15 = 35 > 30: e dislodged, retreats to marren.
			want:    map[string]want{"a": {"hollow", 18}, "b": {"duna", 30}, "e": {"marren", 17}},
			battles: 1,
		},
		{
			name:   "support is cut when the supporter is attacked",
			armies: []*Army{p("a", "velia", 20), p("b", "duna", 30), en("e", "hollow", 25), en("f", "lyde", 10)},
			orders: []model.Order{move("a", "hollow"), supp("b", "a"), move("f", "duna")},
			// b is attacked from Lyde, so no support: 20 v 30, a bounces.
			want:    map[string]want{"a": {"velia", 14}, "b": {"duna", 27}, "e": {"hollow", 22}, "f": {"lyde", 7}},
			battles: 2,
		},
		{
			name:   "support to hold",
			armies: []*Army{p("a", "duna", 20), p("b", "karsa", 20), en("e", "hollow", 30)},
			orders: []model.Order{supp("b", "a"), move("e", "duna")},
			// a defends 20*1.25 + 10 = 35 v 30.
			want:    map[string]want{"a": {"duna", 18}, "b": {"karsa", 20}, "e": {"hollow", 21}},
			battles: 1,
		},
		{
			name:   "support from too far does nothing",
			armies: []*Army{p("a", "duna", 20), p("b", "oros", 30), en("e", "hollow", 30)},
			orders: []model.Order{supp("b", "a"), move("e", "duna")},
			// Oros is not next to Duna Hills: 25 v 30, a dislodged.
			want:    map[string]want{"a": {"karsa", 14}, "b": {"oros", 30}, "e": {"duna", 27}},
			battles: 1,
		},
		{
			name:   "a dislodged army never retreats into the province its attacker left",
			armies: []*Army{p("a", "duna", 10), en("e", "hollow", 30), en("f", "ilth", 30)},
			orders: []model.Order{move("e", "duna")},
			// Neighbours of Duna: hollow (attacker came from), karsa, lyde. Karsa is nearest home.
			want:    map[string]want{"a": {"karsa", 7}, "e": {"duna", 27}},
			battles: 1,
		},
		{
			name:   "friendly armies may share a province",
			armies: []*Army{p("a", "velia", 30), p("b", "karsa", 20)},
			orders: []model.Order{move("b", "velia")},
			want:   map[string]want{"a": {"velia", 30}, "b": {"velia", 20}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := testEngine(t)
			s := state(e, c.armies...)
			res, err := e.Resolve(s, c.orders)
			if err != nil {
				t.Fatal(err)
			}
			for id, w := range c.want {
				a := s.Armies[id]
				if a == nil {
					t.Errorf("army %s disbanded, want %v", id, w)
					continue
				}
				if a.Location != w.loc || a.Strength != w.strength {
					t.Errorf("army %s at %s with %d, want %s with %d", id, a.Location, a.Strength, w.loc, w.strength)
				}
			}
			for _, id := range c.disbanded {
				if s.Armies[id] != nil {
					t.Errorf("army %s should have disbanded", id)
				}
				if !res.WasDisbanded(id) {
					t.Errorf("result does not record %s as disbanded", id)
				}
			}
			if len(res.Battles) != c.battles {
				t.Errorf("got %d battles, want %d: %+v", len(res.Battles), c.battles, res.Battles)
			}
			for prov, side := range c.owners {
				if got := s.Provinces[prov].Owner; got != side {
					t.Errorf("%s owned by %s, want %s", prov, got, side)
				}
			}
		})
	}
}

func TestResolveRejectsBadOrders(t *testing.T) {
	e := testEngine(t)
	for _, o := range []model.Order{
		{ArmyID: "nope", Type: model.Hold},
		move("a", "atlantis"),
		retreat("a", "marren"),                             // not adjacent
		{ArmyID: "a", Type: model.Scout, Target: "marren"}, // not adjacent
		{ArmyID: "a", Type: model.Support, SupportArmyID: "a"},
		{ArmyID: "a", Type: "Charge"},
	} {
		s := state(e, p("a", "velia", 30))
		if _, err := e.Resolve(s, []model.Order{o}); err == nil {
			t.Errorf("order %+v accepted", o)
		}
	}
}

func TestEvaluate(t *testing.T) {
	e := testEngine(t)
	// endTurns resolves and evaluates n turns with no orders.
	endTurns := func(s *GameState, n int) {
		for i := 0; i < n && !s.Over; i++ {
			e.Resolve(s, nil)
			e.Evaluate(s, 10)
			if !s.Over {
				s.Turn++
			}
		}
	}
	t.Run("taking the enemy capital wins once held", func(t *testing.T) {
		s := state(e, p("a", "sarnos", 30))
		e.Resolve(s, []model.Order{move("a", "kethra")})
		e.Evaluate(s, 10)
		if s.Over {
			t.Fatal("won the turn it was taken; it must be held")
		}
		s.Turn++
		endTurns(s, 1)
		if !s.Over || s.Winner != model.Player {
			t.Errorf("got over=%v winner=%s", s.Over, s.Winner)
		}
	})
	t.Run("losing the capital loses once held", func(t *testing.T) {
		s := state(e, p("a", "oros", 30), en("e", "duna", 30))
		e.Resolve(s, []model.Order{move("e", "karsa")})
		e.Evaluate(s, 10)
		if s.Over {
			t.Fatal("lost the turn Karsa fell; the player gets a turn to retake it")
		}
		s.Turn++
		endTurns(s, 1)
		if !s.Over || s.Winner != model.Enemy {
			t.Errorf("got over=%v winner=%s", s.Over, s.Winner)
		}
	})
	t.Run("retaking the capital resets the count", func(t *testing.T) {
		s := state(e, p("a", "velia", 40), en("e", "duna", 30))
		e.Resolve(s, []model.Order{move("e", "karsa")})
		e.Evaluate(s, 10)
		s.Turn++
		e.Resolve(s, []model.Order{move("a", "karsa")}) // 40 v 30*1.5 = 45: fails...
		e.Evaluate(s, 10)
		if !s.Over {
			t.Fatal("control: an unsuccessful counterattack leaves Karsa lost")
		}
		s2 := state(e, p("a", "velia", 50), en("e", "duna", 30))
		e.Resolve(s2, []model.Order{move("e", "karsa")})
		e.Evaluate(s2, 10)
		s2.Turn++
		e.Resolve(s2, []model.Order{move("a", "karsa")}) // 50 > 45: retaken
		e.Evaluate(s2, 10)
		if s2.Over || s2.Streak[model.Enemy] != 0 {
			t.Errorf("retaken: over=%v streak=%d", s2.Over, s2.Streak[model.Enemy])
		}
	})
	t.Run("enough supply centres win once held", func(t *testing.T) {
		e := testEngine(t)
		e.Rules.Victory.SupplyCentresToWin = 4
		s := state(e, p("a", "marren", 30), p("b", "sarnos", 30))
		endTurns := func(s *GameState, n int) {
			for i := 0; i < n && !s.Over; i++ {
				e.Resolve(s, nil)
				e.Evaluate(s, 10)
				if !s.Over {
					s.Turn++
				}
			}
		}
		endTurns(s, 2)
		if !s.Over || s.Winner != model.Player {
			t.Errorf("got over=%v winner=%s outcome=%q", s.Over, s.Winner, s.Outcome)
		}
	})
	t.Run("turn cap ends the game on supply count", func(t *testing.T) {
		s := state(e, p("a", "velia", 30))
		s.Turn = 10
		e.Evaluate(s, 10)
		if !s.Over || s.Winner != model.Neutral {
			t.Errorf("got over=%v winner=%s outcome=%q", s.Over, s.Winner, s.Outcome)
		}
	})
	t.Run("game continues otherwise", func(t *testing.T) {
		s := state(e, p("a", "velia", 30))
		e.Evaluate(s, 10)
		if s.Over {
			t.Errorf("game over: %q", s.Outcome)
		}
	})
}

func TestMusters(t *testing.T) {
	e := testEngine(t)
	// Turn 4 is winter. The player holds Karsa and Velia (2 x 3); the enemy
	// holds Sarnos and Kethra (2 x 3) but has nobody in Kethra.
	s := state(e, p("a", "velia", 20), p("b", "oros", 20), en("e", "sarnos", 20))
	s.Turn = 4
	res, err := e.Resolve(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Armies["a"].Strength != 26 || s.Armies["b"].Strength != 20 {
		t.Errorf("player levies should join the army nearest Karsa: a=%d b=%d", s.Armies["a"].Strength, s.Armies["b"].Strength)
	}
	levy := s.Armies["e-levy-4"]
	if levy == nil || levy.Location != "kethra" || levy.Strength != 6 {
		t.Errorf("enemy should raise a levy in its empty capital: %+v", levy)
	}
	if len(res.Musters) != 2 {
		t.Errorf("musters = %+v", res.Musters)
	}
	obs := e.Perceive(s, res, "g", "a", nil)
	if obs.Reinforced != 6 || obs.Losses != 0 {
		t.Errorf("observation: reinforced %d, losses %d", obs.Reinforced, obs.Losses)
	}
	// Not winter: nothing.
	s.Turn = 5
	if res, _ := e.Resolve(s, nil); len(res.Musters) != 0 {
		t.Errorf("musters outside winter: %+v", res.Musters)
	}
}

func TestPerceive(t *testing.T) {
	e := testEngine(t)
	s := state(e, p("a", "velia", 30), en("e", "hollow", 24), p("b", "karsa", 20))
	s.Armies["b"].GeneralID = "saris"
	res, err := e.Resolve(s, []model.Order{move("a", "hollow")})
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	obs := e.Perceive(s, res, "velk", "a", rng)
	if obs.Location != "hollow" || obs.Strength != 27 || obs.Losses != 3 {
		t.Errorf("own state wrong: %+v", obs)
	}
	if len(obs.Battles) != 1 || obs.Battles[0].Result != "won" || obs.Battles[0].TrueEnemyLosses != 7 {
		t.Fatalf("battle wrong: %+v", obs.Battles)
	}
	if l := obs.Battles[0].EnemyLosses; l < 5 || l > 9 {
		t.Errorf("noisy enemy losses %d outside ±20%% of 7", l)
	}
	if len(obs.Sightings) != 1 || obs.Sightings[0].Province != "marren" {
		t.Fatalf("sightings wrong: %+v", obs.Sightings)
	}
	if st := obs.Sightings[0].Strength; st%5 != 0 || st < 10 || st > 25 {
		t.Errorf("sighted strength %d not a noisy multiple of 5 near 17", st)
	}
	// Same seed, same observation.
	s2 := state(e, p("a", "velia", 30), en("e", "hollow", 24), p("b", "karsa", 20))
	s2.Armies["b"].GeneralID = "saris"
	res2, _ := e.Resolve(s2, []model.Order{move("a", "hollow")})
	obs2 := e.Perceive(s2, res2, "velk", "a", rand.New(rand.NewPCG(1, 2)))
	if obs2.Sightings[0].Strength != obs.Sightings[0].Strength || obs2.Battles[0].EnemyLosses != obs.Battles[0].EnemyLosses {
		t.Error("perception is not deterministic for a fixed seed")
	}
	// Hollow neighbours duna, marren, velia: duna and velia are quiet.
	if len(obs.Quiet) != 2 {
		t.Errorf("quiet = %v", obs.Quiet)
	}
}

func TestEntrenchFromTheSecondTurn(t *testing.T) {
	e := testEngine(t)
	// 24 on plain Velia against 30: first turn of entrenching gives nothing.
	s := state(e, p("a", "velia", 24), en("e", "hollow", 26))
	if _, err := e.Resolve(s, []model.Order{entrench("a")}); err != nil {
		t.Fatal(err)
	}
	if s.Armies["a"].Entrenched != 1 {
		t.Fatalf("entrenched = %d after one turn", s.Armies["a"].Entrenched)
	}
	// Second turn: 24 * (1.0 + 0.25) = 30 > 26, the attack fails.
	if _, err := e.Resolve(s, []model.Order{entrench("a"), move("e", "velia")}); err != nil {
		t.Fatal(err)
	}
	if a := s.Armies["a"]; a == nil || a.Location != "velia" || a.Entrenched != 2 {
		t.Fatalf("entrenched army did not hold: %+v", a)
	}
	// Without the bonus the same attack would have won.
	s2 := state(e, p("a", "velia", 24), en("e", "hollow", 26))
	e.Resolve(s2, []model.Order{move("e", "velia")})
	if s2.Armies["a"] != nil && s2.Armies["a"].Location == "velia" {
		t.Error("control: an unentrenched army should have been dislodged")
	}
	// Any other order resets the count.
	e.Resolve(s, nil)
	if s.Armies["a"].Entrenched != 0 {
		t.Errorf("entrenchment should reset, got %d", s.Armies["a"].Entrenched)
	}
}

func supp(id, whom string) model.Order {
	return model.Order{ArmyID: id, Type: model.Support, SupportArmyID: whom}
}
func entrench(id string) model.Order { return model.Order{ArmyID: id, Type: model.Entrench} }

func refuse(o model.Order) model.Order { o.Stance = model.StanceRefuse; return o }

func TestRefusingBattle(t *testing.T) {
	e := testEngine(t)
	hold := func(id string) model.Order { return model.Order{ArmyID: id, Type: model.Hold} }

	t.Run("an attack too weak to storm the camp is called off", func(t *testing.T) {
		// 24 in camp in the Duna Hills: 24 * (1.25 + 0.5) = 42 against 40.
		s := state(e, p("a", "duna", 24), en("e", "hollow", 40))
		res, err := e.Resolve(s, []model.Order{refuse(hold("a")), move("e", "duna")})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Battles) != 0 || len(res.Declined) != 1 {
			t.Fatalf("battles %v, declined %v", res.Battles, res.Declined)
		}
		if s.Armies["a"].Strength != 24 || s.Armies["e"].Strength != 40 || s.Armies["e"].Location != "hollow" {
			t.Errorf("nobody should lose anyone: a=%+v e=%+v", s.Armies["a"], s.Armies["e"])
		}
		if b, _ := res.BounceOf("e"); b.Reason != "camp" {
			t.Errorf("attacker bounce reason = %q", b.Reason)
		}
		if !res.KeptCamp("a") {
			t.Error("defender should be recorded as keeping to its camp")
		}
	})
	t.Run("control: without the camp the same attack wins", func(t *testing.T) {
		s := state(e, p("a", "duna", 24), en("e", "hollow", 40))
		e.Resolve(s, []model.Order{hold("a"), move("e", "duna")})
		if s.Armies["e"].Location != "duna" {
			t.Error("an army accepting battle should have been dislodged")
		}
	})
	t.Run("a strong enough attack storms the camp", func(t *testing.T) {
		s := state(e, p("a", "duna", 24), en("e", "hollow", 45))
		res, _ := e.Resolve(s, []model.Order{refuse(hold("a")), move("e", "duna")})
		if len(res.Battles) != 1 || s.Armies["e"].Location != "duna" {
			t.Errorf("45 > 42 should storm the camp: battles %v", res.Battles)
		}
	})
	t.Run("there is no refusing battle on an open plain", func(t *testing.T) {
		s := state(e, p("a", "velia", 24), en("e", "hollow", 30))
		res, _ := e.Resolve(s, []model.Order{refuse(hold("a")), move("e", "velia")})
		if len(res.Declined) != 0 || s.Armies["e"].Location != "velia" {
			t.Errorf("a camp on Velia's plain should not hold: declined %v", res.Declined)
		}
	})
	t.Run("an army avoiding battle will not attack", func(t *testing.T) {
		s := state(e, p("a", "velia", 30), en("e", "hollow", 10))
		res, _ := e.Resolve(s, []model.Order{refuse(move("a", "hollow"))})
		if len(res.Battles) != 0 || s.Armies["a"].Location != "velia" {
			t.Errorf("battles %v, a at %s", res.Battles, s.Armies["a"].Location)
		}
		if b, _ := res.BounceOf("a"); b.Reason != "declined" {
			t.Errorf("bounce reason = %q", b.Reason)
		}
	})
	t.Run("blundering into each other is an encounter: no choice", func(t *testing.T) {
		s := state(e, p("a", "velia", 30), en("e", "marren", 20))
		res, _ := e.Resolve(s, []model.Order{refuse(move("a", "hollow")), move("e", "hollow")})
		if len(res.Battles) != 1 || s.Armies["a"].Location != "hollow" {
			t.Errorf("an encounter in an empty province should still be fought: %v", res.Battles)
		}
	})
}
