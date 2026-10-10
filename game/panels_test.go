package game

import (
	"errors"
	"image"
	"math"
	"testing"
	"time"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestPanelMountAndSync constructs every panel over the world-showcase
// sim and pumps it: each frame syncs all panels and advances the kit
// without panicking, and every mounted window stays on screen.
func TestPanelMountAndSync(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		g := newShowcaseGame(t)

		// Every panel must be constructed by New.
		for name, v := range map[string]any{
			"hud": g.hud, "rack": g.rack, "log": g.logP, "inventory": g.inv,
			"fitting": g.fitt, "market": g.mkt, "scanner": g.scan,
			"map": g.mp, "help": g.help, "menu": g.menuWin,
			"station": g.stationW, "kit": g.kit,
		} {
			if v == nil {
				t.Fatalf("%dx%d: panel %s is nil", size[0], size[1], name)
			}
		}

		// Mount everything: every toggle window, the overlays and the
		// docked station services.
		g.open[pInventory], g.open[pFitting], g.open[pMarket] = true, true, true
		g.open[pScanner], g.open[pMap], g.open[pHelp] = true, true, true
		g.push(&MenuScene{g: g})
		dockAt(t, g)
		g.syncScenes()
		g.syncMounts()

		pump(t, g, size[0], size[1])
		pump(t, g, size[0], size[1])

		// The log seam accepts pushes without panicking.
		g.logP.PushLine(sim.SevInfo, "mount smoke")
		g.logP.Sync()

		assertInBounds(t, g, size[0], size[1])
	}
}

// TestToggleKeys: each toggle key opens and closes its panel, the
// overlay keys push and pop their scenes, and the Esc menu is modal for
// the other toggles.
func TestToggleKeys(t *testing.T) {
	g := newShowcaseGame(t)

	toggles := []struct {
		key ebiten.Key
		p   panelID
	}{
		{ebiten.KeyI, pInventory},
		{ebiten.KeyF, pFitting},
		{ebiten.KeyC, pScanner},
		{ebiten.KeyG, pMarket},
	}
	for _, tc := range toggles {
		g.handleKeys(Input{JustPressed: []ebiten.Key{tc.key}})
		if !g.open[tc.p] {
			t.Fatalf("%v: panel %s did not open", tc.key, panelNames[tc.p])
		}
		g.syncMounts()
		if g.mounted[tc.p] == nil {
			t.Fatalf("%v: panel %s not mounted after open", tc.key, panelNames[tc.p])
		}
		g.handleKeys(Input{JustPressed: []ebiten.Key{tc.key}})
		if g.open[tc.p] {
			t.Fatalf("%v: panel %s did not close", tc.key, panelNames[tc.p])
		}
		g.syncMounts()
		if g.mounted[tc.p] != nil {
			t.Fatalf("%v: panel %s still mounted after close", tc.key, panelNames[tc.p])
		}
	}

	// N toggles the MapView scene + window.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}})
	if _, ok := g.top().(*MapScene); !ok || !g.open[pMap] {
		t.Fatal("N did not open the map scene")
	}
	g.syncMounts()
	if g.mounted[pMap] == nil {
		t.Fatal("map window not mounted")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}})
	if _, ok := g.top().(*MapScene); ok || g.open[pMap] {
		t.Fatal("N did not close the map scene")
	}
	g.syncMounts()
	if g.mounted[pMap] != nil {
		t.Fatal("map window still mounted after close")
	}

	// H toggles the Help scene + window.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyH}})
	if _, ok := g.top().(*HelpScene); !ok || !g.open[pHelp] {
		t.Fatal("H did not open the help scene")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyH}})
	if _, ok := g.top().(*HelpScene); ok {
		t.Fatal("H did not close the help scene")
	}

	// Esc opens the modal menu; while it is open no other key toggles.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}})
	if _, ok := g.top().(*MenuScene); !ok || !g.open[pMenu] {
		t.Fatal("Esc did not open the menu")
	}
	g.syncMounts()
	if g.mounted[pMenu] == nil {
		t.Fatal("menu window not mounted")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyI, ebiten.KeyN}})
	if g.open[pInventory] || g.open[pMap] {
		t.Fatal("toggle keys fired while the Esc menu is open")
	}
	// Esc again closes everything menu-related.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}})
	if _, ok := g.top().(*MenuScene); ok || g.open[pMenu] {
		t.Fatal("Esc did not close the menu")
	}
	g.syncMounts()
	if g.mounted[pMenu] != nil {
		t.Fatal("menu window still mounted after Esc")
	}
}

// TestWorldInputGating: world mouse handling is suppressed over every
// measurable panel rect and runs again off-panel.
func TestWorldInputGating(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	for i := 0; i < 60; i++ {
		g.s.Step()
	}
	pump(t, g, 1280, 720)
	sc := g.space

	// Points on the always-mounted chrome strips.
	type point struct {
		name string
		x, y float64
	}
	mustHit := []point{}
	for _, p := range []struct {
		name string
		rect image.Rectangle
	}{
		{"rack", g.rack.Container().GetWidget().Rect},
		{"log", g.logP.Container().GetWidget().Rect},
	} {
		x, y := centerOf(t, p.rect)
		mustHit = append(mustHit, point{p.name, x, y})
	}
	// The HUD bar: the bottom-anchored child of the HUD container.
	for _, ch := range g.hud.Container().Children() {
		w := ch.GetWidget()
		if w.Rect.Dx() > 0 && w.Rect.Min.Y > 400 {
			x, y := centerOf(t, w.Rect)
			mustHit = append(mustHit, point{"hud bar", x, y})
			break
		}
	}

	for _, m := range mustHit {
		if !g.overUI(m.x, m.y) {
			t.Errorf("%s: overUI(%v, %v) = false, want true", m.name, m.x, m.y)
		}
		// Wheel zoom must not fire over a panel.
		zoom := sc.zoom
		if err := sc.Update(Input{MouseX: m.x, MouseY: m.y, Scroll: 1}); err != nil {
			t.Fatal(err)
		}
		if sc.zoom != zoom {
			t.Errorf("%s: wheel zoom fired over the panel", m.name)
		}
		// A click must not reach world handling at all (no double-click
		// bookkeeping, no selection).
		g.kit.Select(0)
		sc.lastClickAt = time.Time{}
		if err := sc.Update(Input{MouseX: m.x, MouseY: m.y, JustClicked: [2]bool{true, false}}); err != nil {
			t.Fatal(err)
		}
		if !sc.lastClickAt.IsZero() {
			t.Errorf("%s: world click processed over the panel", m.name)
		}
	}

	// Off-panel: a clear point on empty space — the handlers run.
	const cx, cy = 640.0, 500.0
	if g.overUI(cx, cy) {
		t.Fatalf("overUI(%v, %v) = true on empty space", cx, cy)
	}
	zoom := sc.zoom
	if err := sc.Update(Input{MouseX: cx, MouseY: cy, Scroll: 1}); err != nil {
		t.Fatal(err)
	}
	if sc.zoom == zoom {
		t.Error("wheel zoom did not fire on empty space")
	}
	sc.lastClickAt = time.Time{}
	if err := sc.Update(Input{MouseX: cx, MouseY: cy, JustClicked: [2]bool{true, false}}); err != nil {
		t.Fatal(err)
	}
	if sc.lastClickAt.IsZero() {
		t.Error("world click did not process on empty space")
	}

	// A mounted toggle window gates just like the chrome strips.
	g.open[pInventory] = true
	g.syncMounts()
	pump(t, g, 1280, 720)
	ix, iy := centerOf(t, g.inv.Win().GetContainer().GetWidget().Rect)
	if !g.overUI(ix, iy) {
		t.Errorf("inventory window not gated: overUI(%v, %v) = false", ix, iy)
	}
}

// TestSaveRebuildLoad: Ctrl+S semantics end-to-end — snapshot, mutate,
// restore — with the kit and every panel rebuilt against the restored
// sim, toggle state preserved and a draw smoke at both design sizes.
func TestSaveRebuildLoad(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		g := newShowcaseGame(t)
		g.applySize(size[0], size[1])
		for i := 0; i < 100; i++ {
			g.s.Step()
		}

		g.open[pInventory] = true
		g.syncMounts()
		if err := g.SaveGame(); err != nil {
			t.Fatalf("SaveGame: %v", err)
		}
		saved := g.s.Time()

		// Mutate the world and the interface state.
		for i := 0; i < 500; i++ {
			g.s.Step()
		}
		if g.s.Time() <= saved {
			t.Fatalf("sim did not advance (t=%v)", g.s.Time())
		}
		g.setSelected(ecs.Entity(42))
		oldKit := g.kit

		if err := g.LoadGame(); err != nil {
			t.Fatalf("LoadGame: %v", err)
		}
		if math.Abs(g.s.Time()-saved) > 1e-9 {
			t.Errorf("%dx%d: restored time = %v, want %v", size[0], size[1], g.s.Time(), saved)
		}
		if g.kit == oldKit {
			t.Error("kit was not rebuilt")
		}
		if g.kit.Selected() != 0 {
			t.Errorf("selection not reset (selected=%v)", g.kit.Selected())
		}
		if !g.open[pInventory] || g.mounted[pInventory] == nil {
			t.Error("toggle state not preserved across the rebuild")
		}
		if g.hud == nil || g.rack == nil || g.logP == nil {
			t.Fatal("panels missing after rebuild")
		}
		// Re-bound: every panel reads the live sim.
		if g.env.Sim != g.s {
			t.Error("env not rebound to the restored sim")
		}
		pump(t, g, size[0], size[1])
		assertInBounds(t, g, size[0], size[1])

		// A failed load reports an error, not a crash.
		g.opts.SavePath += ".missing"
		if err := g.LoadGame(); err == nil {
			t.Error("LoadGame without a file succeeded")
		}
	}
}

// TestSceneTransitionsDock: docking surfaces the station windows and
// hides the space chrome; undocking restores the pre-dock state.
func TestSceneTransitionsDock(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)

	// Pre-dock: space chrome up, scanner open, fitting closed.
	g.open[pScanner] = true
	g.syncMounts()
	if g.mounted[pScanner] == nil {
		t.Fatal("scanner not mounted in space")
	}
	if !chromeShown(g) || g.stShown {
		t.Fatal("chrome/station state wrong in space")
	}

	// Dock: the station scene pushes, services open, chrome hides.
	dockAt(t, g)
	g.syncScenes()
	if _, ok := g.top().(*StationScene); !ok {
		t.Fatal("station scene not pushed on dock")
	}
	g.syncMounts()
	for _, p := range stationPanels {
		if !g.open[p] || g.mounted[p] == nil {
			t.Errorf("station panel %s not surfaced on dock", panelNames[p])
		}
	}
	if g.mounted[pScanner] != nil {
		t.Error("scanner still mounted while docked")
	}
	if chromeShown(g) {
		t.Error("HUD/rack still visible while docked")
	}
	if !g.stShown {
		t.Fatal("station strip not mounted while docked")
	}
	pump(t, g, 1280, 720)

	// Undock: the scene pops and the auto-opened windows close again
	// (their pre-dock state was closed); the scanner returns.
	g.s.Docked.Remove(g.s.Player())
	g.syncScenes()
	if _, ok := g.top().(*StationScene); ok {
		t.Fatal("station scene not popped on undock")
	}
	g.syncMounts()
	for _, p := range stationPanels {
		if g.open[p] || g.mounted[p] != nil {
			t.Errorf("station panel %s not restored on undock", panelNames[p])
		}
	}
	if g.mounted[pScanner] == nil {
		t.Error("scanner not restored after undock")
	}
	if !chromeShown(g) || g.stShown {
		t.Error("chrome/station state wrong after undock")
	}
	pump(t, g, 1280, 720)
	assertInBounds(t, g, 1280, 720)
}

// TestStationPriorState: a window the player had open before docking
// stays open after undocking.
func TestStationPriorState(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	g.open[pFitting] = true
	g.syncMounts()

	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()

	g.s.Docked.Remove(g.s.Player())
	g.syncScenes()
	g.syncMounts()
	if !g.open[pFitting] || g.mounted[pFitting] == nil {
		t.Error("pre-open fitting window was closed by the undock restore")
	}
}

// TestFrameUpdateAndWorldKeys drives the real frame loop headless and
// checks the world key bindings reach the sim.
func TestFrameUpdateAndWorldKeys(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)

	// A few real frames: input snapshot, scene, stepping, event drain,
	// panel sync and the kit pump.
	for i := 0; i < 5; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("Update frame %d: %v", i, err)
		}
	}
	ticks := g.s.Ticks()
	if ticks == 0 {
		t.Fatal("simulation did not step")
	}

	// F1 reaches the sim: the first high slot is the showcase EMP
	// turret, which answers an activation attempt with a guard event
	// ("requires a target") when nothing is selected.
	g.s.Events() // drain whatever the frame loop left
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyF1}})
	g.s.Step()
	if evs := g.s.Events(); len(evs) == 0 {
		t.Fatal("F1 did not reach the simulation (no guard event)")
	}
	fit, ok := g.s.Fittings.Get(g.s.Player())
	if !ok {
		t.Fatal("player has no fitting")
	}
	if rts := fit.Runtimes(content.SlotHigh); len(rts) == 0 || rts[0].Active {
		t.Fatal("turret activated without a target")
	}

	// The remaining world keys must run without panicking (their effects
	// depend on grid state; no selection is set here).
	g.handleKeys(Input{JustPressed: []ebiten.Key{
		ebiten.KeyTab, ebiten.KeyA, ebiten.KeyQ, ebiten.KeyS, ebiten.KeyW,
		ebiten.KeyZ, ebiten.KeyD, ebiten.KeyJ, ebiten.KeyL, ebiten.KeyM,
	}})
	for i := 0; i < 10; i++ {
		g.s.Step()
	}

	// The staged menu load path rebuilds the kit after the pump.
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	oldKit := g.kit
	g.pendingLoad = true
	if err := g.Update(); err != nil {
		t.Fatalf("Update with pending load: %v", err)
	}
	if g.kit == oldKit {
		t.Fatal("staged load did not rebuild the kit")
	}

	// Quit flag terminates the loop.
	g.quit = true
	if err := g.Update(); !errors.Is(err, ebiten.Termination) {
		t.Fatalf("quit Update error = %v, want ebiten.Termination", err)
	}
}

// centerOf returns a rect's center point.
func centerOf(t *testing.T, r image.Rectangle) (float64, float64) {
	t.Helper()
	if r.Dx() == 0 || r.Dy() == 0 {
		t.Fatalf("zero rect %v (layout never ran)", r)
	}
	return float64(r.Min.X + r.Dx()/2), float64(r.Min.Y + r.Dy()/2)
}
