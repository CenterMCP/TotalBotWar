// Package battle owns the authoritative simulation. Views are always redacted.
package battle

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
)

const Width, Height, MaxTurns, WinningScore = 18, 12, 24, 6

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type Unit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Side string `json:"side"`
	Kind string `json:"kind"`
	Point
	HP       int  `json:"hp"`
	Morale   int  `json:"morale"`
	Guard    bool `json:"guard"`
	Scouting bool `json:"scouting"`
}
type Order struct {
	UnitID   string `json:"unit_id"`
	Action   string `json:"action"`
	X        int    `json:"x,omitempty"`
	Y        int    `json:"y,omitempty"`
	TargetID string `json:"target_id,omitempty"`
}
type Battle struct {
	ID       string           `json:"battle_id"`
	Turn     int              `json:"turn"`
	Revision int              `json:"revision"`
	Status   string           `json:"status"`
	Terrain  []string         `json:"terrain"`
	Units    []Unit           `json:"units"`
	Orders   map[string]Order `json:"orders"`
	Explored []bool           `json:"explored"`
	Reports  []string         `json:"reports"`
	Scores   map[string]int   `json:"scores"`
}
type Cell struct {
	Point
	Terrain  string `json:"terrain"`
	Visible  bool   `json:"visible"`
	Explored bool   `json:"explored"`
}
type View struct {
	ID           string           `json:"battle_id"`
	Turn         int              `json:"turn"`
	Revision     int              `json:"revision"`
	Status       string           `json:"status"`
	Width        int              `json:"width"`
	Height       int              `json:"height"`
	Cells        []Cell           `json:"cells"`
	Units        []Unit           `json:"units"`
	Orders       map[string]Order `json:"orders"`
	Reports      []string         `json:"reports"`
	Scores       map[string]int   `json:"scores"`
	Objective    Point            `json:"objective"`
	MaxTurns     int              `json:"max_turns"`
	WinningScore int              `json:"winning_score"`
}

var Objective = Point{8, 5}

func NewBattle() *Battle {
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		panic(err)
	}
	b := &Battle{ID: hex.EncodeToString(token), Turn: 1, Revision: 1, Status: "planning", Orders: map[string]Order{}, Explored: make([]bool, Width*Height), Scores: map[string]int{"player": 0, "enemy": 0}, Reports: []string{"Dawn at Ashen Vale. Seize the watchtower at I6. Hold it for 6 points, or break the enemy army."}}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			t := "plain"
			if (x*7+y*11)%17 < 4 && x > 3 && x < 14 {
				t = "forest"
			}
			if (x == 8 || x == 9) && (y < 3 || y > 8) {
				t = "water"
			}
			if (x == 7 || x == 10) && (y == 4 || y == 7) {
				t = "hill"
			}
			if (Point{x, y}) == Objective {
				t = "tower"
			}
			b.Terrain = append(b.Terrain, t)
		}
	}
	kinds := []string{"infantry", "spears", "archers", "cavalry", "scouts"}
	names := []string{"Iron Guard", "Ashen Pikes", "Ember Bows", "Dawn Riders", "Grey Watch"}
	for i, k := range kinds {
		b.Units = append(b.Units, Unit{ID: fmt.Sprintf("p%d", i+1), Name: names[i], Side: "player", Kind: k, Point: Point{2, 2 + i*2}, HP: 100, Morale: 100}, Unit{ID: fmt.Sprintf("e%d", i+1), Name: []string{"Obsidian Guard", "Thorn Pikes", "Cinder Bows", "Dusk Riders", "Night Watch"}[i], Side: "enemy", Kind: k, Point: Point{15, 2 + i*2}, HP: 100, Morale: 100})
	}
	b.observe()
	return b
}
func alive(u Unit) bool       { return u.HP > 0 && u.Morale > 20 }
func distance(a, b Point) int { return abs(a.X-b.X) + abs(a.Y-b.Y) }
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func (b *Battle) terrain(p Point) string {
	if p.X < 0 || p.X >= Width || p.Y < 0 || p.Y >= Height {
		return "water"
	}
	return b.Terrain[p.Y*Width+p.X]
}
func Movement(kind string) int {
	if kind == "scouts" {
		return 4
	}
	if kind == "cavalry" {
		return 3
	}
	return 2
}
func Range(kind string) int {
	if kind == "archers" {
		return 3
	}
	return 1
}
func (b *Battle) sight(u Unit) int {
	r := 4
	if u.Kind == "scouts" {
		r = 6
	}
	if u.Scouting {
		r += 2
	}
	if b.terrain(u.Point) == "hill" || b.terrain(u.Point) == "tower" {
		r++
	}
	return r
}

// Intermediate forests block line of sight. Forest occupants are concealed beyond two tiles.
func (b *Battle) line(a, c Point) bool {
	steps := max(abs(c.X-a.X), abs(c.Y-a.Y))
	for i := 1; i < steps; i++ {
		p := Point{a.X + (c.X-a.X)*i/steps, a.Y + (c.Y-a.Y)*i/steps}
		if b.terrain(p) == "forest" {
			return false
		}
	}
	return true
}
func (b *Battle) visibility(side string) []bool {
	v := make([]bool, Width*Height)
	for _, u := range b.Units {
		if u.Side != side || !alive(u) {
			continue
		}
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				p := Point{x, y}
				if distance(u.Point, p) <= b.sight(u) && b.line(u.Point, p) {
					v[y*Width+x] = true
				}
			}
		}
	}
	return v
}
func (b *Battle) unitVisible(u Unit, side string, v []bool) bool {
	if !alive(u) {
		return false
	}
	if u.Side == side {
		return true
	}
	if !v[u.Y*Width+u.X] {
		return false
	}
	if b.terrain(u.Point) == "forest" {
		for _, f := range b.Units {
			if f.Side == side && alive(f) && distance(f.Point, u.Point) <= 2 {
				return true
			}
		}
		return false
	}
	return true
}
func (b *Battle) observe() {
	for i, v := range b.visibility("player") {
		b.Explored[i] = b.Explored[i] || v
	}
}
func (b *Battle) View() View {
	vis := b.visibility("player")
	v := View{ID: b.ID, Turn: b.Turn, Revision: b.Revision, Status: b.Status, Width: Width, Height: Height, Cells: []Cell{}, Units: []Unit{}, Orders: map[string]Order{}, Reports: append([]string{}, b.Reports...), Scores: map[string]int{"player": b.Scores["player"], "enemy": b.Scores["enemy"]}, Objective: Objective, MaxTurns: MaxTurns, WinningScore: WinningScore}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			i := y*Width + x
			v.Cells = append(v.Cells, Cell{Point{x, y}, b.Terrain[i], vis[i], b.Explored[i]})
		}
	}
	for _, u := range b.Units {
		if b.unitVisible(u, "player", vis) {
			v.Units = append(v.Units, u)
		}
	}
	for k, o := range b.Orders {
		v.Orders[k] = o
	}
	return v
}
func (b *Battle) unit(id string) *Unit {
	for i := range b.Units {
		if b.Units[i].ID == id {
			return &b.Units[i]
		}
	}
	return nil
}

// Paths use the charted terrain, never hidden occupants; validation cannot reveal enemies.
func (b *Battle) path(from, to Point) []Point {
	if b.terrain(to) == "water" {
		return nil
	}
	queue := []Point{from}
	prev := map[Point]Point{from: from}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if p == to {
			out := []Point{}
			for p != from {
				out = append(out, p)
				p = prev[p]
			}
			slices.Reverse(out)
			return out
		}
		for _, d := range []Point{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
			n := Point{p.X + d.X, p.Y + d.Y}
			if _, ok := prev[n]; ok || b.terrain(n) == "water" {
				continue
			}
			prev[n] = p
			queue = append(queue, n)
		}
	}
	return nil
}
func (b *Battle) checkRevision(revision int) error {
	if b.Status != "planning" {
		return fmt.Errorf("battle is finished: %s", b.Status)
	}
	if revision != b.Revision {
		return fmt.Errorf("stale revision: expected %d; call get_battle before retrying", b.Revision)
	}
	return nil
}
func (b *Battle) Issue(revision int, orders []Order) error {
	if err := b.checkRevision(revision); err != nil {
		return err
	}
	if len(orders) < 1 || len(orders) > 5 {
		return fmt.Errorf("supply 1 to 5 orders")
	}
	seen := map[string]bool{}
	vis := b.visibility("player")
	for _, o := range orders {
		u := b.unit(o.UnitID)
		if u == nil || u.Side != "player" || !alive(*u) {
			return fmt.Errorf("unknown active friendly unit %q", o.UnitID)
		}
		if seen[o.UnitID] {
			return fmt.Errorf("duplicate order for %s", o.UnitID)
		}
		seen[o.UnitID] = true
		switch o.Action {
		case "move":
			p := Point{o.X, o.Y}
			path := b.path(u.Point, p)
			if p == u.Point || len(path) == 0 || len(path) > Movement(u.Kind) {
				return fmt.Errorf("%s can move at most %d orthogonal tiles on land", u.ID, Movement(u.Kind))
			}
		case "attack":
			t := b.unit(o.TargetID)
			if t == nil || t.Side == "player" || !b.unitVisible(*t, "player", vis) {
				return fmt.Errorf("target is not currently visible")
			}
			if distance(u.Point, t.Point) > Range(u.Kind) || !b.line(u.Point, t.Point) {
				return fmt.Errorf("target outside %s range (%d) or line of sight", u.ID, Range(u.Kind))
			}
		case "defend", "scout", "cancel":
		default:
			return fmt.Errorf("action must be move, attack, defend, scout, or cancel")
		}
	}
	for _, o := range orders {
		if o.Action == "cancel" {
			delete(b.Orders, o.UnitID)
		} else {
			b.Orders[o.UnitID] = o
		}
	}
	b.Revision++
	return nil
}
func (b *Battle) enemyOrders() map[string]Order {
	out := map[string]Order{}
	vis := b.visibility("enemy")
	for _, u := range b.Units {
		if u.Side != "enemy" || !alive(u) {
			continue
		}
		goal := Objective
		var nearest *Unit
		for i := range b.Units {
			t := &b.Units[i]
			if t.Side == u.Side || !b.unitVisible(*t, "enemy", vis) {
				continue
			}
			if nearest == nil || distance(u.Point, t.Point) < distance(u.Point, nearest.Point) {
				nearest = t
			}
		}
		if nearest != nil {
			goal = nearest.Point
			if distance(u.Point, goal) <= Range(u.Kind) && b.line(u.Point, goal) {
				out[u.ID] = Order{UnitID: u.ID, Action: "attack", TargetID: nearest.ID}
				continue
			}
		}
		if u.Point == goal {
			out[u.ID] = Order{UnitID: u.ID, Action: "defend"}
			continue
		}
		path := b.path(u.Point, goal)
		if len(path) > 0 {
			p := path[min(Movement(u.Kind), len(path))-1]
			out[u.ID] = Order{UnitID: u.ID, Action: "move", X: p.X, Y: p.Y}
		}
	}
	return out
}
func (b *Battle) End(revision int) error {
	if err := b.checkRevision(revision); err != nil {
		return err
	}
	orders := b.enemyOrders()
	for k, v := range b.Orders {
		orders[k] = v
	}
	b.Reports = []string{fmt.Sprintf("Turn %d resolved.", b.Turn)}
	for i := range b.Units {
		u := &b.Units[i]
		o := orders[u.ID]
		u.Guard = o.Action == "defend" || o.Action == ""
		u.Scouting = o.Action == "scout"
		if u.Guard && alive(*u) {
			u.Morale = min(100, u.Morale+8)
		}
	}
	paths := map[string][]Point{}
	for _, u := range b.Units {
		if alive(u) && orders[u.ID].Action == "move" {
			o := orders[u.ID]
			paths[u.ID] = b.path(u.Point, Point{o.X, o.Y})
		}
	}
	// Alternate initiative every turn; blocked steps consume the movement phase.
	for step := 0; step < 4; step++ {
		for n := 0; n < len(b.Units); n++ {
			i := (n + b.Turn) % len(b.Units)
			u := &b.Units[i]
			p := paths[u.ID]
			if step >= len(p) || !alive(*u) {
				continue
			}
			next := p[step]
			blocked := false
			for _, t := range b.Units {
				if alive(t) && t.ID != u.ID && t.Point == next {
					blocked = true
					break
				}
			}
			if blocked {
				delete(paths, u.ID)
				if u.Side == "player" {
					b.Reports = append(b.Reports, u.Name+" halted: route occupied.")
				}
				continue
			}
			u.Point = next
		}
	}
	b.observe()
	vis := b.visibility("player")
	damage := map[string]int{}
	for _, u := range b.Units {
		o := orders[u.ID]
		if !alive(u) || o.Action != "attack" {
			continue
		}
		t := b.unit(o.TargetID)
		if t == nil || !alive(*t) || distance(u.Point, t.Point) > Range(u.Kind) || !b.line(u.Point, t.Point) || !b.unitVisible(*t, u.Side, b.visibility(u.Side)) {
			if u.Side == "player" {
				b.Reports = append(b.Reports, u.Name+" lost its target.")
			}
			continue
		}
		hit := map[string]int{"infantry": 28, "spears": 25, "archers": 24, "cavalry": 32, "scouts": 18}[u.Kind]
		if u.Kind == "spears" && t.Kind == "cavalry" {
			hit += 18
		}
		if t.Guard {
			hit = hit * 2 / 3
		}
		if b.terrain(t.Point) == "forest" || b.terrain(t.Point) == "hill" || b.terrain(t.Point) == "tower" {
			hit = hit * 3 / 4
		}
		damage[t.ID] += hit
		if u.Side == "player" || b.unitVisible(u, "player", vis) || t.Side == "player" {
			name := u.Name
			if !b.unitVisible(u, "player", vis) {
				name = "Unseen attackers"
			}
			b.Reports = append(b.Reports, fmt.Sprintf("%s hit %s for %d.", name, t.Name, hit))
		}
	}
	for i := range b.Units {
		u := &b.Units[i]
		if !alive(*u) {
			continue
		}
		u.HP = max(0, u.HP-damage[u.ID])
		u.Morale = max(0, u.Morale-damage[u.ID]/2)
		if !alive(*u) && (u.Side == "player" || vis[u.Y*Width+u.X]) {
			b.Reports = append(b.Reports, u.Name+" has broken.")
		}
	}
	counts := map[string]int{}
	for _, u := range b.Units {
		if alive(u) {
			counts[u.Side]++
			if u.Point == Objective {
				b.Scores[u.Side]++
				b.Reports = append(b.Reports, fmt.Sprintf("%s holds the watchtower (+1).", u.Side))
			}
		}
	}
	b.Status = "planning"
	switch {
	case counts["player"] == 0 && counts["enemy"] == 0:
		b.Status = "draw"
	case counts["enemy"] == 0 || b.Scores["player"] >= WinningScore:
		b.Status = "victory"
	case counts["player"] == 0 || b.Scores["enemy"] >= WinningScore:
		b.Status = "defeat"
	case b.Turn >= MaxTurns:
		if b.Scores["player"] > b.Scores["enemy"] {
			b.Status = "victory"
		} else if b.Scores["enemy"] > b.Scores["player"] {
			b.Status = "defeat"
		} else {
			b.Status = "draw"
		}
	}
	if b.Status == "planning" {
		b.Turn++
	} else {
		b.Reports = append(b.Reports, "Battle finished: "+b.Status)
	}
	b.Revision++
	b.Orders = map[string]Order{}
	b.observe()
	return nil
}
