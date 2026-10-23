package game

import (
	"encoding/json"
	"image"
	"math"
	"os"
	"path/filepath"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// Player settings persistence: a JSON sidecar next to the save file,
// <Options.SavePath>.settings.json, holding the UI scale, the fullscreen
// flag and the navigation rail's collapse state:
//
//	{"ui_scale": 1.5, "fullscreen": false}
//	{"ui_scale": 1, "fullscreen": false, "rail_collapsed": true}
//
// rail_collapsed is omitempty: the documented DEFAULT shape above stays
// exactly two keys (an absent key reads as expanded — the rail's boot
// state), and a collapsed rail writes the third.
//
// Like the layout sidecar the contract is robustness over fidelity: a
// missing, corrupt or out-of-ladder file degrades to the defaults (UI
// scale 1.0, windowed) and never errors or crashes; unknown keys are
// ignored. It loads in New, BEFORE the interface is built and before the
// first Layout, so the very first frame is already laid out at the saved
// scale; every change writes it back immediately (best-effort errors are
// reported to the log).
//
// ui_scale is the USER half of the global scale: the installed UI scale
// is uiScale × the display's device factor (see Game.Run / theme.SetScale).

// uiScaleSteps is the settings window's UI-scale ladder — the stepper
// steps through these and only these values, and a persisted ui_scale is
// accepted only if it lands exactly on a rung (anything else is the
// documented default of 1.0).
var uiScaleSteps = [...]float64{0.75, 1.0, 1.25, 1.5, 2.0, 2.5}

// defaultUIScale is the scale with no (or invalid) settings present.
const defaultUIScale = 1.0

// settingsFile is the settings sidecar document.
type settingsFile struct {
	UIScale    float64 `json:"ui_scale"`
	Fullscreen bool    `json:"fullscreen"`
	// RailCollapsed parks the left navigation rail to its edge tab;
	// absent = expanded (the boot default).
	RailCollapsed bool `json:"rail_collapsed,omitempty"`
}

// settingsPath is the sidecar location: the resolved save path plus
// .settings.json (e.g. …/entieve/save.json.settings.json).
func (g *Game) settingsPath() string { return g.SavePath() + ".settings.json" }

// loadSettings reads the settings sidecar into g. Every failure mode —
// missing file, bad JSON, a scale off the ladder — leaves the defaults
// in place (never an error; called from New before anything is built).
func (g *Game) loadSettings() {
	g.uiScale, g.fullscreen = defaultUIScale, false
	g.railCollapsed = false
	data, err := os.ReadFile(g.settingsPath())
	if err != nil {
		return
	}
	var f settingsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return
	}
	if s, ok := scaleOnLadder(f.UIScale); ok {
		g.uiScale = s
	}
	g.fullscreen = f.Fullscreen
	g.railCollapsed = f.RailCollapsed
}

// writeSettings persists g's settings (called on every change).
func (g *Game) writeSettings() error {
	f := settingsFile{UIScale: g.uiScale, Fullscreen: g.fullscreen, RailCollapsed: g.railCollapsed}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := g.settingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// scaleOnLadder reports whether s is (numerically) a step of the ladder.
func scaleOnLadder(s float64) (float64, bool) {
	for _, step := range uiScaleSteps {
		if math.Abs(s-step) < 1e-9 {
			return step, true
		}
	}
	return 0, false
}

// stepUIScale moves the user setting one rung along the ladder (the
// settings stepper's − / +) and applies it live.
func (g *Game) stepUIScale(delta int) {
	idx := 0
	for i, step := range uiScaleSteps {
		if step == g.uiScale {
			idx = i
			break
		}
	}
	idx = clampInt(idx+delta, 0, len(uiScaleSteps)-1)
	g.applyUIScale(uiScaleSteps[idx])
}

// applyUIScale switches the UI to a new user scale, LIVE: settings are
// persisted, the current arrangement is round-tripped through the layout
// sidecar in DESIGN units (at the old scale — rects ÷ old scale on
// write), the global scale is reinstalled and the whole interface is
// rebuilt against it (every metric and face is built at construction:
// rebuild IS the relayout), then the saved design-unit rects are restored
// × new scale. Each window therefore keeps its placement in design space
// and the current log history; selection resets, exactly like LoadGame's
// rebuild.
func (g *Game) applyUIScale(s float64) {
	if s == g.uiScale {
		return
	}
	g.uiScale = s
	if err := g.writeSettings(); err != nil {
		g.report(err, "")
	}
	g.applyScale(true)
}

// applyScale installs uiScale × device into the uikit funnel and rebuilds
// the interface against it. preserveLayout round-trips the sidecar first
// and restores it afterwards; the boot path passes false — nothing has
// been placed or restored yet, so placeDefaults in buildKit is all the
// layout there is (the sidecar is applied later by the first applySize).
func (g *Game) applyScale(preserveLayout bool) {
	s := g.uiScale * g.deviceScale()
	if s == theme.Scale() {
		return // nothing changed (a fresh Game installs its scale in New)
	}
	lines := g.logP.Lines()
	if preserveLayout {
		// Write BEFORE the scale changes: design units = px ÷ old scale.
		if err := g.writeLayout(); err != nil {
			g.report(err, "")
		}
	}
	theme.SetScale(s)
	if err := g.buildKit(); err != nil {
		g.report(err, "")
		return
	}
	g.logP.SetLines(lines)
	if preserveLayout {
		g.applyLayout(g.w, g.h) // design rects × the new scale
	}
}

// setFullscreen flips the window mode and persists it. The flag is also
// applied at boot in Run (before the loop), so a saved fullscreen session
// comes back fullscreen.
func (g *Game) setFullscreen(on bool) {
	if on == g.fullscreen {
		return
	}
	g.fullscreen = on
	ebiten.SetFullscreen(on)
	if err := g.writeSettings(); err != nil {
		g.report(err, "")
	}
}

// resetLayout clears the persisted window arrangement and recomposes the
// built-in defaults: the sidecar file is deleted and the placement
// trackers dropped, so placeIfVirgin treats every window as virgin again
// and placeDefaults re-slots them for the current screen. Open/closed
// flags of this session are untouched — only geometry resets.
func (g *Game) resetLayout() {
	if err := os.Remove(g.layoutPath()); err != nil && !os.IsNotExist(err) {
		g.report(err, "")
		return // a failed clear must not half-apply
	}
	g.restored = map[*widget.Window]bool{}
	g.placed = map[*widget.Window]image.Rectangle{}
	// The pane proportions are layout state too: back to equal shares.
	for _, name := range layoutNames {
		for _, sm := range g.splitSeams(name) {
			sm.Split.Reset()
		}
		// Column widths and hidden columns too: the tables measure again.
		for _, t := range g.tablesOf(name) {
			t.ResetPrefs()
		}
	}
	g.placeDefaults(g.w, g.h)
	g.clampRestored(g.w, g.h)
	g.report(nil, "Window layout reset")
}
