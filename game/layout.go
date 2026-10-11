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
// restored rect is clamped to the window's MinSize and to the screen.

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

// layoutNames are the persisted windows, in a stable write order.
// Menu, station and the HUD/rack chrome are not user layout state.
var layoutNames = []string{
	"overview", "selected", "log", "market", "inventory",
	"fitting", "scanner", "map", "help",
}

// layoutPath is the sidecar location: the resolved save path plus
// .layout.json (e.g. …/entieve/save.json.layout.json).
func (g *Game) layoutPath() string { return g.SavePath() + ".layout.json" }

// layoutTarget resolves a persisted name to its window and — for the
// toggleable windows — its open flag. overview/selected are always-on
// core chrome (hidden while docked, never closed): nil flag, written as
// open, geometry only on read. Unknown names resolve to nil and are
// skipped by both sides.
func (g *Game) layoutTarget(name string) (*widget.Window, *bool) {
	switch name {
	case "overview":
		return g.kit.OverviewWindow(), nil
	case "selected":
		return g.kit.SelectedWindow(), nil
	case "log":
		return g.logP.Win(), &g.open[pLog]
	case "market":
		return g.mkt.Win(), &g.open[pMarket]
	case "inventory":
		return g.inv.Win(), &g.open[pInventory]
	case "fitting":
		return g.fitt.Win(), &g.open[pFitting]
	case "scanner":
		return g.scan.Win(), &g.open[pScanner]
	case "map":
		return g.mp.Win(), &g.open[pMap]
	case "help":
		return g.help.Win(), &g.open[pHelp]
	}
	return nil, nil
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
			X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy(),
			Open: open == nil || *open,
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
func (g *Game) applyLayout(screenW, screenH int) {
	wins := g.readLayout()
	if len(wins) == 0 {
		return
	}
	if g.restored == nil {
		g.restored = map[*widget.Window]bool{}
	}
	screen := image.Rect(0, 0, screenW, screenH)
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
		// Saved wins from here on: the window is the player's
		// arrangement, so placeIfVirgin leaves it alone on every later
		// viewport change (loading must never smash a moved layout).
		g.restored[win] = true
		if open != nil {
			*open = e.Open
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
