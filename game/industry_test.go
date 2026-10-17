package game

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestFactoryPanelMount: B toggles the factory window — in space the flag
// flips but the panel stays unmounted (it is a docked station service),
// docking mounts it, undocking unmounts it again, and while mounted the
// window lays out in bounds with its recipe board fitted.
func TestFactoryPanelMount(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	if g.fact == nil {
		t.Fatal("factory panel not constructed by New")
	}

	// In space: intent only, no mount.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyB}})
	if !g.open[pFactory] {
		t.Fatal("B did not toggle the factory flag")
	}
	g.syncMounts()
	if g.mounted[pFactory] != nil {
		t.Fatal("factory mounted while undocked")
	}

	// Dock: the flag is still set, so the panel surfaces.
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	if g.mounted[pFactory] == nil {
		t.Fatal("factory not mounted while docked with the flag set")
	}
	pump(t, g, 1280, 720)
	r := g.fact.Win().GetContainer().GetWidget().Rect
	if r.Dx() == 0 || r.Dy() == 0 {
		t.Fatal("factory window never laid out")
	}
	if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		t.Fatalf("factory window %v out of bounds", r)
	}

	// B again closes it while docked.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyB}})
	g.syncMounts()
	if g.open[pFactory] || g.mounted[pFactory] != nil {
		t.Fatal("B did not close the factory window")
	}

	// Undock: whatever the flag says, the station service unmounts.
	g.open[pFactory] = true
	g.s.Docked.Remove(g.s.Player())
	g.syncScenes()
	g.syncMounts()
	if g.mounted[pFactory] != nil {
		t.Fatal("factory still mounted after undock")
	}
}

// TestFactorySurfacesAtIndustryStation: the factory is an ordinary
// window now — docking opens only the services view, and the player
// opens the factory from there (its Industry button) or with B (the
// session's own content copy patches the start station with the
// service).
func TestFactorySurfacesAtIndustryStation(t *testing.T) {
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	var patched bool
	for _, cel := range c.Systems["sys-solace"].Celestials {
		if cel.ID == "cel-haven-rest" && cel.Station != nil {
			cel.Station.Services = append(cel.Station.Services, "industry")
			patched = true
		}
	}
	if !patched {
		t.Fatal("fixture: cel-haven-rest missing")
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	g, err := New(Options{Content: c, World: wd, Seed: 42,
		SavePath: filepath.Join(t.TempDir(), "save.json")})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	g.applySize(1280, 720)

	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	if g.open[pFactory] || g.mounted[pFactory] != nil {
		t.Fatal("factory auto-opened on dock — only the services view opens")
	}
	if !g.open[pServices] || g.mounted[pServices] == nil {
		t.Fatal("services view not open at the industry station")
	}
	// The player opens it (B — what the services view's Industry button
	// emits too).
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyB}})
	g.syncMounts()
	if !g.open[pFactory] {
		t.Fatal("factory did not open at an industry station")
	}
	if g.mounted[pFactory] == nil {
		t.Fatal("factory not mounted after the dock frame")
	}
	pump(t, g, 1280, 720)
	r := g.fact.Win().GetContainer().GetWidget().Rect
	if r.Dx() == 0 || r.Dy() == 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		t.Fatalf("factory window %v not laid out in bounds", r)
	}

	// Closing it sticks (the panel is toggleable, B again).
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyB}})
	g.syncMounts()
	if g.open[pFactory] || g.mounted[pFactory] != nil {
		t.Fatal("B did not close the surfaced factory")
	}
}

// TestFactoryKeyDocumented: the binding rides KeyMap (the help overlay's
// single source) with its own action id, and no other key claims it.
func TestFactoryKeyDocumented(t *testing.T) {
	found := 0
	for _, b := range KeyMap {
		if b.Action != "factory" {
			continue
		}
		found++
		if b.Key != ebiten.KeyB || b.Shift || b.Ctrl {
			t.Errorf("factory binding = %+v, want plain B", b)
		}
		if !strings.Contains(b.Desc, "factory") {
			t.Errorf("factory desc = %q, want the word factory", b.Desc)
		}
	}
	if found != 1 {
		t.Fatalf("factory bindings = %d, want exactly 1", found)
	}
	// The help rows derive from KeyMap (one row per binding).
	if len(helpRows()) != len(KeyMap) {
		t.Fatalf("help rows = %d, want %d", len(helpRows()), len(KeyMap))
	}
}

// TestFactoryWindowIsAFullPanel: the panel carries a window, a flag and a
// sync rule like every other panelDef row (the enum-coverage test checks
// the table; this pins the factory's own wiring).
func TestFactoryWindowIsAFullPanel(t *testing.T) {
	g := newShowcaseGame(t)
	d := panelDefs[pFactory]
	if d.name != "factory" {
		t.Errorf("sidecar name = %q, want factory", d.name)
	}
	if d.window(g) == nil || d.open(g) == nil {
		t.Fatal("incomplete factory panelDef")
	}
	if d.sync == nil {
		t.Fatal("factory panel has no sync rule")
	}
	if d.want(g, false) {
		t.Error("factory wants to mount while undocked")
	}
	g.open[pFactory] = true
	if !d.want(g, true) {
		t.Error("factory does not mount while docked with the flag set")
	}
	// The window carries a content floor like the other services.
	if w := d.window(g); w == nil || w.MinSize == nil {
		t.Fatal("factory window has no MinSize")
	}
}
