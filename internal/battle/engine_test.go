package battle

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func emptyBattle() *Battle {
	b := NewBattle()
	for i := range b.Terrain {
		b.Terrain[i] = "plain"
	}
	return b
}
func TestFogNeverExposesHiddenEnemies(t *testing.T) {
	b := NewBattle()
	v := b.View()
	for _, u := range v.Units {
		if u.Side != "player" {
			t.Fatal("enemy leaked at spawn")
		}
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "Obsidian Guard") || strings.Contains(string(raw), `"e1"`) {
		t.Fatal("hidden enemy metadata leaked")
	}
	before := b.Revision
	if err := b.Issue(before, []Order{{UnitID: "p3", Action: "attack", TargetID: "e1"}}); err == nil {
		t.Fatal("attacked unseen enemy")
	}
	if b.Revision != before {
		t.Fatal("invalid order mutated revision")
	}
}
func TestAtomicOrdersAndRevision(t *testing.T) {
	b := NewBattle()
	if err := b.Issue(1, []Order{{UnitID: "p1", Action: "defend"}, {UnitID: "p2", Action: "move", X: 17, Y: 11}}); err == nil {
		t.Fatal("accepted impossible movement")
	}
	if len(b.Orders) != 0 {
		t.Fatal("partially applied batch")
	}
	if err := b.Issue(1, []Order{{UnitID: "p1", Action: "scout"}}); err != nil {
		t.Fatal(err)
	}
	if err := b.End(1); err == nil {
		t.Fatal("accepted stale turn")
	}
	if err := b.End(2); err != nil {
		t.Fatal(err)
	}
	if err := b.End(2); err == nil {
		t.Fatal("duplicate turn accepted")
	}
	if b.Turn != 2 {
		t.Fatal(b.Turn)
	}
}
func TestForestConcealmentAndLineOfSight(t *testing.T) {
	b := emptyBattle()
	b.Units = []Unit{{ID: "p", Side: "player", Kind: "scouts", Point: Point{2, 5}, HP: 100, Morale: 100}, {ID: "e", Side: "enemy", Kind: "infantry", Point: Point{5, 5}, HP: 100, Morale: 100}}
	b.Terrain[5*Width+5] = "forest"
	if b.unitVisible(b.Units[1], "player", b.visibility("player")) {
		t.Fatal("forest target visible at distance 3")
	}
	b.Units[0].X = 3
	if !b.unitVisible(b.Units[1], "player", b.visibility("player")) {
		t.Fatal("near forest target hidden")
	}
	b.Terrain[5*Width+4] = "forest"
	if b.line(b.Units[0].Point, b.Units[1].Point) {
		t.Fatal("forest did not block line")
	}
}
func TestSimultaneousCombatAndTerminalState(t *testing.T) {
	b := emptyBattle()
	b.Units = []Unit{{ID: "p", Side: "player", Kind: "infantry", Point: Point{6, 5}, HP: 20, Morale: 100}, {ID: "e", Side: "enemy", Kind: "infantry", Point: Point{7, 5}, HP: 20, Morale: 100}}
	b.Orders = map[string]Order{"p": {UnitID: "p", Action: "attack", TargetID: "e"}}
	if err := b.End(1); err != nil {
		t.Fatal(err)
	}
	if b.Units[0].HP != 0 || b.Units[1].HP != 0 || b.Status != "draw" {
		t.Fatalf("combat was not simultaneous: %+v", b)
	}
	if err := b.End(b.Revision); err == nil {
		t.Fatal("advanced finished battle")
	}
}
func TestTowerVictory(t *testing.T) {
	b := emptyBattle()
	b.Units[0].Point = Objective
	b.Scores["player"] = 5
	if err := b.End(1); err != nil {
		t.Fatal(err)
	}
	if b.Status != "victory" {
		t.Fatalf("tower not scored: %s", b.Status)
	}
}
func TestTurnLimit(t *testing.T) {
	b := NewBattle()
	b.Turn = MaxTurns
	if err := b.End(1); err != nil {
		t.Fatal(err)
	}
	if b.Status != "draw" {
		t.Fatal(b.Status)
	}
}
func TestStorePersistenceAndConcurrentTurns(t *testing.T) {
	s := NewStore(t.TempDir())
	v, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	s = NewStore(s.dir)
	again, err := s.Access(v.ID, nil)
	if err != nil || again.Revision != 1 {
		t.Fatal("did not survive reload", err)
	}
	var wg sync.WaitGroup
	success := 0
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Access(v.ID, func(b *Battle) error { return b.End(1) })
			if err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("%d duplicate mutations succeeded", success)
	}
	if _, err = s.Access("../not-a-battle", nil); err == nil {
		t.Fatal("path traversal accepted")
	}
}
func TestCompleteMatchAndNoOverlappingUnits(t *testing.T) {
	b := NewBattle()
	for i := 0; i < MaxTurns && b.Status == "planning"; i++ {
		if err := b.End(b.Revision); err != nil {
			t.Fatal(err)
		}
		occupied := map[Point]bool{}
		for _, u := range b.Units {
			if !alive(u) {
				continue
			}
			if occupied[u.Point] {
				t.Fatal("overlapping units")
			}
			occupied[u.Point] = true
		}
		for _, u := range b.View().Units {
			if u.Side == "enemy" && !b.unitVisible(u, "player", b.visibility("player")) {
				t.Fatal("hidden unit leaked")
			}
		}
	}
	if b.Status == "planning" {
		t.Fatal("match did not finish")
	}
}
func TestOccupiedHiddenTileDoesNotLeakInValidation(t *testing.T) {
	b := emptyBattle()
	b.Units = []Unit{{ID: "p", Name: "Scout", Side: "player", Kind: "scouts", Point: Point{2, 2}, HP: 100, Morale: 100}, {ID: "e", Side: "enemy", Kind: "infantry", Point: Point{5, 2}, HP: 100, Morale: 100}}
	b.Terrain[2*Width+5] = "forest"
	if err := b.Issue(1, []Order{{UnitID: "p", Action: "move", X: 5, Y: 2}}); err != nil {
		t.Fatal("validation leaked occupied tile", err)
	}
}
