package game

import (
	uikit "github.com/entieve/entieve/uikit/screens"
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/core"
	"github.com/entieve/entieve/sim"
)

// TestWorldClickSelection: a world click feeds Kit.Select, so the
// Selected Item panel and every selection-driven action read the same
// fresh selection (no stale game-local copy; the Tab cycle-lock adopts
// through the same seam).
func TestWorldClickSelection(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	for i := 0; i < 60; i++ {
		g.s.Step()
	}
	pump(t, g, 1280, 720)
	sc := g.space

	// A visible entity, then pan the camera so it lands on a clear
	// off-panel point (the showcase grid starts spread far apart).
	var target sim.Info
	for _, inf := range g.s.Grid() {
		if inf.Visible && inf.Entity != g.s.Player() {
			target = inf
			break
		}
	}
	if target.Entity.IsZero() {
		t.Fatal("no visible entity to click")
	}
	const clickX, clickY = 640.0, 300.0 // a clear off-panel point (above the bottom pair)
	cam := sc.Camera(1280, 720)
	// Pan the camera so the target lands on that point: the camera
	// center is player + pan, and screen = (world − center)·zoom + size/2.
	center := core.V(
		target.Pos.X-(clickX-640)/cam.Zoom,
		target.Pos.Y-(clickY-360)/cam.Zoom,
	)
	sc.pan = center.Sub(sc.playerPos())
	cam = sc.Camera(1280, 720)
	cx, cy := cam.WorldToScreen(target.Pos)
	if g.overUI(cx, cy) {
		t.Fatalf("click target (%v, %v) sits on a panel", cx, cy)
	}

	g.kit.Select(0)
	if err := sc.Update(Input{
		MouseX: cx, MouseY: cy, JustClicked: [2]bool{true, false},
	}); err != nil {
		t.Fatal(err)
	}
	if g.kit.Selected() != target.Entity {
		t.Fatalf("world click selected %v, want %v", g.kit.Selected(), target.Entity)
	}
	// The Selected Item panel resolves the same selection against the
	// live grid — this is the state its buttons act on.
	info, ok := g.selectedInfo()
	if !ok || info.Entity != target.Entity {
		t.Fatalf("selectedInfo = %v (ok=%v), want %v", info.Entity, ok, target.Entity)
	}
}

// TestChromeOnRootOverlay: the HUD (status + rack pair) and the navigation
// rail mount directly on the kit's root overlay (no full-screen wrapper
// windows), the rack and rail track hover, and their exact rects gate the
// world while empty space does not. The rack rides the HUD's pair, not the
// root. The log moved off the root — it is a window now (see
// TestLogWindowGate).
func TestChromeOnRootOverlay(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	kids := g.kit.Root().Children()
	mounted := map[*widget.Container]bool{}
	for _, ch := range kids {
		c, ok := ch.(*widget.Container)
		if !ok {
			t.Fatalf("root child of type %T, want *widget.Container", ch)
		}
		mounted[c] = true
	}
	if len(kids) != 2 {
		t.Errorf("root overlay has %d children, want exactly the HUD and rail", len(kids))
	}
	for name, c := range map[string]*widget.Container{
		"hud": g.hud.Container(), "rail": g.rail.Container(),
	} {
		if !mounted[c] {
			t.Errorf("%s is not mounted on the root overlay", name)
		}
	}
	// The rack rides the HUD's pair (the right half), not the root.
	if mounted[g.rack.Container()] {
		t.Error("rack is mounted on the root overlay (it belongs to the HUD pair)")
	}
	if !g.rack.Container().GetWidget().Rect.In(g.hud.Container().GetWidget().Rect) {
		t.Errorf("rack %v is not inside the HUD container %v",
			g.rack.Container().GetWidget().Rect, g.hud.Container().GetWidget().Rect)
	}
	if mounted[bodyOf[*uikit.LogPanel](g.host, idLog).Container()] {
		t.Error("log still rides the root overlay (it is a window now)")
	}

	// The rack tracks hover so input.UIHovered covers its padding; the
	// HUD container must not: it spans the whole screen. The log window
	// container tracks hover through the window chrome itself.
	if !g.rack.Container().GetWidget().TrackHover || !bodyOf[*uikit.LogPanel](g.host, idLog).Container().GetWidget().TrackHover ||
		!g.rail.Container().GetWidget().TrackHover {
		t.Error("rack/log/rail containers do not track hover")
	}
	if g.hud.Container().GetWidget().TrackHover {
		t.Error("HUD container tracks hover (it spans the whole screen)")
	}

	// overUI covers every chrome rect exactly: the HUD's bottom pair (the
	// visible bottom child, which carries the rack) and the log.
	hudGated := false
	for _, ch := range g.hud.Container().Children() {
		w := ch.GetWidget()
		if w.IsVisible() && w.Rect.Dx() > 0 && w.Rect.Min.Y > 400 {
			x, y := centerOf(t, w.Rect)
			if !g.overUI(x, y) {
				t.Errorf("hud pair: overUI(%v, %v) = false, want true", x, y)
			}
			hudGated = true
		}
	}
	if !hudGated {
		t.Error("no visible HUD pair laid out")
	}
	for _, p := range []struct {
		name string
		rect image.Rectangle
	}{
		{"rack", g.rack.Container().GetWidget().Rect},
		{"log", bodyOf[*uikit.LogPanel](g.host, idLog).Container().GetWidget().Rect},
		{"rail", g.rail.Container().GetWidget().Rect},
	} {
		x, y := centerOf(t, p.rect)
		if !g.overUI(x, y) {
			t.Errorf("%s: overUI(%v, %v) = false, want true", p.name, x, y)
		}
	}

	// Empty space stays free: a full-screen wrapper window around the
	// chrome would gate the whole screen. (640,300) sits above the
	// bottom-center pair, between the top windows.
	if g.overUI(640, 300) {
		t.Error("empty space is gated (a full-screen chrome wrapper window is back)")
	}
}

// TestCoreWindowsDockFlow: the Overview/Selected Item rects gate world
// input in space, SetCoreVisible hides both while docked (station
// parity) and restores them on undock.
func TestCoreWindowsDockFlow(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)

	ov := g.kit.OverviewWindow().GetContainer().GetWidget()
	sp := g.kit.SelectedWindow().GetContainer().GetWidget()
	if !ov.IsVisible() || !sp.IsVisible() {
		t.Fatal("core windows hidden in space")
	}
	for _, p := range []struct {
		name string
		w    *widget.Widget
	}{
		{"overview", ov}, {"selected", sp},
	} {
		x, y := centerOf(t, p.w.Rect)
		if !g.overUI(x, y) {
			t.Errorf("%s: overUI(%v, %v) = false, want true", p.name, x, y)
		}
	}

	// Dock: the station scene flows SetCoreVisible(false) through
	// syncMounts — both windows hide and stop gating the world. Probe the
	// overview's left margin: the station contract board now covers its
	// centre while docked (a visible panel that gates correctly), so the
	// hidden-overview check must run where only the overview itself
	// could gate.
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	if ov.IsVisible() || sp.IsVisible() {
		t.Error("core windows visible while docked")
	}
	_, cy := centerOf(t, ov.Rect)
	if g.overUI(float64(ov.Rect.Min.X), cy) {
		t.Errorf("hidden overview still gates the world at (%v, %v)", ov.Rect.Min.X, cy)
	}

	// Undock: both come back and gate again.
	g.s.Docked.Remove(g.s.Player())
	g.syncScenes()
	g.syncMounts()
	pump(t, g, 1280, 720)
	if !ov.IsVisible() || !sp.IsVisible() {
		t.Fatal("core windows not restored after undock")
	}
	if x, y := centerOf(t, ov.Rect); !g.overUI(x, y) {
		t.Error("overview does not gate the world after undock")
	}
}

// TestLogHistorySurvivesLoad: Lines() round-trips through the save/load
// rebuild — the log is captured before buildKit and restored after.
func TestLogHistorySurvivesLoad(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	for i := 0; i < 30; i++ {
		g.s.Step()
	}
	bodyOf[*uikit.LogPanel](g.host, idLog).PushLine(sim.SevInfo, "before the load")
	bodyOf[*uikit.LogPanel](g.host, idLog).PushLine(sim.SevGood, "second line")
	before := bodyOf[*uikit.LogPanel](g.host, idLog).Lines()
	if len(before) < 2 {
		t.Fatalf("log lines = %d, want the pushed lines", len(before))
	}
	oldLog := bodyOf[*uikit.LogPanel](g.host, idLog)
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	if err := g.LoadGame(); err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if bodyOf[*uikit.LogPanel](g.host, idLog) == oldLog {
		t.Fatal("log panel was not rebuilt against the restored sim")
	}
	after := bodyOf[*uikit.LogPanel](g.host, idLog).Lines()
	if len(after) != len(before) {
		t.Fatalf("log lines after load = %d, want %d", len(after), len(before))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("line %d after load = %q, want %q", i, after[i], before[i])
		}
	}
}
