package game

import (
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
)

// parkOn places an entity on a world point (the pick reads transforms, never
// the step: no sim state is stepped for the choreography).
func parkOn(g *Game, e ecs.Entity, pos core.Vec2) {
	g.s.Transforms.Set(e, sim.Transform{Pos: pos})
}

// TestHoverReportsTheEntityUnderTheCursor: the scene resolves the pick every
// frame for the overlay's one label — the entity under the cursor there and
// nothing on empty space (labels are hover-only; zoom no longer gates them).
func TestHoverReportsTheEntityUnderTheCursor(t *testing.T) {
	mm := useLoopMouse(t)
	g := newShowcaseGame(t, mm.pointer())
	img := pump(t, g, 1280, 720)
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
		t.Skipf("fixture: station %v, ship %v", station, ship)
	}
	pos := ply.Pos.Add(core.V(0, 24/zoomBase))
	parkOn(g, station, pos)
	parkOn(g, ship, pos)
	atX, atY := panUnderCursor(g, pos)
	mm.moveTo(int(atX), int(atY))

	frame := func() {
		t.Helper()
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
		g.Draw(img)
	}
	frame()
	// The cursor sits on the cluster: the world's pick (and so the label) is
	// the nearest entity, the station.
	if sc.hover != station {
		t.Fatalf("hover = %v on the cluster, want the nearest pick %v", sc.hover, station)
	}

	// Off the cluster: no entity under the cursor, no label.
	cam := sc.Camera(1280, 720)
	freeX, freeY := 0.0, 0.0
	for y := 60.0; y < 300 && freeX == 0; y += 10 {
		for x := 60.0; x < 1100; x += 10 {
			if g.overUI(x, y) {
				continue
			}
			if _, hit := sc.pick(cam, x, y); !hit {
				freeX, freeY = x, y
				break
			}
		}
	}
	if freeX == 0 {
		t.Skip("no clear screen point for the cursor")
	}
	mm.moveTo(int(freeX), int(freeY))
	frame()
	if !sc.hover.IsZero() {
		t.Fatalf("hover = %v on empty space, want no label", sc.hover)
	}
}
