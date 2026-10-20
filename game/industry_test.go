package game

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// TestManufacturingPanelMount: B toggles the manufacturing window — in space the flag
// flips but the panel stays unmounted (it is a docked station service),
// docking mounts it, undocking unmounts it again, and while mounted the
// window lays out in bounds with its recipe board fitted.
func TestManufacturingPanelMount(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	if g.manu == nil {
		t.Fatal("manufacturing panel not constructed by New")
	}

	// In space: intent only, no mount.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyB}})
	if !g.open[pManufacturing] {
		t.Fatal("B did not toggle the manufacturing flag")
	}
	g.syncMounts()
	if g.mounted[pManufacturing] != nil {
		t.Fatal("manufacturing mounted while undocked")
	}

	// Dock: the flag is still set, so the panel surfaces.
	dockAt(t, g)
	g.syncScenes()
	g.syncMounts()
	if g.mounted[pManufacturing] == nil {
		t.Fatal("manufacturing not mounted while docked with the flag set")
	}
	pump(t, g, 1280, 720)
	r := g.manu.Win().GetContainer().GetWidget().Rect
	if r.Dx() == 0 || r.Dy() == 0 {
		t.Fatal("manufacturing window never laid out")
	}
	if r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		t.Fatalf("manufacturing window %v out of bounds", r)
	}

	// B again closes it while docked.
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyB}})
	g.syncMounts()
	if g.open[pManufacturing] || g.mounted[pManufacturing] != nil {
		t.Fatal("B did not close the manufacturing window")
	}

	// Undock: whatever the flag says, the station service unmounts.
	g.open[pManufacturing] = true
	g.s.Docked.Remove(g.s.Player())
	g.syncScenes()
	g.syncMounts()
	if g.mounted[pManufacturing] != nil {
		t.Fatal("manufacturing still mounted after undock")
	}
}

// TestManufacturingSurfacesAtIndustryStation: the manufacturing is an ordinary
// window now — docking opens only the services view, and the player
// opens the manufacturing from there (its Industry button) or with B (the
// session's own content copy patches the start station with the
// service).
func TestManufacturingSurfacesAtIndustryStation(t *testing.T) {
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
	if g.open[pManufacturing] || g.mounted[pManufacturing] != nil {
		t.Fatal("manufacturing auto-opened on dock — only the services view opens")
	}
	if !g.open[pServices] || g.mounted[pServices] == nil {
		t.Fatal("services view not open at the industry station")
	}
	// The player opens it (B — what the services view's Industry button
	// emits too).
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyB}})
	g.syncMounts()
	if !g.open[pManufacturing] {
		t.Fatal("manufacturing did not open at an industry station")
	}
	if g.mounted[pManufacturing] == nil {
		t.Fatal("manufacturing not mounted after the dock frame")
	}
	pump(t, g, 1280, 720)
	r := g.manu.Win().GetContainer().GetWidget().Rect
	if r.Dx() == 0 || r.Dy() == 0 || r.Max.X > 1280 || r.Max.Y > 720 {
		t.Fatalf("manufacturing window %v not laid out in bounds", r)
	}

	// Closing it sticks (the panel is toggleable, B again).
	g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyB}})
	g.syncMounts()
	if g.open[pManufacturing] || g.mounted[pManufacturing] != nil {
		t.Fatal("B did not close the surfaced manufacturing")
	}
}

// TestManufacturingKeyDocumented: the binding rides KeyMap (the help overlay's
// single source) with its own action id, and no other key claims it.
func TestManufacturingKeyDocumented(t *testing.T) {
	found := 0
	for _, b := range KeyMap {
		if b.Action != "manufacturing" {
			continue
		}
		found++
		if b.Key != ebiten.KeyB || b.Shift || b.Ctrl {
			t.Errorf("manufacturing binding = %+v, want plain B", b)
		}
		if !strings.Contains(b.Desc, "manufacturing") {
			t.Errorf("manufacturing desc = %q, want the word manufacturing", b.Desc)
		}
	}
	if found != 1 {
		t.Fatalf("manufacturing bindings = %d, want exactly 1", found)
	}
	// The help rows derive from KeyMap (one row per binding).
	if len(helpRows()) != len(KeyMap) {
		t.Fatalf("help rows = %d, want %d", len(helpRows()), len(KeyMap))
	}
}

// TestManufacturingWindowIsAFullPanel: the panel carries a window, a flag and a
// sync rule like every other panelDef row (the enum-coverage test checks
// the table; this pins the manufacturing's own wiring).
func TestManufacturingWindowIsAFullPanel(t *testing.T) {
	g := newShowcaseGame(t)
	d := panelDefs[pManufacturing]
	if d.name != "manufacturing" {
		t.Errorf("sidecar name = %q, want manufacturing", d.name)
	}
	if d.window(g) == nil || d.open(g) == nil {
		t.Fatal("incomplete manufacturing panelDef")
	}
	if d.sync == nil {
		t.Fatal("manufacturing panel has no sync rule")
	}
	if d.want(g, false) {
		t.Error("manufacturing wants to mount while undocked")
	}
	g.open[pManufacturing] = true
	if !d.want(g, true) {
		t.Error("manufacturing does not mount while docked with the flag set")
	}
	// The window carries a content floor like the other services.
	if w := d.window(g); w == nil || w.MinSize == nil {
		t.Fatal("manufacturing window has no MinSize")
	}
}

// TestLayoutMigratesFactoryToManufacturing: a version-2 sidecar written
// before the Factory → Manufacturing rename restores the saved rect and
// open flag under the new "manufacturing" key (the key maps on read).
func TestLayoutMigratesFactoryToManufacturing(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)

	old := layoutFile{Version: layoutVersion, Windows: map[string]winLayout{
		"factory": {X: 140, Y: 120, W: 420, H: 440, Open: true},
	}}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(g.layoutPath(), data, 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	wins := g.readLayout()
	if _, ok := wins["factory"]; ok {
		t.Fatal("old factory key survived the read")
	}
	if got, ok := wins["manufacturing"]; !ok || got.W != 420 || got.H != 440 {
		t.Fatalf("migrated entry = %+v (%v), want the saved 420×440", got, ok)
	}

	g.applyLayout(1280, 720)
	r := g.manu.Win().GetContainer().GetWidget().Rect
	if r.Min != image.Pt(theme.Px(140), theme.Px(120)) ||
		r.Dx() != theme.Px(420) || r.Dy() != theme.Px(440) {
		t.Fatalf("restored manufacturing rect = %v, want the saved 140,120 420×440", r)
	}
	if !g.open[pManufacturing] {
		t.Fatal("saved open flag not restored under the new key")
	}
}
