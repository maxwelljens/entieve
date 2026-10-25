package game

import (
	"encoding/json"
	"errors"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
	uikit "github.com/entieve/entieve/uikit/screens"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Resolution independence and the settings window:
//   - <SavePath>.settings.json round-trips (defaults on any damage),
//   - a live UI-scale change re-installs the scale, rebuilds the faces
//     and keeps every window's DESIGN-unit placement,
//   - the layout sidecar persists design units that restore at any scale,
//   - Layout hands out device pixels and overUI agrees with the scaled
//     rects the cursor runs in,
//   - P mounts the settings window and the sidecar records it.
//
// Every test that touches the global scale restores it: theme.SetScale
// is process-wide and the baseline suite runs at scale 1.

// resetScale restores the identity scale when the test ends.
func resetScale(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { theme.SetScale(1) })
}

// newScaleGameAt builds the showcase session on savePath with an explicit
// device scale factor (the option tests pin instead of reading a display).
func newScaleGameAt(t *testing.T, savePath string, deviceScale float64) *Game {
	t.Helper()
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	wd, ok := c.World("world-showcase")
	if !ok {
		t.Fatal("world-showcase missing")
	}
	g, err := New(Options{
		Content:     c,
		World:       wd,
		Seed:        42,
		SavePath:    savePath,
		DeviceScale: deviceScale,
	})
	if err != nil {
		t.Fatalf("game.New: %v", err)
	}
	return g
}

// TestSettingsPersistence covers the sidecar's whole contract: defaults
// when absent, a full round-trip, the documented JSON shape, and the two
// damage modes (corrupt file, a scale off the ladder) falling back
// without error.
func TestSettingsPersistence(t *testing.T) {
	g := newShowcaseGame(t)

	// Absent → defaults.
	g.uiScale, g.fullscreen, g.vsync = 7.0, true, false // poisoned before the read
	g.winW, g.winH = 111, 222
	g.loadSettings()
	if g.uiScale != 1.0 || g.fullscreen || !g.vsync {
		t.Errorf("absent file → (%v, %v, vsync %v), want (1, false, true)", g.uiScale, g.fullscreen, g.vsync)
	}
	if w, h := g.windowSize(); w != defaultW || h != defaultH {
		t.Errorf("absent file → window %d×%d, want the %d×%d default", w, h, defaultW, defaultH)
	}

	// Round trip.
	g.uiScale, g.fullscreen = 1.5, true
	if err := g.writeSettings(); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}
	g.uiScale, g.fullscreen, g.vsync = 0, false, false
	g.loadSettings()
	if g.uiScale != 1.5 || !g.fullscreen || !g.vsync {
		t.Errorf("round trip → (%v, %v, vsync %v), want (1.5, true, true)", g.uiScale, g.fullscreen, g.vsync)
	}

	// Documented default shape after a write: the two long-standing keys
	// plus the audio trio — master/music whole percents; shuffle on is the
	// inverted key's absent state.
	data, err := os.ReadFile(g.settingsPath())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("settings JSON: %v", err)
	}
	for _, k := range []string{"ui_scale", "fullscreen", "master_volume", "music_volume"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("settings keys = %v, want %s present", raw, k)
		}
	}
	if len(raw) != 4 {
		t.Errorf("settings keys = %v, want exactly ui_scale, fullscreen, master_volume, music_volume", raw)
	}
	if v, ok := raw["ui_scale"].(float64); !ok || v != 1.5 {
		t.Errorf("ui_scale = %v, want 1.5", raw["ui_scale"])
	}
	if v, ok := raw["fullscreen"].(bool); !ok || !v {
		t.Errorf("fullscreen = %v, want true", raw["fullscreen"])
	}
	if v, ok := raw["master_volume"].(float64); !ok || v != 100 {
		t.Errorf("master_volume = %v, want 100", raw["master_volume"])
	}
	if v, ok := raw["music_volume"].(float64); !ok || v != 70 {
		t.Errorf("music_volume = %v, want 70", raw["music_volume"])
	}

	// Vsync off is the inverted key; the rest of the shape is unchanged.
	g.vsync = false
	if err := g.writeSettings(); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}
	raw = readSettingsMap(t, g.settingsPath())
	if len(raw) != 5 || raw["vsync_off"] != true {
		t.Errorf("vsync-off settings keys = %v, want the default four plus vsync_off: true", raw)
	}
	g.vsync = true
	g.loadSettings()
	if g.vsync {
		t.Error("vsync_off round trip read back on, want the inverted key honoured")
	}

	// A file from the pre-vsync version (no key) boots vsync ON.
	if err := os.WriteFile(g.settingsPath(), []byte(`{"ui_scale":1,"fullscreen":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g.vsync = false
	g.loadSettings()
	if !g.vsync {
		t.Error("a file without vsync_off read vsync off, want on (today's hardcoded boot)")
	}

	// Corrupt → defaults (never an error, never a crash).
	if err := os.WriteFile(g.settingsPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.uiScale, g.fullscreen, g.vsync = 1.5, true, false
	g.loadSettings()
	if g.uiScale != 1.0 || g.fullscreen || !g.vsync {
		t.Errorf("corrupt file → (%v, %v, vsync %v), want (1, false, true)", g.uiScale, g.fullscreen, g.vsync)
	}

	// Off-ladder scale → 1.0, but the flag still reads.
	if err := os.WriteFile(g.settingsPath(), []byte(`{"ui_scale":1.3,"fullscreen":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g.loadSettings()
	if g.uiScale != 1.0 || !g.fullscreen {
		t.Errorf("off-ladder file → (%v, %v), want (1, true)", g.uiScale, g.fullscreen)
	}

	// Every ladder rung is accepted verbatim.
	for _, rung := range []float64{0.75, 1.0, 1.25, 1.5, 2.0, 2.5} {
		if err := os.WriteFile(g.settingsPath(),
			[]byte(mustSettingsJSON(t, rung, false)), 0o644); err != nil {
			t.Fatal(err)
		}
		g.loadSettings()
		if g.uiScale != rung {
			t.Errorf("rung %v loaded as %v", rung, g.uiScale)
		}
	}
}

// mustSettingsJSON renders one settings document (test-side writer).
func mustSettingsJSON(t *testing.T, scale float64, fullscreen bool) string {
	t.Helper()
	data, err := json.Marshal(settingsFile{UIScale: scale, Fullscreen: fullscreen})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestApplyUIScaleLiveRelayout (live-apply): switching the user scale
// re-installs the global scale, rebuilds every metric and face against
// it, persists the setting, keeps the log — and, through the design-unit
// sidecar round trip, leaves each window exactly where it was in design
// space (its pixel rect is the old one × the scale ratio).
func TestApplyUIScaleLiveRelayout(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)
	g.Layout(1280, 720)

	before := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if before == (image.Rectangle{}) {
		t.Fatal("overview has no rect before the scale change")
	}
	faceBefore, _ := text.Measure("Entieve", *g.env.FaceBold(theme.StepBase), 0)
	bodyOf[*uikit.LogPanel](g.host, idLog).PushLine(sim.SevInfo, "kept across the rebuild")

	g.applyUIScale(1.5)

	if got := theme.Scale(); got != 1.5 {
		t.Fatalf("global scale after change = %v, want 1.5", got)
	}
	if g.uiScale != 1.5 {
		t.Errorf("uiScale = %v, want 1.5", g.uiScale)
	}
	// The setting persisted immediately.
	data, err := os.ReadFile(g.settingsPath())
	if err != nil {
		t.Fatalf("settings not written: %v", err)
	}
	if !strings.Contains(string(data), `"ui_scale": 1.5`) {
		t.Errorf("settings file %s lacks ui_scale 1.5", data)
	}
	// Faces rebuilt at the new scale. Glyph advances quantize per size
	// (text/v2 hinting), so "1.5×" holds to a couple of percent, not bit
	// exactly — the point is the measure GREW with the rebuild.
	faceAfter, _ := text.Measure("Entieve", *g.env.FaceBold(theme.StepBase), 0)
	if math.Abs(faceAfter-1.5*faceBefore) > 0.03*faceAfter {
		t.Errorf("face measure after change = %v, want ≈%v (1.5× ±3%%)", faceAfter, 1.5*faceBefore)
	}
	// Metrics rebuilt: the inventory window's floor is now 1.5× design.
	if got := g.host.Window(idInventory).MinSize; got == nil || got.X != 450 || got.Y != 630 {
		t.Errorf("inventory MinSize after change = %v, want (450,630) (1.5× 300×420)", got)
	}
	// Design-unit placement preserved: each coordinate is the old one
	// moved to 1.5× through the sidecar (round to the nearest device px).
	// The Overview is right-anchored: in the UNCHANGED viewport a 1.5×
	// rebuild makes its restored rect overflow the right edge, so the
	// restore clamp slides it fully on-screen (clampRestored) — the same
	// path the right-anchored Selected Item window has always taken. A
	// window whose scaled rect still fits round-trips exactly.
	after := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	want := clampLayoutRect(image.Rect(
		int(math.Round(float64(before.Min.X)*1.5)), int(math.Round(float64(before.Min.Y)*1.5)),
		int(math.Round(float64(before.Max.X)*1.5)), int(math.Round(float64(before.Max.Y)*1.5))),
		*g.kit.OverviewWindow().MinSize, image.Rect(0, 0, g.w, g.h))
	if after != want {
		t.Errorf("overview rect after change = %v, want %v (design units ×1.5, clamped on screen)", after, want)
	}
	// The log survived the rebuild.
	found := false
	for _, l := range bodyOf[*uikit.LogPanel](g.host, idLog).Lines() {
		if strings.Contains(l, "kept across the rebuild") {
			found = true
		}
	}
	if !found {
		t.Errorf("log line lost across the scale rebuild: %v", bodyOf[*uikit.LogPanel](g.host, idLog).Lines())
	}
	// And back: the sidecar now holds the clamped design rect (the
	// right-anchored Overview cannot keep its corner in an unchanged
	// viewport), so the window returns to the spot it was clamped to —
	// on screen, at its design size.
	g.applyUIScale(1.0)
	back := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if wantBack := image.Rect(
		int(math.Round(float64(after.Min.X)/1.5)), int(math.Round(float64(after.Min.Y)/1.5)),
		int(math.Round(float64(after.Max.X)/1.5)), int(math.Round(float64(after.Max.Y)/1.5))); back != wantBack {
		t.Errorf("overview rect at scale 1 again = %v, want the round-tripped design rect %v", back, wantBack)
	}
	if back.Min.X < 0 || back.Min.Y < 0 || back.Max.X > 1280 || back.Max.Y > 720 {
		t.Errorf("overview after the scale round trip is off screen: %v", back)
	}
}

// TestLayoutSidecarDesignUnitsAcrossScales: one sidecar (written at
// scale 1, where design unit == pixel) restores the same windows at 1.5
// and at 2 — each coordinate the design value × the reading scale. This
// is also the seam that keeps v2 files written before the scale work
// loading seamlessly.
func TestLayoutSidecarDesignUnitsAcrossScales(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)
	g.Layout(1280, 720)

	// Pin the saved arrangement to something composed (virgin defaults).
	ov := g.kit.OverviewWindow()
	design := ov.GetContainer().GetWidget().Rect
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	// The sidecar itself must hold design units (scale 1: px/1 == px).
	var f layoutFile
	data, err := os.ReadFile(g.layoutPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if got := f.Windows["overview"]; got.X != design.Min.X || got.Y != design.Min.Y ||
		got.W != design.Dx() || got.H != design.Dy() {
		t.Errorf("sidecar overview = %+v, want design rect %v", got, design)
	}
	// The settings window rides the sidecar too (panelDefs name).
	if _, ok := f.Windows["settings"]; !ok {
		t.Errorf("sidecar lacks the settings window: %v", keysOf(f.Windows))
	}

	for _, sc := range []float64{1.5, 2.0} {
		theme.SetScale(sc)
		g.applyLayout(2560, 1440) // wide enough that nothing clamps
		got := ov.GetContainer().GetWidget().Rect
		want := image.Rect(
			int(math.Round(float64(design.Min.X)*sc)), int(math.Round(float64(design.Min.Y)*sc)),
			int(math.Round(float64(design.Max.X)*sc)), int(math.Round(float64(design.Max.Y)*sc)))
		if got != want {
			t.Errorf("scale %v: overview restored as %v, want %v (design × scale)", sc, got, want)
		}
	}
	theme.SetScale(1)
	g.applyLayout(1280, 720)
	if got := ov.GetContainer().GetWidget().Rect; got != design {
		t.Errorf("back at scale 1: overview = %v, want %v", got, design)
	}
}

// keysOf lists a sidecar's window names (failure-message helper).
func keysOf(m map[string]winLayout) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestLayoutReturnsDevicePixelsAndOverUIAgrees: Layout converts the
// window (device-independent) size to DEVICE pixels through the device
// scale factor — the clamp happens in window units — and because every
// metric scales into those same Layout units, overUI's rect hits agree
// with the scaled chrome the cursor runs in.
func TestLayoutReturnsDevicePixelsAndOverUIAgrees(t *testing.T) {
	resetScale(t)
	g := newScaleGameAt(t, filepath.Join(t.TempDir(), "save.json"), 1.5)

	if got := theme.Scale(); got != 1.5 {
		t.Fatalf("global scale with DeviceScale 1.5 = %v, want 1.5 (uiScale 1 × device)", got)
	}
	w, h := g.Layout(1024, 700)
	if w != 1536 || h != 1050 || g.w != 1536 || g.h != 1050 {
		t.Errorf("Layout(1024,700) = %d×%d (g %d×%d), want 1536×1050 — window px × device",
			w, h, g.w, g.h)
	}
	// The minimum clamp runs in WINDOW units, then converts once.
	w, h = g.Layout(640, 480)
	if w != 1536 || h != 1050 {
		t.Errorf("clamped Layout(640,480) = %d×%d, want 1536×1050 (min 1024×700 × 1.5)", w, h)
	}
	// Metrics agree with those units: the right-anchored overview ends at
	// the right margin (design 1024 − 10 = 1014 → 1521 device px) at the
	// window row's design top (8 → 12), and its rect is the scaled one.
	ov := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if ov.Max.X != 1521 || ov.Min.Y != 12 {
		t.Errorf("overview at device scale = %v, want to end at (1521,12) (design 1014,8 × 1.5)", ov)
	}

	// Refresh ebitenui's hover flag from the real cursor (0,0) before
	// trusting overUI's backstop.
	g.kit.Update()
	if !g.overUI(float64(ov.Min.X+1), float64(ov.Min.Y+1)) {
		t.Errorf("overUI inside the scaled overview (%v) = false", ov.Min)
	}
	if g.overUI(4, 4) {
		t.Error("overUI outside every panel (4,4) = true")
	}
	if g.overUI(float64(ov.Max.X+8), float64(ov.Min.Y+1)) {
		t.Errorf("overUI %d px right of the overview = true", ov.Max.X+8-ov.Min.X)
	}
}

// TestSettingsPanelMounts: P is the documented binding — handleKeys
// flips the panel flag, syncMounts mounts the window, the sidecar records
// it, and the help rows (from KeyMap) carry the key.
func TestSettingsPanelMounts(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)

	if err := g.handleKeys(Input{JustPressed: []ebiten.Key{ebiten.KeyP}}); err != nil {
		t.Fatalf("handleKeys: %v", err)
	}
	if !g.host.IsOpen(idSettings) {
		t.Fatal("P did not open the settings panel")
	}
	g.syncMounts()
	if bodyOf[*uikit.Settings](g.host, idSettings) == nil || !g.host.Mounted(idSettings) {
		t.Fatal("settings window not mounted after P")
	}
	// It is a named sidecar window.
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	data, err := os.ReadFile(g.layoutPath())
	if err != nil {
		t.Fatal(err)
	}
	var f layoutFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Windows["settings"]; !ok {
		t.Errorf("settings window missing from the sidecar: %v", keysOf(f.Windows))
	}
	// The help overlay documents the key (KeyMap → helpRows).
	have := false
	for _, r := range helpRows() {
		if r.Key == "P" {
			have = true
		}
	}
	if !have {
		t.Error("help rows lack the P key binding")
	}
}

// TestSetFullscreenPersists: the toggle flips the mode, hands ebiten the
// same flag and writes it to the settings sidecar.
func TestSetFullscreenPersists(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)
	g.setFullscreen(true)
	if !g.fullscreen {
		t.Fatal("setFullscreen(true) did not stick")
	}
	data, err := os.ReadFile(g.settingsPath())
	if err != nil {
		t.Fatalf("settings not written: %v", err)
	}
	if !strings.Contains(string(data), `"fullscreen": true`) {
		t.Errorf("settings file %s lacks the fullscreen flag", data)
	}
	g.setFullscreen(false)
	if g.fullscreen {
		t.Error("setFullscreen(false) did not stick")
	}
}

// TestRailCollapsePersists: the navigation rail's collapse rides
// <SavePath>.settings.json. A fresh session starts EXPANDED (the key is
// omitempty, so the documented default shape stays exactly ui_scale +
// fullscreen); collapsing through the edge tab writes
// "rail_collapsed": true; a new session on that path boots collapsed
// (buildKit restores the state onto the rebuilt rail); expanding it
// again drops the key back to the default shape.
func TestRailCollapsePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	pump(t, g, 1280, 720)
	if g.rail.Collapsed() {
		t.Fatal("a fresh rail starts expanded")
	}

	// Collapse through the edge tab; the next frame's syncRail mirrors
	// it into the sidecar.
	click(t, g, g.rail.Tab())
	if !g.rail.Collapsed() {
		t.Fatal("edge tab did not collapse the rail")
	}
	pump(t, g, 1280, 720)
	if !g.railCollapsed {
		t.Fatal("collapsed state did not reach the host (syncRail)")
	}
	raw := readSettingsMap(t, g.settingsPath())
	if len(raw) != 5 || raw["rail_collapsed"] != true {
		t.Errorf("collapsed settings keys = %v, want the default four plus rail_collapsed: true", raw)
	}

	// A new session on the same path boots collapsed.
	g2 := newShowcaseGameAt(t, path)
	if !g2.railCollapsed || !g2.rail.Collapsed() {
		t.Error("collapsed rail did not survive the restart")
	}

	// Expand again: the key vanishes — the documented default shape.
	click(t, g, g.rail.Tab())
	pump(t, g, 1280, 720)
	raw = readSettingsMap(t, g.settingsPath())
	if _, ok := raw["rail_collapsed"]; ok || len(raw) != 4 {
		t.Errorf("expanded settings keys = %v, want exactly ui_scale, fullscreen, master_volume, music_volume", raw)
	}
	g3 := newShowcaseGameAt(t, path)
	if g3.rail.Collapsed() {
		t.Error("expanded state did not survive the restart")
	}
}

// readSettingsMap parses the settings sidecar as raw JSON.
func readSettingsMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("settings JSON: %v", err)
	}
	return raw
}

// TestVsyncRoundTripAndLiveApply: vsync defaults ON (the old hardcoded
// boot), a file without the key keeps it on, and the toggle reaches the
// ebiten seam live, persists inverted and survives a restart.
func TestVsyncRoundTripAndLiveApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	if !g.vsync {
		t.Fatal("a fresh session must boot vsync on (today's behaviour)")
	}

	// Pre-vsync file: no key, still on.
	if err := os.WriteFile(g.settingsPath(), []byte(`{"ui_scale":1,"fullscreen":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g.vsync = false
	g.loadSettings()
	if !g.vsync {
		t.Error("a file without vsync_off loaded vsync off, want on")
	}

	// Live off: the seam sees false and the sidecar records the inverted key.
	var calls []bool
	g.vsyncApply = func(on bool) { calls = append(calls, on) }
	g.setVsync(false)
	if g.vsync || len(calls) != 1 || calls[0] {
		t.Errorf("setVsync(false) → vsync %v, seam calls %v; want off and one false call", g.vsync, calls)
	}
	if raw := readSettingsMap(t, g.settingsPath()); raw["vsync_off"] != true {
		t.Errorf("vsync-off sidecar = %v, want vsync_off: true", raw)
	}

	// Restart: still off (default-true only applies when the key is absent).
	g2 := newShowcaseGameAt(t, path)
	if g2.vsync {
		t.Error("vsync off did not survive the restart")
	}

	// Live on again: seam true, and the key drops back to the default shape.
	var on []bool
	g2.vsyncApply = func(v bool) { on = append(on, v) }
	g2.setVsync(true)
	if !g2.vsync || len(on) != 1 || !on[0] {
		t.Errorf("setVsync(true) → vsync %v, seam calls %v; want on and one true call", g2.vsync, on)
	}
	if raw := readSettingsMap(t, g2.settingsPath()); raw["vsync_off"] != nil {
		t.Errorf("vsync-on sidecar = %v, want the key gone", raw)
	}
}

// TestWindowSizePersistsOnQuit: applySize captures the WINDOW size (not
// device pixels), the clean quit writes it to the settings sidecar, a
// restart restores it in windowSize (what Run opens with), and a size
// below the supported minimum clamps instead of booting too small.
func TestWindowSizePersistsOnQuit(t *testing.T) {
	resetScale(t)
	path := filepath.Join(t.TempDir(), "save.json")
	g := newScaleGameAt(t, path, 1.5)
	g.Layout(1100, 800) // device 1650×1200: winW/H must stay window px
	if g.winW != 1100 || g.winH != 800 {
		t.Fatalf("captured window size = %d×%d, want 1100×800 (not device px)", g.winW, g.winH)
	}

	g.quit = true
	if err := g.Update(); !errors.Is(err, ebiten.Termination) {
		t.Fatalf("quit Update error = %v, want ebiten.Termination", err)
	}
	raw := readSettingsMap(t, g.settingsPath())
	if raw["window_w"] != float64(1100) || raw["window_h"] != float64(800) {
		t.Errorf("quit sidecar = %v, want window 1100×800", raw)
	}

	g2 := newScaleGameAt(t, path, 1.5)
	if w, h := g2.windowSize(); w != 1100 || h != 800 {
		t.Errorf("restored windowSize = %d×%d, want 1100×800", w, h)
	}

	// Below the supported minimum → clamp to 1024×700.
	if err := os.WriteFile(g2.settingsPath(),
		[]byte(`{"ui_scale":1,"fullscreen":false,"window_w":640,"window_h":480}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g2.loadSettings()
	if w, h := g2.windowSize(); w != minW || h != minH {
		t.Errorf("clamped windowSize = %d×%d, want %d×%d", w, h, minW, minH)
	}
}

// TestRailSettingsToggle: the settings window's rail row drives the same
// persisted state as the edge tab, live and across a restart.
func TestRailSettingsToggle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.applySize(1280, 720)
	if g.host.services.Settings.RailCollapsed() {
		t.Fatal("fresh rail starts expanded")
	}
	g.host.services.Settings.SetRailCollapsed(true)
	if !g.rail.Collapsed() || !g.railCollapsed {
		t.Fatal("the settings toggle did not park the rail")
	}
	if raw := readSettingsMap(t, g.settingsPath()); raw["rail_collapsed"] != true {
		t.Errorf("collapsed sidecar = %v, want rail_collapsed: true", raw)
	}
	g.host.services.Settings.SetRailCollapsed(false)
	if g.rail.Collapsed() || g.railCollapsed {
		t.Fatal("the settings toggle did not expand the rail")
	}
	if raw := readSettingsMap(t, g.settingsPath()); raw["rail_collapsed"] != nil {
		t.Errorf("expanded sidecar = %v, want the key gone", raw)
	}
}

// TestResetAllSettings: one action restores every default — scale 1×,
// windowed, vsync on, rail expanded, no stored windowed size — re-applies
// them live and leaves exactly the default sidecar shape.
func TestResetAllSettings(t *testing.T) {
	resetScale(t)
	path := filepath.Join(t.TempDir(), "save.json")
	g := newShowcaseGameAt(t, path)
	g.Layout(1100, 800)

	var vsync []bool
	g.vsyncApply = func(on bool) { vsync = append(vsync, on) }
	g.setFullscreen(true)
	g.setVsync(false)
	g.setRailCollapsed(true)
	g.setMasterVolume(90)
	g.setShuffle(false)
	g.applyUIScale(1.5)
	g.flushSize = true // an armed quit write: the size is on disk now
	if err := g.writeSettings(); err != nil {
		t.Fatalf("pre-reset writeSettings: %v", err)
	}
	if raw := readSettingsMap(t, g.settingsPath()); raw["window_w"] == nil || raw["vsync_off"] == nil || raw["rail_collapsed"] == nil || raw["shuffle_off"] == nil {
		t.Fatalf("fixture: non-default settings not persisted: %v", raw)
	}

	g.resetAllSettings()

	if g.uiScale != 1.0 || theme.Scale() != 1.0 {
		t.Errorf("scale after reset = %v (global %v), want 1", g.uiScale, theme.Scale())
	}
	if g.fullscreen || !g.vsync || g.railCollapsed || g.rail.Collapsed() {
		t.Errorf("after reset: fullscreen %v, vsync %v, rail %v/%v; want off/on/expanded",
			g.fullscreen, g.vsync, g.railCollapsed, g.rail.Collapsed())
	}
	if g.masterVol != 100 || g.musicVol != 70 || !g.shuffle {
		t.Errorf("audio after reset = (%d, %d, shuffle %v), want (100, 70, true)",
			g.masterVol, g.musicVol, g.shuffle)
	}
	if len(vsync) == 0 || !vsync[len(vsync)-1] {
		t.Errorf("vsync seam after reset = %v, want a final true call", vsync)
	}
	if w, h := g.windowSize(); w != defaultW || h != defaultH {
		t.Errorf("windowed size after reset = %d×%d, want the %d×%d default", w, h, defaultW, defaultH)
	}
	// The one flush left the default document: no rail/vsync/size/shuffle
	// keys, and the audio percents back at 100/70.
	raw := readSettingsMap(t, g.settingsPath())
	if len(raw) != 4 || raw["ui_scale"] != float64(1) || raw["fullscreen"] != false ||
		raw["master_volume"] != float64(100) || raw["music_volume"] != float64(70) {
		t.Errorf("sidecar after reset = %v, want exactly ui_scale 1, fullscreen false, master 100, music 70", raw)
	}
}

// TestScaleSliderRebuildsAndDropsTheDrag: the notched UI-scale slider
// applies a rung live, which rebuilds the whole interface under the
// slider mid-grab. The replacement panel's slider is idle, so the held
// pointer cannot re-fire the old drag; the scale stays on the rung the
// press chose.
func TestScaleSliderRebuildsAndDropsTheDrag(t *testing.T) {
	resetScale(t)
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	g.host.SetOpen(idSettings, true)
	g.syncMounts()
	pump(t, g, 1280, 720)

	old := bodyOf[*uikit.Settings](g.host, idSettings)
	if old == nil {
		t.Fatal("settings panel not mounted")
	}
	sl := old.ScaleSliderForTest()
	band := sl.HitBand()
	at := image.Pt(sl.ValueX(1.25), band.Min.Y+band.Dy()/2)
	sl.Step(at, true, true)
	if bodyOf[*uikit.Settings](g.host, idSettings) == old {
		t.Fatal("a scale rung did not rebuild the interface")
	}
	if g.uiScale != 1.25 || theme.Scale() != 1.25 {
		t.Fatalf("scale after the slider press = %v (global %v), want 1.25", g.uiScale, theme.Scale())
	}

	pump(t, g, 1280, 720) // lay the rebuilt panel out
	fresh := bodyOf[*uikit.Settings](g.host, idSettings).ScaleSliderForTest()
	if fresh == nil || fresh == sl {
		t.Fatal("the rebuilt panel did not carry a fresh slider")
	}
	if fresh.Grabbed() {
		t.Error("the replacement slider starts grabbed (stale drag state)")
	}
	// A held frame with no fresh press is not a drag: the handle must not
	// jump (and the old drag must not fire through the replaced panel).
	fresh.Step(image.Pt(fresh.ValueX(2.5), fresh.HitBand().Min.Y+1), false, true)
	if g.uiScale != 1.25 {
		t.Errorf("a stale held frame moved the scale to %v, want 1.25", g.uiScale)
	}
	if fresh.Value() != 1.25 {
		t.Errorf("fresh slider value = %v, want the applied rung 1.25", fresh.Value())
	}
}
