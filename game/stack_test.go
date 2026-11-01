package game

import (
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
)

// The stack picker's world half: pickAll is the cluster (depth order, the
// canonical tiebreak), and the scene's hover report opens, holds and drops
// the column without touching pick's one-entity resolution.

// The picker's hover timing (uikit/screens stack.go): frames, never wall
// clock. Duplicated here on purpose: a change to the picker's timing that
// leaves these behind fails this test.
const (
	dwellFrames = 12 // ~200 ms at 60 fps
	graceFrames = 9  // ~150 ms at 60 fps
)

// parkOn places an entity on a world point (the pick reads transforms, never
// the step: no sim state is stepped for the choreography).
func parkOn(g *Game, e ecs.Entity, pos core.Vec2) {
	g.s.Transforms.Set(e, sim.Transform{Pos: pos})
}

// TestPickAllOrdersClusterByDepthAndHandles: a cluster comes back in the
// world's paint order — a ship over rocks over a station — and entities in
// one layer in canonical handle order, so the column never reorders between
// frames.
func TestPickAllOrdersClusterByDepthAndHandles(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	sc := g.space
	ply, _ := g.s.Transforms.Get(g.s.Player())

	var rocks []ecs.Entity
	var station ecs.Entity
	for _, inf := range g.s.Grid() {
		switch inf.Kind {
		case sim.KindAsteroid:
			rocks = append(rocks, inf.Entity)
		case sim.KindStation:
			station = inf.Entity
		}
	}
	if len(rocks) < 2 || station.IsZero() {
		t.Fatalf("fixture: %d rocks, station %v", len(rocks), station)
	}
	rocks = rocks[:2]
	for _, e := range append(rocks, station) {
		parkOn(g, e, ply.Pos)
	}

	cam := sc.Camera(1280, 720)
	x, y := cam.WorldToScreen(ply.Pos)
	got := sc.pickAll(cam, x, y)
	want := map[ecs.Entity]bool{g.s.Player(): true, station: true, rocks[0]: true, rocks[1]: true}
	if len(got) != len(want) {
		t.Fatalf("pickAll returned %d entities (%+v), want the cluster of %d", len(got), got, len(want))
	}
	for _, inf := range got {
		if !want[inf.Entity] {
			t.Errorf("pickAll returned %v (%s), which is not in the cluster", inf.Entity, inf.Name)
		}
	}
	if got[0].Entity != g.s.Player() {
		t.Errorf("column leads with %v (%s), want the player's hull (the top paint layer)",
			got[0].Entity, got[0].Name)
	}
	for i := 1; i < len(got); i++ {
		a, b := render.StackLayer(got[i-1].Kind), render.StackLayer(got[i].Kind)
		if b > a {
			t.Errorf("cluster order %s then %s: layer %d over %d", got[i-1].Kind, got[i].Kind, a, b)
		}
		if a == b && got[i-1].Entity > got[i].Entity {
			t.Errorf("ties out of handle order: %v then %v", got[i-1].Entity, got[i].Entity)
		}
	}
	if got[len(got)-1].Entity != station {
		t.Errorf("column ends with %v (%s), want the station (the bottom paint layer)",
			got[len(got)-1].Entity, got[len(got)-1].Name)
	}
}

// TestStackColumnDwellHoldsAndDismisses drives the picker through the real
// frame loop: the hovered cluster opens the column after the dwell, a plain
// world click on the cluster still resolves the single nearest entity (pick,
// never the column's top), and the column drops after the grace once the
// cursor is on neither it nor the cluster.
func TestStackColumnDwellHoldsAndDismisses(t *testing.T) {
	mm := useLoopMouse(t)
	g := newShowcaseGame(t, mm.pointer())
	img := pump(t, g, 1280, 720)
	sc := g.space
	ply, _ := g.s.Transforms.Get(g.s.Player())

	// The station (layer 3, handle ...299) and a ship (layer 6, handle ...300)
	// on one point 24 px below the player: the station wins the nearest pick,
	// the ship leads the column.
	var station, ship ecs.Entity
	for _, inf := range g.s.Grid() {
		switch inf.Kind {
		case sim.KindStation:
			station = inf.Entity
		case sim.KindShip:
			if ship.IsZero() || inf.Entity < ship {
				ship = inf.Entity
			}
		}
	}
	if station.IsZero() || ship.IsZero() || ship < station {
		t.Skipf("fixture: station %v, ship %v (want a station on the lower handle)", station, ship)
	}
	pos := ply.Pos.Add(core.V(0, 24/zoomBase)) // 24 px below the player: outside the pick radius
	parkOn(g, station, pos)
	parkOn(g, ship, pos)
	atX, atY := panUnderCursor(g, pos)
	at := core.V(atX, atY)
	mm.moveTo(int(at.X), int(at.Y))

	frame := func() {
		t.Helper()
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		g.Draw(img)
	}
	for i := 1; i < dwellFrames; i++ {
		frame()
		if g.kit.StackShown() {
			t.Fatalf("column opened after %d hover frames, want %d", i, dwellFrames)
		}
	}
	frame()
	if !g.kit.StackShown() {
		t.Fatalf("column never opened after %d hover frames on the cluster", dwellFrames)
	}
	// The cursor rests on the cluster, not the column (which seats to the
	// right of it), so the world still owns this press: the nearest pick is
	// the station, the column's top the ship.
	cam := sc.Camera(1280, 720)
	nearest, ok := sc.pick(cam, at.X, at.Y)
	if !ok || nearest.Entity != station {
		t.Fatalf("pick = %v (%v), want the station %v (nearest entity)", nearest.Entity, nearest.Name, station)
	}
	if cluster := sc.pickAll(cam, at.X, at.Y); len(cluster) == 0 || cluster[0].Entity != ship {
		t.Fatalf("cluster top = %+v, want the ship %v", cluster, ship)
	}
	if g.overUI(at.X, at.Y) {
		t.Fatalf("fixture: the cursor (%v, %v) sits on a panel", at.X, at.Y)
	}

	mm.left = true
	frame()
	mm.left = false
	frame()
	if g.kit.Selected() != station {
		t.Fatalf("a plain click on the cluster selected %v, want the nearest pick %v "+
			"(the column must never change pick's resolution)", g.kit.Selected(), station)
	}

	// Both cluster members leave: the column holds through the grace, then
	// drops.
	parkOn(g, station, pos.Add(core.V(1e6, 0)))
	parkOn(g, ship, pos.Add(core.V(1e6, 1e6)))
	for i := 1; i < graceFrames; i++ {
		frame()
		if !g.kit.StackShown() {
			t.Fatalf("column dropped after %d grace frames, want %d", i, graceFrames)
		}
	}
	for i := 0; i < graceFrames; i++ {
		frame()
	}
	if g.kit.StackShown() {
		t.Fatalf("column survived %d grace frames off the cluster", graceFrames*2)
	}
}

// TestStackColumnHoldsOnTheColumn: with the cursor moved onto the open
// column, the picker holds it up indefinitely (the hover left the cluster,
// not the column).
func TestStackColumnHoldsOnTheColumn(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	sc := g.space
	ply, _ := g.s.Transforms.Get(g.s.Player())
	var station, ship ecs.Entity
	for _, inf := range g.s.Grid() {
		switch inf.Kind {
		case sim.KindStation:
			station = inf.Entity
		case sim.KindShip:
			if ship.IsZero() || inf.Entity < ship {
				ship = inf.Entity
			}
		}
	}
	if station.IsZero() || ship.IsZero() {
		t.Skip("fixture: no station or ship on the grid")
	}
	pos := ply.Pos.Add(core.V(0, 24/zoomBase))
	parkOn(g, station, pos)
	parkOn(g, ship, pos)
	atX, atY := panUnderCursor(g, pos)
	at := core.V(atX, atY)

	// Hover the cluster through the scene's own step (no frames needed for
	// the pick, only the report), then hold on the column.
	for i := 0; i < dwellFrames; i++ {
		sc.stackStep(Input{MouseX: at.X, MouseY: at.Y}, sc.Camera(1280, 720), false)
	}
	if !g.kit.StackShown() {
		t.Fatalf("column never opened on the cluster")
	}
	cx, cy := at.X+20, at.Y
	if !g.kit.StackOwns(cx, cy) {
		t.Fatalf("the open column does not own (%v, %v)", cx, cy)
	}
	for i := 0; i < graceFrames*3; i++ {
		sc.stackStep(Input{MouseX: cx, MouseY: cy}, sc.Camera(1280, 720), false)
	}
	if !g.kit.StackShown() {
		t.Fatal("column dropped while the cursor rested on it")
	}
	sc.OnLeave()
	if g.kit.StackShown() {
		t.Fatal("leaving the world scene left the column up")
	}
}
