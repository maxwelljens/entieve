package game

import (
	"encoding/json"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/sim"
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
	g.uiScale, g.fullscreen = 7.0, true // poisoned before the read
	g.loadSettings()
	if g.uiScale != 1.0 || g.fullscreen {
		t.Errorf("absent file → (%v, %v), want (1, false)", g.uiScale, g.fullscreen)
	}

	// Round trip.
	g.uiScale, g.fullscreen = 1.5, true
	if err := g.writeSettings(); err != nil {
		t.Fatalf("writeSettings: %v", err)
	}
	g.uiScale, g.fullscreen = 0, false
	g.loadSettings()
	if g.uiScale != 1.5 || !g.fullscreen {
		t.Errorf("round trip → (%v, %v), want (1.5, true)", g.uiScale, g.fullscreen)
	}

	// Documented shape: {"ui_scale": 1.5, "fullscreen": true}.
	data, err := os.ReadFile(g.settingsPath())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("settings JSON: %v", err)
	}
	if len(raw) != 2 {
		t.Errorf("settings keys = %v, want exactly ui_scale and fullscreen", raw)
	}
	if v, ok := raw["ui_scale"].(float64); !ok || v != 1.5 {
		t.Errorf("ui_scale = %v, want 1.5", raw["ui_scale"])
	}
	if v, ok := raw["fullscreen"].(bool); !ok || !v {
		t.Errorf("fullscreen = %v, want true", raw["fullscreen"])
	}

	// Corrupt → defaults (never an error, never a crash).
	if err := os.WriteFile(g.settingsPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	g.uiScale, g.fullscreen = 1.5, true
	g.loadSettings()
	if g.uiScale != 1.0 || g.fullscreen {
		t.Errorf("corrupt file → (%v, %v), want (1, false)", g.uiScale, g.fullscreen)
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
	g.logP.PushLine(sim.SevInfo, "kept across the rebuild")

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
	if got := panelDefs[pInventory].window(g).MinSize; got == nil || got.X != 450 || got.Y != 630 {
		t.Errorf("inventory MinSize after change = %v, want (450,630) (1.5× 300×420)", got)
	}
	// Design-unit placement preserved: each coordinate is the old one
	// moved to 1.5× through the sidecar (round to the nearest device px).
	after := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	want := image.Rect(
		int(math.Round(float64(before.Min.X)*1.5)), int(math.Round(float64(before.Min.Y)*1.5)),
		int(math.Round(float64(before.Max.X)*1.5)), int(math.Round(float64(before.Max.Y)*1.5)))
	if after != want {
		t.Errorf("overview rect after change = %v, want %v (design units ×1.5)", after, want)
	}
	// The log survived the rebuild.
	found := false
	for _, l := range g.logP.Lines() {
		if strings.Contains(l, "kept across the rebuild") {
			found = true
		}
	}
	if !found {
		t.Errorf("log line lost across the scale rebuild: %v", g.logP.Lines())
	}
	// And back: the design rect round-trips to the original pixels.
	g.applyUIScale(1.0)
	if back := g.kit.OverviewWindow().GetContainer().GetWidget().Rect; back != before {
		t.Errorf("overview rect at scale 1 again = %v, want the original %v", back, before)
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
	// Metrics agree with those units: the overview sits at design 8 →
	// 12 device px and its rect is the scaled one.
	ov := g.kit.OverviewWindow().GetContainer().GetWidget().Rect
	if ov.Min.X != 12 || ov.Min.Y != 12 {
		t.Errorf("overview at device scale = %v, want to start at (12,12) (design 8 × 1.5)", ov)
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
	if !g.open[pSettings] {
		t.Fatal("P did not open the settings panel")
	}
	g.syncMounts()
	if g.sett == nil || g.mounted[pSettings] == nil {
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
