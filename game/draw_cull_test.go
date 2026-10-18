package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestWorldDrawCullsOffscreenBodies pins the frame-cost fix: the world
// body pass must not submit a celestial whose disc cannot reach the
// viewport.
//
// Why it matters (measured, fix/ui-fps): at an AU-scale system the world
// units map to screen coordinates in the 1e7..1e13 px band once the camera
// is tight, and ebiten's vector path flattener (Arc → cubicTo → AddStroke)
// degenerates there — a single off-screen stroked circle cost ~5-20 ms
// against ~15 µs on-screen (pprof put StrokeCircle/AddStroke/Path.Arc at
// 54% of an idle frame; restoring only the missing cull put it back). The
// body layers drew every visible celestial regardless of where it landed,
// so a tight camera paid the whole system's vector-path cost every frame
// for geometry no pixel saw.
//
// The signal is a counter, never a wall clock: the number of body
// primitives the last Draw submitted (render.BodiesDrawnForTest) against
// the number of visible grid bodies. Before the cull the two are equal at
// every framing (every visible body was submitted); after it a mid
// framing submits strictly fewer, while the whole-system framing still
// submits every on-screen body (the cull must not be blunt).
func TestWorldDrawCullsOffscreenBodies(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	img := ebiten.NewImage(1280, 720)

	visible := 0
	for _, inf := range g.s.Grid() {
		if inf.Visible {
			visible++
		}
	}
	if visible < 2 {
		t.Fatalf("fixture: only %d visible grid bodies — the cull pin needs off-screen ones", visible)
	}

	at := func(zoom float64) int {
		t.Helper()
		g.space.zoom = zoom
		for i := 0; i < 5; i++ {
			if err := g.Update(); err != nil {
				t.Fatalf("Update: %v", err)
			}
			g.Draw(img)
		}
		return g.ren.BodiesDrawnForTest()
	}

	// Mid framing: one celestial is on screen, the AU-distance rest are
	// not. Before the cull this submitted all of them.
	mid := at(1e-6)
	if mid < 1 {
		t.Errorf("mid framing (zoom 1e-6) drew no body — the one on-screen celestial must still draw")
	}
	if mid >= visible {
		t.Errorf("mid framing drew %d bodies = all %d visible grid bodies: "+
			"off-screen bodies are still submitted (their vector paths land at "+
			"1e7..1e13 px screen coordinates, where ebiten's flattener degenerates)",
			mid, visible)
	}

	// Whole-system framing: zoomMin fits the system on screen, so every
	// visible body must draw — the cull must not drop on-screen geometry.
	system := at(2e-9)
	if system < visible {
		t.Errorf("whole-system framing drew %d bodies, want all %d visible — "+
			"the cull is dropping on-screen bodies", system, visible)
	}

	t.Logf("visible grid bodies=%d, mid(1e-6) drawn=%d, system(zoomMin) drawn=%d", visible, mid, system)
}
