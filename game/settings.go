package game

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"

	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// Player settings persistence: a JSON sidecar next to the save file,
// <Options.SavePath>.settings.json, holding the UI scale, the fullscreen
// and vsync flags, the navigation rail's collapse state, the two audio
// volume percents, the shuffle flag and the last windowed size:
//
//	{"ui_scale": 1.5, "fullscreen": false}
//	{"ui_scale": 1, "fullscreen": false, "rail_collapsed": true, "vsync_off": true}
//	{"ui_scale": 1, "fullscreen": false, "window_w": 1100, "window_h": 800}
//	{"ui_scale": 1, "fullscreen": false, "master_volume": 60, "music_volume": 30, "shuffle_off": true}
//
// Every key past the first two is omitempty by default, so a file with
// default settings stays close to the documented two-key shape; the two
// volume percents are written as pointers so an explicit 0 is not the
// absent-key default (100 / 70). rail_collapsed true means collapsed;
// vsync_off and shuffle_off are INVERTED (absent/false = on, today's
// hardcoded boot) so a file written before the key existed keeps the
// behaviour it had. The windowed size is written only on a clean quit
// (see quitPersist) — never mid-session — and read back in Run.
//
// Like the layout sidecar the contract is robustness over fidelity: a
// missing, corrupt or out-of-ladder file degrades to the defaults (UI
// scale 1.0, windowed, vsync on, expanded rail, 100/70 audio with
// shuffle on, 1280×720) and never errors or crashes; unknown keys are
// ignored (out-of-range percents clamp). It loads in New, BEFORE
// the interface is built and before the first Layout, so the very first
// frame is already laid out at the saved scale; every change writes it
// back immediately (best-effort errors are reported to the log).
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

// Audio defaults. The volumes are whole percents (0..100) persisted as
// percents; the engine multiplies them at the seam, so a fresh install
// plays at 1.00 × 0.70.
const (
	defaultMasterVolume = 100
	defaultMusicVolume  = 70
	defaultShuffle      = true
)

// settingsFile is the settings sidecar document.
type settingsFile struct {
	UIScale    float64 `json:"ui_scale"`
	Fullscreen bool    `json:"fullscreen"`
	// RailCollapsed parks the left navigation rail to its edge tab;
	// absent = expanded (the boot default).
	RailCollapsed bool `json:"rail_collapsed,omitempty"`
	// VsyncOff is the inverted vsync flag: absent = on, which is what
	// every build before this key did unconditionally.
	VsyncOff bool `json:"vsync_off,omitempty"`
	// MasterVolume/MusicVolume are whole percents; absent = 100 / 70.
	// Pointers keep an explicit 0 distinct from "key not written".
	MasterVolume *int `json:"master_volume,omitempty"`
	MusicVolume  *int `json:"music_volume,omitempty"`
	// ShuffleOff is the inverted shuffle flag (the vsync_off precedent):
	// absent = on, the fresh-install default.
	ShuffleOff bool `json:"shuffle_off,omitempty"`
	// WindowW/H are the last WINDOWED size (window pixels), written on
	// clean quit only; absent = the 1280×720 boot default.
	WindowW int `json:"window_w,omitempty"`
	WindowH int `json:"window_h,omitempty"`
}

// settingsPath is the sidecar location: the resolved save path plus
// .settings.json (e.g. …/entieve/save.json.settings.json).
func (g *Game) settingsPath() string { return g.SavePath() + ".settings.json" }

// loadSettings reads the settings sidecar into g. Every failure mode —
// missing file, bad JSON, a scale off the ladder — leaves the defaults
// in place (never an error; called from New before anything is built).
func (g *Game) loadSettings() {
	g.uiScale, g.fullscreen, g.vsync, g.railCollapsed = defaultUIScale, false, true, false
	g.masterVol, g.musicVol, g.shuffle = defaultMasterVolume, defaultMusicVolume, defaultShuffle
	g.winW, g.winH = 0, 0
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
	g.vsync = !f.VsyncOff
	g.railCollapsed = f.RailCollapsed
	if f.MasterVolume != nil {
		g.masterVol = clampInt(*f.MasterVolume, 0, 100)
	}
	if f.MusicVolume != nil {
		g.musicVol = clampInt(*f.MusicVolume, 0, 100)
	}
	g.shuffle = !f.ShuffleOff
	if f.WindowW > 0 && f.WindowH > 0 {
		g.winW, g.winH = f.WindowW, f.WindowH
	}
}

// writeSettings persists g's settings (called on every change, and on
// quit with the windowed size). flushSize gates the size keys: they are
// only meaningful for a real window and are written by quitPersist.
func (g *Game) writeSettings() error {
	master, musicVol := g.masterVol, g.musicVol
	f := settingsFile{
		UIScale:       g.uiScale,
		Fullscreen:    g.fullscreen,
		RailCollapsed: g.railCollapsed,
		VsyncOff:      !g.vsync,
		MasterVolume:  &master,
		MusicVolume:   &musicVol,
		ShuffleOff:    !g.shuffle,
	}
	if g.flushSize {
		f.WindowW, f.WindowH = g.windowSize()
	}
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

// windowSize is the size Run opens the window at: the last windowed size
// read from the sidecar, clamped to the supported minimum, else the
// 1280×720 default. Fullscreen is unaffected — the size is the windowed
// restore size.
func (g *Game) windowSize() (int, int) {
	w, h := g.winW, g.winH
	if w <= 0 || h <= 0 {
		return defaultW, defaultH
	}
	return max(w, minW), max(h, minH)
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

// setUIScale applies one rung of the ladder absolutely (the settings
// slider's handle, which snaps to these values). A rung off the ladder is
// ignored — the sidecar reader's rule; the slider can only produce rungs.
func (g *Game) setUIScale(v float64) {
	if s, ok := scaleOnLadder(v); ok {
		g.applyUIScale(s)
	}
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
	lines := bodyOf[*uikit.LogPanel](g.host, idLog).Lines()
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
	bodyOf[*uikit.LogPanel](g.host, idLog).SetLines(lines)
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

// applyVsync is the ebiten seam for the vsync flag: nil means the real
// display call, tests swap it for a recorder (the flag is unreadable).
func (g *Game) applyVsync(on bool) {
	if g.vsyncApply != nil {
		g.vsyncApply(on)
		return
	}
	ebiten.SetVsyncEnabled(on)
}

// setVsync flips vsync live and persists it. Run applies the saved flag
// at boot through the same seam.
func (g *Game) setVsync(on bool) {
	if on == g.vsync {
		return
	}
	g.vsync = on
	g.applyVsync(on)
	if err := g.writeSettings(); err != nil {
		g.report(err, "")
	}
}

// setRailCollapsed parks or expands the navigation rail from the
// settings window (the edge tab goes through the same state via
// syncRail) and persists it.
func (g *Game) setRailCollapsed(on bool) {
	if on == g.railCollapsed {
		return
	}
	g.railCollapsed = on
	g.rail.SetCollapsed(on)
	if err := g.writeSettings(); err != nil {
		g.report(err, "")
	}
}

// applyMusicSettings pushes the two volume percents and the shuffle flag
// into the seam. Called after load and on every audio setting change; the
// engine applies master × music to the current track and keeps the pair
// for the next one.
func (g *Game) applyMusicSettings() {
	g.music.SetVolume(float64(g.masterVol)/100, float64(g.musicVol)/100)
	g.music.SetShuffle(g.shuffle)
}

// setMasterVolume applies a whole percent (0..100) live at the music
// seam WITHOUT writing the sidecar: the settings slider streams these
// while the pointer moves, and PersistSettings is the drag's one write.
func (g *Game) setMasterVolume(v int) {
	v = clampInt(v, 0, 100)
	if v == g.masterVol {
		return
	}
	g.masterVol = v
	g.applyMusicSettings()
}

// setMusicVolume is the music half of setMasterVolume's contract.
func (g *Game) setMusicVolume(v int) {
	v = clampInt(v, 0, 100)
	if v == g.musicVol {
		return
	}
	g.musicVol = v
	g.applyMusicSettings()
}

// persistSettings is the settings slider's release point: one sidecar
// write per drag, however many values the drag streamed.
func (g *Game) persistSettings() {
	if err := g.writeSettings(); err != nil {
		g.report(err, "")
	}
}

// setShuffle flips the track order (shuffled pool vs declared order),
// live and persisted. The current track keeps playing.
func (g *Game) setShuffle(on bool) {
	if on == g.shuffle {
		return
	}
	g.shuffle = on
	g.applyMusicSettings()
	if err := g.writeSettings(); err != nil {
		g.report(err, "")
	}
}

// resetAllSettings restores every setting to its default — scale 1×,
// windowed, vsync on, rail expanded, master 100% / music 70% / shuffle on,
// no persisted windowed size — then rebuilds the interface at 1× (the
// live-apply path) and flushes the sidecar once.
func (g *Game) resetAllSettings() {
	g.uiScale = defaultUIScale
	g.fullscreen = false
	g.vsync = true
	g.railCollapsed = false
	g.masterVol, g.musicVol, g.shuffle = defaultMasterVolume, defaultMusicVolume, defaultShuffle
	g.winW, g.winH = 0, 0
	g.flushSize = false
	ebiten.SetFullscreen(false)
	g.applyVsync(true)
	g.applyScale(true)
	g.rail.SetCollapsed(false) // also when applyScale short-circuits
	g.applyMusicSettings()
	if err := g.writeSettings(); err != nil {
		g.report(err, "")
	}
	g.report(nil, "Settings reset")
}

// resetLayout clears the persisted window arrangement and recomposes the
// built-in defaults: the sidecar file is deleted and the placement
// trackers dropped, so ScreenHost.Place treats every window as virgin again
// and placeDefaults re-slots them for the current screen. Open/closed
// flags of this session are untouched — only geometry resets.
func (g *Game) resetLayout() {
	if err := os.Remove(g.layoutPath()); err != nil && !os.IsNotExist(err) {
		g.report(err, "")
		return // a failed clear must not half-apply
	}
	g.host.ResetPlacement()
	// The pane proportions are layout state too: back to equal shares.
	for _, name := range layoutNames {
		e := g.layoutEntry(name)
		if e == nil {
			continue
		}
		for _, sm := range e.Splits() {
			sm.Split.Reset()
		}
		// Column widths and hidden columns too: the tables measure again.
		for _, t := range e.Tables() {
			t.ResetPrefs()
		}
	}
	g.placeDefaults(g.w, g.h)
	g.host.ClampRestored(g.w, g.h)
	g.report(nil, "Window layout reset")
}
