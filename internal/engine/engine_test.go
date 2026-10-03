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

func p(id, loc string, str int) *Army { return &Army{ID: id, Side: model.Player, Location: loc, Strength: str} }
func en(id, loc string, str int) *Army {
	return &Army{ID: id, Side: model.Enemy, Location: loc, Strength: str}
}
func move(id, to string) model.Order { return model.Order{ArmyID: id, Type: model.MoveToward, Target: to} }
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
			name:   "head-to-head: border battle, then the winner attacks the loser at home",
			armies: []*Army{p("a", "velia", 30), en("e", "hollow", 20)},
			orders: []model.Order{move("a", "hollow"), move("e", "velia")},
			// Border: 30 v 20, a -> 27, e -> 14. Hollow: 27 v 14*1.2, a -> 24, e -> 10, retreats.
			want:    map[string]want{"a": {"hollow", 24}, "e": {"marren", 10}},
			battles: 2,
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
		retreat("a", "marren"), // not adjacent
		{ArmyID: "a", Type: model.Entrench},
	} {
		s := state(e, p("a", "velia", 30))
		if _, err := e.Resolve(s, []model.Order{o}); err == nil {
			t.Errorf("order %+v accepted", o)
		}
	}
}

func TestEvaluate(t *testing.T) {
	e := testEngine(t)
	t.Run("taking the enemy capital wins", func(t *testing.T) {
		s := state(e, p("a", "sarnos", 30))
		if _, err := e.Resolve(s, []model.Order{move("a", "kethra")}); err != nil {
			t.Fatal(err)
		}
		e.Evaluate(s, 10)
		if !s.Over || s.Winner != model.Player {
			t.Errorf("got over=%v winner=%s", s.Over, s.Winner)
		}
	})
	t.Run("losing the capital loses", func(t *testing.T) {
		s := state(e, p("a", "oros", 30), en("e", "duna", 30))
		if _, err := e.Resolve(s, []model.Order{move("e", "karsa")}); err != nil {
			t.Fatal(err)
		}
		e.Evaluate(s, 10)
		if !s.Over || s.Winner != model.Enemy {
			t.Errorf("got over=%v winner=%s", s.Over, s.Winner)
		}
	})
	t.Run("four supply centres win", func(t *testing.T) {
		s := state(e, p("a", "marren", 30), p("b", "sarnos", 30))
		if _, err := e.Resolve(s, nil); err != nil {
			t.Fatal(err)
		}
		e.Evaluate(s, 10)
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
