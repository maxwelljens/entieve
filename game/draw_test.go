package game

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	"github.com/hajimehoshi/ebiten/v2"
)

// newSmokeGame builds a game session over the embedded content with a
// temp save path (tests must never touch the user config dir).
func newSmokeGame(t *testing.T) *Game {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	g, err := New(Options{Content: c, SavePath: filepath.Join(t.TempDir(), "save.json")})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	return g
}

// newShowcaseGame builds a session over the world-showcase sim, the same
// fixture the uikit panel tests drive, on a fresh temp save path.
func newShowcaseGame(t *testing.T) *Game {
	t.Helper()
	return newShowcaseGameAt(t, filepath.Join(t.TempDir(), "save.json"))
}

// newShowcaseGameAt builds the same session on an explicit save path —
// several sessions can share one path to simulate restarts (and their
// layout sidecars).
func newShowcaseGameAt(t *testing.T, savePath string) *Game {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing from content")
	}
	g, err := New(Options{
		Content:  c,
		World:    wd,
		Seed:     42,
		SavePath: savePath,
	})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	return g
}

// newShowcaseGameLoaded builds the same session restored from the save
// file at path through Options.Save (the --load path), so a test can
// restart a session exactly as a fresh process would — docked state,
// wallet and all — on the same path (and its layout sidecar).
func newShowcaseGameLoaded(t *testing.T, savePath string) *Game {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing from content")
	}
	data, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read save: %v", err)
	}
	var save sim.Save
	if err := json.Unmarshal(data, &save); err != nil {
		t.Fatalf("parse save: %v", err)
	}
	g, err := New(Options{
		Content:  c,
		World:    wd,
		Seed:     42,
		Save:     &save,
		SavePath: savePath,
	})
	if err != nil {
		t.Fatalf("game.New(--load): %v", err)
	}
	return g
}

// pump runs two full frames at the given size through the PRODUCTION
// pipeline — Game.Update (handleKeys → top scene → fixed-step sim →
// event drain → syncScenes → syncPanels → syncMounts → kit pump →
// staged load) followed by Game.Draw — and returns the drawn image (the
// second frame settles layout). Driving a hand-assembled subset here
// would let a reordered Update pass the whole suite; the ordering pins
// below (mount reconciliation, same-frame event drain, staged load) live
// on this seam. Manual sync calls before pump stay valid — Update runs
// them again.
func pump(t *testing.T, g *Game, w, h int) *ebiten.Image {
	t.Helper()
	g.applySize(w, h)
	img := ebiten.NewImage(w, h)
	for i := 0; i < 2; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("Update frame %d: %v", i, err)
		}
		g.Draw(img)
	}
	return img
}

// layoutPump runs the production size path at a DEVICE scale: Layout
// (which multiplies the window px by the device factor, so the design
// viewport stays w×h and the whole layout is scale-invariant), then two
// full frames through Game.Update/Draw like pump. It is the size path a
// real window takes; pump's applySize is the design-px-only shortcut.
func layoutPump(t *testing.T, g *Game, w, h int) {
	t.Helper()
	g.Layout(w, h)
	img := ebiten.NewImage(g.w, g.h)
	for i := 0; i < 2; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("Update frame %d: %v", i, err)
		}
		g.Draw(img)
	}
}

// mountedRects returns the screen rect of every visible panel: the kit's
// core windows, the mounted toggle windows (the log and the services
// view among them) and the root-overlay chrome (the HUD's pair — status
// and rack — and the rail). Hidden panels are skipped — they never lay
// out and keep a stale rect.
func mountedRects(g *Game) []struct {
	name string
	rect image.Rectangle
} {
	var out []struct {
		name string
		rect image.Rectangle
	}
	add := func(name string, w *widget.Widget) {
		if !w.IsVisible() {
			return
		}
		out = append(out, struct {
			name string
			rect image.Rectangle
		}{name, w.Rect})
	}
	add("overview", g.kit.OverviewWindow().GetContainer().GetWidget())
	add("selected", g.kit.SelectedWindow().GetContainer().GetWidget())
	for p := panelID(0); p < numPanels; p++ {
		if g.mounted[p] != nil {
			add(panelNames[p], g.panelWindow(p).GetContainer().GetWidget())
		}
	}
	// Root-overlay chrome: the HUD's visible children (the target-box row
	// is hidden until something is locked; hud.1 is the bottom-center pair,
	// which carries the rack) and the navigation rail.
	if g.hud != nil && isVisible(g.hud.Container().GetWidget()) {
		for i, ch := range g.hud.Container().Children() {
			add("hud."+strconv.Itoa(i), ch.GetWidget())
		}
	}
	if g.rail != nil {
		add("rail", g.rail.Container().GetWidget())
	}
	return out
}

// chromeShown reports whether the space chrome (HUD + module rack) is
// visible on the root overlay.
func chromeShown(g *Game) bool {
	return isVisible(g.hud.Container().GetWidget()) &&
		isVisible(g.rack.Container().GetWidget())
}

// panelNames labels the panel ids for test output — derived from the
// Screen spec (the sidecar key, or the lowercased title for the screen
// that is not persisted).
var panelNames = func() map[panelID]string {
	m := make(map[panelID]string, len(screens))
	for _, s := range screens {
		m[s.ID] = screenLabel(s)
	}
	return m
}()

// defaultOverviewRect is the Overview's default placement for a w-wide
// viewport at the identity scale: the upper-right corner (placeDefaults),
// at the window's 380×409 design default size (uikit's overview
// constructor; the height clears the module rack at the 1024×700
// minimum).
func defaultOverviewRect(w int) image.Rectangle {
	return image.Rect(w-10-380, 8, w-10, 8+409)
}

// assertInBounds fails the test if any mounted window sticks out of the
// screen.
func assertInBounds(t *testing.T, g *Game, w, h int) {
	t.Helper()
	for _, r := range mountedRects(g) {
		if r.rect.Dx() == 0 && r.rect.Dy() == 0 {
			t.Errorf("%s: never laid out (zero rect)", r.name)
			continue
		}
		if r.rect.Min.X < 0 || r.rect.Min.Y < 0 || r.rect.Max.X > w || r.rect.Max.Y > h {
			t.Errorf("%dx%d: %s window %v out of bounds", w, h, r.name, r.rect)
		}
	}
}

// assertNoOverlap fails the test if any two visible default panels share
// screen space: the default placement must be pairwise disjoint at both
// design sizes (toggling extra windows on top of each other is allowed —
// they are modal overlays or user-dragged).
//
// One documented exception: at the 1024×700 minimum the log's content
// floor (logMinW = 260 design px) is wider than the pocket left of the
// bottom-center pair, so the two overlap there — pre-existing (the old
// bottom bar did the same once its labels filled in) and pinned by
// TestVirginPlacementTable's comment. At 1280×720 the pair clears it.
func assertNoOverlap(t *testing.T, g *Game, w, h int) {
	t.Helper()
	rects := mountedRects(g)
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			if w < 1280 && pairPair(rects[i].name, rects[j].name) {
				continue
			}
			if rects[i].rect.Overlaps(rects[j].rect) {
				t.Errorf("%dx%d: default %s %v overlaps %s %v",
					w, h, rects[i].name, rects[i].rect, rects[j].name, rects[j].rect)
			}
		}
	}
}

// pairPair reports the log / bottom-pair overlap allowed at the 1024
// minimum only (see assertNoOverlap).
func pairPair(a, b string) bool { return (a == "log" && b == "hud.1") || (a == "hud.1" && b == "log") }

// TestSceneDrawSmoke draws every scene (space world + interface, help
// overlay, starmap overlay, Esc menu, station view) at the design and
// minimum window sizes — headless offscreen only; geometry bugs surface
// as panics or out-of-bounds windows.
func TestSceneDrawSmoke(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {1024, 700}} {
		g := newSmokeGame(t)
		g.applySize(size[0], size[1])
		img := ebiten.NewImage(size[0], size[1])

		// Let the sim populate the grid (visible entities, effects).
		for i := 0; i < 300; i++ {
			g.s.Step()
		}
		g.Draw(img)
		assertInBounds(t, g, size[0], size[1])
		assertNoOverlap(t, g, size[0], size[1])

		// Help overlay: full two-column key map.
		g.push(&HelpScene{g: g})
		g.syncMounts()
		pump(t, g, size[0], size[1])
		g.Draw(img)

		// Starmap overlay on top of the help overlay.
		g.push(&MapScene{g: g})
		g.syncMounts()
		pump(t, g, size[0], size[1])
		g.Draw(img)

		// Esc menu (modal) above both.
		g.push(&MenuScene{g: g})
		g.syncMounts()
		pump(t, g, size[0], size[1])
		g.Draw(img)
		g.pop()
		g.pop()
		g.pop()
		g.syncMounts()

		// Station view: docked chrome + the services view.
		dockAt(t, g)
		g.syncScenes()
		g.syncMounts()
		pump(t, g, size[0], size[1])
		g.Draw(img)
		assertInBounds(t, g, size[0], size[1])
		assertNoOverlap(t, g, size[0], size[1])
	}
}

// TestHelpWindowFits: the help window (every KeyMap row over the sensor
// blurb) sizes itself inside the minimum window.
func TestHelpWindowFits(t *testing.T) {
	g := newSmokeGame(t)
	g.open[pHelp] = true
	g.syncMounts()
	pump(t, g, 1024, 700)
	r := g.help.Win().GetContainer().GetWidget().Rect
	if r.Dx() == 0 || r.Dy() == 0 {
		t.Fatal("help window never laid out")
	}
	if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > 1024 || r.Max.Y > 700 {
		t.Fatalf("help window %v out of bounds at 1024x700", r)
	}
	if len(helpRows()) != len(KeyMap) {
		t.Fatalf("help rows = %d, want %d (one per KeyMap binding)", len(helpRows()), len(KeyMap))
	}
}

// dockAt docks the player at the first station of the current system.
func dockAt(t *testing.T, g *Game) {
	t.Helper()
	d := g.s.System()
	if d == nil {
		t.Fatal("no current system")
	}
	for _, cel := range d.Celestials {
		if cel.Station != nil {
			g.s.Docked.Set(g.s.Player(), sim.Docked{At: cel.ID})
			return
		}
	}
	t.Fatal("no station celestial in the starting system")
}

// ---------------------------------------------------------------------------
// Frame-pipeline ordering pins. These drive the real Game.Update/Draw
// (through pump's seam), so reordering the production pipeline fails
// here instead of shipping behind a green suite.
// ---------------------------------------------------------------------------

// TestFrameReconcilesMounts: after one frame the mounted set equals
// desired(open, docked) for every panel, and the dock/undock transition
// itself (syncScenes → scene OnEnter/OnLeave → syncMounts) happens inside
// that frame — no manual sync between the state change and the Update.
func TestFrameReconcilesMounts(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)

	check := func(stage string) {
		t.Helper()
		docked := g.s.Docked.Has(g.s.Player())
		for p := panelID(0); p < numPanels; p++ {
			want := panelDefs[p].want(g, docked)
			if got := g.mounted[p] != nil; got != want {
				t.Errorf("%s: %s mounted=%v, want %v (open=%v docked=%v)",
					stage, panelNames[p], got, want, *panelDefs[p].open(g), docked)
			}
		}
	}

	// Space with mixed intent: one frame mounts inventory and log,
	// leaves market closed, keeps the space-only scanner up.
	g.open[pInventory], g.open[pMarket] = true, false
	g.open[pScanner] = true
	if err := g.Update(); err != nil {
		t.Fatalf("space frame: %v", err)
	}
	check("space")
	if g.mounted[pInventory] == nil || g.mounted[pMarket] != nil || g.mounted[pScanner] == nil {
		t.Error("space mounts do not follow the intent the frame was given")
	}

	// Dock (state changes only): the frame pushes the station scene, so
	// the services view — the ONE window docking opens — surfaces and
	// the scanner drops without any manual sync.
	dockAt(t, g)
	if err := g.Update(); err != nil {
		t.Fatalf("dock frame: %v", err)
	}
	check("docked")
	if g.mounted[pScanner] != nil {
		t.Error("scanner still mounted while docked")
	}
	if !g.open[pServices] || g.mounted[pServices] == nil {
		t.Error("services view not opened by the dock frame")
	}
	for _, p := range []panelID{pMarket, pFitting, pManufacturing} {
		if g.open[p] || g.mounted[p] != nil {
			t.Errorf("docking opened %s as well — only the services view auto-opens", panelNames[p])
		}
	}

	// Undock: the frame pops the scene, which closes the services view;
	// the player's own windows (the inventory they opened) stay exactly
	// as they were, and the scanner comes back.
	g.s.Docked.Remove(g.s.Player())
	if err := g.Update(); err != nil {
		t.Fatalf("undock frame: %v", err)
	}
	check("undocked")
	if g.open[pServices] || g.mounted[pServices] != nil {
		t.Error("services view still open after the undock frame")
	}
	if g.mounted[pScanner] == nil {
		t.Error("scanner not restored by the undock frame")
	}
	if g.mounted[pInventory] == nil {
		t.Error("player-opened inventory closed by the undock frame")
	}
}

// TestFrameDrainsSimEventSameFrame: an event produced by the sim step
// inside Game.Update is in the log before that Update returns — the
// drain is part of the frame, not a later one.
func TestFrameDrainsSimEventSameFrame(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	if n := len(g.s.Missions()); n != 0 {
		t.Fatalf("fixture: %d missions already accepted", n)
	}
	before := len(g.logP.Lines())
	g.s.Queue(sim.CmdAcceptMission{ID: "contract-solace-cleanup-1"})

	// Whichever frame's sim step runs the queued command must also drain
	// its event into the log inside the same call.
	stepped := false
	for i := 0; i < 8 && !stepped; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if len(g.s.Missions()) == 0 {
			continue // the frame's step has not run the command yet
		}
		stepped = true
		lines := g.logP.Lines()
		if len(lines) <= before {
			t.Fatalf("mission accepted but its event never reached the log in that frame (%d → %d lines)",
				before, len(lines))
		}
		if joined := strings.Join(lines[before:], "\n"); !strings.Contains(joined, "Contract accepted:") {
			t.Errorf("log lines added by the frame do not carry the event: %q", lines[len(lines)-1])
		}
	}
	if !stepped {
		t.Fatal("queued mission never ran (no sim step in 8 frames)")
	}
}

// TestPendingLoadSwapsKitAfterPump: the staged menu load rebuilds the
// kit AFTER kit.Update returned. The probe below runs inside kit.Update
// (ebitenui dispatches deferred button events at the end of ui.Update),
// so it is the mid-pump observation point: it must still see the old
// kit, and the new kit must exist once the frame is over.
func TestPendingLoadSwapsKitAfterPump(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	if err := g.SaveGame(); err != nil {
		t.Fatalf("SaveGame: %v", err)
	}
	oldKit := g.kit

	g.push(&MenuScene{g: g})
	g.syncMounts()
	if g.mounted[pMenu] == nil {
		t.Fatal("menu window not mounted")
	}
	load := menuButton(t, g, "Load")

	probed, sawOldKit := false, false
	load.ClickedEvent.AddHandler(func(interface{}) {
		probed, sawOldKit = true, g.kit == oldKit
	})
	load.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: load})

	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !probed {
		t.Fatal("Load click never dispatched (the probe must run inside kit.Update)")
	}
	if !sawOldKit {
		t.Error("kit swapped while kit.Update was still pumping (staged load ran mid-pump)")
	}
	if g.kit == oldKit {
		t.Fatal("staged load did not rebuild the kit after the pump")
	}
	if g.mounted[pMenu] == nil {
		t.Error("menu not remounted by the rebuild")
	}
}

// menuButton walks the Esc menu window for the button with the label.
func menuButton(t *testing.T, g *Game, label string) *widget.Button {
	t.Helper()
	var found *widget.Button
	// Recurse through the Containerer interface, not the concrete
	// *widget.Container: a window body is wrapped in the kit's frame-clip
	// container (uikit/screens/winclip.go), so a concrete-only walk stops
	// at the wrapper.
	var walk func(c widget.Containerer)
	walk = func(c widget.Containerer) {
		for _, ch := range c.Children() {
			switch v := ch.(type) {
			case *widget.Button:
				if v.Text() != nil && v.Text().Label == label {
					found = v
				}
			case widget.Containerer:
				walk(v)
			}
		}
	}
	ctr, ok := g.menuWin.GetContainer().(widget.Containerer)
	if !ok {
		t.Fatalf("menu container is %T", g.menuWin.GetContainer())
	}
	walk(ctr)
	if found == nil {
		t.Fatalf("menu has no %q button", label)
	}
	return found
}
