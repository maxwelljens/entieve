package game

import (
	"image"
	"path/filepath"
	"strconv"
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

// pump runs two full interface frames at the given size (Sync → Update →
// Draw; the second frame settles layout) and returns the drawn image.
func pump(t *testing.T, g *Game, w, h int) *ebiten.Image {
	t.Helper()
	g.applySize(w, h)
	img := ebiten.NewImage(w, h)
	for i := 0; i < 2; i++ {
		g.syncPanels()
		if err := g.kit.Update(); err != nil {
			t.Fatalf("kit.Update: %v", err)
		}
		g.Draw(img)
	}
	return img
}

// mountedRects returns the screen rect of every visible panel: the kit's
// core windows, the mounted toggle windows (the log among them), the
// station strip and the root-overlay chrome. Hidden panels are skipped —
// they never lay out and keep a stale rect.
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
	if g.stShown {
		add("station", g.stationW.GetContainer().GetWidget())
	}
	// Root-overlay chrome: the HUD's visible children (the target-box row
	// is hidden until something is locked) and the rack.
	if g.hud != nil && isVisible(g.hud.Container().GetWidget()) {
		for i, ch := range g.hud.Container().Children() {
			add("hud."+strconv.Itoa(i), ch.GetWidget())
		}
	}
	if g.rack != nil {
		add("rack", g.rack.Container().GetWidget())
	}
	return out
}

// chromeShown reports whether the space chrome (HUD + module rack) is
// visible on the root overlay.
func chromeShown(g *Game) bool {
	return isVisible(g.hud.Container().GetWidget()) &&
		isVisible(g.rack.Container().GetWidget())
}

// panelNames labels the panel ids for test output.
var panelNames = map[panelID]string{
	pInventory: "inventory",
	pFitting:   "fitting",
	pMarket:    "market",
	pScanner:   "scanner",
	pMap:       "map",
	pHelp:      "help",
	pLog:       "log",
	pMenu:      "menu",
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
func assertNoOverlap(t *testing.T, g *Game, w, h int) {
	t.Helper()
	rects := mountedRects(g)
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].rect.Overlaps(rects[j].rect) {
				t.Errorf("%dx%d: default %s %v overlaps %s %v",
					w, h, rects[i].name, rects[i].rect, rects[j].name, rects[j].rect)
			}
		}
	}
}

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

		// Station view: docked chrome + the undock strip.
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
