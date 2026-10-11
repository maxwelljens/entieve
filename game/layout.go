package game

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"

	"github.com/ebitenui/ebitenui/widget"
)

// Window-layout persistence: a JSON sidecar next to the save file,
// <Options.SavePath>.layout.json, holding each named window's geometry
// and open state. The sim save (sim.Save) stays untouched — the CLI
// tools read it — so layout lives entirely outside the save format.
//
// Robustness contract: a missing, corrupt, wrong-version or partial
// sidecar degrades to the built-in defaults (never an error, never a
// crash); unknown keys are ignored on read and dropped on write; every
// restored rect is clamped to the window's MinSize and to the screen —
// at restore time AND again on every later viewport change
// (clampRestored), so shrinking the window after a restore can never
// strand a restored window off-screen.

// layoutVersion tags the sidecar format. A higher version in a file is
// ignored wholesale rather than guessed at.
const layoutVersion = 1

// winLayout is one persisted window: top-left corner, size and — for
// the toggleable windows — whether it was open.
type winLayout struct {
	X    int  `json:"x"`
	Y    int  `json:"y"`
	W    int  `json:"w"`
	H    int  `json:"h"`
	Open bool `json:"open"`
}

// layoutFile is the sidecar document.
type layoutFile struct {
	Version int                  `json:"version"`
	Windows map[string]winLayout `json:"windows"`
}

// layoutNames are the persisted windows, in a stable write order: the
// two always-on core windows (geometry only — see layoutTarget) plus
// every panel whose panelDef carries a sidecar name (the Esc menu is
// scene-owned and stays out). Menu, station and the HUD/rack chrome are
// not user layout state.
var layoutNames = func() []string {
	names := []string{"overview", "selected"}
	for p := panelID(0); p < numPanels; p++ {
		if panelDefs[p].name != "" {
			names = append(names, panelDefs[p].name)
		}
	}
	return names
}()

// layoutPath is the sidecar location: the resolved save path plus
// .layout.json (e.g. …/entieve/save.json.layout.json).
func (g *Game) layoutPath() string { return g.SavePath() + ".layout.json" }

// panelByName resolves a sidecar name to its panel id (ok=false for the
// core windows, the menu's empty key and unknown names).
func panelByName(name string) (panelID, bool) {
	if name == "" {
		return 0, false
	}
	for p := panelID(0); p < numPanels; p++ {
		if panelDefs[p].name == name {
			return p, true
		}
	}
	return 0, false
}

// layoutTarget resolves a persisted name to its window and — for the
// toggleable windows — its open flag. overview/selected are always-on
// core chrome (hidden while docked, never closed): nil flag, written as
// open, geometry only on read. Every other name comes from panelDefs;
// unknown names (and the menu's empty key) resolve to nil and are
// skipped by both sides.
func (g *Game) layoutTarget(name string) (*widget.Window, *bool) {
	switch name {
	case "overview":
		return g.kit.OverviewWindow(), nil
	case "selected":
		return g.kit.SelectedWindow(), nil
	}
	if p, ok := panelByName(name); ok {
		return panelDefs[p].window(g), panelDefs[p].open(g)
	}
	return nil, nil
}

// persistOpen picks the open flag writeLayout records for a named
// window: the core windows always write open, a toggleable one writes
// its flag — except the three station services while the station scene
// forces them open. Their sidecar entry carries the PRE-dock intent
// (stationPrior), or quitting while docked would reopen them in space on
// the next boot.
func (g *Game) persistOpen(name string, open *bool) bool {
	if open == nil {
		return true
	}
	if g.s.Docked.Has(g.s.Player()) {
		if p, ok := panelByName(name); ok && isStationPanel(p) {
			return g.stationPrior[p]
		}
	}
	return *open
}

// writeLayout persists every named window's geometry and open state
// (hidden windows too — their rects survive a dock). Errors bubble up
// so SaveGame can report them; the clean-quit path is best-effort.
func (g *Game) writeLayout() error {
	f := layoutFile{Version: layoutVersion, Windows: map[string]winLayout{}}
	for _, name := range layoutNames {
		win, open := g.layoutTarget(name)
		if win == nil {
			continue
		}
		r := win.GetContainer().GetWidget().Rect
		f.Windows[name] = winLayout{
			X:    r.Min.X,
			Y:    r.Min.Y,
			W:    r.Dx(),
			H:    r.Dy(),
			Open: g.persistOpen(name, open),
		}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := g.layoutPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// readLayout loads the sidecar. Every failure mode — missing file, bad
// JSON, a newer version — returns nil: defaults win and nothing crashes.
func (g *Game) readLayout() map[string]winLayout {
	data, err := os.ReadFile(g.layoutPath())
	if err != nil {
		return nil
	}
	var f layoutFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	if f.Version > layoutVersion {
		return nil
	}
	return f.Windows
}

// applyLayout restores the sidecar over the built-in defaults: geometry
// (clamped to MinSize and the screen) for every named window present,
// open state for the toggleable ones, and the scene stack for the map
// and help overlays, which only exist while their scene is up. Call it
// after the kit is built and the first real screen size is known —
// startup (first applySize) and after LoadGame's rebuild. A partial
// sidecar only restores the windows it holds, and each restored window
// is flagged as the player's arrangement so the composed defaults never
// overwrite it on a later resize (see placeIfVirgin).
//
// While the station scene is running, the dock services' restored flags
// are the pre-dock intent (persistOpen writes them that way): they are
// adopted into stationPrior and the forced-open state is re-asserted.
// When the scene is NOT running yet (booting a docked save), the intent
// is left in the flags and StationScene.OnEnter picks it up as the
// pre-dock state when it pushes.
func (g *Game) applyLayout(screenW, screenH int) {
	wins := g.readLayout()
	if len(wins) == 0 {
		return
	}
	if g.restored == nil {
		g.restored = map[*widget.Window]bool{}
	}
	if g.placed == nil {
		g.placed = map[*widget.Window]image.Rectangle{}
	}
	screen := image.Rect(0, 0, screenW, screenH)
	dockedScene := g.s.Docked.Has(g.s.Player()) && g.findScene(func(s Scene) bool {
		_, ok := s.(*StationScene)
		return ok
	})
	for _, name := range layoutNames {
		e, ok := wins[name]
		if !ok {
			continue
		}
		win, open := g.layoutTarget(name)
		if win == nil {
			continue
		}
		minSize := image.Point{X: 1, Y: 1}
		if win.MinSize != nil {
			minSize = *win.MinSize
		}
		saved := image.Rect(e.X, e.Y, e.X+e.W, e.Y+e.H)
		win.SetLocation(clampLayoutRect(saved, minSize, screen))
		// Track the rect actually applied (SetLocation may grow it), so
		// the placement tracker never reports a restored window as
		// player-moved — see placeIfVirgin and TestPlacementTracker.
		g.placed[win] = win.GetContainer().GetWidget().Rect
		// Saved wins from here on: the window is the player's
		// arrangement, so placeIfVirgin leaves it alone on every later
		// viewport change (loading must never smash a moved layout).
		g.restored[win] = true
		if open != nil {
			*open = e.Open
			if p, named := panelByName(name); named && dockedScene && isStationPanel(p) {
				g.stationPrior[p] = e.Open // pre-dock intent, adopted
			}
		}
	}
	if dockedScene {
		for _, p := range stationPanels {
			g.open[p] = true // the scene forces its services open
		}
	}
	// The map and help windows only exist through their overlay scenes
	// (modal + dim backdrop): sync the stack to the restored flags.
	syncOverlay := func(open bool, is func(Scene) bool, make func() Scene) {
		has := g.findScene(is)
		switch {
		case open && !has:
			g.push(make())
		case !open && has && is(g.top()):
			g.pop() // OnLeave re-asserts open=false
		}
	}
	syncOverlay(g.open[pMap],
		func(s Scene) bool { _, ok := s.(*MapScene); return ok },
		func() Scene { return &MapScene{g: g} })
	syncOverlay(g.open[pHelp],
		func(s Scene) bool { _, ok := s.(*HelpScene); return ok },
		func() Scene { return &HelpScene{g: g} })
	// Apply restored open flags immediately (LoadGame rebuilt the mounts
	// before this ran).
	g.syncMounts()
}

// clampRestored re-establishes the on-screen guarantee for the windows
// restored from the sidecar whenever the viewport changes: their rects
// are clamped to the new screen (and to MinSize) exactly like at restore
// time, while the restored flag stays set — the player's relative
// arrangement survives (only what no longer fits slides back in), and
// placeDefaults still never stomps it. Call it from applySize after the
// defaults composed. Restored windows are tracked, so the placement
// tracker keeps reading true afterwards.
func (g *Game) clampRestored(w, h int) {
	screen := image.Rect(0, 0, w, h)
	for win, restored := range g.restored {
		if !restored {
			continue
		}
		minSize := image.Point{X: 1, Y: 1}
		if win.MinSize != nil {
			minSize = *win.MinSize
		}
		r := win.GetContainer().GetWidget().Rect
		if c := clampLayoutRect(r, minSize, screen); c != r {
			win.SetLocation(c)
		}
		g.placed[win] = win.GetContainer().GetWidget().Rect
	}
}

// clampLayoutRect fits a restored rect onto the screen at min size:
// the size grows to min first (top-left held), then shrinks to the
// screen when it still overflows, then the position slides fully
// on-screen. A min larger than the screen wins over fitting (the
// window cannot be smaller than its content anyway). Pure —
// unit-tested by TestClampLayoutRect.
func clampLayoutRect(r image.Rectangle, minSize image.Point, screen image.Rectangle) image.Rectangle {
	minW, minH := max(minSize.X, 1), max(minSize.Y, 1)
	w := min(max(r.Dx(), minW), max(screen.Dx(), minW))
	h := min(max(r.Dy(), minH), max(screen.Dy(), minH))
	x := clampInt(r.Min.X, screen.Min.X, screen.Max.X-w)
	y := clampInt(r.Min.Y, screen.Min.Y, screen.Max.Y-h)
	return image.Rect(x, y, x+w, y+h)
}

// clampInt slides v into [lo, hi]; an empty range (min size larger than
// the screen) anchors at lo.
func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	return min(max(v, lo), hi)
}
