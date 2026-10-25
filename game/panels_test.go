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
	uikit "github.com/entieve/entieve/uikit/screens"
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
			"hud": g.hud, "rack": g.rack, "log": bodyOf[*uikit.LogPanel](g.host, idLog), "inventory": bodyOf[*uikit.Inventory](g.host, idInventory),
			"fitting": bodyOf[*uikit.Fitting](g.host, idFitting), "market": bodyOf[*uikit.Market](g.host, idMarket), "manufacturing": bodyOf[*uikit.Manufacturing](g.host, idManufacturing),
			"refinery": bodyOf[*uikit.Refinery](g.host, idRefinery),
			"scanner":  bodyOf[*uikit.Scanner](g.host, idScanner), "map": bodyOf[*uikit.MapView](g.host, idMap), "help": bodyOf[*uikit.HelpPanel](g.host, idHelp), "menu": g.host.Window(idMenu),
			"station": g.svcWin, "kit": g.kit, "infowin": g.kit.InfoWindow(),
		} {
			if v == nil {
				t.Fatalf("%dx%d: panel %s is nil", size[0], size[1], name)
			}
		}

		// Mount everything: every toggle window, the overlays and the
		// docked station services.
		g.host.SetOpen(idInventory, true)
		g.host.SetOpen(idFitting, true)
		g.host.SetOpen(idMarket, true)
		g.host.SetOpen(idManufacturing, true)
		g.host.SetOpen(idRefinery, true)
		g.host.SetOpen(idScanner, true)
		g.host.SetOpen(idMap, true)
		g.host.SetOpen(idHelp, true)
		g.push(&MenuScene{g: g})
		dockAt(t, g)
		g.syncScenes()
		g.syncMounts()

		pump(t, g, size[0], size[1])
		pump(t, g, size[0], size[1])

		// The log seam accepts pushes without panicking.
		bodyOf[*uikit.LogPanel](g.host, idLog).PushLine(sim.SevInfo, "mount smoke")
		bodyOf[*uikit.LogPanel](g.host, idLog).Sync()

		assertInBounds(t, g, size[0], size[1])
	}
}

// TestPanelsShareTheKitsEnv: the extra panels are built from the kit's
// own environment (uikit.Kit.Env) — one theme/palette/face set per
// session — and that theme is the one the kit's own windows carry. A
// second uikit.NewEnv construction would hand the panels a different
// theme than the kit pumps.
func TestPanelsShareTheKitsEnv(t *testing.T) {
	g := newShowcaseGame(t)
	env := g.kit.Env()
	if g.env.WidgetTheme() != env.WidgetTheme() {
		t.Error("panel env theme is not the kit's theme")
	}
	if g.env.WidgetTheme() != g.kit.OverviewWindow().GetContainer().GetWidget().GetTheme() {
		t.Error("kit windows and the panels do not share one theme")
	}
	if g.env.Pal() != env.Pal() || g.env.TS() != env.TS() {
		t.Error("panel env palette/type scale differs from the kit's")
	}
	if g.env.Defs() != g.opts.Content || g.env.Player() != g.s.Player() {
		t.Error("panel env not bound to this session's sim/content")
	}
}

// TestPanelDefsCoverEnum: the Screen spec covers the enum — one row per
// panel id, in enum order, well formed (the full malformed-row matrix is
// TestScreenSpecRejectsMalformedRows) — and every derived registry entry
// resolves against a live session. A forgotten row is silent in
// production — the id would never mount, never persist and its want would
// read as "closed" — so the table is checked against the enum (and
// against the sidecar name list) here.
func TestPanelDefsCoverEnum(t *testing.T) {
	// Pass 1: the spec table itself (no session needed) — a broken row is
	// reported before anything tries to call into it.
	if probs := screenSpecErrors(screens); len(probs) > 0 {
		for _, p := range probs {
			t.Errorf("screen spec: %s", p)
		}
		t.FailNow() // the first frame would panic on a missing builder
	}
	seen := map[ScreenID]bool{}
	for _, s := range screens {
		if s.ID == "" || seen[s.ID] {
			t.Fatalf("screens: empty or duplicate id %q", s.ID)
		}
		seen[s.ID] = true
	}

	// Pass 2: the rows against a live session — every id resolves a real
	// built panel/window, and its mount rule computes in both views.
	g := newShowcaseGame(t)
	for _, s := range screens {
		if g.host.Window(s.ID) == nil {
			t.Errorf("panel %s: Window() returned nil", s.ID)
		}
		if g.host.Panel(s.ID) == nil {
			t.Errorf("panel %s: Panel() returned nil", s.ID)
		}
		_ = g.host.Want(s.ID, Room{})
		_ = g.host.Want(s.ID, Room{Docked: true})
	}

	// layoutNames = the core windows + every non-transient row, in spec
	// order — the sidecar and the table cannot drift apart.
	want := coreLayoutNames()
	for _, s := range screens {
		if s.DockPolicy != DockTransient {
			want = append(want, string(s.ID))
		}
	}
	if len(layoutNames) != len(want) {
		t.Fatalf("layoutNames = %v, want %v", layoutNames, want)
	}
	for i, name := range layoutNames {
		if name != want[i] {
			t.Errorf("layoutNames[%d] = %q, want %q", i, name, want[i])
		}
	}
	// Every named entry resolves to a window; rows keep a live open flag,
	// the core windows do not (infowin is written closed, never restored).
	for _, name := range layoutNames {
		e := g.layoutEntry(name)
		if e == nil || e.Win() == nil {
			t.Errorf("layoutEntry(%q) resolved no window", name)
		}
		if g.host.Slot(name) == nil && name != "overview" && name != "selected" && name != "infowin" {
			t.Errorf("layoutEntry(%q) is neither core nor a row", name)
		}
	}
	if g.layoutEntry("no-such-panel") != nil {
		t.Error("layoutEntry(unknown) resolved an entry")
	}
}

// TestToggleKeys: each toggle key opens and closes its panel, the
// overlay keys push and pop their scenes, and the Esc menu is modal for
// the other toggles.
func TestToggleKeys(t *testing.T) {
	g := newShowcaseGame(t)

	toggles := []struct {
		key ebiten.Key
		p   ScreenID
	}{
		{ebiten.KeyI, idInventory},
		{ebiten.KeyF, idFitting},
		{ebiten.KeyC, idScanner},
		{ebiten.KeyG, idMarket},
	}
	for _, tc := range toggles {
		g.handleKeys(Input{JustPressed: []ebiten.Key{tc.key}})
		if !g.host.IsOpen(tc.p) {
			t.Fatalf("%v: panel %s did not open", tc.key, panelNames[tc.p])
		}
		g.syncMounts()
		if !g.host.Mounted(tc.p) {
			t.Fatalf("%v: panel %s not mounted after open", tc.key, panelNames[tc.p])
		}
		g.handleKeys(Input{JustPressed: []ebiten.Key{tc.key}})
		if g.host.IsOpen(tc.p) {
			t.Fatalf("%v: panel %s did not close", tc.key, panelNames[tc.p])
		}
		g.syncMounts()
		if g.host.Mounted(tc.p) {
			t.Fatalf("%v: panel %s still mounted after close", tc.key, panelNames[tc.p])
		}
	}

	// Shift+L toggles the log window (open by default); plain L keeps
	// its loot binding and must not touch the log.
	shiftL := Input{JustPressed: []ebiten.Key{ebiten.KeyL}, KeysDown: []ebiten.Key{ebiten.KeyShift}}
	if !g.host.IsOpen(idLog) {
		t.Fatal("log window must start open")
	}
	g.handleKeys(shiftL)
	if g.host.IsOpen(idLog) {
		t.Fatal("Shift+L did not close the log")
	}
	g.syncMounts()
	if g.host.Mounted(idLog) {
		t.Fatal("log window still mounted after close")
	}
	g.handleKeys(shiftL)
	g.syncMounts()
	if !g.host.IsOpen(idLog) || !g.host.Mounted(idLog) {
		t.Fatal("Shift+L did not re-open the log window")
	}
	beforeLog := g.host.IsOpen(idLog)
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyL}})
	if g.host.IsOpen(idLog) != beforeLog {
		t.Error("plain L toggled the log (loot binding broken)")
	}

	// N toggles the Starmap window (a plain window: no scene).
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}})
	if !g.host.IsOpen(idMap) {
		t.Fatal("N did not open the starmap window")
	}
	g.syncMounts()
	if !g.host.Mounted(idMap) {
		t.Fatal("map window not mounted")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyN}})
	if g.host.IsOpen(idMap) {
		t.Fatal("N did not close the starmap window")
	}
	g.syncMounts()
	if g.host.Mounted(idMap) {
		t.Fatal("map window still mounted after close")
	}

	// H toggles the Help scene + window.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyH}})
	if _, ok := g.top().(*HelpScene); !ok || !g.host.IsOpen(idHelp) {
		t.Fatal("H did not open the help scene")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyH}})
	if _, ok := g.top().(*HelpScene); ok {
		t.Fatal("H did not close the help scene")
	}

	// Esc opens the modal menu; while it is open no other key toggles.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}})
	if _, ok := g.top().(*MenuScene); !ok || !g.host.IsOpen(idMenu) {
		t.Fatal("Esc did not open the menu")
	}
	g.syncMounts()
	if !g.host.Mounted(idMenu) {
		t.Fatal("menu window not mounted")
	}
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyI, ebiten.KeyN}})
	if g.host.IsOpen(idInventory) || g.host.IsOpen(idMap) {
		t.Fatal("toggle keys fired while the Esc menu is open")
	}
	// Esc again closes everything menu-related.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyEscape}})
	if _, ok := g.top().(*MenuScene); ok || g.host.IsOpen(idMenu) {
		t.Fatal("Esc did not close the menu")
	}
	g.syncMounts()
	if g.host.Mounted(idMenu) {
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
		{"log", bodyOf[*uikit.LogPanel](g.host, idLog).Container().GetWidget().Rect},
	} {
		x, y := centerOf(t, p.rect)
		mustHit = append(mustHit, point{p.name, x, y})
	}
	// The HUD pair: the bottom-anchored child of the HUD container.
	for _, ch := range g.hud.Container().Children() {
		w := ch.GetWidget()
		if w.Rect.Dx() > 0 && w.Rect.Min.Y > 400 {
			x, y := centerOf(t, w.Rect)
			mustHit = append(mustHit, point{"hud pair", x, y})
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

	// Off-panel: a clear point on empty space above the bottom chrome —
	// the handlers run.
	const cx, cy = 640.0, 300.0
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
	g.host.SetOpen(idInventory, true)
	g.syncMounts()
	pump(t, g, 1280, 720)
	ix, iy := centerOf(t, bodyOf[*uikit.Inventory](g.host, idInventory).Win().GetContainer().GetWidget().Rect)
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

		g.host.SetOpen(idInventory, true)
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
		if !g.host.IsOpen(idInventory) || !g.host.Mounted(idInventory) {
			t.Error("toggle state not preserved across the rebuild")
		}
		if g.hud == nil || g.rack == nil || bodyOf[*uikit.LogPanel](g.host, idLog) == nil {
			t.Fatal("panels missing after rebuild")
		}
		// Re-bound: every panel reads the live sim.
		if g.env.Player() != g.s.Player() {
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

// TestSceneTransitionsDock: docking opens exactly one window — the
// station services view — and hides the space chrome; undocking closes
// that view and leaves everything else as the player had it.
func TestSceneTransitionsDock(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)

	// Pre-dock: space chrome up, scanner open, fitting closed.
	g.host.SetOpen(idScanner, true)
	g.syncMounts()
	if !g.host.Mounted(idScanner) {
		t.Fatal("scanner not mounted in space")
	}
	if !chromeShown(g) {
		t.Fatal("space chrome wrong in space")
	}

	// Dock: the station scene pushes, the services view opens (alone),
	// chrome hides.
	dockAt(t, g)
	g.syncScenes()
	if _, ok := g.top().(*StationScene); !ok {
		t.Fatal("station scene not pushed on dock")
	}
	g.syncMounts()
	if !g.host.IsOpen(idServices) || !g.host.Mounted(idServices) {
		t.Error("services view not surfaced on dock")
	}
	for _, p := range []ScreenID{idInventory, idFitting, idMarket} {
		if g.host.IsOpen(p) || g.host.Mounted(p) {
			t.Errorf("docking opened %s too — only the services view auto-opens", panelNames[p])
		}
	}
	if g.host.Mounted(idScanner) {
		t.Error("scanner still mounted while docked")
	}
	if chromeShown(g) {
		t.Error("HUD/rack still visible while docked")
	}
	pump(t, g, 1280, 720)

	// Undock: the scene pops and closes the services view; nothing else
	// moved, and the scanner returns.
	g.s.Docked.Remove(g.s.Player())
	g.syncScenes()
	if _, ok := g.top().(*StationScene); ok {
		t.Fatal("station scene not popped on undock")
	}
	g.syncMounts()
	if g.host.IsOpen(idServices) || g.host.Mounted(idServices) {
		t.Error("services view still open after undock")
	}
	if !g.host.Mounted(idScanner) {
		t.Error("scanner not restored after undock")
	}
	if !chromeShown(g) {
		t.Error("space chrome wrong after undock")
	}
	pump(t, g, 1280, 720)
	assertInBounds(t, g, 1280, 720)
}

// TestStationPriorState: a window the player had open before docking
// stays open after undocking (undock closes only the services view).
func TestStationPriorState(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	g.host.SetOpen(idFitting, true)
	g.syncMounts()

	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()

	g.s.Docked.Remove(g.s.Player())
	g.syncScenes()
	g.syncMounts()
	if !g.host.IsOpen(idFitting) || !g.host.Mounted(idFitting) {
		t.Error("pre-open fitting window was closed by the undock")
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

// TestNextLockIdx pins the Tab cycle's anchoring semantics: the step is
// the position after the SELECTED candidate in the freshly sorted list
// (wrapping), and a selection outside the list restarts at the nearest —
// a remembered index would skip or repeat entries once the distances
// re-sort. The showcase grid spawns no lockable entities, so the pure
// step is where this can be pinned exactly.
func TestNextLockIdx(t *testing.T) {
	cands := []sim.Info{{Entity: 10}, {Entity: 11}, {Entity: 12}}
	cases := []struct {
		name string
		sel  ecs.Entity
		want int
	}{
		{"no selection starts at the nearest", 0, 0},
		{"advances from the first candidate", 10, 1},
		{"advances from the middle", 11, 2},
		{"wraps after the farthest", 12, 0},
		{"non-candidate selection restarts", 99, 0},
	}
	for _, c := range cases {
		if got := nextLockIdx(cands, c.sel); got != c.want {
			t.Errorf("%s: nextLockIdx(_, %v) = %d, want %d", c.name, c.sel, got, c.want)
		}
	}
	// A single candidate always wraps to itself.
	if got := nextLockIdx(cands[:1], 10); got != 0 {
		t.Errorf("single candidate: nextLockIdx = %d, want 0", got)
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
