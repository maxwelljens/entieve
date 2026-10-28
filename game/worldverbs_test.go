package game

import (
	"testing"

	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/hajimehoshi/ebiten/v2"
)

// panUnderCursor pans the space scene so world point pos lands on the
// clear off-panel screen point the click tests use, and returns that
// screen point.
func panUnderCursor(g *Game, pos core.Vec2) (float64, float64) {
	sc := g.space
	const clickX, clickY = 640.0, 300.0
	cam := sc.Camera(1280, 720)
	// The camera center is player + pan, and
	// screen = (world − center)·zoom + size/2.
	center := core.V(
		pos.X-(clickX-640)/cam.Zoom,
		pos.Y-(clickY-360)/cam.Zoom,
	)
	sc.pan = center.Sub(sc.playerPos())
	cam = sc.Camera(1280, 720)
	return cam.WorldToScreen(pos)
}

// TestRightClickOrbitKeepsItsDefaultRange pins the click path's explicit
// range: the right-click orbit flies at uikit.OrbitRanges[0] even after
// the Selected Item selector was walked to its last rung. The intent
// carries the number — a zero Range would silently follow the selector
// instead (the old direct queue passed the constant).
func TestRightClickOrbitKeepsItsDefaultRange(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	for i := 0; i < 60; i++ {
		g.s.Step()
	}
	pump(t, g, 1280, 720)

	var target sim.Info
	for _, inf := range g.s.Grid() {
		if inf.Visible && inf.Entity != g.s.Player() {
			target = inf
			break
		}
	}
	if target.Entity.IsZero() {
		t.Fatal("fixture: no visible entity to click")
	}
	cx, cy := panUnderCursor(g, target.Pos)
	if g.overUI(cx, cy) {
		t.Fatalf("click target (%v, %v) sits on a panel", cx, cy)
	}

	// Walk the orbit selector away from its first rung.
	last := uikit.OrbitRanges[len(uikit.OrbitRanges)-1]
	fireSelector(t, g, "Orbit range:", len(uikit.OrbitRanges)-1)
	if got := g.kit.OrbitRange(); got != last {
		t.Fatalf("orbit selector = %v, want %v", got, last)
	}

	if err := g.space.Update(Input{
		MouseX: cx, MouseY: cy, JustClicked: [2]bool{false, true},
	}); err != nil {
		t.Fatal(err)
	}
	g.s.Step()
	intent, ok := g.s.Intents.Get(g.s.Player())
	if !ok || intent.Kind != sim.IntentOrbit {
		t.Fatalf("right-click intent = %+v (ok=%v), want IntentOrbit", intent, ok)
	}
	if intent.Target != target.Entity || intent.Range != uikit.OrbitRanges[0] {
		t.Errorf("right-click orbit = {Target:%v Range:%v}, want {Target:%v Range:%v}",
			intent.Target, intent.Range, target.Entity, uikit.OrbitRanges[0])
	}
}

// TestShiftTabNoLongerLocksAStation pins the disclosed behavior change:
// Shift+Tab used to stand a CmdLock at whatever was selected (the sim then
// locked a station if it was in range); the unified verb table refuses a
// station exactly like the Selected Item panel always did.
func TestShiftTabNoLongerLocksAStation(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	for i := 0; i < 3; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("Update frame %d: %v", i, err)
		}
	}

	var station sim.Info
	for _, inf := range g.s.Grid() {
		if inf.Kind == sim.KindStation {
			station = inf
			break
		}
	}
	if station.Entity.IsZero() {
		t.Fatal("fixture: no station on the grid")
	}
	// Park the station inside lock range: the old key path would have
	// locked it, so an empty Targeting is the new answer, not a range miss.
	ply, _ := g.s.Transforms.Get(g.s.Player())
	g.s.Transforms.Set(station.Entity, sim.Transform{Pos: ply.Pos.Add(core.V(1000, 0))})
	g.setSelected(station.Entity)
	g.s.Events() // drain the frame loop's log

	g.handleKeys(Input{
		KeysDown:    []ebiten.Key{ebiten.KeyShift},
		JustPressed: []ebiten.Key{ebiten.KeyTab},
	})
	g.s.Step()
	if tg, ok := g.s.Targetings.Get(g.s.Player()); ok && len(tg.Locks) != 0 {
		t.Errorf("Shift+Tab locked a station: %v", tg.Locks)
	}
}
