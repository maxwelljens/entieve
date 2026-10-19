package game

import (
	"encoding/json"
	"image"
	"math"
	"os"
	"path/filepath"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
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

// layoutVersion tags the sidecar format. Only the exact current version
// is read: version 1 was written by builds whose restore clamp could
// fold the market/inventory/fitting trio onto one rect (and whose
// writeLayout then persisted that fold back), so v1 files fall back to
// the defaults wholesale instead of pinning the mess; a higher version
// is ignored for the usual forward-compat reason (never guess at an
// unknown format).
//
// Scale independence: every rect field is persisted in DESIGN units —
// the pixel layout at UI scale 1 (current px ÷ theme.Scale() on write,
// theme.Px on read). Version-2 files were all written at scale 1, where
// design unit == pixel, so they load seamlessly at any scale: one saved
// arrangement works at 0.75× and at 2.5×, and a session that changes
// scale mid-run round-trips through the same convention (applyScale).
const layoutVersion = 2

// winLayout is one persisted window: top-left corner, size and — for
// the toggleable windows — whether it was open. X/Y/W/H are DESIGN
// units (see layoutVersion).
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
// two always-on core windows plus the Show Info window (geometry only —
// see layoutTarget) plus every Screen row that carries a sidecar name
// (the Esc menu is scene-owned and stays out). Menu, station and the
// HUD/rack chrome are not user layout state.
//
// Assigned in init from the spec (uikit.go), like panelDefs.
var layoutNames []string

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
// core chrome (hidden while docked, never closed) and infowin is the Show
// Info window (geometry only, see persistOpen): nil flag, geometry on
// read. Every other name comes from panelDefs; unknown names (and the
// menu's empty key) resolve to nil and are skipped by both sides.
func (g *Game) layoutTarget(name string) (*widget.Window, *bool) {
	switch name {
	case "overview":
		return g.kit.OverviewWindow(), nil
	case "selected":
		return g.kit.SelectedWindow(), nil
	case "infowin":
		return g.kit.InfoWindow(), nil
	}
	if p, ok := panelByName(name); ok {
		return panelDefs[p].window(g), panelDefs[p].open(g)
	}
	return nil, nil
}

// persistOpen picks the open flag writeLayout records for a named
// window: the core windows always write open, a toggleable one writes
// its LIVE flag, and the Show Info window is geometry-only (always
// closed, never restored — it appears on demand, never at boot). No
// docked special case remains: nothing is force-opened on dock any
// more, so the sidecar holds exactly the state the player last saw (the
// services view's flag is the station scene's, which is open exactly
// while docked).
func (g *Game) persistOpen(name string, open *bool) bool {
	if name == "infowin" {
		return false
	}
	if open == nil {
		return true
	}
	return *open
}

// writeLayout persists every named window's geometry and open state
// (hidden windows too — their rects survive a dock). Geometry goes to
// the sidecar in DESIGN units (see layoutVersion). Errors bubble up
// so SaveGame can report them; the clean-quit path is best-effort.
func (g *Game) writeLayout() error {
	f := layoutFile{Version: layoutVersion, Windows: map[string]winLayout{}}
	s := theme.Scale()
	for _, name := range layoutNames {
		win, open := g.layoutTarget(name)
		if win == nil {
			continue
		}
		r := win.GetContainer().GetWidget().Rect
		w, h := r.Dx(), r.Dy()
		// Never persist degenerate geometry: a zero rect round-trips to
		// a zero rect and would be pinned as saved-wins forever. Grow
		// (top-left held) to the window's MinSize — exactly what the
		// read-side clamp would apply anyway.
		minW, minH := 1, 1
		if win.MinSize != nil {
			minW, minH = max(win.MinSize.X, 1), max(win.MinSize.Y, 1)
		}
		if w < minW {
			w = minW
		}
		if h < minH {
			h = minH
		}
		f.Windows[name] = winLayout{
			// px → design units (÷ current scale): the sidecar never
			// records the scale, so the same numbers restore at any of
			// them. The MinSize growth above ran in live px first, so
			// the design rect still satisfies the scaled floor on read.
			X:    int(math.Round(float64(r.Min.X) / s)),
			Y:    int(math.Round(float64(r.Min.Y) / s)),
			W:    int(math.Round(float64(w) / s)),
			H:    int(math.Round(float64(h) / s)),
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

// readLayout loads the sidecar (rects in DESIGN units — see
// layoutVersion). Every failure mode — missing file, bad
// JSON, or a version that is not exactly the current one (stale
// version-1 files predating the service-trio reclaim, and any newer
// unknown format) — returns nil: defaults win and nothing crashes.
func (g *Game) readLayout() map[string]winLayout {
	data, err := os.ReadFile(g.layoutPath())
	if err != nil {
		return nil
	}
	var f layoutFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	if f.Version != layoutVersion {
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
// Restored flags are plain player state now (nothing docks-open the
// service windows): a docked boot reads the services view's flag from
// the sidecar, and StationScene.OnEnter opens it on the first frame
// anyway — docking always lands on the services view.
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
		// Sidecar rects are DESIGN units: × current scale back into live
		// px (at scale 1 this is the exact historical pixel rect).
		x, y := theme.Px(e.X), theme.Px(e.Y)
		saved := image.Rect(x, y, x+theme.Px(e.W), y+theme.Px(e.H))
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
