package game

import (
	"image"
	"math"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/docstore"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// Window-layout persistence: a JSON sidecar next to the save file,
// <Options.SavePath>.layout.json, holding each named window's geometry,
// open state and split-pane ratios. The sim save (sim.Save) stays
// untouched — the CLI tools read it — so layout lives entirely outside
// the save format.
//
// Robustness contract: a missing, corrupt, wrong-version or partial
// sidecar degrades to the built-in defaults (never an error, never a
// crash); unknown keys are ignored on read and dropped on write; every
// restored rect is clamped to the window's MinSize and to the screen,
// and its left edge to the rail gutter (applyLayout) — at restore time;
// later viewport changes re-clamp to MinSize and the screen
// (ScreenHost.ClampRestored), so shrinking the window after a restore can never
// strand a restored window off-screen.

// layoutVersion tags the sidecar format. Version 2 (rects + open flags
// only) is still read — existing user sidecars must survive the split
// ratios' arrival — and every write is version 3. Version 1 files fall
// back to the defaults wholesale: they were written by builds whose
// restore clamp could fold the market/inventory/fitting trio onto one
// rect (and whose writeLayout then persisted that fold back), so reading
// them would pin the mess. A higher version is ignored for the usual
// forward-compat reason (never guess at an unknown format).
//
// Scale independence: every rect field is persisted in DESIGN units —
// the pixel layout at UI scale 1 (current px ÷ theme.Scale() on write,
// theme.Px on read). Version-2 files were all written at scale 1, where
// design unit == pixel, so they load seamlessly at any scale: one saved
// arrangement works at 0.75× and at 2.5×, and a session that changes
// scale mid-run round-trips through the same convention (applyScale).
const layoutVersion = 3

// layoutReadFrom is the oldest sidecar version readLayout accepts (see
// layoutVersion for why version 1 is dropped).
const layoutReadFrom = 2

// winLayout is one persisted window: top-left corner, size and — for
// the toggleable windows — whether it was open, plus the split panes'
// seam ratios (name → the lower pane's share of its pair, 0.5 = equal).
// X/Y/W/H are DESIGN units (see layoutVersion).
type winLayout struct {
	X      int                `json:"x"`
	Y      int                `json:"y"`
	W      int                `json:"w"`
	H      int                `json:"h"`
	Open   bool               `json:"open"`
	Splits map[string]float64 `json:"splits,omitempty"`
	// Tables is the window's tables' column state, table id → column key
	// → width (design units) and hidden flag. It arrived without a format
	// bump: an absent map is the measured default, which is exactly what a
	// v3 file written before it means.
	Tables map[string]map[string]widgets.ColumnPref `json:"tables,omitempty"`
}

// layoutFile is the sidecar document.
type layoutFile struct {
	Version int                  `json:"version"`
	Windows map[string]winLayout `json:"windows"`
}

// layoutEntry is one persisted window as the sidecar sees it: geometry
// always, an open value for the rows that have one, panes and tables only
// when the panel implements the facet. A row with panes or tables thus
// persists BECAUSE it implements the optional interface — the silent
// no-op bug class cannot come back through a forgotten name switch.
type layoutEntry interface {
	Win() *widget.Window
	PersistOpen() bool
	RestoreOpen(bool)
	Splits() []widgets.Seam
	Tables() []*widgets.Table
}

// coreLayoutEntry is the three kit-owned windows' sidecar row: the two
// always-on core windows (geometry + open=true) and Show Info (geometry
// only — it appears on demand, never at boot).
type coreLayoutEntry struct {
	name   string
	win    func(*uikit.Kit) *widget.Window
	tables func(*uikit.Kit) []*widgets.Table
	open   bool
}

var coreLayout = []coreLayoutEntry{
	{
		name: "overview", win: (*uikit.Kit).OverviewWindow, open: true,
		tables: func(k *uikit.Kit) []*widgets.Table { return []*widgets.Table{k.OverviewTable()} },
	},
	{name: "selected", win: (*uikit.Kit).SelectedWindow, open: true},
	{name: "infowin", win: (*uikit.Kit).InfoWindow, open: false},
}

// coreEntry adapts a core row to the layoutEntry interface.
type coreEntry struct {
	kit *uikit.Kit
	e   coreLayoutEntry
}

func (c coreEntry) Win() *widget.Window    { return c.e.win(c.kit) }
func (c coreEntry) PersistOpen() bool      { return c.e.open }
func (c coreEntry) RestoreOpen(bool)       {}
func (c coreEntry) Splits() []widgets.Seam { return nil }
func (c coreEntry) Tables() []*widgets.Table {
	if c.e.tables == nil {
		return nil
	}
	return c.e.tables(c.kit)
}

func coreLayoutNames() []string {
	names := make([]string, len(coreLayout))
	for i, e := range coreLayout {
		names[i] = e.name
	}
	return names
}

// layoutNames are the persisted windows, in a stable write order: the
// three core windows plus every Screen row that is not transient (the Esc
// menu is scene-owned and stays out). Assigned in init from the spec.
var layoutNames []string

// layoutPath is the sidecar location: the resolved save path plus
// .layout.json (e.g. …/entieve/save.json.layout.json).
func (g *Game) layoutPath() string { return g.SavePath() + ".layout.json" }

// layoutEntry resolves a persisted name to its window: the core windows,
// then a host row by its id (which is the sidecar key).
func (g *Game) layoutEntry(name string) layoutEntry {
	for _, e := range coreLayout {
		if e.name == name {
			return coreEntry{kit: g.kit, e: e}
		}
	}
	if s := g.host.Slot(name); s != nil {
		return s
	}
	return nil
}

// writeLayout persists every named window's geometry, open state and
// split-pane ratios (hidden windows too — their rects survive a dock).
// Geometry goes to the sidecar in DESIGN units (see layoutVersion). Errors
// bubble up so SaveGame can report them; the clean-quit path is
// best-effort.
func (g *Game) writeLayout() error {
	f := layoutFile{Version: layoutVersion, Windows: map[string]winLayout{}}
	s := theme.Scale()
	for _, name := range layoutNames {
		e := g.layoutEntry(name)
		if e == nil {
			continue
		}
		win := e.Win()
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
			X:      int(math.Round(float64(r.Min.X) / s)),
			Y:      int(math.Round(float64(r.Min.Y) / s)),
			W:      int(math.Round(float64(w) / s)),
			H:      int(math.Round(float64(h) / s)),
			Open:   e.PersistOpen(),
			Splits: splitRatios(e),
			Tables: tablePrefs(e),
		}
	}
	return docstore.Versioned(g.layoutPath(), layoutVersion, layoutReadFrom).Save(f)
}

// readLayout loads the sidecar (rects in DESIGN units — see
// layoutVersion). Every failure mode — missing file, bad
// JSON, a version older than 2 (stale version-1 files predating the
// service-trio reclaim) or newer than the current one (an unknown
// format) — returns nil: defaults win and nothing crashes.
func (g *Game) readLayout() map[string]winLayout {
	var f layoutFile
	if err := docstore.Versioned(g.layoutPath(), layoutVersion, layoutReadFrom).Load(&f); err != nil {
		return nil
	}
	// A version-2 file simply carries no split ratios, so every seam keeps
	// its equal-share default and the next write is v3.
	return migrateLayoutNames(f.Windows)
}

// migrateLayoutNames carries window geometry across renames of persisted
// windows: the manufacturing window's sidecar key was "factory" before the
// Factory → Manufacturing rename, and its saved rect must survive. The new
// key wins if both are present (a file written by a later build).
func migrateLayoutNames(w map[string]winLayout) map[string]winLayout {
	if v, ok := w["factory"]; ok {
		if _, has := w["manufacturing"]; !has {
			w["manufacturing"] = v
		}
		delete(w, "factory")
	}
	return w
}

// splitRatios is the write side: each named seam's current ratio, nil
// when the window has no panes.
func splitRatios(e layoutEntry) map[string]float64 {
	seams := e.Splits()
	if len(seams) == 0 {
		return nil
	}
	ratios := make(map[string]float64, len(seams))
	for _, sm := range seams {
		ratios[sm.Name] = sm.Split.Ratio(sm.Index)
	}
	return ratios
}

// applySplits is the read side: restore the stored ratios (the split
// clamps anything out of range); a seam the file does not name keeps its
// equal-share default.
func applySplits(e layoutEntry, ratios map[string]float64) {
	for _, sm := range e.Splits() {
		if f, ok := ratios[sm.Name]; ok {
			sm.Split.SetRatio(sm.Index, f)
		}
	}
}

// tablePrefs is the write side: each table id's column state, the first
// table carrying an id speaking for it (the inventory's holds agree by
// construction — they share one set).
func tablePrefs(e layoutEntry) map[string]map[string]widgets.ColumnPref {
	var out map[string]map[string]widgets.ColumnPref
	for _, t := range e.Tables() {
		id, p := t.PrefsID(), t.Prefs()
		if id == "" || len(p) == 0 {
			continue
		}
		if out == nil {
			out = map[string]map[string]widgets.ColumnPref{}
		}
		if _, ok := out[id]; !ok {
			out[id] = p
		}
	}
	return out
}

// applyTablePrefs is the read side: hand every table its stored column
// set (a table the file does not name keeps the measured freeze).
func applyTablePrefs(e layoutEntry, prefs map[string]map[string]widgets.ColumnPref) {
	for _, t := range e.Tables() {
		if p, ok := prefs[t.PrefsID()]; ok {
			t.SetPrefs(p)
		}
	}
}

// applyLayout restores the sidecar over the built-in defaults: geometry
// (clamped to MinSize and the screen) for every named window present,
// open state for the toggleable ones, and the scene stack for the help
// overlay, which only exists while its scene is up. Call it
// after the kit is built and the first real screen size is known —
// startup (first applySize) and after LoadGame's rebuild. A partial
// sidecar only restores the windows it holds, and each restored window
// is flagged as the player's arrangement so the composed defaults never
// overwrite it on a later resize (see ScreenHost.Place).
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
	screen := image.Rect(0, 0, screenW, screenH)
	for _, name := range layoutNames {
		saved, ok := wins[name]
		if !ok {
			continue
		}
		entry := g.layoutEntry(name)
		if entry == nil {
			continue
		}
		win := entry.Win()
		if win == nil {
			continue
		}
		minSize := image.Point{X: 1, Y: 1}
		if win.MinSize != nil {
			minSize = *win.MinSize
		}
		// Sidecar rects are DESIGN units: × current scale back into live
		// px (at scale 1 this is the exact historical pixel rect).
		x, y := theme.Px(saved.X), theme.Px(saved.Y)
		rect := image.Rect(x, y, x+theme.Px(saved.W), y+theme.Px(saved.H))
		r := clampLayoutRect(rect, minSize, screen)
		// The rail is top chrome painting over every window, so a sidecar
		// written before it widened can park a title bar under the gutter,
		// out of reach. Floor the left edge only, keeping the saved size
		// and y (for an over-wide window the rail beats the right edge —
		// the same rule anchorRect applies to defaults).
		if floor := railClearX(); r.Min.X < floor {
			r = r.Add(image.Pt(floor-r.Min.X, 0))
		}
		win.SetLocation(r)
		// Track the rect actually applied (SetLocation may grow it), so
		// the placement tracker never reports a restored window as
		// player-moved — see ScreenHost.Place and TestPlacementTracker.
		g.host.MarkRestored(win)
		entry.RestoreOpen(saved.Open)
		// The split ratios (v3): a v2 file names none, so every seam
		// keeps its equal-share default.
		applySplits(entry, saved.Splits)
		// The column state (additive, no version bump): a file without it
		// leaves each table at its measured freeze.
		applyTablePrefs(entry, saved.Tables)
	}
	// The help window only exists through its overlay scene (modal + dim
	// backdrop): sync the stack to the restored flag. The starmap is an
	// ordinary window now — its restored open flag mounts it like the
	// rest (syncMounts below).
	syncOverlay := func(open bool, is func(Scene) bool, make func() Scene) {
		has := g.findScene(is)
		switch {
		case open && !has:
			g.push(make())
		case !open && has && is(g.top()):
			g.pop() // OnLeave re-asserts open=false
		}
	}
	syncOverlay(g.host.IsOpen(idHelp),
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
