// Package mapdata loads and validates the map, ruleset and scenario files,
// and answers graph questions (paths, distances) about the map.
package mapdata

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/deosjr/foc/internal/model"
	"gopkg.in/yaml.v3"
)

// ProvinceDef is one province as written in the map file.
type ProvinceDef struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Terrain string     `json:"terrain"`
	Supply  bool       `json:"supply"`
	Owner   model.Side `json:"owner"`
	Capital bool       `json:"capital"`
	Pos     [2]float64 `json:"pos"`
	// Aliases are alternative names. Capitalised ones count as mentions of the
	// province in report validation; all of them help the mock decision model.
	Aliases []string `json:"aliases"`
}

// Map is a loaded, validated map with its adjacency graph.
type Map struct {
	Name      string             `json:"name"`
	Terrain   map[string]float64 `json:"terrain"`
	Provinces []ProvinceDef      `json:"provinces"`
	Edges     [][2]string        `json:"edges"`

	byID map[string]*ProvinceDef
	adj  map[string][]string // sorted neighbour ids
}

// LoadMap reads and validates a map file.
func LoadMap(path string) (*Map, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Map
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("map %s: %w", path, err)
	}
	if err := m.init(); err != nil {
		return nil, fmt.Errorf("map %s: %w", path, err)
	}
	return &m, nil
}

func (m *Map) init() error {
	m.byID = map[string]*ProvinceDef{}
	m.adj = map[string][]string{}
	capitals := map[model.Side]bool{}
	for i := range m.Provinces {
		p := &m.Provinces[i]
		if p.ID == "" || p.Name == "" {
			return fmt.Errorf("province %d: missing id or name", i)
		}
		if _, dup := m.byID[p.ID]; dup {
			return fmt.Errorf("duplicate province id %q", p.ID)
		}
		if _, ok := m.Terrain[p.Terrain]; !ok {
			return fmt.Errorf("province %s: unknown terrain %q", p.ID, p.Terrain)
		}
		switch p.Owner {
		case model.Player, model.Enemy, model.Neutral:
		default:
			return fmt.Errorf("province %s: bad owner %q", p.ID, p.Owner)
		}
		if p.Capital {
			if capitals[p.Owner] {
				return fmt.Errorf("side %s has two capitals", p.Owner)
			}
			capitals[p.Owner] = true
		}
		m.byID[p.ID] = p
		m.adj[p.ID] = nil
	}
	if !capitals[model.Player] || !capitals[model.Enemy] {
		return fmt.Errorf("both player and enemy need a capital")
	}
	for _, e := range m.Edges {
		a, b := e[0], e[1]
		if m.byID[a] == nil || m.byID[b] == nil {
			return fmt.Errorf("edge %v: unknown province", e)
		}
		if a == b {
			return fmt.Errorf("edge %v: self loop", e)
		}
		m.adj[a] = append(m.adj[a], b)
		m.adj[b] = append(m.adj[b], a)
	}
	for id := range m.adj {
		sort.Strings(m.adj[id])
	}
	// The graph must be connected, or couriers could not reach everyone.
	start := m.Provinces[0].ID
	if n := len(m.distances(start)); n != len(m.Provinces) {
		return fmt.Errorf("map is not connected: %d of %d provinces reachable from %s", n, len(m.Provinces), start)
	}
	return nil
}

// Province returns the definition for an id, or nil.
func (m *Map) Province(id string) *ProvinceDef { return m.byID[id] }

// NameOf returns the display name for a province id (or the id if unknown).
func (m *Map) NameOf(id string) string {
	if p := m.byID[id]; p != nil {
		return p.Name
	}
	return id
}

// IDs returns all province ids, sorted.
func (m *Map) IDs() []string {
	ids := make([]string, 0, len(m.byID))
	for id := range m.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Neighbours returns the sorted adjacent province ids.
func (m *Map) Neighbours(id string) []string { return m.adj[id] }

// Adjacent reports whether two provinces share a road.
func (m *Map) Adjacent(a, b string) bool {
	for _, n := range m.adj[a] {
		if n == b {
			return true
		}
	}
	return false
}

// Capital returns the capital province id of a side.
func (m *Map) Capital(s model.Side) string {
	for _, p := range m.Provinces {
		if p.Capital && p.Owner == s {
			return p.ID
		}
	}
	return ""
}

// Defence returns the terrain defence multiplier of a province.
func (m *Map) Defence(id string) float64 {
	if p := m.byID[id]; p != nil {
		return m.Terrain[p.Terrain]
	}
	return 1
}

func (m *Map) distances(from string) map[string]int {
	dist := map[string]int{from: 0}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range m.adj[cur] {
			if _, seen := dist[n]; !seen {
				dist[n] = dist[cur] + 1
				queue = append(queue, n)
			}
		}
	}
	return dist
}

// Distance is the number of roads between two provinces, or -1.
func (m *Map) Distance(a, b string) int {
	d, ok := m.distances(a)[b]
	if !ok {
		return -1
	}
	return d
}

// Path returns the shortest path from a to b inclusive of both ends.
// Ties are broken deterministically by preferring the lowest province id at
// each step.
func (m *Map) Path(a, b string) []string {
	if m.byID[a] == nil || m.byID[b] == nil {
		return nil
	}
	toB := m.distances(b)
	if _, ok := toB[a]; !ok {
		return nil
	}
	path := []string{a}
	for cur := a; cur != b; {
		for _, n := range m.adj[cur] { // sorted, so the lowest id wins ties
			if toB[n] == toB[cur]-1 {
				cur = n
				break
			}
		}
		path = append(path, cur)
	}
	return path
}

// NextStep returns the next province on the shortest path from a toward b,
// or a itself if a == b.
func (m *Map) NextStep(a, b string) string {
	p := m.Path(a, b)
	if len(p) < 2 {
		return a
	}
	return p[1]
}

// Ruleset holds every era-specific number.
type Ruleset struct {
	Name           string `yaml:"name"`
	MenPerStrength int    `yaml:"men_per_strength"`
	DisbandBelow   int    `yaml:"disband_below"`
	// SupportFraction of a supporter's strength is added to the army it
	// supports; EntrenchBonus is added to the terrain multiplier of an army
	// entrenched for a second turn or more.
	SupportFraction float64 `yaml:"support_fraction"`
	EntrenchBonus   float64 `yaml:"entrench_bonus"`
	Casualties      struct {
		Loser    float64 `yaml:"loser"`
		Winner   float64 `yaml:"winner"`
		Standoff float64 `yaml:"standoff"`
	} `yaml:"casualties"`
	Courier struct {
		ProvincesPerTurn int     `yaml:"provinces_per_turn"`
		Interception     float64 `yaml:"interception"` // per province on the route near the enemy
	} `yaml:"courier"`
	Perception struct {
		AdjacentStrengthNoise float64 `yaml:"adjacent_strength_noise"`
		StrengthRounding      int     `yaml:"strength_rounding"`
		EnemyLossesNoise      float64 `yaml:"enemy_losses_noise"`
	} `yaml:"perception"`
	Distortion struct {
		CautionBase  float64 `yaml:"caution_base"`
		CautionScale float64 `yaml:"caution_scale"`
		VanityOwn    float64 `yaml:"vanity_own"`
		VanityEnemy  float64 `yaml:"vanity_enemy"`
		Omission     float64 `yaml:"omission"` // P(omit bad news) = omission × (1 − Honesty)
	} `yaml:"distortion"`
	Victory struct {
		SupplyCentresToWin int `yaml:"supply_centres_to_win"`
	} `yaml:"victory"`
	Seasons []string `yaml:"seasons"`
}

// LoadRuleset reads and validates a ruleset file.
func LoadRuleset(path string) (*Ruleset, error) {
	var r Ruleset
	if err := loadYAML(path, &r); err != nil {
		return nil, err
	}
	if r.Courier.ProvincesPerTurn < 1 {
		return nil, fmt.Errorf("ruleset %s: courier.provinces_per_turn must be >= 1", path)
	}
	if len(r.Seasons) == 0 {
		return nil, fmt.Errorf("ruleset %s: no seasons", path)
	}
	if r.Perception.StrengthRounding < 1 {
		r.Perception.StrengthRounding = 1
	}
	if r.MenPerStrength < 1 {
		r.MenPerStrength = 1
	}
	return &r, nil
}

// Season returns a label like "Spring, year 1" for a turn number (1-based).
func (r *Ruleset) Season(turn int) string {
	if turn < 1 {
		turn = 1
	}
	n := len(r.Seasons)
	return fmt.Sprintf("%s, year %d", r.Seasons[(turn-1)%n], (turn-1)/n+1)
}

// ArmyDef places one army in a scenario.
type ArmyDef struct {
	ID        string     `yaml:"id"`
	Side      model.Side `yaml:"side"`
	Location  string     `yaml:"location"`
	Strength  int        `yaml:"strength"`
	General   string     `yaml:"general"`
	Commander string     `yaml:"commander"`
}

// Intel is something the player knows at the start of the game.
type Intel struct {
	Province      string `yaml:"province"`
	EnemyStrength int    `yaml:"enemy_strength"`
}

// Scenario is a starting position on a map.
type Scenario struct {
	Name        string              `yaml:"name"`
	TurnCap     int                 `yaml:"turn_cap"`
	EnemyAI     string              `yaml:"enemy_ai"` // scripted (follows enemy_routes) | heuristic
	Armies      []ArmyDef           `yaml:"armies"`
	EnemyRoutes map[string][]string `yaml:"enemy_routes"`
	Intel       []Intel             `yaml:"intel"`
}

// LoadScenario reads a scenario and validates it against the map.
func LoadScenario(path string, m *Map) (*Scenario, error) {
	var s Scenario
	if err := loadYAML(path, &s); err != nil {
		return nil, err
	}
	if s.TurnCap < 1 {
		return nil, fmt.Errorf("scenario %s: turn_cap must be >= 1", path)
	}
	switch s.EnemyAI {
	case "":
		s.EnemyAI = "scripted"
	case "scripted", "heuristic":
	default:
		return nil, fmt.Errorf("scenario %s: enemy_ai must be scripted or heuristic, not %q", path, s.EnemyAI)
	}
	seen := map[string]bool{}
	for _, a := range s.Armies {
		if seen[a.ID] {
			return nil, fmt.Errorf("scenario %s: duplicate army %s", path, a.ID)
		}
		seen[a.ID] = true
		if m.Province(a.Location) == nil {
			return nil, fmt.Errorf("scenario %s: army %s in unknown province %s", path, a.ID, a.Location)
		}
		if a.Side == model.Player && a.General == "" {
			return nil, fmt.Errorf("scenario %s: player army %s has no general", path, a.ID)
		}
	}
	for army, route := range s.EnemyRoutes {
		if !seen[army] {
			return nil, fmt.Errorf("scenario %s: route for unknown army %s", path, army)
		}
		for _, p := range route {
			if m.Province(p) == nil {
				return nil, fmt.Errorf("scenario %s: route for %s has unknown province %s", path, army, p)
			}
		}
	}
	for _, in := range s.Intel {
		if m.Province(in.Province) == nil {
			return nil, fmt.Errorf("scenario %s: intel on unknown province %s", path, in.Province)
		}
	}
	return &s, nil
}

// LoadGenerals reads the generals file.
func LoadGenerals(path string) (map[string]*model.General, error) {
	var f struct {
		Generals []*model.General `yaml:"generals"`
	}
	if err := loadYAML(path, &f); err != nil {
		return nil, err
	}
	out := map[string]*model.General{}
	for _, g := range f.Generals {
		if g.ID == "" || g.Name == "" {
			return nil, fmt.Errorf("generals %s: general without id or name", path)
		}
		out[g.ID] = g
	}
	return out, nil
}

func loadYAML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
